package expertise

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	aliceName  = "alice"
	aliceEmail = "alice@example.com"
	bobName    = "bob"
	bobEmail   = "bob@example.com"
)

func newSilentExpertiseLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func requireSystemGit(t *testing.T) {
	t.Helper()
	cli := NewGitCLI(newSilentExpertiseLogger())
	if !cli.Available() {
		t.Skipf("system git unavailable: %s", cli.UnavailableReason())
	}
}

func makeFixtureRepo(t *testing.T) string {
	t.Helper()
	requireSystemGit(t)

	dir := t.TempDir()
	runGit(t, dir, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-01-15T10:00:00+00:00",
	}, "init", "-b", "main")
	runGit(t, dir, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-01-15T10:00:00+00:00",
	}, "config", "user.name", aliceName)
	runGit(t, dir, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-01-15T10:00:00+00:00",
	}, "config", "user.email", aliceEmail)

	writeFile(t, filepath.Join(dir, "main.go"), numberedLines("package main", 10))
	writeFile(t, filepath.Join(dir, ".gitattributes"), "generated.go linguist-generated=true\n")
	runGit(t, dir, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-01-15T10:00:00+00:00",
	}, "add", "main.go", ".gitattributes")
	runGit(t, dir, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-01-15T10:00:00+00:00",
	}, "commit", "-m", "alice creates main")

	appendFile(t, filepath.Join(dir, "main.go"), numberedLines("// bob line", 6))
	writeFile(t, filepath.Join(dir, "util.go"), numberedLines("package util", 4))
	writeFile(t, filepath.Join(dir, "generated.go"), numberedLines("// generated", 3))
	runGit(t, dir, gitIdentity{
		name:  bobName,
		email: bobEmail,
		date:  "2026-06-15T10:00:00+00:00",
	}, "add", "main.go", "util.go", "generated.go")
	runGit(t, dir, gitIdentity{
		name:  bobName,
		email: bobEmail,
		date:  "2026-06-15T10:00:00+00:00",
	}, "commit", "-m", "bob appends main and util")

	replaceLine(t, filepath.Join(dir, "main.go"), "// bob line 6", "// bob typo fix 6")
	runGit(t, dir, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-07-14T10:00:00+00:00",
	}, "add", "main.go")
	runGit(t, dir, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-07-14T10:00:00+00:00",
	}, "commit", "-m", "alice typo fix")

	return dir
}

type gitIdentity struct {
	name  string
	email string
	date  string
}

