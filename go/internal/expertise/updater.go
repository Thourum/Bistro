package expertise

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Updater struct {
	log      *slog.Logger
	store    *Store
	git      expertiseGit
	identity func(ctx context.Context) Identity
	Now      func() time.Time
}

func NewUpdater(log *slog.Logger, store *Store, git expertiseGit, identity func(ctx context.Context) Identity) *Updater {
	if log == nil {
		log = slog.Default()
	}
	if identity == nil {
		identity = func(context.Context) Identity { return Identity{} }
	}
	return &Updater{
		log:      log,
		store:    store,
		git:      git,
		identity: identity,
		Now:      time.Now,
	}
}

func (updater *Updater) Update(ctx context.Context, repoID, repoPath string, touchedFiles []string) error {
	state, ok, err := updater.readyState(ctx, repoID)
	if err != nil || !ok {
		return err
	}

	headSHA, err := updater.git.HeadSHA(ctx, repoPath)
	if err != nil {
		updater.log.Warn("skipping expertise update after HEAD lookup failure", "repo_id", repoID, "error", err)
		return nil
	}

	candidates, err := updater.updateCandidates(ctx, repoID, repoPath, touchedFiles, headSHA)
	if err != nil {
		updater.log.Warn("skipping expertise update after candidate filtering failure", "repo_id", repoID, "error", err)
		return nil
	}

	now := updater.now()
	if updater.headMoved(state, headSHA) {
		if err := updater.setIndexedSHA(ctx, state, headSHA, now); err != nil {
			return err
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	signals, err := updater.signals(ctx, repoID, repoPath, headSHA, now)
	if err != nil {
		updater.log.Warn("skipping expertise update after signal gathering failure", "repo_id", repoID, "error", err)
		return nil
	}

	for _, file := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		updater.updateFile(ctx, repoID, repoPath, file, signals)
	}
	return nil
}

func (updater *Updater) readyState(ctx context.Context, repoID string) (ExpertiseRepoState, bool, error) {
	state, err := updater.store.RepoState(ctx, repoID)
	if errors.Is(err, sql.ErrNoRows) {
		return ExpertiseRepoState{}, false, nil
	}
	if err != nil {
		return ExpertiseRepoState{}, false, err
	}
	return state, state.Status == ExpertiseStatusReady, nil
}

func (updater *Updater) updateCandidates(ctx context.Context, repoID, repoPath string, touchedFiles []string, headSHA string) ([]string, error) {
	if len(touchedFiles) == 0 {
		return nil, nil
	}

	existingFiles, err := updater.existingFiles(ctx, repoPath)
	if err != nil {
		return nil, err
	}
	excluder, err := updater.excluder(ctx, repoID)
	if err != nil {
		return nil, err
	}

	uniqueExisting := make([]string, 0, len(touchedFiles))
	seen := make(map[string]bool, len(touchedFiles))
	for _, file := range touchedFiles {
		if seen[file] || excluder.Excluded(file) || !existingFiles[file] {
			continue
		}
		seen[file] = true
		uniqueExisting = append(uniqueExisting, file)
	}
	if len(uniqueExisting) == 0 {
		return nil, nil
	}

	generated, err := updater.git.GeneratedFiles(ctx, repoPath, uniqueExisting)
	if err != nil {
		return nil, err
	}

	candidates := make([]string, 0, len(uniqueExisting))
	for _, file := range uniqueExisting {
		if generated[file] || updater.fileFresh(ctx, repoID, file, headSHA) {
			continue
		}
		candidates = append(candidates, file)
	}
	return candidates, nil
}

func (updater *Updater) existingFiles(ctx context.Context, repoPath string) (map[string]bool, error) {
	files, err := updater.git.LsFiles(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("expertise update ls-files: %w", err)
	}
	existing := make(map[string]bool, len(files))
	for _, file := range files {
		existing[file] = true
	}
	return existing, nil
}

func (updater *Updater) excluder(ctx context.Context, repoID string) (Excluder, error) {
	var userGlobs []string
	if err := updater.store.Setting(ctx, excludesSettingKey(repoID), &userGlobs); err != nil && !errors.Is(err, ErrSettingNotFound) {
		return Excluder{}, fmt.Errorf("load expertise excludes for repo %s: %w", repoID, err)
	}
	return New(userGlobs), nil
}

func (updater *Updater) fileFresh(ctx context.Context, repoID, filePath, headSHA string) bool {
	rows, err := updater.store.GetFileExpertise(ctx, repoID, filePath)
	if err != nil {
		updater.log.Warn("treating expertise file as stale after row lookup failure", "repo_id", repoID, "file_path", filePath, "error", err)
		return false
	}
	if len(rows) == 0 {
		return false
	}
	for _, row := range rows {
		if row.ComputedAtSHA != headSHA {
			return false
		}
	}
	return true
}

func (updater *Updater) signals(ctx context.Context, repoID, repoPath, headSHA string, now time.Time) (expertiseSignals, error) {
	logResult, err := updater.git.LogNumstat(ctx, repoPath, now.AddDate(0, -CommitWindowMonths, 0))
	if err != nil {
		return expertiseSignals{}, fmt.Errorf("expertise update log numstat: %w", err)
	}
	activeAuthors, err := updater.git.ActiveAuthors(ctx, repoPath, now.AddDate(0, 0, -DormantAfterDays))
	if err != nil {
		return expertiseSignals{}, fmt.Errorf("expertise update active authors: %w", err)
	}
	firstCommits, err := updater.git.LogFirstCommits(ctx, repoPath)
	if err != nil {
		return expertiseSignals{}, fmt.Errorf("expertise update first commits: %w", err)
	}
	return expertiseSignals{
		repoID:        repoID,
		headSHA:       headSHA,
		now:           now,
		identity:      updater.identity(ctx),
		commitAges:    commitAgesByFileAndAuthor(logResult.Events, now),
		lastCommitAt:  copyTimeMap(logResult.LastCommitAt),
		activeAuthors: copyBoolMap(activeAuthors),
		firstCommits:  copyStringMap(firstCommits),
	}, nil
}

func (updater *Updater) updateFile(ctx context.Context, repoID, repoPath, filePath string, signals expertiseSignals) {
	blame, total, err := updater.git.BlameFile(ctx, repoPath, filePath)
	if err != nil {
		updater.log.Warn("skipping expertise file after update blame failure", "repo_id", repoID, "file_path", filePath, "error", err)
		return
	}
	rows := buildFileExpertiseRows(filePath, blame, total, signals)
	if err := updater.store.ReplaceFileRows(ctx, repoID, filePath, rows); err != nil {
		updater.log.Warn("skipping expertise file after update write failure", "repo_id", repoID, "file_path", filePath, "error", err)
	}
}

func (updater *Updater) headMoved(state ExpertiseRepoState, headSHA string) bool {
	return !state.IndexedSHA.Valid || state.IndexedSHA.String != headSHA
}

func (updater *Updater) setIndexedSHA(ctx context.Context, state ExpertiseRepoState, headSHA string, now time.Time) error {
	updated := ExpertiseRepoState{
		RepoID:           state.RepoID,
		Enabled:          state.Enabled,
		Status:           ExpertiseStatusReady,
		IndexedSHA:       sql.NullString{String: headSHA, Valid: true},
		IndexedAt:        sql.NullTime{Time: now, Valid: true},
		ErrorCode:        sql.NullString{},
		Error:            sql.NullString{},
		FilesTotal:       state.FilesTotal,
		FilesDone:        state.FilesDone,
		NudgeDismissedAt: state.NudgeDismissedAt,
	}
	if err := updater.store.SetRepoState(ctx, updated); err != nil {
		return fmt.Errorf("update expertise indexed sha %s: %w", state.RepoID, err)
	}
	return nil
}

func (updater *Updater) now() time.Time {
	if updater.Now == nil {
		return time.Now()
	}
	return updater.Now()
}
