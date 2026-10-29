package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/expertise"
)

const (
	expertiseSettingsAliasEmailsKey       = "alias_emails"
	expertiseSettingsMinSurvivingLinesKey = "min_surviving_lines"
	expertiseSettingsExcludesPrefix       = "excludes:"
	expertiseRepoNotLocal                 = "repo_not_local"
	expertiseGitUnavailable               = "git_unavailable"
	expertisePRNotFound                   = "pr_not_found"
	expertiseNotEnabled                   = "expertise_not_enabled"
	expertiseNotReady                     = "expertise_not_ready"
	expertiseDataReady                    = "ready"
	expertiseDataUnavailable              = "unavailable"
)

// Expertise manages local indexing and per-file expertise for cached PRs.
type Expertise struct {
	Engine      expertiseEngine
	Repos       expertiseRepoStore
	Data        expertiseDataStore
	Settings    expertiseSettingsStore
	CurrentUser currentUserReader
	Events      Emitter
	Panics      PanicReporter
	Log         *slog.Logger
}

type ExpertiseRepo struct {
	RepoID    string
	Source    string
	LocalPath string
}
type ExpertiseSettings struct {
	AliasEmails       []string            `json:"alias_emails"`
	MinSurvivingLines int                 `json:"min_surviving_lines"`
	Excludes          map[string][]string `json:"excludes"`
}
type ExpertiseData struct {
	PRID   string              `json:"pr_id"`
	RepoID string              `json:"repo_id,omitempty"`
	Status string              `json:"status"`
	Reason string              `json:"reason,omitempty"`
	Files  []ExpertiseFileData `json:"files"`
}
type ExpertiseFileData struct {
	FilePath string            `json:"file_path"`
	Experts  []ExpertiseExpert `json:"experts"`
	YourRank *int              `json:"your_rank"`
}
type ExpertiseExpert struct {
	AuthorEmail    string  `json:"author_email"`
	AuthorLogin    *string `json:"author_login"`
	SurvivingLines int     `json:"surviving_lines"`
	TotalLines     int     `json:"total_lines"`
	CommitScore    float64 `json:"commit_score"`
	IsOriginal     bool    `json:"is_original"`
	Score          float64 `json:"score"`
	Dormant        bool    `json:"dormant"`
}

// Enable requires a user-linked checkout and available system git.
func (s *Expertise) Enable(ctx context.Context, req RepoRef) error {
	return guardErr(s.Panics, "Expertise.Enable", func() error {
		repoID := NormalizeID(req.RepoID)
		if repoID == "" {
			return Errorf(CodeInvalid, "repo_id is required")
		}
		repo, ok, err := s.Repos.ExpertiseRepo(ctx, repoID)
		if err != nil {
			return err
		}
		if !ok || repo.Source != repoLocationSourceLocal || repo.LocalPath == "" {
			return WithReason(CodeInvalid, expertiseRepoNotLocal, "link local checkout to enable expertise")
		}
		if s.Engine == nil {
			return Errorf(CodeInternal, "expertise engine is nil")
		}
		if !s.Engine.GitAvailable() {
			message := s.Engine.GitUnavailableReason()
			if message == "" {
				message = "system git is unavailable"
			}
			return WithReason(CodeInvalid, expertiseGitUnavailable, message)
		}
		return s.Engine.Enable(ctx, repoID, repo.LocalPath)
	})
}

// Disable stops indexing and disables expertise for the repository.
func (s *Expertise) Disable(ctx context.Context, req RepoRef) error {
	return guardErr(s.Panics, "Expertise.Disable", func() error {
		repoID := NormalizeID(req.RepoID)
		if repoID == "" {
			return Errorf(CodeInvalid, "repo_id is required")
		}
		if s.Engine == nil {
			return Errorf(CodeInternal, "expertise engine is nil")
		}
		return s.Engine.Disable(ctx, repoID)
	})
}

