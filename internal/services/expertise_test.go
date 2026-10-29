package services

import (
	"context"
	"database/sql"
	"errors"
	"github.com/itterno/bistro/internal/services/servicestest"
	"reflect"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/expertise"
	"github.com/itterno/bistro/internal/store/storetest"
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
	repos map[string]ExpertiseRepo
}

func (f *fakeExpertiseRepoStore) ExpertiseRepo(_ context.Context, repoID string) (ExpertiseRepo, bool, error) {
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
	settings  ExpertiseSettings
	dismissed string
	err       error
}

func (f *fakeExpertiseSettingsStore) ExpertiseSettings(_ context.Context) (ExpertiseSettings, error) {
	return f.settings, f.err
}

func (f *fakeExpertiseSettingsStore) DismissExpertiseNudge(_ context.Context, repoID string) error {
	if f.err != nil {
		return f.err
	}
	f.dismissed = repoID
	return nil
}

func (f *fakeExpertiseSettingsStore) SetExpertiseSettings(_ context.Context, settings ExpertiseSettings) error {
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

func (f *fakeExpertiseIdentityReader) Get(_ context.Context, _ string) (string, bool, error) {
	return f.login, f.found, f.err
}

func TestExpertiseEnableStartsIndexingForLocalRepo(t *testing.T) {
	engine := &fakeExpertiseEngine{gitAvailable: true}
	repos := &fakeExpertiseRepoStore{repos: map[string]ExpertiseRepo{
		"github:owner/repo": {RepoID: "github:owner/repo", Source: "local", LocalPath: "/tmp/repo"},
	}}
	s := &Expertise{Engine: engine, Repos: repos, Log: testRepoLogger()}

	err := s.Enable(context.Background(), RepoRef{RepoID: "GitHub:Owner/Repo"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if engine.enabledRepoID != "github:owner/repo" || engine.enabledRepoPath != "/tmp/repo" {
		t.Fatalf("enabled repo/path = %q/%q", engine.enabledRepoID, engine.enabledRepoPath)
	}

}

func TestExpertiseEnableRejectsCacheRepo(t *testing.T) {
	engine := &fakeExpertiseEngine{gitAvailable: true}
	repos := &fakeExpertiseRepoStore{repos: map[string]ExpertiseRepo{
		"github:owner/repo": {RepoID: "github:owner/repo", Source: "cache", LocalPath: "/tmp/repo"},
	}}
	s := &Expertise{Engine: engine, Repos: repos, Log: testRepoLogger()}

	err := s.Enable(context.Background(), RepoRef{RepoID: "GitHub:Owner/Repo"})
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeInvalid || se.Reason != "repo_not_local" {
		t.Fatalf("error = %v", err)
	}
	if engine.enabledRepoID != "" {
		t.Fatalf("engine should not be called, got %q", engine.enabledRepoID)
	}
}

func TestExpertiseEnableRejectsGitUnavailable(t *testing.T) {
	engine := &fakeExpertiseEngine{gitAvailable: false, gitReason: "git not found"}
	repos := &fakeExpertiseRepoStore{repos: map[string]ExpertiseRepo{
		"github:owner/repo": {RepoID: "github:owner/repo", Source: "local", LocalPath: "/tmp/repo"},
	}}
	s := &Expertise{Engine: engine, Repos: repos, Log: testRepoLogger()}

	err := s.Enable(context.Background(), RepoRef{RepoID: "GitHub:Owner/Repo"})
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeInvalid || se.Reason != "git_unavailable" {
		t.Fatalf("error = %v", err)
	}
}

func TestExpertiseDisableCancelsAndDisables(t *testing.T) {
	engine := &fakeExpertiseEngine{gitAvailable: true}
	s := &Expertise{Engine: engine, Log: testRepoLogger()}

	err := s.Disable(context.Background(), RepoRef{RepoID: "GitHub:Owner/Repo"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if engine.disabledRepoID != "github:owner/repo" {
		t.Fatalf("disabled repo = %q", engine.disabledRepoID)
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
	s := &Expertise{Data: store, Settings: &fakeExpertiseSettingsStore{}, CurrentUser: current, Log: testRepoLogger()}

	resp, err := s.Get(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if resp.Status != "ready" {
		t.Fatalf("status = %s", resp.Status)
	}
	files := resp.Files
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
	s := &Expertise{Data: store, Settings: &fakeExpertiseSettingsStore{}, Log: testRepoLogger()}

	resp, err := s.Get(context.Background(), PRRef{PRID: "pr-1"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if resp.Status != "unavailable" || resp.Files == nil || len(resp.Files) != 0 {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Reason != "expertise_not_enabled" {
		t.Fatalf("reason should be populated")
	}
}

func TestExpertiseNudgeDismiss(t *testing.T) {
	settings := &fakeExpertiseSettingsStore{}
	s := &Expertise{Settings: settings, Log: testRepoLogger()}

	err := s.DismissNudge(context.Background(), RepoRef{RepoID: "GitHub:Owner/Repo"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if settings.dismissed != "github:owner/repo" {
		t.Fatalf("dismissed repo = %q", settings.dismissed)
	}

}

func TestExpertiseSettingsRoundTrip(t *testing.T) {
	store := &fakeExpertiseSettingsStore{settings: ExpertiseSettings{
		AliasEmails:       []string{"alice@example.com"},
		MinSurvivingLines: 25,
		Excludes:          map[string][]string{"github:owner/repo": []string{"dist/**"}},
	}}
	events := &servicestest.Recorder{}
	s := &Expertise{Settings: store, Events: events, Log: testRepoLogger()}

	getResp, err := s.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("get handler: %v", err)
	}
	if getResp.MinSurvivingLines != 25 {
		t.Fatalf("settings = %+v", getResp)
	}

	req := ExpertiseSettings{
		AliasEmails:       []string{"alice@users.noreply.github.com"},
		MinSurvivingLines: 60,
		Excludes:          map[string][]string{"GitHub:Owner/Repo": {"vendor/**"}},
	}
	updated, err := s.SetSettings(context.Background(), req)
	if err != nil {
		t.Fatalf("set handler: %v", err)
	}
	payloads := events.Named(EventExpertiseSettingsUpdated)
	if len(payloads) != 1 || !reflect.DeepEqual(payloads[0], updated) || !reflect.DeepEqual(updated.Excludes["github:owner/repo"], []string{"vendor/**"}) || req.Excludes["GitHub:Owner/Repo"] == nil {
		t.Fatalf("settings event or normalization incorrect: %#v", payloads)
	}
	if store.settings.MinSurvivingLines != 60 || store.settings.AliasEmails[0] != "alice@users.noreply.github.com" {
		t.Fatalf("settings not updated: %+v", store.settings)
	}
}

func TestSetExpertiseSettingsRejectsInvalidMinLines(t *testing.T) {
	store := &fakeExpertiseSettingsStore{}
	s := &Expertise{Settings: store, Log: testRepoLogger()}

	_, err := s.SetSettings(context.Background(), ExpertiseSettings{MinSurvivingLines: 0})
	if err == nil {
		t.Fatalf("expected invalid min_surviving_lines error")
	}
}

func TestGetExpertisePropagatesStoreErrors(t *testing.T) {
	store := &fakeExpertiseDataStore{err: errors.New("db unavailable")}
	s := &Expertise{Data: store, Settings: &fakeExpertiseSettingsStore{}, Log: testRepoLogger()}

	_, err := s.Get(context.Background(), PRRef{PRID: "pr-1"})
	if err == nil {
		t.Fatalf("expected store error")
	}
}

func TestExpertiseMissingPRAndStorageFailure(t *testing.T) {
	for _, test := range []struct {
		cause error
		code  Code
	}{
		{nil, ""}, {errors.New("db unavailable"), CodeInternal}, {context.Canceled, CodeCancelled}, {context.DeadlineExceeded, CodeTimeout},
	} {
		s := &Expertise{Data: &fakeExpertiseDataStore{err: test.cause}}
		got, err := s.Get(context.Background(), PRRef{PRID: "pr-missing"})
		if test.cause == nil {
			if err != nil || got.Status != "unavailable" || got.Reason != "pr_not_found" || got.Files == nil || len(got.Files) != 0 {
				t.Fatalf("missing PR = %+v, error = %v", got, err)
			}
			continue
		}
		var se *ServiceError
		if !errors.As(err, &se) || se.Code != test.code {
			t.Fatalf("storage error = %v", err)
		}
	}
}

func TestExpertiseSQLSettingsPersistExcludesAndDismissal(t *testing.T) {
	db := storetest.Open(t).DB
	ctx := context.Background()
	index := expertise.NewStore(db)
	s := &Expertise{Settings: NewExpertiseSQLSettingsStore(db, index)}
	defaults, err := s.GetSettings(ctx)
	if err != nil || defaults.MinSurvivingLines != expertise.DefaultMinSurvivingLines || defaults.Excludes == nil || defaults.AliasEmails == nil {
		t.Fatalf("defaults = %+v, error = %v", defaults, err)
	}
	const repoID = "github:owner/repo"
	if err := index.UpsertFileRows(ctx, []expertise.FileExpertiseRow{
		{RepoID: repoID, FilePath: "vendor/lib.go", AuthorEmail: "alice@example.com", SurvivingLines: 100, Score: 90, ComputedAt: time.Now(), ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "main.go", AuthorEmail: "alice@example.com", SurvivingLines: 100, Score: 90, ComputedAt: time.Now(), ComputedAtSHA: "sha"},
	}); err != nil {
		t.Fatal(err)
	}
	req := ExpertiseSettings{MinSurvivingLines: 60, Excludes: map[string][]string{repoID: {"vendor/**"}}}
	updated, err := s.SetSettings(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(loaded, updated) {
		t.Fatalf("persisted settings = %+v, error = %v", loaded, err)
	}
	excluded, err := index.GetFileExpertise(ctx, repoID, "vendor/lib.go")
	if err != nil || len(excluded) != 0 {
		t.Fatal("excluded expertise retained")
	}
	retained, err := index.GetFileExpertise(ctx, repoID, "main.go")
	if err != nil || len(retained) != 1 {
		t.Fatal("unexcluded expertise removed")
	}
	if err := s.DismissNudge(ctx, RepoRef{RepoID: "GitHub:Owner/Repo"}); err != nil {
		t.Fatal(err)
	}
	state, err := index.RepoState(ctx, repoID)
	if err != nil || !state.NudgeDismissedAt.Valid {
		t.Fatalf("dismissal not persisted: %v", err)
	}
}

func TestRequestPRListPopulatesTouchesYou(t *testing.T) {
	db := storetest.Open(t).DB
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
	s := &PR{DB: db, TouchesYou: engine, CurrentUser: &fakeExpertiseIdentityReader{login: "alice", found: true}, Log: testRepoLogger()}

	resp, err := s.List(context.Background(), PRListRequest{Filter: "all", Limit: 10})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	prs := resp.PRs
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
