package expertise

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	gitProbeTimeout       = 3 * time.Second
	gitDefaultTimeout     = 30 * time.Second
	gitBlameTimeout       = 30 * time.Second
	gitLogTimeout         = 120 * time.Second
	gitGeneratedTimeout   = 30 * time.Second
	gitHeadSHALength      = 40
	logHeaderFieldCount   = 4
	numstatFieldCount     = 3
	checkAttrFieldCount   = 3
	checkAttrGeneratedSet = "set"
)

var gitSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type AuthorKey struct {
	Name  string
	Email string
}

type LogEvent struct {
	Author       AuthorKey
	FilePath     string
	ChangedLines int
	CommittedAt  time.Time
}

type LogResult struct {
	Events       []LogEvent
	LastCommitAt map[string]time.Time
}

type GitCLI struct {
	log               *slog.Logger
	available         bool
	unavailableReason string
}

func NewGitCLI(log *slog.Logger) *GitCLI {
	if log == nil {
		log = slog.Default()
	}
	cli := &GitCLI{log: log}
	cli.probeGit()
	return cli
}

func (cli *GitCLI) Available() bool { return cli.available }

func (cli *GitCLI) UnavailableReason() string { return cli.unavailableReason }

func (cli *GitCLI) probeGit() {
	ctx, cancel := context.WithTimeout(context.Background(), gitProbeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "git", "--version").Output()
	if err != nil {
		cli.available = false
		cli.unavailableReason = fmt.Sprintf("git not found or probe failed: %v", err)
		cli.log.Info("git unavailable for expertise", "reason", cli.unavailableReason)
		return
	}

	version := strings.TrimSpace(string(out))
	if version == "" {
		cli.available = false
		cli.unavailableReason = "git version probe returned empty output"
		cli.log.Info("git unavailable for expertise", "reason", cli.unavailableReason)
		return
	}

	cli.available = true
	cli.log.Info("git available for expertise", "version", version)
}