// Get keeps unavailable expertise as data rather than treating an unindexed repo as a failure.
func (s *Expertise) Get(ctx context.Context, req PRRef) (ExpertiseData, error) {
	return guard(s.Panics, "Expertise.Get", func() (ExpertiseData, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" {
			return ExpertiseData{}, Errorf(CodeInvalid, "pr_id is required")
		}
		repoID, ok, err := s.Data.ExpertisePRRepo(ctx, prID)
		if err != nil {
			return ExpertiseData{}, err
		}
		if !ok {
			return expertiseUnavailable(prID, expertisePRNotFound), nil
		}
		state, hasState, err := s.Data.RepoState(ctx, repoID)
		if err != nil {
			return ExpertiseData{}, err
		}
		if !hasState || !state.Enabled || state.Status != expertise.ExpertiseStatusReady {
			return expertiseUnavailable(prID, expertiseUnavailableReason(state, hasState)), nil
		}
		return s.readyData(ctx, PRRef{PRID: prID}, RepoRef{RepoID: repoID})
	})
}

func (s *Expertise) readyData(ctx context.Context, pr PRRef, repo RepoRef) (ExpertiseData, error) {
	files, err := s.Data.ExpertisePRFiles(ctx, pr.PRID)
	if err != nil {
		return ExpertiseData{}, err
	}
	ranking, err := s.ranking(ctx)
	if err != nil {
		return ExpertiseData{}, err
	}
	response := ExpertiseData{PRID: pr.PRID, RepoID: repo.RepoID, Status: expertiseDataReady, Files: make([]ExpertiseFileData, 0, len(files))}
	for _, file := range files {
		rows, err := s.Data.FileExpertise(ctx, repo.RepoID, file)
		if err != nil {
			return ExpertiseData{}, err
		}
		response.Files = append(response.Files, expertiseFilePayload(file, rows, ranking))
	}
	return response, nil
}

// DismissNudge persists the repository's onboarding-nudge dismissal.
func (s *Expertise) DismissNudge(ctx context.Context, req RepoRef) error {
	return guardErr(s.Panics, "Expertise.DismissNudge", func() error {
		repoID := NormalizeID(req.RepoID)
		if repoID == "" {
			return Errorf(CodeInvalid, "repo_id is required")
		}
		return s.Settings.DismissExpertiseNudge(ctx, repoID)
	})
}

// GetSettings loads defaults and persisted aliases, thresholds and excludes.
func (s *Expertise) GetSettings(ctx context.Context) (ExpertiseSettings, error) {
	return guard(s.Panics, "Expertise.GetSettings", func() (ExpertiseSettings, error) {
		return s.Settings.ExpertiseSettings(ctx)
	})
}

// SetSettings persists preferences and removes expertise rows excluded by the new globs.
func (s *Expertise) SetSettings(ctx context.Context, req ExpertiseSettings) (ExpertiseSettings, error) {
	return guard(s.Panics, "Expertise.SetSettings", func() (ExpertiseSettings, error) {
		if req.MinSurvivingLines <= 0 {
			return ExpertiseSettings{}, Errorf(CodeInvalid, "min_surviving_lines must be greater than 0")
		}
		settings := req
		if settings.AliasEmails == nil {
			settings.AliasEmails = []string{}
		}
		settings.Excludes = make(map[string][]string, len(req.Excludes))
		for repoID, globs := range req.Excludes {
			id := NormalizeID(repoID)
			if id == "" {
				return ExpertiseSettings{}, Errorf(CodeInvalid, "excludes repo_id is required")
			}
			if _, exists := settings.Excludes[id]; exists {
				return ExpertiseSettings{}, Errorf(CodeInvalid, "duplicate excludes repo_id")
			}
			settings.Excludes[id] = globs
		}
		if err := s.Settings.SetExpertiseSettings(ctx, settings); err != nil {
			return ExpertiseSettings{}, err
		}
		if s.Events != nil {
			s.Events.Emit(EventExpertiseSettingsUpdated, settings)
		}
		return settings, nil
	})
}

type expertiseRanking struct {
	identity expertise.Identity
	minLines int
}

func (s *Expertise) ranking(ctx context.Context) (expertiseRanking, error) {
	ranking := expertiseRanking{minLines: expertise.DefaultMinSurvivingLines}
	if s.CurrentUser != nil {
		login, ok, err := s.CurrentUser.Get(ctx, githubProvider)
		if err != nil {
			return expertiseRanking{}, err
		}
		if ok {
			ranking.identity.Login = login
		}
	}
	if s.Settings != nil {
		settings, err := s.Settings.ExpertiseSettings(ctx)
		if err != nil {
			return expertiseRanking{}, err
		}
		ranking.identity.AliasEmails = settings.AliasEmails
		if settings.MinSurvivingLines > 0 {
			ranking.minLines = settings.MinSurvivingLines
		}
	}
	return ranking, nil
}

