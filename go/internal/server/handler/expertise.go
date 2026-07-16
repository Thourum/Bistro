package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/ws"
)

const (
	expertiseSettingsAliasEmailsKey       = "alias_emails"
	expertiseSettingsMinSurvivingLinesKey = "min_surviving_lines"
	expertiseSettingsExcludesPrefix       = "excludes:"
)

type expertiseEngine interface {
	Enable(ctx context.Context, repoID, repoPath string) error
	Disable(ctx context.Context, repoID string) error
	GitAvailable() bool
	GitUnavailableReason() string
}

type expertiseRepoStore interface {
	ExpertiseRepo(ctx context.Context, repoID string) (expertiseRepo, bool, error)
}

type expertiseDataStore interface {
	ExpertisePRRepo(ctx context.Context, prID string) (string, bool, error)
	ExpertisePRFiles(ctx context.Context, prID string) ([]string, error)
	FileExpertise(ctx context.Context, repoID, filePath string) ([]expertise.FileExpertiseRow, error)
	RepoState(ctx context.Context, repoID string) (expertise.ExpertiseRepoState, bool, error)
}

type expertiseSettingsStore interface {
	ExpertiseSettings(ctx context.Context) (expertiseSettings, error)
	DismissExpertiseNudge(ctx context.Context, repoID string) error
	SetExpertiseSettings(ctx context.Context, settings expertiseSettings) error
}

type expertiseIdentityReader interface {
	GetCurrentUser(ctx context.Context, provider string) (string, bool, error)
}

type ExpertiseHandlerDeps struct {
	Engine      expertiseEngine
	Repos       expertiseRepoStore
	Data        expertiseDataStore
	Settings    expertiseSettingsStore
	CurrentUser expertiseIdentityReader
	Log         *slog.Logger
}

type expertiseRepo struct {
	RepoID    string
	Source    string
	LocalPath string
}

type expertiseSettings struct {
	AliasEmails       []string            `json:"alias_emails"`
	MinSurvivingLines int                 `json:"min_surviving_lines"`
	ExcludesByRepo    map[string][]string `json:"excludes"`
}

type expertiseRepoSQLStore struct {
	db *sql.DB
}

type expertiseDataSQLStore struct {
	db    *sql.DB
	store *expertise.Store
}

type expertiseSettingsSQLStore struct {
	db    *sql.DB
	store *expertise.Store
}

type expertiseRepoRequest struct {
	RepoID string `json:"repo_id"`
}

type expertisePRRequest struct {
	PRID string `json:"pr_id"`
}

type expertiseFileData struct {
	FilePath string            `json:"file_path"`
	Experts  []expertiseExpert `json:"experts"`
	YourRank *int              `json:"your_rank"`
}

type expertiseExpert struct {
	AuthorEmail    string  `json:"author_email"`
	AuthorLogin    *string `json:"author_login"`
	SurvivingLines int     `json:"surviving_lines"`
	TotalLines     int     `json:"total_lines"`
	CommitScore    float64 `json:"commit_score"`
	IsOriginal     bool    `json:"is_original"`
	Score          float64 `json:"score"`
	Dormant        bool    `json:"dormant"`
}

func NewExpertiseEnableHandler(deps ExpertiseHandlerDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := expertiseLogger(ctx, msg, deps.Log)
		req, err := decodeExpertiseRepoRequest(msg, "EXPERTISE_ENABLE")
		if err != nil {
			return nil, err
		}
		repo, ok, err := deps.Repos.ExpertiseRepo(ctx, req.RepoID)
		if err != nil {
			return nil, err
		}
		if !ok || repo.Source != "local" || repo.LocalPath == "" {
			return expertiseErrorResponse(msg.ID, req.RepoID, "repo_not_local", "link a local checkout to enable expertise"), nil
		}
		if deps.Engine != nil && !deps.Engine.GitAvailable() {
			message := deps.Engine.GitUnavailableReason()
			if message == "" {
				message = "system git is unavailable"
			}
			return expertiseErrorResponse(msg.ID, req.RepoID, "git_unavailable", message), nil
		}
		if deps.Engine == nil {
			return nil, errors.New("expertise engine is nil")
		}
		if err := deps.Engine.Enable(ctx, req.RepoID, repo.LocalPath); err != nil {
			logger.Warn("expertise enable failed", "repo_id", req.RepoID, "error", err)
			return nil, err
		}
		return expertiseEnvelope(msg.ID, "EXPERTISE_ENABLE_ACCEPTED", map[string]interface{}{"repo_id": req.RepoID}), nil
	}
}

