package handler

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/expertise"
)

type fakeExpertiseEngine struct {
	enabledRepoID   string
	enabledRepoPath string
	disabledRepoID  string
	enableErr       error
	disableErr      error
	gitAvailable    bool
	gitReason       string
}

func (f *fakeExpertiseEngine) Enable(_ context.Context, repoID, repoPath string) error {
	f.enabledRepoID = repoID
	f.enabledRepoPath = repoPath
	return f.enableErr
}

func (f *fakeExpertiseEngine) Disable(_ context.Context, repoID string) error {
	f.disabledRepoID = repoID
	return f.disableErr
}

func (f *fakeExpertiseEngine) GitAvailable() bool { return f.gitAvailable }

func (f *fakeExpertiseEngine) GitUnavailableReason() string { return f.gitReason }

type fakeExpertiseRepoStore struct {
	repos map[string]expertiseRepo
}

func (f *fakeExpertiseRepoStore) ExpertiseRepo(_ context.Context, repoID string) (expertiseRepo, bool, error) {
	repo, ok := f.repos[repoID]
	return repo, ok, nil
}

type fakeExpertiseDataStore struct {
	repoID string
	files  map[string][]expertise.FileExpertiseRow
	state  expertise.ExpertiseRepoState
	err    error
}

func (f *fakeExpertiseDataStore) ExpertisePRRepo(_ context.Context, prID string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	if prID == "" || f.repoID == "" {
		return "", false, nil
	}
	return f.repoID, true, nil
}

func (f *fakeExpertiseDataStore) ExpertisePRFiles(_ context.Context, prID string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if prID == "" {
		return nil, nil
	}
	files := make([]string, 0, len(f.files))
	for file := range f.files {
		files = append(files, file)
	}
	return files, nil
}

func (f *fakeExpertiseDataStore) FileExpertise(_ context.Context, repoID, filePath string) ([]expertise.FileExpertiseRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	if repoID != f.repoID {
		return nil, nil
	}
	return f.files[filePath], nil
}

func (f *fakeExpertiseDataStore) RepoState(_ context.Context, repoID string) (expertise.ExpertiseRepoState, bool, error) {
	if f.err != nil {
		return expertise.ExpertiseRepoState{}, false, f.err
	}
	if repoID != f.repoID || f.state.RepoID == "" {
		return expertise.ExpertiseRepoState{}, false, nil
	}
	return f.state, true, nil
}

type fakeExpertiseSettingsStore struct {
	settings  expertiseSettings
	dismissed string
	err       error
}

func (f *fakeExpertiseSettingsStore) ExpertiseSettings(_ context.Context) (expertiseSettings, error) {
	return f.settings, f.err
}

func (f *fakeExpertiseSettingsStore) DismissExpertiseNudge(_ context.Context, repoID string) error {
	if f.err != nil {
		return f.err
	}
	f.dismissed = repoID
	return nil
}

func (f *fakeExpertiseSettingsStore) SetExpertiseSettings(_ context.Context, settings expertiseSettings) error {
	if f.err != nil {
		return f.err
	}
	f.settings = settings
	return nil
}

type fakeExpertiseIdentityReader struct {
	login string
	found bool
	err   error
}

func (f *fakeExpertiseIdentityReader) GetCurrentUser(_ context.Context, _ string) (string, bool, error) {
	return f.login, f.found, f.err
}