func (cli *GitCLI) HeadSHA(ctx context.Context, repoPath string) (string, error) {
	out, err := cli.runGit(ctx, repoPath, gitDefaultTimeout, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	sha := strings.TrimSpace(string(out))
	if !gitSHAPattern.MatchString(sha) {
		return "", fmt.Errorf("expertise git: HEAD SHA %q is not %d-char lowercase hex", sha, gitHeadSHALength)
	}
	return sha, nil
}

func (cli *GitCLI) LsFiles(ctx context.Context, repoPath string) ([]string, error) {
	out, err := cli.runGit(ctx, repoPath, gitDefaultTimeout, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	return parseNULSeparated(string(out)), nil
}

func (cli *GitCLI) BlameFile(ctx context.Context, repoPath, path string) (map[AuthorKey]int, int, error) {
	out, err := cli.runGit(ctx, repoPath, gitBlameTimeout, "blame", "--line-porcelain", "HEAD", "--", path)
	if err != nil {
		return nil, 0, err
	}
	return parseBlamePorcelain(string(out))
}

func (cli *GitCLI) LogNumstat(ctx context.Context, repoPath string, since time.Time) (*LogResult, error) {
	out, err := cli.runGit(ctx, repoPath, gitLogTimeout,
		"log", "--numstat", "--no-renames", "--pretty=format:%H%x00%ae%x00%an%x00%aI", "--since="+since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return parseLogNumstat(string(out))
}

func (cli *GitCLI) ActiveAuthors(ctx context.Context, repoPath string, since time.Time) (map[string]bool, error) {
	out, err := cli.runGit(ctx, repoPath, gitLogTimeout, "log", "--pretty=format:%ae", "--since="+since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return parseActiveAuthors(string(out)), nil
}

func (cli *GitCLI) LogFirstCommits(ctx context.Context, repoPath string) (map[string]string, error) {
	out, err := cli.runGit(ctx, repoPath, gitLogTimeout, "log", "--reverse", "--diff-filter=A", "--name-only", "--no-renames", "--pretty=format:%x00%ae")
	if err != nil {
		return nil, err
	}
	return parseFirstCommits(string(out)), nil
}

func (cli *GitCLI) GeneratedFiles(ctx context.Context, repoPath string, paths []string) (map[string]bool, error) {
	generated := make(map[string]bool)
	if len(paths) == 0 {
		return generated, nil
	}

	input := strings.Join(paths, "\x00") + "\x00"
	out, err := cli.runGitWithInput(ctx, repoPath, gitGeneratedTimeout, []byte(input), "check-attr", "linguist-generated", "--stdin", "-z")
	if err != nil {
		return nil, err
	}
	return parseGeneratedFiles(string(out)), nil
}

func (cli *GitCLI) runGit(ctx context.Context, repoPath string, timeout time.Duration, args ...string) ([]byte, error) {
	return cli.runGitWithInput(ctx, repoPath, timeout, nil, args...)
}

func (cli *GitCLI) runGitWithInput(ctx context.Context, repoPath string, timeout time.Duration, input []byte, args ...string) ([]byte, error) {
	if !cli.available {
		reason := cli.unavailableReason
		if reason == "" {
			reason = "git unavailable"
		}
		return nil, fmt.Errorf("expertise git unavailable: %s", reason)
	}
	if repoPath == "" {
		return nil, fmt.Errorf("expertise git: repoPath is empty (would fall back to process cwd)")
	}
	if _, err := os.Stat(repoPath); err != nil {
		return nil, fmt.Errorf("expertise git: repoPath %q is not accessible: %w", repoPath, err)
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "git", args...)
	cmd.Dir = repoPath
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if runCtx.Err() != nil {
			return nil, fmt.Errorf("expertise git %s timed out or cancelled: %w", strings.Join(args, " "), runCtx.Err())
		}
		return nil, fmt.Errorf("expertise git %s exited non-zero: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func parseNULSeparated(output string) []string {
	parts := strings.Split(output, "\x00")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func parseBlamePorcelain(output string) (map[AuthorKey]int, int, error) {
	counts := make(map[AuthorKey]int)
	current := AuthorKey{}
	total := 0

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "author "):
			current = AuthorKey{Name: strings.TrimPrefix(line, "author "), Email: current.Email}
		case strings.HasPrefix(line, "author-mail "):
			current = AuthorKey{Name: current.Name, Email: stripMailBrackets(strings.TrimPrefix(line, "author-mail "))}
		case strings.HasPrefix(line, "\t"):
			if current.Email == "" && current.Name == "" {
				return nil, 0, fmt.Errorf("blame porcelain line missing author metadata")
			}
			counts[current]++
			total++
		}
	}

	return counts, total, nil
}

func parseLogNumstat(output string) (*LogResult, error) {
	result := &LogResult{LastCommitAt: make(map[string]time.Time)}
	var current *LogEvent

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.Contains(line, "\x00") {
			commit, err := parseLogHeader(line)
			if err != nil {
				return nil, err
			}
			current = &commit
			setLatestCommit(result.LastCommitAt, commit.Author.Email, commit.CommittedAt)
			continue
		}
		if current == nil {
			continue
		}
		event, ok, err := parseNumstatLine(line, *current)
		if err != nil {
			return nil, err
		}
		if ok {
			result.Events = append(result.Events, event)
		}
	}

	return result, nil
}

func parseLogHeader(line string) (LogEvent, error) {
	fields := strings.Split(line, "\x00")
	if len(fields) != logHeaderFieldCount {
		return LogEvent{}, fmt.Errorf("git log header has %d fields, want %d: %q", len(fields), logHeaderFieldCount, line)
	}
	committedAt, err := time.Parse(time.RFC3339, fields[3])
	if err != nil {
		return LogEvent{}, fmt.Errorf("parse git log commit time %q: %w", fields[3], err)
	}
	return LogEvent{Author: AuthorKey{Email: fields[1], Name: fields[2]}, CommittedAt: committedAt}, nil
}

func parseNumstatLine(line string, current LogEvent) (LogEvent, bool, error) {
	fields := strings.Split(line, "\t")
	if len(fields) < numstatFieldCount {
		return LogEvent{}, false, nil
	}
	if fields[0] == "-" || fields[1] == "-" {
		return LogEvent{}, false, nil
	}
	added, err := strconv.Atoi(fields[0])
	if err != nil {
		return LogEvent{}, false, fmt.Errorf("parse numstat added lines %q: %w", fields[0], err)
	}
	deleted, err := strconv.Atoi(fields[1])
	if err != nil {
		return LogEvent{}, false, fmt.Errorf("parse numstat deleted lines %q: %w", fields[1], err)
	}
	return LogEvent{
		Author:       current.Author,
		CommittedAt:  current.CommittedAt,
		ChangedLines: added + deleted,
		FilePath:     fields[2],
	}, true, nil
}

func parseActiveAuthors(output string) map[string]bool {
	active := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		email := strings.TrimSpace(line)
		if email != "" {
			active[email] = true
		}
	}
	return active
}

func parseFirstCommits(output string) map[string]string {
	firstCommits := make(map[string]string)
	currentEmail := ""

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "\x00") {
			currentEmail = strings.TrimSpace(strings.TrimPrefix(line, "\x00"))
			continue
		}
		path := strings.TrimSpace(line)
		if path == "" || currentEmail == "" {
			continue
		}
		if _, exists := firstCommits[path]; !exists {
			firstCommits[path] = currentEmail
		}
	}

	return firstCommits
}

func parseGeneratedFiles(output string) map[string]bool {
	generated := make(map[string]bool)
	fields := strings.Split(output, "\x00")
	for index := 0; index+checkAttrFieldCount-1 < len(fields); {
		path := fields[index]
		if path == "" {
			index++
			continue
		}
		attribute := fields[index+1]
		value := fields[index+2]
		generated[path] = attribute == "linguist-generated" && isGeneratedAttributeValue(value)
		index += checkAttrFieldCount
	}
	return generated
}

func isGeneratedAttributeValue(value string) bool {
	return value == checkAttrGeneratedSet || strings.EqualFold(value, "true")
}

func setLatestCommit(lastCommitAt map[string]time.Time, email string, committedAt time.Time) {
	if existing, exists := lastCommitAt[email]; !exists || committedAt.After(existing) {
		lastCommitAt[email] = committedAt
	}
}

func stripMailBrackets(mail string) string {
	trimmed := strings.TrimSpace(mail)
	trimmed = strings.TrimPrefix(trimmed, "<")
	return strings.TrimSuffix(trimmed, ">")
}