func expertiseFilePayload(filePath string, rows []expertise.FileExpertiseRow, ranking expertiseRanking) ExpertiseFileData {
	experts := make([]ExpertiseExpert, 0, len(rows))
	var yourRank *int
	qualifyingRank := 0
	for _, row := range rows {
		var login *string
		if row.AuthorLogin.Valid {
			value := row.AuthorLogin.String
			login = &value
		}
		experts = append(experts, ExpertiseExpert{AuthorEmail: row.AuthorEmail, AuthorLogin: login,
			SurvivingLines: row.SurvivingLines, TotalLines: row.TotalLines, CommitScore: row.CommitScore,
			IsOriginal: row.IsOriginal, Score: row.Score, Dormant: row.Dormant})
		if row.Dormant || !expertise.IsExpert(row.SurvivingLines, ranking.minLines, row.Score) {
			continue
		}
		qualifyingRank++
		if yourRank == nil && ranking.identity.Matches(row.AuthorLogin.String, row.AuthorEmail) {
			rank := qualifyingRank
			yourRank = &rank
		}
	}
	return ExpertiseFileData{FilePath: filePath, Experts: experts, YourRank: yourRank}
}

func expertiseUnavailable(prID, reason string) ExpertiseData {
	return ExpertiseData{PRID: prID, Status: expertiseDataUnavailable, Reason: reason, Files: []ExpertiseFileData{}}
}

func expertiseUnavailableReason(state expertise.ExpertiseRepoState, hasState bool) string {
	if !hasState || !state.Enabled {
		return expertiseNotEnabled
	}
	if state.ErrorCode.Valid && state.ErrorCode.String != "" {
		return state.ErrorCode.String
	}
	if state.Status != "" {
		return state.Status
	}
	return expertiseNotReady
}

type expertiseRepoSQLStore struct{ db *sql.DB }
type expertiseDataSQLStore struct {
	db    *sql.DB
	store *expertise.Store
}
type expertiseSettingsSQLStore struct {
	db    *sql.DB
	store *expertise.Store
}

// NewExpertiseSQLRepoStore supplies SQL-backed repository lookups.
func NewExpertiseSQLRepoStore(db *sql.DB) *expertiseRepoSQLStore {
	return &expertiseRepoSQLStore{db: db}
}

// NewExpertiseSQLDataStore supplies SQL-backed PR and expertise lookups.
func NewExpertiseSQLDataStore(db *sql.DB, store *expertise.Store) *expertiseDataSQLStore {
	return &expertiseDataSQLStore{db: db, store: store}
}

// NewExpertiseSQLSettingsStore supplies SQL-backed settings storage.
func NewExpertiseSQLSettingsStore(db *sql.DB, store *expertise.Store) *expertiseSettingsSQLStore {
	return &expertiseSettingsSQLStore{db: db, store: store}
}

// ExpertiseRepo retrieves local checkout information for a repository.
func (s *expertiseRepoSQLStore) ExpertiseRepo(ctx context.Context, repoID string) (ExpertiseRepo, bool, error) {
	var repo ExpertiseRepo
	err := s.db.QueryRowContext(ctx, `SELECT id, COALESCE(source, ''), COALESCE(local_path, '') FROM repos WHERE id = ?`, repoID).Scan(&repo.RepoID, &repo.Source, &repo.LocalPath)
	if errors.Is(err, sql.ErrNoRows) {
		return ExpertiseRepo{}, false, nil
	}
	if err != nil {
		return ExpertiseRepo{}, false, fmt.Errorf("get expertise repo %s: %w", repoID, err)
	}
	return repo, true, nil
}

// ExpertisePRRepo resolves the owning repository for a PR.
func (s *expertiseDataSQLStore) ExpertisePRRepo(ctx context.Context, prID string) (string, bool, error) {
	var repoID string
	err := s.db.QueryRowContext(ctx, `SELECT repo_id FROM prs WHERE id = ?`, prID).Scan(&repoID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get expertise PR repo %s: %w", prID, err)
	}
	return repoID, true, nil
}

