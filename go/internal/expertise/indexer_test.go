package expertise

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

const fixtureRepoID = "github:owner/repo"

func TestIndexerIndexesFixtureRepo(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db.DB)
	repoPath := makeFixtureRepo(t)
	ctx := context.Background()
	now := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)

	indexer := NewIndexer(newSilentExpertiseLogger(), store, NewGitCLI(newSilentExpertiseLogger()), func(context.Context) Identity {
		return Identity{Login: aliceName, AccountEmail: aliceEmail}
	})
	indexer.Now = func() time.Time { return now }
	indexer.Workers = 2
	if err := store.UpsertFileRows(ctx, []FileExpertiseRow{
		{RepoID: fixtureRepoID, FilePath: "generated.go", AuthorEmail: aliceEmail, SurvivingLines: 99, TotalLines: 100, Score: 99, ComputedAt: now.Add(-time.Hour), ComputedAtSHA: "old"},
		{RepoID: fixtureRepoID, FilePath: "vendor/dependency.go", AuthorEmail: aliceEmail, SurvivingLines: 99, TotalLines: 100, Score: 99, ComputedAt: now.Add(-time.Hour), ComputedAtSHA: "old"},
	}); err != nil {
		t.Fatalf("seed stale rows: %v", err)
	}

	if err := indexer.Index(ctx, fixtureRepoID, repoPath); err != nil {
		t.Fatalf("Index: %v", err)
	}

	mainRows, err := store.GetFileExpertise(ctx, fixtureRepoID, "main.go")
	if err != nil {
		t.Fatalf("GetFileExpertise main.go: %v", err)
	}
	if len(mainRows) != 2 {
		t.Fatalf("main.go rows = %d, want 2: %#v", len(mainRows), mainRows)
	}
	alice := requireExpertiseRow(t, mainRows, aliceEmail)
	bob := requireExpertiseRow(t, mainRows, bobEmail)
	if !alice.IsOriginal {
		t.Error("alice IsOriginal = false, want true")
	}
	if alice.AuthorLogin.String != aliceName || !alice.AuthorLogin.Valid {
		t.Errorf("alice AuthorLogin = %#v, want valid alice", alice.AuthorLogin)
	}
	if bob.SurvivingLines <= 0 {
		t.Errorf("bob SurvivingLines = %d, want > 0", bob.SurvivingLines)
	}
	if bob.AuthorLogin.Valid {
		t.Errorf("bob AuthorLogin = %#v, want null", bob.AuthorLogin)
	}
	assertScoreRange(t, alice.Score)
	assertScoreRange(t, bob.Score)
	if alice.Dormant {
		t.Error("alice dormant = true, want false")
	}
	if !bob.Dormant {
		t.Error("bob dormant = false, want true")
	}

	attributesRows, err := store.GetFileExpertise(ctx, fixtureRepoID, ".gitattributes")
	if err != nil {
		t.Fatalf("GetFileExpertise .gitattributes: %v", err)
	}
	if len(attributesRows) != 1 || attributesRows[0].AuthorEmail != aliceEmail {
		t.Fatalf(".gitattributes rows = %#v, want one alice row", attributesRows)
	}

	utilRows, err := store.GetFileExpertise(ctx, fixtureRepoID, "util.go")
	if err != nil {
		t.Fatalf("GetFileExpertise util.go: %v", err)
	}
	if len(utilRows) != 1 || utilRows[0].AuthorEmail != bobEmail {
		t.Fatalf("util.go rows = %#v, want one bob row", utilRows)
	}

	generatedRows, err := store.GetFileExpertise(ctx, fixtureRepoID, "generated.go")
	if err != nil {
		t.Fatalf("GetFileExpertise generated.go: %v", err)
	}
	if len(generatedRows) != 0 {
		t.Fatalf("generated.go rows = %#v, want none", generatedRows)
	}
	vendorRows, err := store.GetFileExpertise(ctx, fixtureRepoID, "vendor/dependency.go")
	if err != nil {
		t.Fatalf("GetFileExpertise vendor/dependency.go: %v", err)
	}
	if len(vendorRows) != 0 {
		t.Fatalf("vendor/dependency.go rows = %#v, want none", vendorRows)
	}

	state, err := store.RepoState(ctx, fixtureRepoID)
	if err != nil {
		t.Fatalf("RepoState: %v", err)
	}
	head, err := NewGitCLI(newSilentExpertiseLogger()).HeadSHA(ctx, repoPath)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if state.Status != ExpertiseStatusReady {
		t.Fatalf("state.Status = %q, want %q", state.Status, ExpertiseStatusReady)
	}
	if !state.IndexedSHA.Valid || state.IndexedSHA.String != head {
		t.Fatalf("state.IndexedSHA = %#v, want %q", state.IndexedSHA, head)
	}
	if !state.FilesTotal.Valid || state.FilesTotal.Int64 != 3 {
		t.Fatalf("state.FilesTotal = %#v, want 3", state.FilesTotal)
	}
	if !state.FilesDone.Valid || state.FilesDone.Int64 != 3 {
		t.Fatalf("state.FilesDone = %#v, want 3", state.FilesDone)
	}
}