func NewExpertiseDisableHandler(deps ExpertiseHandlerDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		req, err := decodeExpertiseRepoRequest(msg, "EXPERTISE_DISABLE")
		if err != nil {
			return nil, err
		}
		if deps.Engine == nil {
			return nil, errors.New("expertise engine is nil")
		}
		if err := deps.Engine.Disable(ctx, req.RepoID); err != nil {
			return nil, err
		}
		return expertiseEnvelope(msg.ID, "EXPERTISE_DISABLE_ACCEPTED", map[string]interface{}{"repo_id": req.RepoID}), nil
	}
}

func NewGetExpertiseHandler(deps ExpertiseHandlerDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req expertisePRRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid GET_EXPERTISE payload: %w", err)
		}
		if req.PRID == "" {
			return nil, errors.New("GET_EXPERTISE requires pr_id")
		}
		if deps.Data == nil {
			return nil, errors.New("expertise data store is nil")
		}
		repoID, ok, err := deps.Data.ExpertisePRRepo(ctx, req.PRID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return expertiseUnavailable(msg.ID, req.PRID, "pr_not_found"), nil
		}
		state, hasState, err := deps.Data.RepoState(ctx, repoID)
		if err != nil {
			return nil, err
		}
		if !hasState || !state.Enabled || state.Status != expertise.ExpertiseStatusReady {
			return expertiseUnavailable(msg.ID, req.PRID, expertiseUnavailableReason(state, hasState)), nil
		}
		files, err := deps.Data.ExpertisePRFiles(ctx, req.PRID)
		if err != nil {
			return nil, err
		}
		identity, minLines := expertiseIdentityAndMinLines(ctx, deps)
		fileData := make([]expertiseFileData, 0, len(files))
		for _, file := range files {
			rows, err := deps.Data.FileExpertise(ctx, repoID, file)
			if err != nil {
				return nil, err
			}
			fileData = append(fileData, expertiseFilePayload(file, rows, identity, minLines))
		}
		return expertiseEnvelope(msg.ID, "EXPERTISE_DATA", map[string]interface{}{
			"pr_id":   req.PRID,
			"repo_id": repoID,
			"status":  "ready",
			"files":   fileData,
		}), nil
	}
}

func NewExpertiseNudgeDismissHandler(deps ExpertiseHandlerDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		req, err := decodeExpertiseRepoRequest(msg, "EXPERTISE_NUDGE_DISMISS")
		if err != nil {
			return nil, err
		}
		if deps.Settings == nil {
			return nil, errors.New("expertise settings store is nil")
		}
		if err := deps.Settings.DismissExpertiseNudge(ctx, req.RepoID); err != nil {
			return nil, err
		}
		return expertiseEnvelope(msg.ID, "EXPERTISE_NUDGE_DISMISSED", map[string]interface{}{"repo_id": req.RepoID}), nil
	}
}

func NewGetExpertiseSettingsHandler(deps ExpertiseHandlerDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		if deps.Settings == nil {
			return nil, errors.New("expertise settings store is nil")
		}
		settings, err := deps.Settings.ExpertiseSettings(ctx)
		if err != nil {
			return nil, err
		}
		return expertiseEnvelope(msg.ID, "EXPERTISE_SETTINGS", map[string]interface{}{
			"alias_emails":        settings.AliasEmails,
			"min_surviving_lines": settings.MinSurvivingLines,
			"excludes":            settings.ExcludesByRepo,
		}), nil
	}
}

