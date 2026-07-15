package expertise

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	storedb "github.com/itterno/bistro/internal/store/db"
)

var ErrSettingNotFound = errors.New("expertise setting not found")

type Store struct {
	db      *sql.DB
	queries *storedb.Queries
}

type FileExpertiseRow struct {
	RepoID         string
	FilePath       string
	AuthorEmail    string
	AuthorLogin    sql.NullString
	SurvivingLines int
	TotalLines     int
	CommitScore    float64
	IsOriginal     bool
	Score          float64
	LastCommitAt   sql.NullTime
	Dormant        bool
	ComputedAt     time.Time
	ComputedAtSHA  string
}

type ExpertiseRepoState struct {
	RepoID           string
	Enabled          bool
	Status           string
	IndexedSHA       sql.NullString
	IndexedAt        sql.NullTime
	ErrorCode        sql.NullString
	Error            sql.NullString
	FilesTotal       sql.NullInt64
	FilesDone        sql.NullInt64
	NudgeDismissedAt sql.NullTime
}

type Expert struct {
	RepoID      string
	FilePath    string
	AuthorEmail string
	AuthorLogin sql.NullString
	Score       float64
	Dormant     bool
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db:      db,
		queries: storedb.New(db),
	}
}

func (s *Store) UpsertFileRows(ctx context.Context, rows []FileExpertiseRow) error {
	if len(rows) == 0 {
		return nil
	}

	return s.withTx(ctx, func(queries *storedb.Queries) error {
		return upsertFileRows(ctx, queries, rows)
	})
}

func (s *Store) ReplaceFileRows(ctx context.Context, repoID, filePath string, rows []FileExpertiseRow) error {
	return s.withTx(ctx, func(queries *storedb.Queries) error {
		if err := queries.DeleteFileExpertiseForFile(ctx, storedb.DeleteFileExpertiseForFileParams{
			RepoID:   repoID,
			FilePath: filePath,
		}); err != nil {
			return fmt.Errorf("delete file expertise %s %s: %w", repoID, filePath, err)
		}

		return upsertFileRows(ctx, queries, rows)
	})
}

func (s *Store) GetFileExpertise(ctx context.Context, repoID, filePath string) ([]FileExpertiseRow, error) {
	rows, err := s.queries.GetFileExpertise(ctx, storedb.GetFileExpertiseParams{RepoID: repoID, FilePath: filePath})
	if err != nil {
		return nil, fmt.Errorf("get file expertise %s %s: %w", repoID, filePath, err)
	}

	mapped := make([]FileExpertiseRow, 0, len(rows))
	for _, row := range rows {
		mapped = append(mapped, fileExpertiseFromDB(row))
	}
	return mapped, nil
}

func (s *Store) RepoState(ctx context.Context, repoID string) (ExpertiseRepoState, error) {
	state, err := s.queries.GetExpertiseRepoState(ctx, repoID)
	if err != nil {
		return ExpertiseRepoState{}, fmt.Errorf("get expertise repo state %s: %w", repoID, err)
	}
	return repoStateFromDB(state), nil
}

func (s *Store) SetRepoState(ctx context.Context, state ExpertiseRepoState) error {
	if err := s.queries.UpsertExpertiseRepoState(ctx, upsertRepoStateParams(state)); err != nil {
		return fmt.Errorf("set expertise repo state %s: %w", state.RepoID, err)
	}
	return nil
}

func (s *Store) SetProgress(ctx context.Context, repoID string, filesDone, filesTotal int) error {
	if err := s.queries.UpdateExpertiseIndexProgress(ctx, storedb.UpdateExpertiseIndexProgressParams{
		RepoID:     repoID,
		FilesDone:  sql.NullInt64{Int64: int64(filesDone), Valid: true},
		FilesTotal: sql.NullInt64{Int64: int64(filesTotal), Valid: true},
	}); err != nil {
		return fmt.Errorf("set expertise index progress %s: %w", repoID, err)
	}
	return nil
}

func (s *Store) InterruptedRepos(ctx context.Context) ([]string, error) {
	repos, err := s.queries.ListInterruptedExpertiseRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("list interrupted expertise repos: %w", err)
	}
	return repos, nil
}

func (s *Store) Setting(ctx context.Context, key string, value any) error {
	raw, err := s.queries.GetExpertiseSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSettingNotFound
	}
	if err != nil {
		return fmt.Errorf("get expertise setting %s: %w", key, err)
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return fmt.Errorf("decode expertise setting %s: %w", key, err)
	}
	return nil
}

func (s *Store) SetSetting(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode expertise setting %s: %w", key, err)
	}
	if err := s.queries.UpsertExpertiseSetting(ctx, storedb.UpsertExpertiseSettingParams{Key: key, Value: string(raw)}); err != nil {
		return fmt.Errorf("set expertise setting %s: %w", key, err)
	}
	return nil
}