func runGit(t *testing.T, dir string, identity gitIdentity, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+identity.name,
		"GIT_AUTHOR_EMAIL="+identity.email,
		"GIT_COMMITTER_NAME="+identity.name,
		"GIT_COMMITTER_EMAIL="+identity.email,
		"GIT_AUTHOR_DATE="+identity.date,
		"GIT_COMMITTER_DATE="+identity.date,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitCLIProbeAvailable(t *testing.T) {
	cli := NewGitCLI(newSilentExpertiseLogger())
	if cli == nil {
		t.Fatal("NewGitCLI returned nil")
	}
	if cli.Available() && cli.UnavailableReason() != "" {
		t.Errorf("Available=true but UnavailableReason=%q", cli.UnavailableReason())
	}
	if !cli.Available() && cli.UnavailableReason() == "" {
		t.Error("Available=false but UnavailableReason is empty")
	}
}

func TestGitCLIIntegration(t *testing.T) {
	dir := makeFixtureRepo(t)
	cli := NewGitCLI(newSilentExpertiseLogger())
	ctx := context.Background()

	head, err := cli.HeadSHA(ctx, dir)
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if len(head) != 40 || strings.Trim(head, "0123456789abcdef") != "" {
		t.Fatalf("HeadSHA = %q, want 40 lowercase hex chars", head)
	}

	files, err := cli.LsFiles(ctx, dir)
	if err != nil {
		t.Fatalf("LsFiles: %v", err)
	}
	sort.Strings(files)
	assertContains(t, files, "main.go")
	assertContains(t, files, "util.go")

	blame, total, err := cli.BlameFile(ctx, dir, "main.go")
	if err != nil {
		t.Fatalf("BlameFile: %v", err)
	}
	if total != 16 {
		t.Fatalf("BlameFile total = %d, want 16", total)
	}
	if got := blame[AuthorKey{Name: aliceName, Email: aliceEmail}]; got != 11 {
		t.Errorf("alice surviving lines = %d, want 11", got)
	}
	if got := blame[AuthorKey{Name: bobName, Email: bobEmail}]; got != 5 {
		t.Errorf("bob surviving lines = %d, want 5", got)
	}
	if got := sumAuthorCounts(blame); got != total {
		t.Errorf("sumAuthorCounts = %d, want total %d", got, total)
	}

	logResult, err := cli.LogNumstat(ctx, dir, time.Date(2025, time.July, 15, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LogNumstat: %v", err)
	}
	assertLogEvent(t, logResult.Events, aliceEmail, aliceName, "main.go", 10, time.Date(2026, time.January, 15, 10, 0, 0, 0, time.UTC))
	assertLogEvent(t, logResult.Events, bobEmail, bobName, "main.go", 6, time.Date(2026, time.June, 15, 10, 0, 0, 0, time.UTC))
	assertLogEvent(t, logResult.Events, bobEmail, bobName, "util.go", 4, time.Date(2026, time.June, 15, 10, 0, 0, 0, time.UTC))
	assertLogEvent(t, logResult.Events, aliceEmail, aliceName, "main.go", 2, time.Date(2026, time.July, 14, 10, 0, 0, 0, time.UTC))
	if got := logResult.LastCommitAt[aliceEmail]; !got.Equal(time.Date(2026, time.July, 14, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("alice LastCommitAt = %v", got)
	}
	if got := logResult.LastCommitAt[bobEmail]; !got.Equal(time.Date(2026, time.June, 15, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("bob LastCommitAt = %v", got)
	}

	active, err := cli.ActiveAuthors(ctx, dir, time.Date(2026, time.April, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ActiveAuthors: %v", err)
	}
	if !active[aliceEmail] || !active[bobEmail] {
		t.Errorf("ActiveAuthors = %v, want alice and bob active", active)
	}
	active, err = cli.ActiveAuthors(ctx, dir, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ActiveAuthors late since: %v", err)
	}
	if !active[aliceEmail] || active[bobEmail] {
		t.Errorf("ActiveAuthors late = %v, want alice active and bob dormant", active)
	}

	firstCommits, err := cli.LogFirstCommits(ctx, dir)
	if err != nil {
		t.Fatalf("LogFirstCommits: %v", err)
	}
	if got := firstCommits["main.go"]; got != aliceEmail {
		t.Errorf("first main.go author = %q, want %q", got, aliceEmail)
	}
	if got := firstCommits["util.go"]; got != bobEmail {
		t.Errorf("first util.go author = %q, want %q", got, bobEmail)
	}

	generated, err := cli.GeneratedFiles(ctx, dir, []string{"main.go", "generated.go", "util.go"})
	if err != nil {
		t.Fatalf("GeneratedFiles: %v", err)
	}
	if generated["main.go"] {
		t.Error("main.go marked generated, want false")
	}
	if !generated["generated.go"] {
		t.Error("generated.go not marked generated")
	}
	if generated["util.go"] {
		t.Error("util.go marked generated, want false")
	}
}

func TestGitCLIRejectsEmptyRepoPath(t *testing.T) {
	cli := &GitCLI{log: newSilentExpertiseLogger(), available: true}
	if _, err := cli.LsFiles(context.Background(), ""); err == nil {
		t.Fatal("LsFiles with empty repoPath returned nil error")
	}
}

func TestGitCLIParseFunctions(t *testing.T) {
	blame, total, err := parseBlamePorcelain("author Alice\nauthor-mail <alice@example.com>\n\ta\nauthor Bob\nauthor-mail <bob@example.com>\n\tb\n")
	if err != nil {
		t.Fatalf("parseBlamePorcelain: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if blame[AuthorKey{Name: "Alice", Email: "alice@example.com"}] != 1 {
		t.Errorf("alice count = %d, want 1", blame[AuthorKey{Name: "Alice", Email: "alice@example.com"}])
	}
	if blame[AuthorKey{Name: "Bob", Email: "bob@example.com"}] != 1 {
		t.Errorf("bob count = %d, want 1", blame[AuthorKey{Name: "Bob", Email: "bob@example.com"}])
	}

	logOutput := strings.Join([]string{
		"abc\x00alice@example.com\x00Alice\x002026-01-15T10:00:00+00:00",
		"10\t0\tmain.go",
		"-\t-\tbinary.dat",
		"",
	}, "\n")
	result, err := parseLogNumstat(logOutput)
	if err != nil {
		t.Fatalf("parseLogNumstat: %v", err)
	}
	if len(result.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(result.Events))
	}
	if got := result.Events[0].ChangedLines; got != 10 {
		t.Errorf("ChangedLines = %d, want 10", got)
	}
	if _, ok := result.LastCommitAt["alice@example.com"]; !ok {
		t.Error("LastCommitAt missing alice")
	}

	first := parseFirstCommits("\x00alice@example.com\nmain.go\n\x00bob@example.com\nmain.go\nutil.go\n")
	if got := first["main.go"]; got != "alice@example.com" {
		t.Errorf("first main.go = %q, want alice@example.com", got)
	}
	if got := first["util.go"]; got != "bob@example.com" {
		t.Errorf("first util.go = %q, want bob@example.com", got)
	}

	generated := parseGeneratedFiles("main.go\x00linguist-generated\x00unspecified\x00\x00generated.go\x00linguist-generated\x00set\x00true\x00")
	if generated["main.go"] {
		t.Error("main.go generated = true, want false")
	}
	if !generated["generated.go"] {
		t.Error("generated.go generated = false, want true")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func appendFile(t *testing.T, path string, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open append %s: %v", path, err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
}

func replaceLine(t *testing.T, path string, oldLine string, newLine string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	updated := strings.Replace(string(content), oldLine, newLine, 1)
	if updated == string(content) {
		t.Fatalf("replaceLine did not find %q in %s", oldLine, path)
	}
	writeFile(t, path, updated)
}

func numberedLines(prefix string, count int) string {
	var builder strings.Builder
	for i := 1; i <= count; i++ {
		builder.WriteString(fmt.Sprintf("%s %d\n", prefix, i))
	}
	return builder.String()
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%v does not contain %q", values, want)
}

func sumAuthorCounts(counts map[AuthorKey]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func assertLogEvent(t *testing.T, events []LogEvent, email, name, filePath string, changedLines int, committedAt time.Time) {
	t.Helper()
	for _, event := range events {
		if event.Author.Email == email && event.Author.Name == name && event.FilePath == filePath && event.ChangedLines == changedLines && event.CommittedAt.Equal(committedAt) {
			return
		}
	}
	t.Fatalf("missing log event email=%s name=%s file=%s changed=%d at=%v in %#v", email, name, filePath, changedLines, committedAt, events)
}