func NewSetExpertiseSettingsHandler(deps ExpertiseHandlerDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var settings expertiseSettings
		if err := json.Unmarshal(msg.Data, &settings); err != nil {
			return nil, fmt.Errorf("invalid SET_EXPERTISE_SETTINGS payload: %w", err)
		}
		if settings.MinSurvivingLines <= 0 {
			return nil, errors.New("min_surviving_lines must be greater than 0")
		}
		if settings.AliasEmails == nil {
			settings.AliasEmails = []string{}
		}
		if settings.ExcludesByRepo == nil {
			settings.ExcludesByRepo = map[string][]string{}
		}
		if deps.Settings == nil {
			return nil, errors.New("expertise settings store is nil")
		}
		if err := deps.Settings.SetExpertiseSettings(ctx, settings); err != nil {
			return nil, err
		}
		return expertiseEnvelope(msg.ID, "EXPERTISE_SETTINGS_UPDATED", map[string]interface{}{
			"alias_emails":        settings.AliasEmails,
			"min_surviving_lines": settings.MinSurvivingLines,
			"excludes":            settings.ExcludesByRepo,
		}), nil
	}
}

func NewExpertiseSQLRepoStore(db *sql.DB) *expertiseRepoSQLStore {
	return &expertiseRepoSQLStore{db: db}
}

func NewExpertiseSQLDataStore(db *sql.DB, store *expertise.Store) *expertiseDataSQLStore {
	return &expertiseDataSQLStore{db: db, store: store}
}

func NewExpertiseSQLSettingsStore(db *sql.DB, store *expertise.Store) *expertiseSettingsSQLStore {
	return &expertiseSettingsSQLStore{db: db, store: store}
}

func (s *expertiseRepoSQLStore) ExpertiseRepo(ctx context.Context, repoID string) (expertiseRepo, bool, error) {
	var repo expertiseRepo
	err := s.db.QueryRowContext(ctx, `SELECT id, COALESCE(source, ''), COALESCE(local_path, '') FROM repos WHERE id = ?`, repoID).Scan(&repo.RepoID, &repo.Source, &repo.LocalPath)
	if errors.Is(err, sql.ErrNoRows) {
		return expertiseRepo{}, false, nil
	}
	if err != nil {
		return expertiseRepo{}, false, fmt.Errorf("get expertise repo %s: %w", repoID, err)
	}
	return repo, true, nil
}

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

func (s *expertiseDataSQLStore) FileExpertise(ctx context.Context, repoID, filePath string) ([]expertise.FileExpertiseRow, error) {
	return s.store.GetFileExpertise(ctx, repoID, filePath)
}

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

func (s *expertiseSettingsSQLStore) ExpertiseSettings(ctx context.Context) (expertiseSettings, error) {
	settings := expertiseSettings{
		AliasEmails:       []string{},
		MinSurvivingLines: expertise.DefaultMinSurvivingLines,
		ExcludesByRepo:    map[string][]string{},
	}
	var aliases []string
	if err := s.store.Setting(ctx, expertiseSettingsAliasEmailsKey, &aliases); err != nil && !errors.Is(err, expertise.ErrSettingNotFound) {
		return expertiseSettings{}, err
	} else if err == nil {
		settings.AliasEmails = aliases
	}
	var minLines int
	if err := s.store.Setting(ctx, expertiseSettingsMinSurvivingLinesKey, &minLines); err != nil && !errors.Is(err, expertise.ErrSettingNotFound) {
		return expertiseSettings{}, err
	} else if err == nil && minLines > 0 {
		settings.MinSurvivingLines = minLines
	}
	excludes, err := s.loadExcludes(ctx)
	if err != nil {
		return expertiseSettings{}, err
	}
	settings.ExcludesByRepo = excludes
	return settings, nil
}

func (s *expertiseSettingsSQLStore) DismissExpertiseNudge(ctx context.Context, repoID string) error {
	return s.store.DismissNudge(ctx, repoID)
}