// ExpertisePRFiles lists the paths with stored PR diffs.
func (s *expertiseDataSQLStore) ExpertisePRFiles(ctx context.Context, prID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT file_path FROM pr_diffs WHERE pr_id = ? ORDER BY file_path`, prID)
	if err != nil {
		return nil, fmt.Errorf("get expertise PR files %s: %w", prID, err)
	}
	defer rows.Close()
	files := []string{}
	for rows.Next() {
		var file string
		if err := rows.Scan(&file); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

// FileExpertise returns indexed expertise for one path.
func (s *expertiseDataSQLStore) FileExpertise(ctx context.Context, repoID, filePath string) ([]expertise.FileExpertiseRow, error) {
	return s.store.GetFileExpertise(ctx, repoID, filePath)
}

// RepoState reports whether expertise indexing is ready.
func (s *expertiseDataSQLStore) RepoState(ctx context.Context, repoID string) (expertise.ExpertiseRepoState, bool, error) {
	state, err := s.store.RepoState(ctx, repoID)
	if errors.Is(err, sql.ErrNoRows) {
		return expertise.ExpertiseRepoState{}, false, nil
	}
	if err != nil {
		return expertise.ExpertiseRepoState{}, false, err
	}
	return state, true, nil
}

// ExpertiseSettings loads stored settings with defaults for unset values.
func (s *expertiseSettingsSQLStore) ExpertiseSettings(ctx context.Context) (ExpertiseSettings, error) {
	settings := ExpertiseSettings{
		AliasEmails:       []string{},
		MinSurvivingLines: expertise.DefaultMinSurvivingLines,
		Excludes:          map[string][]string{},
	}
	var aliases []string
	if err := s.store.Setting(ctx, expertiseSettingsAliasEmailsKey, &aliases); err != nil && !errors.Is(err, expertise.ErrSettingNotFound) {
		return ExpertiseSettings{}, err
	} else if err == nil {
		settings.AliasEmails = aliases
	}
	var minLines int
	if err := s.store.Setting(ctx, expertiseSettingsMinSurvivingLinesKey, &minLines); err != nil && !errors.Is(err, expertise.ErrSettingNotFound) {
		return ExpertiseSettings{}, err
	} else if err == nil && minLines > 0 {
		settings.MinSurvivingLines = minLines
	}
	excludes, err := s.loadExcludes(ctx)
	if err != nil {
		return ExpertiseSettings{}, err
	}
	settings.Excludes = excludes
	return settings, nil
}

// DismissExpertiseNudge persists the dismissed nudge for a repository.
func (s *expertiseSettingsSQLStore) DismissExpertiseNudge(ctx context.Context, repoID string) error {
	return s.store.DismissNudge(ctx, repoID)
}

// SetExpertiseSettings stores settings and removes excluded expertise rows.
func (s *expertiseSettingsSQLStore) SetExpertiseSettings(ctx context.Context, settings ExpertiseSettings) error {
	values := map[string]any{
		expertiseSettingsAliasEmailsKey:       settings.AliasEmails,
		expertiseSettingsMinSurvivingLinesKey: settings.MinSurvivingLines,
	}
	for repoID, excludes := range settings.Excludes {
		values[expertiseSettingsExcludesPrefix+repoID] = excludes
	}
	if err := s.store.SetSettings(ctx, values); err != nil {
		return err
	}
	for repoID, excludes := range settings.Excludes {
		if err := s.store.DeleteExcludedRows(ctx, repoID, expertise.New(excludes)); err != nil {
			return err
		}
	}
	return nil
}

func (s *expertiseSettingsSQLStore) loadExcludes(ctx context.Context) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM expertise_settings WHERE key LIKE ?`, expertiseSettingsExcludesPrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("list expertise excludes settings: %w", err)
	}
	defer rows.Close()
	excludes := map[string][]string{}
	for rows.Next() {
		var key string
		var raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var globs []string
		if err := json.Unmarshal([]byte(raw), &globs); err != nil {
			return nil, fmt.Errorf("decode expertise excludes %s: %w", key, err)
		}
		repoID := key[len(expertiseSettingsExcludesPrefix):]
		excludes[repoID] = globs
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return excludes, nil
}
