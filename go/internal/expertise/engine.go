package expertise

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

const maxExpertsPerFile = 3

type Engine struct {
	log     *slog.Logger
	store   *Store
	git     expertiseGit
	indexer *Indexer
	updater *Updater
	mu      sync.Mutex
	running map[string]runningIndex
}

type runningIndex struct {
	generation uint64
	cancel     context.CancelFunc
	done       chan struct{}
}

func NewEngine(log *slog.Logger, store *Store, git expertiseGit, indexer *Indexer, updater *Updater) *Engine {
	if log == nil {
		log = slog.Default()
	}
	return &Engine{
		log:     log,
		store:   store,
		git:     git,
		indexer: indexer,
		updater: updater,
		running: make(map[string]runningIndex),
	}
}

func (engine *Engine) Enable(ctx context.Context, repoID, repoPath string) error {
	if err := engine.validateEnable(repoID, repoPath); err != nil {
		return err
	}
	if err := engine.store.SetRepoState(ctx, ExpertiseRepoState{RepoID: repoID, Enabled: true, Status: ExpertiseStatusIndexing}); err != nil {
		return fmt.Errorf("enable expertise repo %s: %w", repoID, err)
	}

	engine.startIndex(repoID, repoPath)
	return nil
}

func (engine *Engine) Disable(ctx context.Context, repoID string) error {
	engine.cancel(repoID)
	if err := engine.store.SetRepoState(ctx, ExpertiseRepoState{RepoID: repoID, Enabled: false, Status: ExpertiseStatusIdle, NudgeDismissedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}}); err != nil {
		return fmt.Errorf("disable expertise repo %s: %w", repoID, err)
	}
	return nil
}

func (engine *Engine) OnPRSynced(ctx context.Context, repoID, repoPath string, files []string) {
	if engine == nil || engine.updater == nil {
		return
	}
	if err := engine.updater.Update(ctx, repoID, repoPath, files); err != nil {
		engine.log.Warn("expertise update failed", "repo_id", repoID, "error", err)
	}
}

func (engine *Engine) RestartInterrupted(ctx context.Context, resolvePath func(repoID string) (string, bool)) error {
	if engine == nil || engine.store == nil {
		return errors.New("expertise engine is nil")
	}
	repoIDs, err := engine.store.InterruptedRepos(ctx)
	if err != nil {
		return err
	}
	for _, repoID := range repoIDs {
		repoPath, ok := resolvePath(repoID)
		if !ok {
			continue
		}
		if err := engine.validateEnable(repoID, repoPath); err != nil {
			engine.log.Warn("skipping expertise restart", "repo_id", repoID, "error", err)
			continue
		}
		engine.startIndex(repoID, repoPath)
	}
	return nil
}

func (engine *Engine) ShouldNudge(ctx context.Context, repoID string, account Account) (bool, error) {
	return ShouldNudge(ctx, engine.store, repoID, account)
}

func (engine *Engine) GitAvailable() bool {
	return engine != nil && engine.git != nil && engine.git.Available()
}

func (engine *Engine) GitUnavailableReason() string {
	if engine == nil || engine.git == nil {
		return "system git is unavailable"
	}
	if engine.git.Available() {
		return ""
	}
	return engine.git.UnavailableReason()
}

func (engine *Engine) TouchesYou(ctx context.Context, repoID string, prFiles []string, identity Identity, minLines int) (bool, error) {
	if engine == nil || engine.store == nil {
		return false, errors.New("expertise engine is nil")
	}
	for _, file := range prFiles {
		rows, err := engine.store.GetFileExpertise(ctx, repoID, file)
		if err != nil {
			return false, err
		}
		if fileTouchesIdentity(rows, identity, minLines) {
			return true, nil
		}
	}
	return false, nil
}