func (s *Store) ReviewedPRCount(ctx context.Context, repoID, accountProvider, accountLogin, login string) (int64, error) {
	count, err := s.queries.CountReviewedPRsForRepo(ctx, storedb.CountReviewedPRsForRepoParams{
		RepoID:          repoID,
		AccountProvider: accountProvider,
		AccountLogin:    accountLogin,
		Login:           login,
	})
	if err != nil {
		return 0, fmt.Errorf("count reviewed PRs for repo %s: %w", repoID, err)
	}
	return count, nil
}

func (s *Store) IsLocalRepo(ctx context.Context, repoID string) (bool, error) {
	var source sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT source FROM repos WHERE id = ?`, repoID).Scan(&source); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("get expertise repo source %s: %w", repoID, err)
	}
	return source.Valid && source.String == "local", nil
}

func (s *Store) TopExpertsByFile(ctx context.Context, repoID string) (map[string][]Expert, error) {
	rows, err := s.queries.GetTopExpertsForFiles(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("get top experts for repo %s: %w", repoID, err)
	}

	expertsByFile := make(map[string][]Expert)
	for _, row := range rows {
		expertsByFile[row.FilePath] = append(expertsByFile[row.FilePath], Expert{
			RepoID:      row.RepoID,
			FilePath:    row.FilePath,
			AuthorEmail: row.AuthorEmail,
			AuthorLogin: row.AuthorLogin,
			Score:       row.Score,
			Dormant:     row.Dormant != 0,
		})
	}
	return expertsByFile, nil
}

func (s *Store) withTx(ctx context.Context, fn func(*storedb.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin expertise store tx: %w", err)
	}

	if err := fn(s.queries.WithTx(tx)); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("rollback expertise store tx after %w: %v", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit expertise store tx: %w", err)
	}
	return nil
}

func upsertFileRows(ctx context.Context, queries *storedb.Queries, rows []FileExpertiseRow) error {
	for _, row := range rows {
		if err := queries.UpsertFileExpertise(ctx, upsertFileExpertiseParams(row)); err != nil {
			return fmt.Errorf("upsert file expertise %s %s %s: %w", row.RepoID, row.FilePath, row.AuthorEmail, err)
		}
	}
	return nil
}

func upsertFileExpertiseParams(row FileExpertiseRow) storedb.UpsertFileExpertiseParams {
	return storedb.UpsertFileExpertiseParams{
		RepoID:         row.RepoID,
		FilePath:       row.FilePath,
		AuthorEmail:    row.AuthorEmail,
		AuthorLogin:    row.AuthorLogin,
		SurvivingLines: int64(row.SurvivingLines),
		TotalLines:     int64(row.TotalLines),
		CommitScore:    row.CommitScore,
		IsOriginal:     boolToInt64(row.IsOriginal),
		Score:          row.Score,
		LastCommitAt:   row.LastCommitAt,
		Dormant:        boolToInt64(row.Dormant),
		ComputedAt:     row.ComputedAt,
		ComputedAtSha:  row.ComputedAtSHA,
	}
}

func fileExpertiseFromDB(row storedb.FileExpertise) FileExpertiseRow {
	return FileExpertiseRow{
		RepoID:         row.RepoID,
		FilePath:       row.FilePath,
		AuthorEmail:    row.AuthorEmail,
		AuthorLogin:    row.AuthorLogin,
		SurvivingLines: int(row.SurvivingLines),
		TotalLines:     int(row.TotalLines),
		CommitScore:    row.CommitScore,
		IsOriginal:     row.IsOriginal != 0,
		Score:          row.Score,
		LastCommitAt:   row.LastCommitAt,
		Dormant:        row.Dormant != 0,
		ComputedAt:     row.ComputedAt,
		ComputedAtSHA:  row.ComputedAtSha,
	}
}

func upsertRepoStateParams(state ExpertiseRepoState) storedb.UpsertExpertiseRepoStateParams {
	return storedb.UpsertExpertiseRepoStateParams{
		RepoID:           state.RepoID,
		Enabled:          boolToInt64(state.Enabled),
		Status:           state.Status,
		IndexedSha:       state.IndexedSHA,
		IndexedAt:        state.IndexedAt,
		ErrorCode:        state.ErrorCode,
		Error:            state.Error,
		FilesTotal:       state.FilesTotal,
		FilesDone:        state.FilesDone,
		NudgeDismissedAt: state.NudgeDismissedAt,
	}
}

func repoStateFromDB(state storedb.ExpertiseRepoState) ExpertiseRepoState {
	return ExpertiseRepoState{
		RepoID:           state.RepoID,
		Enabled:          state.Enabled != 0,
		Status:           state.Status,
		IndexedSHA:       state.IndexedSha,
		IndexedAt:        state.IndexedAt,
		ErrorCode:        state.ErrorCode,
		Error:            state.Error,
		FilesTotal:       state.FilesTotal,
		FilesDone:        state.FilesDone,
		NudgeDismissedAt: state.NudgeDismissedAt,
	}
}

func boolToInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
