package expertise

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	ExpertiseStatusIdle     = "idle"
	ExpertiseStatusIndexing = "indexing"
	ExpertiseStatusReady    = "ready"
	ExpertiseStatusError    = "error"

	ExpertiseIndexProgressEvent = "EXPERTISE_INDEX_PROGRESS"
	ExpertiseReadyEvent         = "EXPERTISE_READY"
	ExpertiseErrorEvent         = "EXPERTISE_ERROR"

	expertiseErrorCodeIndexError     = "index_error"
	expertiseErrorCodeGitUnavailable = "git_unavailable"
	expertiseProgressEveryFiles      = 25
	defaultIndexerWorkers            = 4
)

type expertiseGit interface {
	Available() bool
	UnavailableReason() string
	HeadSHA(context.Context, string) (string, error)
	LsFiles(context.Context, string) ([]string, error)
	GeneratedFiles(context.Context, string, []string) (map[string]bool, error)
	LogNumstat(context.Context, string, time.Time) (*LogResult, error)
	ActiveAuthors(context.Context, string, time.Time) (map[string]bool, error)
	LogFirstCommits(context.Context, string) (map[string]string, error)
	BlameFile(context.Context, string, string) (map[AuthorKey]int, int, error)
}

type Indexer struct {
	log      *slog.Logger
	store    *Store
	git      expertiseGit
	identity func(ctx context.Context) Identity
	emit     func(eventType string, payload any)
	Now      func() time.Time
	Workers  int
}

type indexProgressPayload struct {
	RepoID     string `json:"repo_id"`
	FilesDone  int    `json:"files_done"`
	FilesTotal int    `json:"files_total"`
}

type indexReadyPayload struct {
	RepoID       string `json:"repo_id"`
	IndexedSHA   string `json:"indexed_sha"`
	FilesTotal   int    `json:"files_total"`
	SkippedFiles int    `json:"skipped_files"`
}