func (engine *Engine) TouchesYouForPRs(ctx context.Context, prIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(prIDs))
	for _, prID := range prIDs {
		result[prID] = false
	}
	if engine == nil || engine.store == nil || len(prIDs) == 0 {
		return result, nil
	}

	experts, err := engine.store.ExpertiseForPRs(ctx, prIDs)
	if err != nil {
		return nil, err
	}
	identity := Identity{}
	minLines := DefaultMinSurvivingLines
	if engine.indexer != nil {
		identity = engine.indexer.identity(ctx)
		minLines = engine.minSurvivingLines(ctx)
	}

	checkedByPRFile := make(map[string]map[string]int)
	for _, expert := range experts {
		if result[expert.PRID] || expert.Dormant || !IsExpert(expert.SurvivingLines, minLines, expert.Score) {
			continue
		}
		byFile, ok := checkedByPRFile[expert.PRID]
		if !ok {
			byFile = make(map[string]int)
		}
		if byFile[expert.FilePath] >= maxExpertsPerFile {
			checkedByPRFile[expert.PRID] = byFile
			continue
		}
		byFile[expert.FilePath]++
		checkedByPRFile[expert.PRID] = byFile
		if identity.Matches(expert.AuthorLogin.String, expert.AuthorEmail) {
			result[expert.PRID] = true
		}
	}
	return result, nil
}

func (engine *Engine) Shutdown() {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	runningIndexes := make([]runningIndex, 0, len(engine.running))
	for repoID, running := range engine.running {
		runningIndexes = append(runningIndexes, running)
		delete(engine.running, repoID)
	}
	engine.mu.Unlock()
	for _, running := range runningIndexes {
		running.cancel()
		<-running.done
	}
}

func (engine *Engine) minSurvivingLines(ctx context.Context) int {
	var minLines int
	if err := engine.store.Setting(ctx, "min_surviving_lines", &minLines); err != nil || minLines <= 0 {
		return DefaultMinSurvivingLines
	}
	return minLines
}

func (engine *Engine) validateEnable(repoID, repoPath string) error {
	if engine == nil || engine.store == nil || engine.indexer == nil {
		return errors.New("expertise engine is nil")
	}
	if engine.git == nil {
		return fmt.Errorf("%s: system git is unavailable", expertiseErrorCodeGitUnavailable)
	}
	if !engine.git.Available() {
		return fmt.Errorf("%s: %s", expertiseErrorCodeGitUnavailable, engine.git.UnavailableReason())
	}
	if _, err := os.Stat(repoPath); err != nil {
		return fmt.Errorf("stat expertise repo path %s for %s: %w", repoPath, repoID, err)
	}
	return nil
}

func (engine *Engine) startIndex(repoID, repoPath string) {
	engine.cancel(repoID)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	engine.mu.Lock()
	generation := engine.nextGenerationLocked(repoID)
	engine.running[repoID] = runningIndex{generation: generation, cancel: cancel, done: done}
	engine.mu.Unlock()

	go func() {
		defer close(done)
		defer engine.clearRunning(repoID, generation)
		if err := engine.indexer.Index(ctx, repoID, repoPath); err != nil {
			if errors.Is(err, context.Canceled) {
				engine.finishCanceledIndex(repoID)
				return
			}
			engine.log.Warn("expertise indexing failed", "repo_id", repoID, "error", err)
		}
	}()
}

func (engine *Engine) finishCanceledIndex(repoID string) {
	if err := engine.store.SetRepoState(context.Background(), ExpertiseRepoState{
		RepoID:    repoID,
		Enabled:   true,
		Status:    ExpertiseStatusError,
		ErrorCode: sql.NullString{String: expertiseErrorCodeIndexError, Valid: true},
		Error:     sql.NullString{String: context.Canceled.Error(), Valid: true},
	}); err != nil {
		engine.log.Warn("failed to set expertise cancellation state", "repo_id", repoID, "error", err)
	}
}

func (engine *Engine) nextGenerationLocked(repoID string) uint64 {
	current, ok := engine.running[repoID]
	if !ok {
		return 1
	}
	return current.generation + 1
}

func (engine *Engine) cancel(repoID string) {
	engine.mu.Lock()
	running, ok := engine.running[repoID]
	if ok {
		delete(engine.running, repoID)
	}
	engine.mu.Unlock()
	if ok {
		running.cancel()
		<-running.done
	}
}

func (engine *Engine) clearRunning(repoID string, generation uint64) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	current, ok := engine.running[repoID]
	if ok && current.generation == generation {
		delete(engine.running, repoID)
	}
}

func fileTouchesIdentity(rows []FileExpertiseRow, identity Identity, minLines int) bool {
	matchesChecked := 0
	for _, row := range rows {
		if row.Dormant || !IsExpert(row.SurvivingLines, minLines, row.Score) {
			continue
		}
		matchesChecked++
		if identity.Matches(row.AuthorLogin.String, row.AuthorEmail) {
			return true
		}
		if matchesChecked == maxExpertsPerFile {
			return false
		}
	}
	return false
}