func TestExpertiseEnableStartsIndexingForLocalRepo(t *testing.T) {
	engine := &fakeExpertiseEngine{gitAvailable: true}
	repos := &fakeExpertiseRepoStore{repos: map[string]expertiseRepo{
		"github:owner/repo": {RepoID: "github:owner/repo", Source: "local", LocalPath: "/tmp/repo"},
	}}
	h := NewExpertiseEnableHandler(ExpertiseHandlerDeps{Engine: engine, Repos: repos, Log: testRepoLogger()})

	resp, err := callHandler(t, h, "EXPERTISE_ENABLE", map[string]string{"repo_id": "github:owner/repo"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if engine.enabledRepoID != "github:owner/repo" || engine.enabledRepoPath != "/tmp/repo" {
		t.Fatalf("enabled repo/path = %q/%q", engine.enabledRepoID, engine.enabledRepoPath)
	}
	if responseType(resp) != "EXPERTISE_ENABLE_ACCEPTED" {
		t.Fatalf("type = %q, want EXPERTISE_ENABLE_ACCEPTED", responseType(resp))
	}
}

func TestExpertiseEnableRejectsCacheRepo(t *testing.T) {
	engine := &fakeExpertiseEngine{gitAvailable: true}
	repos := &fakeExpertiseRepoStore{repos: map[string]expertiseRepo{
		"github:owner/repo": {RepoID: "github:owner/repo", Source: "cache", LocalPath: "/tmp/repo"},
	}}
	h := NewExpertiseEnableHandler(ExpertiseHandlerDeps{Engine: engine, Repos: repos, Log: testRepoLogger()})

	resp, err := callHandler(t, h, "EXPERTISE_ENABLE", map[string]string{"repo_id": "github:owner/repo"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if responseType(resp) != "EXPERTISE_ERROR" {
		t.Fatalf("type = %q, want EXPERTISE_ERROR", responseType(resp))
	}
	data := extractData(t, resp)
	if data["code"] != "repo_not_local" {
		t.Fatalf("code = %v, want repo_not_local", data["code"])
	}
	if engine.enabledRepoID != "" {
		t.Fatalf("engine should not be called, got %q", engine.enabledRepoID)
	}
}

func TestExpertiseEnableRejectsGitUnavailable(t *testing.T) {
	engine := &fakeExpertiseEngine{gitAvailable: false, gitReason: "git not found"}
	repos := &fakeExpertiseRepoStore{repos: map[string]expertiseRepo{
		"github:owner/repo": {RepoID: "github:owner/repo", Source: "local", LocalPath: "/tmp/repo"},
	}}
	h := NewExpertiseEnableHandler(ExpertiseHandlerDeps{Engine: engine, Repos: repos, Log: testRepoLogger()})

	resp, err := callHandler(t, h, "EXPERTISE_ENABLE", map[string]string{"repo_id": "github:owner/repo"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if responseType(resp) != "EXPERTISE_ERROR" {
		t.Fatalf("type = %q, want EXPERTISE_ERROR", responseType(resp))
	}
	data := extractData(t, resp)
	if data["code"] != "git_unavailable" {
		t.Fatalf("code = %v, want git_unavailable", data["code"])
	}
}

func TestExpertiseDisableCancelsAndDisables(t *testing.T) {
	engine := &fakeExpertiseEngine{gitAvailable: true}
	h := NewExpertiseDisableHandler(ExpertiseHandlerDeps{Engine: engine, Log: testRepoLogger()})

	resp, err := callHandler(t, h, "EXPERTISE_DISABLE", map[string]string{"repo_id": "github:owner/repo"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if engine.disabledRepoID != "github:owner/repo" {
		t.Fatalf("disabled repo = %q", engine.disabledRepoID)
	}
	if responseType(resp) != "EXPERTISE_DISABLE_ACCEPTED" {
		t.Fatalf("type = %q, want EXPERTISE_DISABLE_ACCEPTED", responseType(resp))
	}
}

func TestGetExpertiseReturnsTopExpertsAndYourRank(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	store := &fakeExpertiseDataStore{
		repoID: "github:owner/repo",
		state:  expertise.ExpertiseRepoState{RepoID: "github:owner/repo", Enabled: true, Status: expertise.ExpertiseStatusReady},
		files: map[string][]expertise.FileExpertiseRow{
			"main.go": {
				{RepoID: "github:owner/repo", FilePath: "main.go", AuthorEmail: "alice@example.com", AuthorLogin: sql.NullString{String: "alice", Valid: true}, SurvivingLines: 80, TotalLines: 100, CommitScore: 0.5, IsOriginal: true, Score: 74, ComputedAt: now, ComputedAtSHA: "sha"},
				{RepoID: "github:owner/repo", FilePath: "main.go", AuthorEmail: "bob@example.com", AuthorLogin: sql.NullString{String: "bob", Valid: true}, SurvivingLines: 60, TotalLines: 100, CommitScore: 0.1, Score: 36, ComputedAt: now, ComputedAtSHA: "sha"},
			},
		},
	}
	current := &fakeExpertiseIdentityReader{login: "alice", found: true}
	h := NewGetExpertiseHandler(ExpertiseHandlerDeps{Data: store, Settings: &fakeExpertiseSettingsStore{}, CurrentUser: current, Log: testRepoLogger()})

	resp, err := callHandler(t, h, "GET_EXPERTISE", map[string]string{"pr_id": "pr-1"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if responseType(resp) != "EXPERTISE_DATA" {
		t.Fatalf("type = %q, want EXPERTISE_DATA", responseType(resp))
	}
	data := extractData(t, resp)
	if data["status"] != "ready" {
		t.Fatalf("status = %v, want ready", data["status"])
	}
	files := data["files"].([]expertiseFileData)
	if len(files) != 1 {
		t.Fatalf("files length = %d, want 1", len(files))
	}
	if files[0].FilePath != "main.go" || files[0].YourRank == nil || *files[0].YourRank != 1 {
		t.Fatalf("file data = %+v, want main.go with your_rank 1", files[0])
	}
	if len(files[0].Experts) != 2 || files[0].Experts[0].AuthorEmail != "alice@example.com" {
		t.Fatalf("experts = %+v", files[0].Experts)
	}
}

func TestGetExpertiseUnavailableWhenRepoNotIndexed(t *testing.T) {
	store := &fakeExpertiseDataStore{
		repoID: "github:owner/repo",
		state:  expertise.ExpertiseRepoState{RepoID: "github:owner/repo", Enabled: false, Status: expertise.ExpertiseStatusIdle},
		files:  map[string][]expertise.FileExpertiseRow{"main.go": nil},
	}
	h := NewGetExpertiseHandler(ExpertiseHandlerDeps{Data: store, Settings: &fakeExpertiseSettingsStore{}, Log: testRepoLogger()})

	resp, err := callHandler(t, h, "GET_EXPERTISE", map[string]string{"pr_id": "pr-1"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	data := extractData(t, resp)
	if responseType(resp) != "EXPERTISE_DATA" || data["status"] != "unavailable" {
		t.Fatalf("response = %#v", resp)
	}
	if data["reason"] == "" {
		t.Fatalf("reason should be populated")
	}
}

func TestExpertiseNudgeDismiss(t *testing.T) {
	settings := &fakeExpertiseSettingsStore{}
	h := NewExpertiseNudgeDismissHandler(ExpertiseHandlerDeps{Settings: settings, Log: testRepoLogger()})

	resp, err := callHandler(t, h, "EXPERTISE_NUDGE_DISMISS", map[string]string{"repo_id": "github:owner/repo"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if settings.dismissed != "github:owner/repo" {
		t.Fatalf("dismissed repo = %q", settings.dismissed)
	}
	if responseType(resp) != "EXPERTISE_NUDGE_DISMISSED" {
		t.Fatalf("type = %q, want EXPERTISE_NUDGE_DISMISSED", responseType(resp))
	}
}

func TestExpertiseSettingsRoundTrip(t *testing.T) {
	store := &fakeExpertiseSettingsStore{settings: expertiseSettings{
		AliasEmails:       []string{"alice@example.com"},
		MinSurvivingLines: 25,
		ExcludesByRepo:    map[string][]string{"github:owner/repo": []string{"dist/**"}},
	}}
	getHandler := NewGetExpertiseSettingsHandler(ExpertiseHandlerDeps{Settings: store, Log: testRepoLogger()})
	setHandler := NewSetExpertiseSettingsHandler(ExpertiseHandlerDeps{Settings: store, Log: testRepoLogger()})

	getResp, err := callHandler(t, getHandler, "GET_EXPERTISE_SETTINGS", map[string]string{})
	if err != nil {
		t.Fatalf("get handler: %v", err)
	}
	if responseType(getResp) != "EXPERTISE_SETTINGS" {
		t.Fatalf("get type = %q", responseType(getResp))
	}

	_, err = callHandler(t, setHandler, "SET_EXPERTISE_SETTINGS", expertiseSettings{
		AliasEmails:       []string{"alice@users.noreply.github.com"},
		MinSurvivingLines: 60,
		ExcludesByRepo:    map[string][]string{"github:owner/repo": []string{"vendor/**"}},
	})
	if err != nil {
		t.Fatalf("set handler: %v", err)
	}
	if store.settings.MinSurvivingLines != 60 || store.settings.AliasEmails[0] != "alice@users.noreply.github.com" {
		t.Fatalf("settings not updated: %+v", store.settings)
	}
}

func TestSetExpertiseSettingsRejectsInvalidMinLines(t *testing.T) {
	store := &fakeExpertiseSettingsStore{}
	h := NewSetExpertiseSettingsHandler(ExpertiseHandlerDeps{Settings: store, Log: testRepoLogger()})

	_, err := callHandler(t, h, "SET_EXPERTISE_SETTINGS", expertiseSettings{MinSurvivingLines: 0})
	if err == nil {
		t.Fatalf("expected invalid min_surviving_lines error")
	}
}

func TestGetExpertisePropagatesStoreErrors(t *testing.T) {
	store := &fakeExpertiseDataStore{err: errors.New("db unavailable")}
	h := NewGetExpertiseHandler(ExpertiseHandlerDeps{Data: store, Settings: &fakeExpertiseSettingsStore{}, Log: testRepoLogger()})

	_, err := callHandler(t, h, "GET_EXPERTISE", map[string]string{"pr_id": "pr-1"})
	if err == nil {
		t.Fatalf("expected store error")
	}
}

func responseType(resp interface{}) string {
	env, ok := resp.(map[string]interface{})
	if !ok {
		return ""
	}
	value, _ := env["type"].(string)
	return value
}

func TestRequestPRListPopulatesTouchesYou(t *testing.T) {
	db := newHandlerTestDB(t)
	repoID := "github:owner/repo"
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	insertExpertisePRListRow(t, db, "pr-qualifies", repoID, "qualifies.go", 1)
	insertExpertisePRListRow(t, db, "pr-below-threshold", repoID, "below.go", 2)
	insertExpertisePRListRow(t, db, "pr-other-user", repoID, "other.go", 3)

	expertiseStore := expertise.NewStore(db)
	if err := expertiseStore.SetRepoState(context.Background(), expertise.ExpertiseRepoState{
		RepoID: repoID, Enabled: true, Status: expertise.ExpertiseStatusReady,
	}); err != nil {
		t.Fatalf("SetRepoState: %v", err)
	}
	if err := expertiseStore.UpsertFileRows(context.Background(), []expertise.FileExpertiseRow{
		{RepoID: repoID, FilePath: "qualifies.go", AuthorEmail: "first@example.com", SurvivingLines: 80, TotalLines: 100, Score: 90, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "qualifies.go", AuthorEmail: "alice@example.com", SurvivingLines: expertise.DefaultMinSurvivingLines, TotalLines: 100, Score: 70, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "qualifies.go", AuthorEmail: "third@example.com", SurvivingLines: 80, TotalLines: 100, Score: 60, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "below.go", AuthorEmail: "alice@example.com", SurvivingLines: expertise.DefaultMinSurvivingLines - 1, TotalLines: 100, Score: expertise.ExpertScoreFloor - 1, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "other.go", AuthorEmail: "bob@example.com", SurvivingLines: 100, TotalLines: 100, Score: 95, ComputedAt: now, ComputedAtSHA: "sha"},
	}); err != nil {
		t.Fatalf("UpsertFileRows: %v", err)
	}
	indexer := expertise.NewIndexer(testRepoLogger(), expertiseStore, nil, func(context.Context) expertise.Identity {
		return expertise.Identity{Login: "alice", AliasEmails: []string{"alice@example.com"}}
	})
	engine := expertise.NewEngine(testRepoLogger(), expertiseStore, nil, indexer, nil)
	h := NewRequestPRListHandlerWithExpertise(db, engine, testRepoLogger())

	resp, err := callHandler(t, h, "REQUEST_PR_LIST", map[string]interface{}{"filter": "all", "limit": 10})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	data := extractData(t, resp)
	prs := data["prs"].([]prRow)
	got := map[string]bool{}
	for _, pr := range prs {
		got[pr.ID] = pr.TouchesYou
	}
	want := map[string]bool{"pr-qualifies": true, "pr-below-threshold": false, "pr-other-user": false}
	for prID, expected := range want {
		if got[prID] != expected {
			t.Fatalf("touches_you[%s] = %v, want %v (all %#v)", prID, got[prID], expected, got)
		}
	}
}

func insertExpertisePRListRow(t *testing.T, db *sql.DB, prID, repoID, filePath string, number int) {
	t.Helper()
	created := time.Date(2026, 7, 15, 12, number, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha, repo_id)
		VALUES (?, 'github', 'owner', 'repo', ?, 'title', 'author', 'open', ?, ?, 'main', 'feature', 'base', 'head', ?)`, prID, number, created, created, repoID); err != nil {
		t.Fatalf("insert pr %s: %v", prID, err)
	}
	if _, err := db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, ?, '', 0, 0)`, prID, filePath); err != nil {
		t.Fatalf("insert pr_diff %s %s: %v", prID, filePath, err)
	}
}