type expertiseErrorPayload struct {
	RepoID  string `json:"repo_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type indexFileResult struct {
	filePath string
	rows     []FileExpertiseRow
	err      error
}

type expertiseSignals struct {
	repoID        string
	headSHA       string
	now           time.Time
	identity      Identity
	commitAges    map[string]map[string][]time.Duration
	lastCommitAt  map[string]time.Time
	activeAuthors map[string]bool
	firstCommits  map[string]string
}

func NewIndexer(log *slog.Logger, store *Store, git expertiseGit, identity func(ctx context.Context) Identity) *Indexer {
	if log == nil {
		log = slog.Default()
	}
	if identity == nil {
		identity = func(context.Context) Identity { return Identity{} }
	}
	return &Indexer{
		log:      log,
		store:    store,
		git:      git,
		identity: identity,
		Now:      time.Now,
		Workers:  defaultIndexerWorkers,
	}
}

func (indexer *Indexer) WithEmitter(emit func(eventType string, payload any)) *Indexer {
	indexer.emit = emit
	return indexer
}

func (indexer *Indexer) Index(ctx context.Context, repoID, repoPath string) error {
	if err := indexer.setIndexing(ctx, repoID); err != nil {
		return err
	}

	headSHA, files, signals, err := indexer.prepareIndex(ctx, repoID, repoPath)
	if err != nil {
		return indexer.finishWithError(ctx, repoID, err)
	}

	if err := indexer.store.SetProgress(ctx, repoID, 0, len(files)); err != nil {
		return indexer.finishWithError(ctx, repoID, err)
	}

	processed, skipped, err := indexer.indexFiles(ctx, repoID, repoPath, files, signals)
	if err != nil {
		return indexer.finishWithError(ctx, repoID, err)
	}

	readyState := ExpertiseRepoState{
		RepoID:     repoID,
		Enabled:    true,
		Status:     ExpertiseStatusReady,
		IndexedSHA: sql.NullString{String: headSHA, Valid: true},
		IndexedAt:  sql.NullTime{Time: signals.now, Valid: true},
		FilesTotal: sql.NullInt64{Int64: int64(len(files)), Valid: true},
		FilesDone:  sql.NullInt64{Int64: int64(processed), Valid: true},
		ErrorCode:  sql.NullString{},
		Error:      sql.NullString{},
	}
	if err := indexer.store.SetRepoState(ctx, readyState); err != nil {
		return indexer.finishWithError(ctx, repoID, err)
	}
	indexer.emitEvent(ExpertiseReadyEvent, indexReadyPayload{RepoID: repoID, IndexedSHA: headSHA, FilesTotal: len(files), SkippedFiles: skipped})
	return nil
}

func (indexer *Indexer) setIndexing(ctx context.Context, repoID string) error {
	state := ExpertiseRepoState{
		RepoID:  repoID,
		Enabled: true,
		Status:  ExpertiseStatusIndexing,
	}
	if err := indexer.store.SetRepoState(ctx, state); err != nil {
		return fmt.Errorf("set expertise repo indexing state %s: %w", repoID, err)
	}
	return nil
}

func (indexer *Indexer) prepareIndex(ctx context.Context, repoID, repoPath string) (string, []string, expertiseSignals, error) {
	now := indexer.now()
	headSHA, err := indexer.git.HeadSHA(ctx, repoPath)
	if err != nil {
		return "", nil, expertiseSignals{}, fmt.Errorf("expertise index head sha: %w", err)
	}

	files, err := indexer.indexableFiles(ctx, repoID, repoPath)
	if err != nil {
		return "", nil, expertiseSignals{}, err
	}
	if err := indexer.store.DeleteRepoRows(ctx, repoID); err != nil {
		return "", nil, expertiseSignals{}, err
	}

	logResult, err := indexer.git.LogNumstat(ctx, repoPath, now.AddDate(0, -CommitWindowMonths, 0))
	if err != nil {
		return "", nil, expertiseSignals{}, fmt.Errorf("expertise index log numstat: %w", err)
	}
	activeAuthors, err := indexer.git.ActiveAuthors(ctx, repoPath, now.AddDate(0, 0, -DormantAfterDays))
	if err != nil {
		return "", nil, expertiseSignals{}, fmt.Errorf("expertise index active authors: %w", err)
	}
	firstCommits, err := indexer.git.LogFirstCommits(ctx, repoPath)
	if err != nil {
		return "", nil, expertiseSignals{}, fmt.Errorf("expertise index first commits: %w", err)
	}

	signals := expertiseSignals{
		repoID:        repoID,
		headSHA:       headSHA,
		now:           now,
		identity:      indexer.identity(ctx),
		commitAges:    commitAgesByFileAndAuthor(logResult.Events, now),
		lastCommitAt:  copyTimeMap(logResult.LastCommitAt),
		activeAuthors: copyBoolMap(activeAuthors),
		firstCommits:  copyStringMap(firstCommits),
	}
	return headSHA, files, signals, nil
}

func (indexer *Indexer) indexableFiles(ctx context.Context, repoID, repoPath string) ([]string, error) {
	files, err := indexer.git.LsFiles(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("expertise index ls-files: %w", err)
	}

	excluder, err := indexer.excluder(ctx, repoID)
	if err != nil {
		return nil, err
	}
	globFiltered := make([]string, 0, len(files))
	for _, file := range files {
		if !excluder.Excluded(file) {
			globFiltered = append(globFiltered, file)
		}
	}

	generated, err := indexer.git.GeneratedFiles(ctx, repoPath, globFiltered)
	if err != nil {
		return nil, fmt.Errorf("expertise index generated files: %w", err)
	}
	filtered := make([]string, 0, len(globFiltered))
	for _, file := range globFiltered {
		if !generated[file] {
			filtered = append(filtered, file)
		}
	}
	return filtered, nil
}

func (indexer *Indexer) excluder(ctx context.Context, repoID string) (Excluder, error) {
	var userGlobs []string
	if err := indexer.store.Setting(ctx, excludesSettingKey(repoID), &userGlobs); err != nil && !errors.Is(err, ErrSettingNotFound) {
		return Excluder{}, fmt.Errorf("load expertise excludes for repo %s: %w", repoID, err)
	}
	return New(userGlobs), nil
}

func (indexer *Indexer) indexFiles(ctx context.Context, repoID, repoPath string, files []string, signals expertiseSignals) (int, int, error) {
	results := indexer.blameFiles(ctx, repoPath, files, signals)
	processed := 0
	skipped := 0
	for result := range results {
		if ctx.Err() != nil {
			return processed, skipped, ctx.Err()
		}
		processed++
		if result.err != nil {
			skipped++
			indexer.log.Warn("skipping expertise file after blame failure", "repo_id", repoID, "file_path", result.filePath, "error", result.err)
		} else if err := indexer.store.ReplaceFileRows(ctx, repoID, result.filePath, result.rows); err != nil {
			return processed, skipped, err
		}

		if processed%expertiseProgressEveryFiles == 0 || processed == len(files) {
			if err := indexer.store.SetProgress(ctx, repoID, processed, len(files)); err != nil {
				return processed, skipped, err
			}
			indexer.emitEvent(ExpertiseIndexProgressEvent, indexProgressPayload{RepoID: repoID, FilesDone: processed, FilesTotal: len(files)})
			if ctx.Err() != nil {
				return processed, skipped, ctx.Err()
			}
		}
	}
	return processed, skipped, nil
}

func (indexer *Indexer) blameFiles(ctx context.Context, repoPath string, files []string, signals expertiseSignals) <-chan indexFileResult {
	workerCount := indexer.workerCount(len(files))
	jobs := make(chan string)
	results := make(chan indexFileResult, len(files))
	var wg sync.WaitGroup

	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				blame, total, err := indexer.git.BlameFile(ctx, repoPath, file)
				if err != nil {
					results <- indexFileResult{filePath: file, err: err}
					continue
				}
				results <- indexFileResult{filePath: file, rows: buildFileExpertiseRows(file, blame, total, signals)}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, file := range files {
			select {
			case <-ctx.Done():
				return
			case jobs <- file:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

func (indexer *Indexer) workerCount(fileCount int) int {
	if fileCount <= 0 {
		return 0
	}
	if indexer.Workers <= 0 {
		return min(defaultIndexerWorkers, fileCount)
	}
	return min(indexer.Workers, fileCount)
}

func (indexer *Indexer) finishWithError(ctx context.Context, repoID string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	state := ExpertiseRepoState{
		RepoID:    repoID,
		Enabled:   true,
		Status:    ExpertiseStatusError,
		ErrorCode: sql.NullString{String: indexer.errorCode(), Valid: true},
		Error:     sql.NullString{String: err.Error(), Valid: true},
	}
	if setErr := indexer.store.SetRepoState(contextWithoutCancellation(ctx), state); setErr != nil {
		return fmt.Errorf("%w; additionally failed to set expertise error state: %v", err, setErr)
	}
	indexer.emitEvent(ExpertiseErrorEvent, expertiseErrorPayload{RepoID: repoID, Code: state.ErrorCode.String, Message: err.Error()})
	return err
}

func (indexer *Indexer) errorCode() string {
	if indexer.git != nil && !indexer.git.Available() {
		return expertiseErrorCodeGitUnavailable
	}
	return expertiseErrorCodeIndexError
}

func (indexer *Indexer) emitEvent(eventType string, payload any) {
	if indexer.emit != nil {
		indexer.emit(eventType, payload)
	}
}

func (indexer *Indexer) now() time.Time {
	if indexer.Now == nil {
		return time.Now()
	}
	return indexer.Now()
}

func buildFileExpertiseRows(filePath string, blame map[AuthorKey]int, total int, signals expertiseSignals) []FileExpertiseRow {
	rows := make([]FileExpertiseRow, 0, len(blame))
	for author, surviving := range blame {
		commitScore := CommitScore(signals.commitAges[filePath][author.Email])
		lastCommitAt := nullTime(signals.lastCommitAt[author.Email])
		isOriginal := signals.firstCommits[filePath] == author.Email
		score := Score(surviving, total, commitScore, isOriginal)
		rows = append(rows, FileExpertiseRow{
			RepoID:         signals.repoID,
			FilePath:       filePath,
			AuthorEmail:    author.Email,
			AuthorLogin:    authorLogin(signals.identity, author),
			SurvivingLines: surviving,
			TotalLines:     total,
			CommitScore:    commitScore,
			IsOriginal:     isOriginal,
			Score:          score,
			LastCommitAt:   lastCommitAt,
			Dormant:        !signals.activeAuthors[author.Email],
			ComputedAt:     signals.now,
			ComputedAtSHA:  signals.headSHA,
		})
	}
	return rows
}

func commitAgesByFileAndAuthor(events []LogEvent, now time.Time) map[string]map[string][]time.Duration {
	commitAges := make(map[string]map[string][]time.Duration)
	for _, event := range events {
		if event.ChangedLines <= SubstantialLineThreshold {
			continue
		}
		byAuthor, exists := commitAges[event.FilePath]
		if !exists {
			byAuthor = make(map[string][]time.Duration)
		}
		byAuthor[event.Author.Email] = append(byAuthor[event.Author.Email], now.Sub(event.CommittedAt))
		commitAges[event.FilePath] = byAuthor
	}
	return commitAges
}

func authorLogin(identity Identity, author AuthorKey) sql.NullString {
	if identity.Login == "" || !identity.Matches(author.Name, author.Email) {
		return sql.NullString{}
	}
	return sql.NullString{String: identity.Login, Valid: true}
}

func nullTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: value, Valid: true}
}

func excludesSettingKey(repoID string) string {
	return "excludes:" + repoID
}

func copyTimeMap(input map[string]time.Time) map[string]time.Time {
	output := make(map[string]time.Time, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func copyBoolMap(input map[string]bool) map[string]bool {
	output := make(map[string]bool, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func copyStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func contextWithoutCancellation(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