func TestIndexerEmitsProgressEveryTwentyFiveFiles(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db.DB)
	git := newFakeIndexerGit(50)
	ctx := context.Background()
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	events := make([]indexProgressPayload, 0)

	indexer := NewIndexer(newSilentExpertiseLogger(), store, git, func(context.Context) Identity {
		return Identity{Login: aliceName, AccountEmail: aliceEmail}
	})
	indexer.Now = func() time.Time { return now }
	indexer.Workers = 4
	indexer.emit = func(eventType string, payload any) {
		if eventType != ExpertiseIndexProgressEvent {
			return
		}
		progress, ok := payload.(indexProgressPayload)
		if !ok {
			t.Fatalf("progress payload type = %T, want indexProgressPayload", payload)
		}
		events = append(events, progress)
	}

	if err := indexer.Index(ctx, fixtureRepoID, t.TempDir()); err != nil {
		t.Fatalf("Index: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("progress events = %#v, want two events", events)
	}
	if events[0].FilesDone != 25 || events[0].FilesTotal != 50 {
		t.Fatalf("first progress = %#v, want 25/50", events[0])
	}
	if events[1].FilesDone != 50 || events[1].FilesTotal != 50 {
		t.Fatalf("second progress = %#v, want 50/50", events[1])
	}
}

func TestIndexerCancellationLeavesStatusIndexing(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db.DB)
	git := newFakeIndexerGit(3)
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)

	indexer := NewIndexer(newSilentExpertiseLogger(), store, git, func(context.Context) Identity {
		return Identity{Login: aliceName, AccountEmail: aliceEmail}
	})
	indexer.Now = func() time.Time { return now }
	indexer.Workers = 1
	indexer.emit = func(eventType string, payload any) {
		if eventType == ExpertiseIndexProgressEvent {
			cancel()
		}
	}

	err := indexer.Index(ctx, fixtureRepoID, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Index error = %v, want context.Canceled", err)
	}

	state, err := store.RepoState(context.Background(), fixtureRepoID)
	if err != nil {
		t.Fatalf("RepoState after cancel: %v", err)
	}
	if state.Status != ExpertiseStatusIndexing {
		t.Fatalf("state.Status = %q, want %q", state.Status, ExpertiseStatusIndexing)
	}
	if state.IndexedSHA.Valid {
		t.Fatalf("state.IndexedSHA = %#v, want invalid", state.IndexedSHA)
	}
}

func requireExpertiseRow(t *testing.T, rows []FileExpertiseRow, authorEmail string) FileExpertiseRow {
	t.Helper()
	for _, row := range rows {
		if row.AuthorEmail == authorEmail {
			return row
		}
	}
	t.Fatalf("rows %#v missing author %s", rows, authorEmail)
	return FileExpertiseRow{}
}

func assertScoreRange(t *testing.T, score float64) {
	t.Helper()
	if score <= 0 || score > 100 {
		t.Fatalf("score = %f, want (0,100]", score)
	}
}

type fakeIndexerGit struct {
	files []string
}

func newFakeIndexerGit(fileCount int) *fakeIndexerGit {
	files := make([]string, 0, fileCount)
	for index := 0; index < fileCount; index++ {
		files = append(files, "file"+string(rune('a'+index%26))+".go")
	}
	return &fakeIndexerGit{files: files}
}

func (git *fakeIndexerGit) Available() bool { return true }

func (git *fakeIndexerGit) UnavailableReason() string { return "" }

func (git *fakeIndexerGit) HeadSHA(context.Context, string) (string, error) {
	return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil
}

func (git *fakeIndexerGit) LsFiles(context.Context, string) ([]string, error) {
	files := make([]string, len(git.files))
	copy(files, git.files)
	return files, nil
}

func (git *fakeIndexerGit) GeneratedFiles(context.Context, string, []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (git *fakeIndexerGit) LogNumstat(context.Context, string, time.Time) (*LogResult, error) {
	return &LogResult{LastCommitAt: map[string]time.Time{
		aliceEmail: time.Date(2026, time.July, 14, 10, 0, 0, 0, time.UTC),
	}, Events: nil}, nil
}

func (git *fakeIndexerGit) ActiveAuthors(context.Context, string, time.Time) (map[string]bool, error) {
	return map[string]bool{aliceEmail: true}, nil
}

func (git *fakeIndexerGit) LogFirstCommits(context.Context, string) (map[string]string, error) {
	first := make(map[string]string, len(git.files))
	for _, file := range git.files {
		first[file] = aliceEmail
	}
	return first, nil
}

func (git *fakeIndexerGit) BlameFile(context.Context, string, string) (map[AuthorKey]int, int, error) {
	return map[AuthorKey]int{{Name: aliceName, Email: aliceEmail}: 1}, 1, nil
}

var _ expertiseGit = (*fakeIndexerGit)(nil)
var _ = sql.NullString{}