func (s *expertiseSettingsSQLStore) SetExpertiseSettings(ctx context.Context, settings expertiseSettings) error {
	values := map[string]any{
		expertiseSettingsAliasEmailsKey:       settings.AliasEmails,
		expertiseSettingsMinSurvivingLinesKey: settings.MinSurvivingLines,
	}
	for repoID, excludes := range settings.ExcludesByRepo {
		values[expertiseSettingsExcludesPrefix+repoID] = excludes
	}
	if err := s.store.SetSettings(ctx, values); err != nil {
		return err
	}
	for repoID, excludes := range settings.ExcludesByRepo {
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

func decodeExpertiseRepoRequest(msg ws.InboundMessage, messageType string) (expertiseRepoRequest, error) {
	var req expertiseRepoRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return req, fmt.Errorf("invalid %s payload: %w", messageType, err)
	}
	if req.RepoID == "" {
		return req, fmt.Errorf("%s requires repo_id", messageType)
	}
	return req, nil
}

func expertiseIdentityAndMinLines(ctx context.Context, deps ExpertiseHandlerDeps) (expertise.Identity, int) {
	identity := expertise.Identity{}
	if deps.CurrentUser != nil {
		if login, ok, err := deps.CurrentUser.GetCurrentUser(ctx, "github"); err == nil && ok {
			identity.Login = login
		}
	}
	minLines := expertise.DefaultMinSurvivingLines
	if deps.Settings != nil {
		settings, err := deps.Settings.ExpertiseSettings(ctx)
		if err == nil {
			identity.AliasEmails = settings.AliasEmails
			if settings.MinSurvivingLines > 0 {
				minLines = settings.MinSurvivingLines
			}
		}
	}
	return identity, minLines
}

func expertiseFilePayload(filePath string, rows []expertise.FileExpertiseRow, identity expertise.Identity, minLines int) expertiseFileData {
	experts := make([]expertiseExpert, 0, len(rows))
	var yourRank *int
	qualifyingRank := 0
	for _, row := range rows {
		expert := expertiseExpert{
			AuthorEmail:    row.AuthorEmail,
			AuthorLogin:    nullStringPtr(row.AuthorLogin),
			SurvivingLines: row.SurvivingLines,
			TotalLines:     row.TotalLines,
			CommitScore:    row.CommitScore,
			IsOriginal:     row.IsOriginal,
			Score:          row.Score,
			Dormant:        row.Dormant,
		}
		experts = append(experts, expert)
		if !row.Dormant && expertise.IsExpert(row.SurvivingLines, minLines, row.Score) {
			qualifyingRank++
			if yourRank == nil && identity.Matches(row.AuthorLogin.String, row.AuthorEmail) {
				rank := qualifyingRank
				yourRank = &rank
			}
		}
	}
	return expertiseFileData{FilePath: filePath, Experts: experts, YourRank: yourRank}
}

func expertiseUnavailable(id, prID, reason string) map[string]interface{} {
	return expertiseEnvelope(id, "EXPERTISE_DATA", map[string]interface{}{
		"pr_id":  prID,
		"status": "unavailable",
		"reason": reason,
		"files":  []expertiseFileData{},
	})
}

func expertiseUnavailableReason(state expertise.ExpertiseRepoState, hasState bool) string {
	if !hasState || !state.Enabled {
		return "expertise_not_enabled"
	}
	if state.ErrorCode.Valid && state.ErrorCode.String != "" {
		return state.ErrorCode.String
	}
	if state.Status != "" {
		return state.Status
	}
	return "expertise_not_ready"
}

func expertiseErrorResponse(id, repoID, code, message string) map[string]interface{} {
	return expertiseEnvelope(id, "EXPERTISE_ERROR", map[string]interface{}{"repo_id": repoID, "code": code, "message": message})
}

func expertiseEnvelope(id, messageType string, data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"id": id, "type": messageType, "data": data}
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func expertiseLogger(ctx context.Context, msg ws.InboundMessage, log *slog.Logger) *slog.Logger {
	logger := logging.FromContext(ctx)
	if log != nil {
		logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
	}
	return logger
}
