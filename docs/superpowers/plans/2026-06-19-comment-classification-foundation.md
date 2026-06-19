# Comment Classification Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Classify every PR comment by author type, bot vendor, severity, and category using a YAML rules engine that runs in Go during sync, persists to SQLite, and surfaces in the Electron UI (sidebar counter + severity-ordered overview).

**Architecture:** A new `internal/classify` Go package loads YAML rule files from the user config dir (seeded from embedded built-ins on first launch, presence-gated update thereafter). The sync `Writer` classifies each comment before insert and stores the result plus a coarse rules fingerprint. The comments WebSocket handler re-classifies any comment whose fingerprint is stale before serving. The renderer reads the new fields, shows a per-PR counter in the sidebar, and reorders the overview comment list by severity with nits collapsed.

**Tech Stack:** Go 1.26 (`gopkg.in/yaml.v3`, stdlib `regexp`/`crypto/sha256`/`embed`), SQLite (`mattn/go-sqlite3`), React + TypeScript, TanStack Store, neverthrow.

**Spec:** `docs/superpowers/specs/2026-06-19-comment-classification-foundation-design.md`

---

## File Structure

**New Go files (`go/internal/classify/`):**
- `rules.go` — YAML schema (`RuleFile`, `Classifier`), parse + validate, compile body/path matchers to `*regexp.Regexp`.
- `engine.go` — `Engine` holding the loaded ruleset; `Classify(author, body, file) Result`.
- `loader.go` — read `*.yml` from a dir (alphabetical), parse each, skip+log bad files, build `Engine` + fingerprint. `Load(dir)`.
- `fingerprint.go` — `Fingerprint(dir) string`: sha256 over sorted `name:sha256(content)` pairs.
- `seed.go` — `SeedAndUpdate(dir)`: first-launch seed, presence-gated overwrite, embedded built-ins.
- `builtin/coderabbit.yml`, `builtin/greptile.yml`, `builtin/zz-fallback.yml` — starter rules (`//go:embed builtin/*.yml`).
- `*_test.go` for rules, engine, loader, fingerprint, seed.

**Modified Go files:**
- `go/internal/store/migrations/012_add_comment_classification.sql` — new columns + state table.
- `go/internal/sync/writer.go` — `Writer` gains an `*classify.Engine` + fingerprint; `writeComment` persists classification and state row.
- `go/internal/sync/orchestrator.go:378` — pass engine into `NewWriter`.
- `go/internal/server/handler/comments.go` — `commentRow` gains classification fields; handler runs `RefreshIfStale` before query; takes an engine + fingerprint provider.
- `go/internal/server/app.go` — build the engine, call `SeedAndUpdate`, inject into orchestrator + comments handler.
- `go/cmd/bistro-sync/main.go` — `--rules-dir` flag (default `os.UserConfigDir()/bistro/rules`).

**Modified TS files:**
- `electron/src/renderer/types/index.ts` — extend `Comment`.
- `electron/src/renderer/utils/severity.ts` (new) — rank + color map + counts helper.
- `electron/src/renderer/components/CommentClassificationCounter.tsx` (new) — sidebar counter.
- `electron/src/renderer/components/StatusSidebar.tsx` — render the counter.
- `electron/src/renderer/pages/PRDetailPage.tsx` — sort + collapse + gutter in `RecentComments`.
- `electron/src/renderer/components/CommentThread.tsx` — accept `severity`/`vendor`, render marker (prepared; inline threads still disabled).
- `electron/tests/unit/severity.test.ts`, `CommentClassificationCounter.test.tsx` (new).

**Conventions to follow:**
- Go: constructor `NewX(log *slog.Logger, ...)`, `if log == nil { log = slog.Default() }`; wrap errors with `fmt.Errorf("...: %w", err)`; per-item errors logged and skipped, never abort the tx (see `writer.go`).
- Migrations: `-- Migration: NNN` + `-- Description:` header lines (parser reads `Description:`), `IF NOT EXISTS` where applicable.
- TS: explicit types on exported fns/props, string-literal unions over enums, immutable updates, neverthrow for fallible paths, no `console.log` (use the existing `log` logger).

---

## Task 1: Migration 012 — classification columns + state table

**Files:**
- Create: `go/internal/store/migrations/012_add_comment_classification.sql`
- Test: `go/internal/store/migration_runner_test.go` (add a case) — or reuse existing migration test harness.

- [ ] **Step 1: Write the migration SQL**

```sql
-- Migration: 012_add_comment_classification
-- Description: Add classification columns to comments + rules-fingerprint state table

ALTER TABLE comments ADD COLUMN author_type TEXT;            -- 'human'|'bot'|'unknown'
ALTER TABLE comments ADD COLUMN bot_vendor TEXT;             -- 'coderabbit'|'greptile'|...
ALTER TABLE comments ADD COLUMN severity TEXT;               -- 'critical'|'warning'|'suggestion'|'nit'|'info'
ALTER TABLE comments ADD COLUMN category TEXT;               -- 'security'|'correctness'|'performance'|'style'|'readability'|'docs'
ALTER TABLE comments ADD COLUMN classification_source TEXT;  -- 'rule:<file>:<name>'|'fallback'|'unclassified'

CREATE TABLE IF NOT EXISTS comment_classification_state (
    comment_id  TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
);
```

- [ ] **Step 2: Write a failing test that the migration applies and columns exist**

Add to `go/internal/store/migration_runner_test.go` (match existing test style — open in-memory DB, run migrations, assert):

```go
func TestMigration012AddsClassificationColumns(t *testing.T) {
	db := newTestDB(t) // existing helper that runs all migrations
	cols := map[string]bool{}
	rows, err := db.Query(`SELECT name FROM pragma_table_info('comments')`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		cols[n] = true
	}
	for _, want := range []string{"author_type", "bot_vendor", "severity", "category", "classification_source"} {
		if !cols[want] {
			t.Errorf("comments missing column %q", want)
		}
	}
	if _, err := db.Exec(`INSERT INTO comment_classification_state (comment_id, fingerprint) VALUES ('x','y')`); err != nil {
		t.Errorf("state table not usable: %v", err)
	}
}
```

> If `newTestDB` doesn't exist, copy the DB-open + `NewMigrationRunner(...).Run(ctx)` sequence from an existing `store` test.

- [ ] **Step 3: Run the test**

Run: `cd go && go test ./internal/store/ -run TestMigration012 -v`
Expected: PASS (migration auto-discovered by the embedded runner).

- [ ] **Step 4: Commit**

```bash
git add go/internal/store/migrations/012_add_comment_classification.sql go/internal/store/migration_runner_test.go
git commit -m "feat(store): migration 012 — comment classification columns + state table"
```

---

## Task 2: Rule schema + parser (`rules.go`)

**Files:**
- Create: `go/internal/classify/rules.go`, `go/internal/classify/rules_test.go`

- [ ] **Step 1: Add yaml.v3 to direct requires**

Run: `cd go && go get gopkg.in/yaml.v3@v3.0.1`
Expected: `go.mod` now lists `gopkg.in/yaml.v3` in the direct require block.

- [ ] **Step 2: Write failing tests**

```go
package classify

import "testing"

func TestParseRuleFile_DirectSeverity(t *testing.T) {
	rf, err := ParseRuleFile("coderabbit.yml", []byte(`
name: CodeRabbit
vendor: coderabbit
match:
  author: ["coderabbitai[bot]"]
classifiers:
  - name: nitpick
    body_pattern: 'Nitpick'
    severity: nit
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rf.Vendor != "coderabbit" || len(rf.Classifiers) != 1 {
		t.Fatalf("unexpected parse: %+v", rf)
	}
	if !rf.MatchesAuthor("coderabbitai[bot]") || rf.MatchesAuthor("alice") {
		t.Fatal("author match wrong")
	}
}

func TestParseRuleFile_BadRegexFailsAtLoad(t *testing.T) {
	_, err := ParseRuleFile("bad.yml", []byte(`
name: Bad
classifiers:
  - name: x
    body_pattern: '([unclosed'
    severity: nit
`))
	if err == nil {
		t.Fatal("expected bad-regex parse error")
	}
}

func TestParseRuleFile_RejectsClassifierWithNoOutcome(t *testing.T) {
	_, err := ParseRuleFile("x.yml", []byte(`
name: X
classifiers:
  - name: noop
    body_pattern: 'foo'
`))
	if err == nil {
		t.Fatal("expected error: classifier needs severity or severity_map")
	}
}

func TestGlobToRegexp_DoubleStar(t *testing.T) {
	re, err := globToRegexp("**/auth/**")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("go/internal/auth/callback.go") {
		t.Error("** glob should match nested path")
	}
	if re.MatchString("go/internal/store/db.go") {
		t.Error("glob matched unrelated path")
	}
}
```

- [ ] **Step 3: Implement `rules.go`**

```go
package classify

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Severity / category are free-form strings validated against these sets at
// parse time. The engine is the single writer of these values to the DB.
var validSeverity = map[string]bool{
	"critical": true, "warning": true, "suggestion": true, "nit": true, "info": true,
}

// RuleFile is one parsed YAML rule file.
type RuleFile struct {
	FileName    string // e.g. "coderabbit.yml" — used in classification_source
	Name        string `yaml:"name"`
	Vendor      string `yaml:"vendor"` // bot_vendor value; empty for heuristics
	Match       Match  `yaml:"match"`
	Classifiers []*Classifier `yaml:"classifiers"`
}

// Match gates whether a file's classifiers apply to a comment. Empty match
// (no authors) applies to all comments — used by the fallback heuristics file.
type Match struct {
	Authors []string `yaml:"author"`
}

// Classifier is one rule within a file.
type Classifier struct {
	Name        string            `yaml:"name"`
	BodyPattern string            `yaml:"body_pattern"`
	Path        string            `yaml:"path"`
	Severity    string            `yaml:"severity"`
	SeverityMap map[string]string `yaml:"severity_map"`
	Category    string            `yaml:"category"`
	Weight      float64           `yaml:"weight"`

	bodyRe *regexp.Regexp
	pathRe *regexp.Regexp
}

// ParseRuleFile parses and validates one rule file. Bad regex or a classifier
// with no outcome is a hard error so the loader can skip the whole file.
func ParseRuleFile(fileName string, data []byte) (*RuleFile, error) {
	var rf RuleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("yaml parse %s: %w", fileName, err)
	}
	rf.FileName = fileName
	if rf.Name == "" {
		return nil, fmt.Errorf("%s: missing 'name'", fileName)
	}
	for _, c := range rf.Classifiers {
		if c.Name == "" {
			return nil, fmt.Errorf("%s: classifier missing 'name'", fileName)
		}
		hasOutcome := c.Severity != "" || len(c.SeverityMap) > 0
		if !hasOutcome {
			return nil, fmt.Errorf("%s/%s: classifier needs 'severity' or 'severity_map'", fileName, c.Name)
		}
		if c.Severity != "" && !validSeverity[c.Severity] {
			return nil, fmt.Errorf("%s/%s: invalid severity %q", fileName, c.Name, c.Severity)
		}
		for k, v := range c.SeverityMap {
			if !validSeverity[v] {
				return nil, fmt.Errorf("%s/%s: invalid severity %q for key %q", fileName, c.Name, v, k)
			}
		}
		if c.BodyPattern != "" {
			re, err := regexp.Compile(c.BodyPattern)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: bad body_pattern: %w", fileName, c.Name, err)
			}
			c.bodyRe = re
		}
		if c.Path != "" {
			re, err := globToRegexp(c.Path)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: bad path glob: %w", fileName, c.Name, err)
			}
			c.pathRe = re
		}
	}
	return &rf, nil
}

// MatchesAuthor reports whether this file's match block covers the author.
// A file with no authors matches everyone (fallback heuristics).
func (rf *RuleFile) MatchesAuthor(author string) bool {
	if len(rf.Match.Authors) == 0 {
		return true
	}
	for _, a := range rf.Match.Authors {
		if a == author {
			return true
		}
	}
	return false
}

// ownsAuthor reports whether this file explicitly names the author (used for
// vendor identification — distinct from MatchesAuthor's "empty = all").
func (rf *RuleFile) ownsAuthor(author string) bool {
	for _, a := range rf.Match.Authors {
		if a == author {
			return true
		}
	}
	return false
}

// globToRegexp converts a path glob to an anchored RE2 regexp.
// `**` matches across path separators, `*` matches within a segment.
// ponytail: minimal glob; swap for doublestar only if rule authors need full glob semantics.
func globToRegexp(glob string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	i := 0
	for i < len(glob) {
		switch {
		case strings.HasPrefix(glob[i:], "**"):
			b.WriteString(".*")
			i += 2
		case glob[i] == '*':
			b.WriteString("[^/]*")
			i++
		case glob[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
			i++
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
```

- [ ] **Step 4: Run tests**

Run: `cd go && go test ./internal/classify/ -run 'TestParseRuleFile|TestGlobToRegexp' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go/internal/classify/rules.go go/internal/classify/rules_test.go go/go.mod go/go.sum
git commit -m "feat(classify): YAML rule schema, parser, and glob matcher"
```

---

## Task 3: Fingerprint (`fingerprint.go`)

**Files:**
- Create: `go/internal/classify/fingerprint.go`, `go/internal/classify/fingerprint_test.go`

- [ ] **Step 1: Write failing tests**

```go
package classify

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFingerprint_ChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yml", "name: A")
	fp1, err := Fingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "a.yml", "name: A2")
	fp2, _ := Fingerprint(dir)
	if fp1 == fp2 {
		t.Fatal("fingerprint should change when content changes")
	}
}

func TestFingerprint_StableAcrossReorder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yml", "name: A")
	writeFile(t, dir, "b.yml", "name: B")
	fp1, _ := Fingerprint(dir)
	fp2, _ := Fingerprint(dir) // same dir, same files
	if fp1 != fp2 {
		t.Fatal("fingerprint must be deterministic")
	}
}
```

- [ ] **Step 2: Implement `fingerprint.go`**

```go
package classify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Fingerprint returns a coarse hash over all *.yml files in dir: sha256 of the
// sorted list of "name:sha256(content)" pairs. Any content change → new hash.
// A missing dir yields a stable empty-set fingerprint (not an error).
func Fingerprint(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return hashString(""), nil
		}
		return "", fmt.Errorf("read rules dir: %w", err)
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return "", fmt.Errorf("read %s: %w", e.Name(), err)
		}
		parts = append(parts, e.Name()+":"+hashString(string(content)))
	}
	sort.Strings(parts)
	return hashString(strings.Join(parts, "\n")), nil
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 3: Run tests**

Run: `cd go && go test ./internal/classify/ -run TestFingerprint -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add go/internal/classify/fingerprint.go go/internal/classify/fingerprint_test.go
git commit -m "feat(classify): coarse rules-directory fingerprint"
```

---

## Task 4: Engine (`engine.go`)

**Files:**
- Create: `go/internal/classify/engine.go`, `go/internal/classify/engine_test.go`

- [ ] **Step 1: Write failing tests (the classification matrix)**

```go
package classify

import "testing"

func engineFrom(t *testing.T, files ...*RuleFile) *Engine {
	t.Helper()
	return NewEngine(files, "fp-test")
}

func mustParse(t *testing.T, name, body string) *RuleFile {
	t.Helper()
	rf, err := ParseRuleFile(name, []byte(body))
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return rf
}

func TestClassify_AuthorAndEmojiSeverity(t *testing.T) {
	cr := mustParse(t, "coderabbit.yml", `
name: CodeRabbit
vendor: coderabbit
match: { author: ["coderabbitai[bot]"] }
classifiers:
  - name: emoji
    body_pattern: '^(?P<emoji>🔒|💡)'
    severity_map: { "🔒": critical, "💡": nit }
`)
	e := engineFrom(t, cr)

	r := e.Classify("coderabbitai[bot]", "🔒 secret leaked", "")
	if r.AuthorType != "bot" || r.BotVendor != "coderabbit" || r.Severity != "critical" {
		t.Fatalf("got %+v", r)
	}
	r2 := e.Classify("coderabbitai[bot]", "💡 tiny nit", "")
	if r2.Severity != "nit" {
		t.Fatalf("got %+v", r2)
	}
}

func TestClassify_UnknownBotByLogin(t *testing.T) {
	e := engineFrom(t)
	r := e.Classify("randomtool[bot]", "hi", "")
	if r.AuthorType != "bot" || r.BotVendor != "" {
		t.Fatalf("expected unknown bot, got %+v", r)
	}
}

func TestClassify_HumanUnclassified(t *testing.T) {
	e := engineFrom(t)
	r := e.Classify("alice", "looks good", "")
	if r.AuthorType != "human" || r.Severity != "" || r.ClassificationSource != "unclassified" {
		t.Fatalf("got %+v", r)
	}
}

func TestClassify_LastFileWins(t *testing.T) {
	base := mustParse(t, "coderabbit.yml", `
name: CodeRabbit
vendor: coderabbit
match: { author: ["coderabbitai[bot]"] }
classifiers:
  - name: nitpick
    body_pattern: 'Nitpick'
    severity: nit
`)
	override := mustParse(t, "coderabbit.company.yml", `
name: CodeRabbitCompany
vendor: coderabbit
match: { author: ["coderabbitai[bot]"] }
classifiers:
  - name: nitpick_is_suggestion
    body_pattern: 'Nitpick'
    severity: suggestion
`)
	e := engineFrom(t, base, override) // already sorted by loader; pass in order
	r := e.Classify("coderabbitai[bot]", "Nitpick: rename", "")
	if r.Severity != "suggestion" {
		t.Fatalf("override (last) should win, got %+v", r)
	}
}

func TestClassify_WeightedMaxWins_DirectBeatsWeighted(t *testing.T) {
	fb := mustParse(t, "zz-fallback.yml", `
name: Heuristics
classifiers:
  - name: nit_words
    body_pattern: '(consider|minor)'
    severity: nit
    weight: 0.4
  - name: security_words
    body_pattern: '(SQL injection|secret)'
    severity: critical
    weight: 0.9
`)
	e := engineFrom(t, fb)
	// both weighted match → higher weight (critical) wins
	r := e.Classify("alice", "consider the SQL injection risk", "")
	if r.Severity != "critical" {
		t.Fatalf("weighted max should win, got %+v", r)
	}
}
```

- [ ] **Step 2: Implement `engine.go`**

```go
package classify

import (
	"regexp"
	"strings"
)

var botLoginRe = regexp.MustCompile(`\[bot\]$`)

// severityRank orders severities for "max weight wins" ties and is reused by
// the renderer conceptually (kept here as the Go source of truth).
var severityRank = map[string]int{
	"critical": 5, "warning": 4, "suggestion": 3, "nit": 2, "info": 1,
}

// Result is the classification of one comment.
type Result struct {
	AuthorType           string // 'human'|'bot'|'unknown'
	BotVendor            string // '' if human or unknown bot
	Severity             string // '' if unclassified
	Category             string
	ClassificationSource string // 'rule:<file>:<name>' | 'unclassified'
}

// Engine holds an ordered, pre-compiled ruleset and the fingerprint that
// produced it.
type Engine struct {
	files       []*RuleFile
	fingerprint string
}

// NewEngine wraps already-parsed files (loader passes them in alphabetical
// order). fingerprint is stored so the writer can persist it alongside results.
func NewEngine(files []*RuleFile, fingerprint string) *Engine {
	return &Engine{files: files, fingerprint: fingerprint}
}

// Fingerprint returns the fingerprint of the loaded ruleset.
func (e *Engine) Fingerprint() string { return e.fingerprint }

// Classify runs all matching classifiers and resolves a single Result.
// Resolution: direct-severity classifiers — last match (in load order) wins;
// weighted classifiers tally per severity and max-weight wins; a direct match
// outranks any weighted result.
func (e *Engine) Classify(author, body, file string) Result {
	r := Result{AuthorType: "human", ClassificationSource: "unclassified"}

	// Author type + vendor.
	for _, rf := range e.files {
		if rf.ownsAuthor(author) {
			r.AuthorType = "bot"
			r.BotVendor = rf.Vendor
		}
	}
	if r.AuthorType != "bot" && botLoginRe.MatchString(author) {
		r.AuthorType = "bot" // unknown vendor
	}

	var (
		direct       *matchHit
		weightBySev  = map[string]float64{}
		weightSource = map[string]string{}
	)

	for _, rf := range e.files {
		if !rf.MatchesAuthor(author) {
			continue
		}
		for _, c := range rf.Classifiers {
			sev, ok := classifierResult(c, body, file)
			if !ok {
				continue
			}
			src := "rule:" + rf.FileName + ":" + c.Name
			if c.Weight > 0 {
				weightBySev[sev] += c.Weight
				weightSource[sev] = src
			} else {
				direct = &matchHit{sev: sev, cat: c.Category, src: src} // last wins
			}
		}
	}

	switch {
	case direct != nil:
		r.Severity = direct.sev
		r.Category = direct.cat
		r.ClassificationSource = direct.src
	case len(weightBySev) > 0:
		best := ""
		for sev := range weightBySev {
			if best == "" || weightBySev[sev] > weightBySev[best] ||
				(weightBySev[sev] == weightBySev[best] && severityRank[sev] > severityRank[best]) {
				best = sev
			}
		}
		r.Severity = best
		r.ClassificationSource = weightSource[best]
	}
	return r
}

type matchHit struct{ sev, cat, src string }

// classifierResult reports the severity a classifier assigns, if it matches.
func classifierResult(c *Classifier, body, file string) (string, bool) {
	if c.pathRe != nil && !c.pathRe.MatchString(file) {
		return "", false
	}
	if c.bodyRe != nil {
		m := c.bodyRe.FindStringSubmatch(body)
		if m == nil {
			return "", false
		}
		if len(c.SeverityMap) > 0 {
			// Map keyed on a named capture group's value, or first non-empty group.
			key := firstCapture(c.bodyRe, m)
			if sev, ok := c.SeverityMap[key]; ok {
				return sev, true
			}
			return "", false
		}
		return c.Severity, true
	}
	// No body pattern: matches on path alone (path already checked) or always.
	if len(c.SeverityMap) > 0 {
		return "", false // severity_map needs a capture; no body → no match
	}
	return c.Severity, true
}

func firstCapture(re *regexp.Regexp, m []string) string {
	for i, name := range re.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		if strings.TrimSpace(m[i]) != "" {
			return m[i]
		}
	}
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
```

- [ ] **Step 3: Run tests**

Run: `cd go && go test ./internal/classify/ -run TestClassify -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add go/internal/classify/engine.go go/internal/classify/engine_test.go
git commit -m "feat(classify): classification engine with weighted resolution"
```

---

## Task 5: Built-in rule files + loader (`loader.go`)

**Files:**
- Create: `go/internal/classify/builtin/coderabbit.yml`, `builtin/greptile.yml`, `builtin/zz-fallback.yml`
- Create: `go/internal/classify/loader.go`, `go/internal/classify/loader_test.go`

> **Scope note:** only these **three** built-in files ship in the 3.1–3.4 slice
> (`coderabbit.yml`, `greptile.yml`, `zz-fallback.yml`). The spec's full vendor
> list (copilot, qodo, cursor-bugbot, claude-code, codex, sourcery) is deferred —
> each is added later as its own rule file once we have a sample of its comment
> format. `//go:embed builtin/*.yml` picks up exactly what's in the directory, so
> leaving the others absent is safe and breaks nothing. The `zz-fallback.yml`
> generic `[bot]` detection (via the engine's `botLoginRe`, Task 4) already covers
> unknown vendors as `bot:unknown`, satisfying the acceptance bar.

- [ ] **Step 1: Write the built-in YAML files**

`builtin/coderabbit.yml`:
```yaml
name: CodeRabbit
vendor: coderabbit
match:
  author: ["coderabbitai[bot]", "coderabbit-ai[bot]"]
classifiers:
  - name: emoji_severity
    body_pattern: '(?P<emoji>🔒|⚠️|🛠️|💡)'
    severity_map:
      "🔒": critical
      "⚠️": warning
      "🛠️": suggestion
      "💡": nit
  - name: nitpick_section
    body_pattern: 'Nitpick'
    severity: nit
```

`builtin/greptile.yml`:
```yaml
name: Greptile
vendor: greptile
match:
  author: ["greptileai[bot]", "greptile-apps[bot]"]
classifiers:
  - name: severity_field
    body_pattern: '(?i)severity:\s*(?P<level>critical|high|medium|low)'
    severity_map:
      critical: critical
      high: warning
      medium: suggestion
      low: nit
```

`builtin/zz-fallback.yml` (heuristics, loads last):
```yaml
name: Heuristics
classifiers:
  - name: nit_words
    body_pattern: '(?i)(nit:|nitpick|consider|you could|minor|stylistic|preference|would be cleaner)'
    severity: nit
    weight: 0.4
  - name: security_words
    body_pattern: '(?i)(sql injection|xss|credential|secret|password|auth bypass|race condition|memory leak)'
    severity: critical
    category: security
    weight: 0.7
```

> Note: `category` on a weighted classifier is intentionally not propagated by the
> engine in this slice (only severity is weighted/tallied). It's harmless metadata
> for now; wiring category through weighted results is a later refinement.

- [ ] **Step 2: Write failing loader tests**

```go
package classify

import "testing"

func TestLoad_AlphabeticalOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "coderabbit.yml", `
name: CR
vendor: coderabbit
match: { author: ["cr[bot]"] }
classifiers: [{ name: a, body_pattern: 'X', severity: nit }]
`)
	writeFile(t, dir, "coderabbit.company.yml", `
name: CRC
vendor: coderabbit
match: { author: ["cr[bot]"] }
classifiers: [{ name: b, body_pattern: 'X', severity: warning }]
`)
	e, err := Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := e.Classify("cr[bot]", "X", "")
	if r.Severity != "warning" {
		t.Fatalf("company file (loads last) should win, got %+v", r)
	}
}

func TestLoad_SkipsBadFileKeepsRest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.yml", `
name: Good
classifiers: [{ name: a, body_pattern: 'ok', severity: nit }]
`)
	writeFile(t, dir, "bad.yml", `name: Bad
classifiers: [{ name: x, body_pattern: '([', severity: nit }]`)
	e, err := Load(dir, nil)
	if err != nil {
		t.Fatalf("loader must not fail on one bad file: %v", err)
	}
	if r := e.Classify("alice", "ok", ""); r.Severity != "nit" {
		t.Fatalf("good rule should still apply, got %+v", r)
	}
}
```

- [ ] **Step 3: Implement `loader.go`**

```go
package classify

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Load reads all *.yml files from dir in alphabetical order, parses each, skips
// (and logs) any that fail to parse, and returns a ready Engine carrying the
// directory fingerprint. A missing dir yields an empty engine (everything
// classifies as unclassified) rather than an error.
func Load(dir string, log *slog.Logger) (*Engine, error) {
	if log == nil {
		log = slog.Default()
	}
	fp, err := Fingerprint(dir)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return NewEngine(nil, fp), nil
		}
		return nil, fmt.Errorf("read rules dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	files := make([]*RuleFile, 0, len(names))
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			log.Warn("classify: skip unreadable rule file", "file", name, "err", err)
			continue
		}
		rf, err := ParseRuleFile(name, content)
		if err != nil {
			log.Warn("classify: skip invalid rule file", "file", name, "err", err)
			continue
		}
		files = append(files, rf)
	}
	return NewEngine(files, fp), nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd go && go test ./internal/classify/ -run TestLoad -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go/internal/classify/builtin/ go/internal/classify/loader.go go/internal/classify/loader_test.go
git commit -m "feat(classify): built-in rule files and directory loader"
```

---

## Task 6: Seed + presence-gated update (`seed.go`)

**Files:**
- Create: `go/internal/classify/seed.go`, `go/internal/classify/seed_test.go`

- [ ] **Step 1: Write failing tests**

```go
package classify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSeedAndUpdate_FirstLaunchSeedsAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rules")
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "coderabbit.yml")); err != nil {
		t.Fatal("expected coderabbit.yml seeded")
	}
	if _, err := os.Stat(filepath.Join(dir, "zz-fallback.yml")); err != nil {
		t.Fatal("expected zz-fallback.yml seeded")
	}
}

func TestSeedAndUpdate_PresentFileOverwritten(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "coderabbit.yml", "name: STALE")
	writeFile(t, dir, "keepme.yml", "name: dummy") // dir non-empty → update mode
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "coderabbit.yml"))
	if string(got) == "name: STALE" {
		t.Fatal("present built-in should be overwritten with default")
	}
}

func TestSeedAndUpdate_DeletedFileStaysGone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "greptile.yml", "name: G") // dir non-empty, coderabbit absent
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "coderabbit.yml")); !os.IsNotExist(err) {
		t.Fatal("deleted built-in must not be recreated in update mode")
	}
}

func TestSeedAndUpdate_CustomFileUntouched(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "coderabbit.company.yml", "name: MINE")
	if err := SeedAndUpdate(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "coderabbit.company.yml"))
	if string(got) != "name: MINE" {
		t.Fatal("custom file must never be touched")
	}
}
```

- [ ] **Step 2: Implement `seed.go`**

```go
package classify

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed builtin/*.yml
var builtinFS embed.FS

// SeedAndUpdate writes built-in rule files to dir on first launch (empty dir),
// and on every subsequent launch overwrites only the built-in files that still
// exist. Files the user deleted stay gone; user-authored files (any name not
// matching a built-in) are never touched.
func SeedAndUpdate(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create rules dir: %w", err)
	}

	dirIsEmpty, err := noYAML(dir)
	if err != nil {
		return err
	}

	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return fmt.Errorf("read embedded builtins: %w", err)
	}

	for _, e := range entries {
		name := e.Name()
		content, err := builtinFS.ReadFile("builtin/" + name)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", name, err)
		}
		target := filepath.Join(dir, name)

		if dirIsEmpty {
			if err := os.WriteFile(target, content, 0o644); err != nil {
				return fmt.Errorf("seed %s: %w", name, err)
			}
			continue
		}
		// Update mode: overwrite only if present; respect deletion.
		if _, err := os.Stat(target); err == nil {
			if err := os.WriteFile(target, content, 0o644); err != nil {
				return fmt.Errorf("update %s: %w", name, err)
			}
		}
	}
	return nil
}

func noYAML(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read rules dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") {
			return false, nil
		}
	}
	return true, nil
}
```

- [ ] **Step 3: Run tests**

Run: `cd go && go test ./internal/classify/ -run TestSeedAndUpdate -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add go/internal/classify/seed.go go/internal/classify/seed_test.go
git commit -m "feat(classify): first-launch seed + presence-gated update of built-ins"
```

---

## Task 7: Engine benchmark (performance guard)

**Files:**
- Modify: `go/internal/classify/engine_test.go`

- [ ] **Step 1: Add a benchmark over the built-in ruleset on a realistic PR**

```go
func BenchmarkClassify50Comments(b *testing.B) {
	dir := b.TempDir()
	if err := SeedAndUpdate(dir); err != nil {
		b.Fatal(err)
	}
	e, err := Load(dir, nil)
	if err != nil {
		b.Fatal(err)
	}
	bodies := []string{
		"🔒 secret leaked in config", "💡 consider renaming this",
		"Nitpick: trailing whitespace", "looks good to me",
		"severity: high — possible SQL injection",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 50; j++ {
			_ = e.Classify("coderabbitai[bot]", bodies[j%len(bodies)], "go/internal/auth/x.go")
		}
	}
}
```

- [ ] **Step 2: Run the benchmark to confirm it's well under budget**

Run: `cd go && go test ./internal/classify/ -bench BenchmarkClassify50 -run '^$' -benchtime 100x`
Expected: completes; per-op time for 50 comments far below 50ms (sanity check, not a hard assert).

- [ ] **Step 3: Commit**

```bash
git add go/internal/classify/engine_test.go
git commit -m "test(classify): benchmark 50-comment classification against built-ins"
```

---

## Task 8: Wire engine into the sync Writer

**Files:**
- Modify: `go/internal/sync/writer.go`
- Modify: `go/internal/sync/orchestrator.go:378`
- Test: `go/internal/sync/writer_test.go` (add a case; create if absent)

- [ ] **Step 1: Write a failing test that a classified comment persists its severity + state row**

```go
func TestWriteComment_PersistsClassification(t *testing.T) {
	db := newSyncTestDB(t) // helper that opens DB + runs migrations
	rulesDir := t.TempDir()
	if err := classify.SeedAndUpdate(rulesDir); err != nil {
		t.Fatal(err)
	}
	engine, err := classify.Load(rulesDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter(nil, db).WithClassifier(engine)

	results := []*PRFetchResult{{
		PR: &provider.PR{ID: "gh:o/r#1", Provider: "github", Owner: "o", Repo: "r", Number: 1,
			Title: "t", Author: "a", State: "open", BaseBranch: "main", HeadBranch: "f",
			BaseSHA: "b", HeadSHA: "h"},
		Comments: []*provider.Comment{{ID: "c1", PRID: "gh:o/r#1", Author: "coderabbitai[bot]",
			Body: "🔒 secret leaked", CreatedAt: time.Now(), UpdatedAt: time.Now()}},
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatal(err)
	}

	var sev, vendor, fp string
	row := db.QueryRow(`SELECT c.severity, c.bot_vendor, s.fingerprint
		FROM comments c JOIN comment_classification_state s ON s.comment_id = c.id
		WHERE c.id = 'c1'`)
	if err := row.Scan(&sev, &vendor, &fp); err != nil {
		t.Fatal(err)
	}
	if sev != "critical" || vendor != "coderabbit" || fp != engine.Fingerprint() {
		t.Fatalf("got sev=%q vendor=%q fp=%q", sev, vendor, fp)
	}
}
```

- [ ] **Step 2: Add the classifier field + `WithClassifier` and persist in `writeComment`**

In `writer.go`, add the import `"github.com/itterno/bistro/internal/classify"`, add a field and builder:

```go
type Writer struct {
	log        *slog.Logger
	db         *store.DB
	classifier *classify.Engine // nil → comments persist unclassified
}

// WithClassifier attaches a classification engine. Returns the writer for chaining.
func (w *Writer) WithClassifier(e *classify.Engine) *Writer {
	w.classifier = e
	return w
}
```

Replace the `writeComment` query + exec to include the new columns and the state row:

```go
func (w *Writer) writeComment(ctx context.Context, tx *sql.Tx, prID string, comment *provider.Comment) error {
	if comment.ID == "" {
		return fmt.Errorf("comment ID cannot be empty")
	}
	if prID == "" {
		return fmt.Errorf("PR ID cannot be empty for comment %s", comment.ID)
	}

	var authorType, botVendor, severity, category, source *string
	fingerprint := ""
	if w.classifier != nil {
		r := w.classifier.Classify(comment.Author, comment.Body, comment.FilePath)
		authorType = strPtr(r.AuthorType)
		botVendor = strPtrOrNil(r.BotVendor)
		severity = strPtrOrNil(r.Severity)
		category = strPtrOrNil(r.Category)
		source = strPtr(r.ClassificationSource)
		fingerprint = w.classifier.Fingerprint()
	}

	const query = `
		INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, line_number, file_path,
			author_type, bot_vendor, severity, category, classification_source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			body                  = excluded.body,
			updated_at            = excluded.updated_at,
			author_type           = excluded.author_type,
			bot_vendor            = excluded.bot_vendor,
			severity              = excluded.severity,
			category              = excluded.category,
			classification_source = excluded.classification_source
	`
	if _, err := tx.ExecContext(ctx, query,
		comment.ID, prID, comment.Author, comment.Body,
		comment.CreatedAt, comment.UpdatedAt, comment.LineNo, comment.FilePath,
		authorType, botVendor, severity, category, source,
	); err != nil {
		return fmt.Errorf("failed to write comment %s: %w", comment.ID, err)
	}

	if w.classifier != nil {
		const stateQ = `
			INSERT INTO comment_classification_state (comment_id, fingerprint)
			VALUES (?, ?)
			ON CONFLICT(comment_id) DO UPDATE SET fingerprint = excluded.fingerprint`
		if _, err := tx.ExecContext(ctx, stateQ, comment.ID, fingerprint); err != nil {
			return fmt.Errorf("failed to write classification state %s: %w", comment.ID, err)
		}
	}
	return nil
}

func strPtr(s string) *string { return &s }
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
```

In `orchestrator.go:378`, attach the engine (the orchestrator gains a `classifier *classify.Engine` field set via a `WithClassifier` builder — mirror the existing `WithDiffStore` pattern):

```go
writer := NewWriter(o.log, o.store)
if o.classifier != nil {
	writer = writer.WithClassifier(o.classifier)
}
```

Add to `Orchestrator`:
```go
// WithClassifier attaches a classification engine used when writing comments.
func (o *Orchestrator) WithClassifier(e *classify.Engine) *Orchestrator {
	o.classifier = e
	return o
}
```

- [ ] **Step 3: Run tests**

Run: `cd go && go test ./internal/sync/ -run TestWriteComment_PersistsClassification -v`
Expected: PASS. Then `go build ./...` to confirm orchestrator wiring compiles.

- [ ] **Step 4: Commit**

```bash
git add go/internal/sync/writer.go go/internal/sync/orchestrator.go go/internal/sync/writer_test.go
git commit -m "feat(sync): classify comments on write and persist fingerprint state"
```

---

## Task 9: RefreshIfStale + serve classification in comments handler

**Files:**
- Modify: `go/internal/server/handler/comments.go`
- Test: `go/internal/server/handler/comments_test.go`

- [ ] **Step 1: Write a failing test that stale comments are re-classified on request**

> **Test helpers:** `comments_test.go` already exists (the original handler had
> tests). Check it for an existing DB-open helper and an exec helper first and
> reuse them. If `newHandlerTestDB`/`mustExec` are absent, define them with the
> same pattern as Task 1's `newTestDB`: open an in-memory `*store.DB`
> (`store.NewDB(":memory:")` or the existing test constructor) and run
> `NewMigrationRunner(nil, db.DB).Run(ctx)`; `mustExec` is a one-line
> `t.Helper()` wrapper around `db.Exec` that `t.Fatal`s on error.

```go
func TestRequestComments_RefreshesStale(t *testing.T) {
	db := newHandlerTestDB(t) // existing helper pattern in comments_test.go
	// Insert a comment classified under an OLD fingerprint.
	mustExec(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('p1','github','o','r',1,'t','a','open','2026-01-01','2026-01-01','main','f','b','h')`)
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, severity)
		VALUES ('c1','p1','coderabbitai[bot]','🔒 secret','2026-01-01','2026-01-01','nit')`)
	mustExec(t, db, `INSERT INTO comment_classification_state (comment_id, fingerprint) VALUES ('c1','OLD')`)

	rulesDir := t.TempDir()
	if err := classify.SeedAndUpdate(rulesDir); err != nil {
		t.Fatal(err)
	}
	engine, _ := classify.Load(rulesDir, nil)
	h := NewRequestCommentsHandler(db, engine, testRepoLogger())

	resp, err := h(context.Background(), ws.InboundMessage{ID: "1", Type: "REQUEST_COMMENTS",
		Data: json.RawMessage(`{"pr_id":"p1"}`)})
	if err != nil {
		t.Fatal(err)
	}
	// The emoji rule should have re-classified c1 from 'nit' to 'critical'.
	var sev string
	db.QueryRow(`SELECT severity FROM comments WHERE id='c1'`).Scan(&sev)
	if sev != "critical" {
		t.Fatalf("stale comment not re-classified, severity=%q", sev)
	}
	_ = resp
}
```

- [ ] **Step 2: Extend `commentRow`, add `RefreshIfStale`, thread engine into the handler**

Add classification fields to `commentRow`:
```go
type commentRow struct {
	ID         string  `json:"id"`
	PRID       string  `json:"pr_id"`
	Author     string  `json:"author"`
	Body       string  `json:"body"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
	Line       *int    `json:"line,omitempty"`
	File       *string `json:"file,omitempty"`
	AuthorType *string `json:"author_type,omitempty"`
	BotVendor  *string `json:"bot_vendor,omitempty"`
	Severity   *string `json:"severity,omitempty"`
	Category   *string `json:"category,omitempty"`
	Source     *string `json:"classification_source,omitempty"`
}
```

Change `prLister` usage: the handler needs `Exec` too. Define a local interface in `comments.go`:
```go
// commentStore is the subset of *sql.DB the comments handler needs.
type commentStore interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
}
```

Update the signature and add the refresh pass (engine may be nil → skip refresh):
```go
func NewRequestCommentsHandler(db commentStore, engine *classify.Engine, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		// ... unmarshal req as before ...

		if engine != nil {
			if err := refreshIfStale(db, engine, req.PRID); err != nil {
				logger.Warn("classification refresh failed", "pr_id", req.PRID, "err", err)
				// non-fatal: serve whatever is cached
			}
		}

		rows, err := db.Query(
			`SELECT id, pr_id, author, body, created_at, updated_at, line_number, file_path,
				author_type, bot_vendor, severity, category, classification_source
			 FROM comments WHERE pr_id = ?
			 ORDER BY created_at ASC`, req.PRID)
		// ... scan into the extended commentRow (add the 5 new &c.* targets) ...
	}
}

// refreshIfStale re-classifies any comment whose stored fingerprint differs
// from the engine's current fingerprint. Coarse: one fingerprint per ruleset.
func refreshIfStale(db commentStore, engine *classify.Engine, prID string) error {
	current := engine.Fingerprint()
	rows, err := db.Query(`
		SELECT c.id, c.author, c.body, c.file_path
		FROM comments c
		LEFT JOIN comment_classification_state s ON s.comment_id = c.id
		WHERE c.pr_id = ? AND (s.fingerprint IS NULL OR s.fingerprint != ?)`,
		prID, current)
	if err != nil {
		return fmt.Errorf("select stale comments: %w", err)
	}
	type stale struct{ id, author, body string; file *string }
	var todo []stale
	for rows.Next() {
		var s stale
		if err := rows.Scan(&s.id, &s.author, &s.body, &s.file); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, s)
	}
	rows.Close()

	for _, s := range todo {
		file := ""
		if s.file != nil {
			file = *s.file
		}
		r := engine.Classify(s.author, s.body, file)
		if _, err := db.Exec(`UPDATE comments
			SET author_type=?, bot_vendor=?, severity=?, category=?, classification_source=?
			WHERE id=?`,
			nilEmpty(r.AuthorType), nilEmpty(r.BotVendor), nilEmpty(r.Severity),
			nilEmpty(r.Category), nilEmpty(r.ClassificationSource), s.id); err != nil {
			return fmt.Errorf("update comment %s: %w", s.id, err)
		}
		if _, err := db.Exec(`INSERT INTO comment_classification_state (comment_id, fingerprint)
			VALUES (?, ?) ON CONFLICT(comment_id) DO UPDATE SET fingerprint = excluded.fingerprint`,
			s.id, current); err != nil {
			return fmt.Errorf("update state %s: %w", s.id, err)
		}
	}
	return nil
}

func nilEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
```

Update the registration in `app.go:205` (done fully in Task 11):
```go
a.wsServer.RegisterHandler("REQUEST_COMMENTS", handler.NewRequestCommentsHandler(a.db.DB, a.classifier, a.log))
```

> `*store.DB` embeds `*sql.DB`, which already has `Query` and `Exec`, so it
> satisfies `commentStore` without changes.

- [ ] **Step 3: Run tests**

Run: `cd go && go test ./internal/server/handler/ -run TestRequestComments -v`
Expected: PASS (existing comments tests updated to pass `nil` engine where they don't exercise classification).

- [ ] **Step 4: Commit**

```bash
git add go/internal/server/handler/comments.go go/internal/server/handler/comments_test.go
git commit -m "feat(handler): serve classification fields and refresh stale comments on open"
```

---

## Task 10: App + main wiring (rules dir, seed, engine injection)

**Files:**
- Modify: `go/cmd/bistro-sync/main.go`
- Modify: `go/internal/server/app.go`

- [ ] **Step 1: Add `--rules-dir` flag in `main.go`**

Default to the user config dir; fall back to a relative path if unavailable (mirror the `paths.go` env-override style):

```go
defaultRulesDir := "./rules"
if cfg, err := os.UserConfigDir(); err == nil {
	defaultRulesDir = filepath.Join(cfg, "bistro", "rules")
}
rulesDir := flag.String("rules-dir", defaultRulesDir, "Directory holding classification rule YAML files")
```

Pass `*rulesDir` into the App constructor / options (follow how `dbPath` is threaded).

- [ ] **Step 2: In `app.go`, add the engine field, seed, and load at startup**

Add to the `App` struct:
```go
rulesDir   string
classifier *classify.Engine
```

During startup (where the DB is opened / before handlers register), add:
```go
if a.rulesDir != "" {
	if err := classify.SeedAndUpdate(a.rulesDir); err != nil {
		a.log.Warn("classify: seed/update failed; continuing with in-memory builtins", "err", err)
	}
	engine, err := classify.Load(a.rulesDir, a.log)
	if err != nil {
		a.log.Warn("classify: load failed; classification disabled this run", "err", err)
	} else {
		a.classifier = engine
		a.log.Info("classify: rules loaded", "dir", a.rulesDir, "fingerprint", engine.Fingerprint())
	}
}
```

Where the orchestrator is constructed (the `onAuth` block around line 188 and any other construction site), attach the classifier:
```go
a.orchestrator = sync.NewOrchestrator(a.log, prov, a.db, a.gitClient).
	WithDiffStore(diffStore).
	WithClassifier(a.classifier)
```

Update the comments handler registration:
```go
a.wsServer.RegisterHandler("REQUEST_COMMENTS", handler.NewRequestCommentsHandler(a.db.DB, a.classifier, a.log))
```

> `WithClassifier(nil)` must be safe (no-op) — both Orchestrator and Writer treat
> a nil engine as "skip classification," already handled in Task 8.

- [ ] **Step 3: Build + run the full Go test suite**

Run: `cd go && go build ./... && go test ./...`
Expected: build clean, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add go/cmd/bistro-sync/main.go go/internal/server/app.go
git commit -m "feat(sync): seed rules and inject classification engine at startup"
```

---

## Task 11: Renderer — extend Comment type + severity utilities

**Files:**
- Modify: `electron/src/renderer/types/index.ts`
- Create: `electron/src/renderer/utils/severity.ts`, `electron/tests/unit/severity.test.ts`

- [ ] **Step 1: Extend the `Comment` interface**

In `types/index.ts`, add to `Comment`:
```typescript
  author_type?: 'human' | 'bot' | 'unknown';
  bot_vendor?: string;
  severity?: 'critical' | 'warning' | 'suggestion' | 'nit' | 'info';
  category?: string;
  classification_source?: string;
```

- [ ] **Step 2: Write failing tests for the severity utilities**

`tests/unit/severity.test.ts`:
```typescript
import { describe, expect, it } from 'vitest';
import { countComments, severityRank, sortBySeverity } from '../../src/renderer/utils/severity';
import type { Comment } from '../../src/renderer/types';

const mk = (over: Partial<Comment>): Comment => ({
  id: Math.random().toString(), pr_id: 'p', author: 'a', body: '',
  created_at: '2026-01-01', updated_at: '2026-01-01', ...over,
});

describe('severity utils', () => {
  it('ranks critical above nit', () => {
    expect(severityRank('critical')).toBeGreaterThan(severityRank('nit'));
  });

  it('sorts critical first, unclassified last', () => {
    const sorted = sortBySeverity([
      mk({ id: '1', severity: 'nit', created_at: '2026-01-01' }),
      mk({ id: '2', severity: 'critical', created_at: '2026-01-02' }),
      mk({ id: '3', created_at: '2026-01-03' }),
    ]);
    expect(sorted.map((c) => c.id)).toEqual(['2', '1', '3']);
  });

  it('counts by severity and vendor', () => {
    const counts = countComments([
      mk({ severity: 'critical', author_type: 'bot', bot_vendor: 'coderabbit' }),
      mk({ severity: 'nit', author_type: 'bot', bot_vendor: 'coderabbit' }),
      mk({ author_type: 'human' }),
    ]);
    expect(counts.bySeverity.critical).toBe(1);
    expect(counts.bySeverity.nit).toBe(1);
    expect(counts.human).toBe(1);
    expect(counts.byVendor.coderabbit).toBe(2);
  });
});
```

- [ ] **Step 3: Implement `utils/severity.ts`**

```typescript
import type { Comment } from '../types';

export type Severity = 'critical' | 'warning' | 'suggestion' | 'nit' | 'info';

const RANK: Record<Severity, number> = {
  critical: 5, warning: 4, suggestion: 3, nit: 2, info: 1,
};

/** Severity color classes, shared by the sidebar counter and the overview gutter. */
export const SEVERITY_COLOR: Record<Severity, string> = {
  critical: 'bg-error',
  warning: 'bg-amber-500',
  suggestion: 'bg-primary',
  nit: 'bg-outline',
  info: 'bg-outline',
};

/** Severities that collapse by default in the overview list. */
export const COLLAPSED_BY_DEFAULT: ReadonlySet<Severity> = new Set(['nit', 'info']);

export function severityRank(severity?: string): number {
  if (severity && severity in RANK) return RANK[severity as Severity];
  return 0; // unclassified sinks to the bottom
}

/** Immutable sort: severity desc, then created_at asc. */
export function sortBySeverity(comments: ReadonlyArray<Comment>): Comment[] {
  return [...comments].sort((a, b) => {
    const d = severityRank(b.severity) - severityRank(a.severity);
    if (d !== 0) return d;
    return a.created_at.localeCompare(b.created_at);
  });
}

export interface CommentCounts {
  bySeverity: Partial<Record<Severity, number>>;
  byVendor: Record<string, number>;
  human: number;
  bot: number;
}

export function countComments(comments: ReadonlyArray<Comment>): CommentCounts {
  const counts: CommentCounts = { bySeverity: {}, byVendor: {}, human: 0, bot: 0 };
  for (const c of comments) {
    if (c.severity) {
      const s = c.severity as Severity;
      counts.bySeverity[s] = (counts.bySeverity[s] ?? 0) + 1;
    }
    if (c.author_type === 'bot') {
      counts.bot += 1;
      const vendor = c.bot_vendor ?? 'other';
      counts.byVendor[vendor] = (counts.byVendor[vendor] ?? 0) + 1;
    } else if (c.author_type === 'human') {
      counts.human += 1;
    }
  }
  return counts;
}
```

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm test severity`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/types/index.ts electron/src/renderer/utils/severity.ts electron/tests/unit/severity.test.ts
git commit -m "feat(ui): comment severity types, sort, and counting utilities"
```

---

## Task 12: Renderer — sidebar classification counter

**Files:**
- Create: `electron/src/renderer/components/CommentClassificationCounter.tsx`, `electron/tests/unit/CommentClassificationCounter.test.tsx`
- Modify: `electron/src/renderer/components/StatusSidebar.tsx`

- [ ] **Step 1: Write a failing component test**

```tsx
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import CommentClassificationCounter from '../../src/renderer/components/CommentClassificationCounter';
import type { Comment } from '../../src/renderer/types';

const mk = (o: Partial<Comment>): Comment => ({
  id: Math.random().toString(), pr_id: 'p', author: 'a', body: '',
  created_at: '2026-01-01', updated_at: '2026-01-01', ...o,
});

describe('CommentClassificationCounter', () => {
  it('shows severity and human counts', () => {
    render(<CommentClassificationCounter comments={[
      mk({ severity: 'critical', author_type: 'bot', bot_vendor: 'coderabbit' }),
      mk({ author_type: 'human' }),
    ]} />);
    expect(screen.getByText(/Critical/i)).toBeInTheDocument();
    expect(screen.getByText('1', { selector: '*' })).toBeTruthy();
  });

  it('renders nothing when there are no classified comments', () => {
    const { container } = render(<CommentClassificationCounter comments={[mk({})]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
```

- [ ] **Step 2: Implement the component**

```tsx
import { useMemo } from 'react';
import type { Comment } from '../types';
import { countComments, type Severity } from '../utils/severity';

interface CommentClassificationCounterProps {
  comments: ReadonlyArray<Comment>;
}

const SEVERITY_LABEL: Record<Severity, string> = {
  critical: 'Critical', warning: 'Warning', suggestion: 'Suggestion', nit: 'Nit', info: 'Info',
};
const SEVERITY_TEXT: Record<Severity, string> = {
  critical: 'text-error',
  warning: 'text-amber-600 dark:text-amber-400',
  suggestion: 'text-primary',
  nit: 'text-on-surface-variant',
  info: 'text-on-surface-variant',
};
const ORDER: Severity[] = ['critical', 'warning', 'suggestion', 'nit', 'info'];

export default function CommentClassificationCounter({
  comments,
}: CommentClassificationCounterProps) {
  const counts = useMemo(() => countComments(comments), [comments]);

  const hasSeverity = ORDER.some((s) => counts.bySeverity[s]);
  if (!hasSeverity && counts.human === 0 && counts.bot === 0) return null;

  return (
    <section>
      <h3 className="text-xs font-bold text-on-surface-variant uppercase mb-2">Comments</h3>
      <ul className="space-y-1 text-xs">
        {counts.human > 0 && (
          <li className="flex justify-between">
            <span className="text-on-surface">Human</span>
            <span className="text-on-surface-variant">{counts.human}</span>
          </li>
        )}
        {ORDER.filter((s) => counts.bySeverity[s]).map((s) => (
          <li key={s} className="flex justify-between">
            <span className={SEVERITY_TEXT[s]}>{SEVERITY_LABEL[s]}</span>
            <span className="text-on-surface-variant">{counts.bySeverity[s]}</span>
          </li>
        ))}
      </ul>
      {Object.keys(counts.byVendor).length > 0 && (
        <p className="mt-2 text-[10px] text-on-surface-variant">
          {Object.entries(counts.byVendor)
            .map(([v, n]) => `${v} ${n}`)
            .join(' · ')}
        </p>
      )}
    </section>
  );
}
```

- [ ] **Step 3: Render it in `StatusSidebar.tsx`**

Add a `comments` prop and place the counter after the CI section:
```tsx
import type { CICheck, Comment, PR } from '../types';
import CommentClassificationCounter from './CommentClassificationCounter';

interface StatusSidebarProps {
  pr: PR;
  ciChecks: CICheck[];
  comments: ReadonlyArray<Comment>;
}

export default function StatusSidebar({ pr, ciChecks, comments }: StatusSidebarProps) {
  // ... existing sections ...
  // after the CI Checks </section>:
  //   <CommentClassificationCounter comments={comments} />
}
```

Then update the `StatusSidebar` call site — it's in `PRDetailPage.tsx` (the one
component that renders both the sidebar and the comment list) — to pass `comments`.

- [ ] **Step 4: Run tests + typecheck**

Run: `cd electron && pnpm test CommentClassificationCounter && pnpm build`
Expected: tests PASS, build clean.

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/components/CommentClassificationCounter.tsx electron/src/renderer/components/StatusSidebar.tsx electron/tests/unit/CommentClassificationCounter.test.tsx
git commit -m "feat(ui): per-PR comment classification counter in sidebar"
```

---

## Task 13: Renderer — severity-ordered overview with collapsed nits

**Files:**
- Modify: `electron/src/renderer/pages/PRDetailPage.tsx` (the `RecentComments` component)
- Test: `electron/tests/unit/PRDetailPage.test.tsx` (add a case)

- [ ] **Step 1: Write a failing test that critical sorts first and nits collapse**

```tsx
it('orders critical comments before nits and collapses nits', () => {
  render(<RecentComments comments={[
    mk({ id: 'nit', severity: 'nit', body: 'nit body' }),
    mk({ id: 'crit', severity: 'critical', body: 'critical body' }),
  ]} />);
  const bodies = screen.getAllByTestId('comment-card');
  expect(bodies[0]).toHaveAttribute('data-severity', 'critical');
  // nit is collapsed → its body text is not rendered until expanded
  expect(screen.queryByText('nit body')).not.toBeInTheDocument();
});
```

> If `RecentComments` is not exported, export it for testing (or move it to its
> own file `components/RecentComments.tsx` — preferred per the small-files rule).
> Bringing it into its own component file is acceptable here.

- [ ] **Step 2: Update `RecentComments` to sort, collapse, and add a gutter**

Replace the body of `RecentComments` with severity-aware rendering. Key changes:
- `const ordered = useMemo(() => sortBySeverity(comments), [comments]);`
- Render all `ordered` (not `.slice(0, 5)` — or keep a cap but after sorting).
- Each card: `data-severity={c.severity ?? 'none'}`, `data-testid="comment-card"`, a left gutter `<span>` colored via `SEVERITY_COLOR`.
- For `COLLAPSED_BY_DEFAULT` severities, render header only with a local `useState` expand toggle; clicking the header reveals the body.

```tsx
import { COLLAPSED_BY_DEFAULT, SEVERITY_COLOR, sortBySeverity, type Severity } from '../utils/severity';

function CommentCard({ c }: { c: Comment }) {
  const collapsible = !!c.severity && COLLAPSED_BY_DEFAULT.has(c.severity as Severity);
  const [expanded, setExpanded] = useState(!collapsible);
  const gutter = c.severity ? SEVERITY_COLOR[c.severity as Severity] : 'bg-transparent';

  return (
    <div
      data-testid="comment-card"
      data-severity={c.severity ?? 'none'}
      className="relative p-6 pl-7 bg-surface-container-lowest hover:bg-surface-container-low transition-colors flex flex-col space-y-3"
    >
      <span className={`absolute left-0 top-0 bottom-0 w-[2px] ${gutter}`} aria-hidden />
      <button
        type="button"
        onClick={() => collapsible && setExpanded((v) => !v)}
        className="flex justify-between items-center text-left"
      >
        <span className="font-mono text-[10px] font-bold text-on-surface uppercase">
          @{c.author}{c.bot_vendor ? ` · ${c.bot_vendor}` : ''}
        </span>
        <span className="font-mono text-[10px] text-outline">{relativeTime(c.created_at)}</span>
      </button>
      {expanded && (
        <p className="text-sm text-on-surface-variant font-medium leading-relaxed whitespace-pre-wrap">
          {c.body}
        </p>
      )}
    </div>
  );
}
```

And in `RecentComments`:
```tsx
const ordered = useMemo(() => sortBySeverity(comments), [comments]);
// ... map ordered → <CommentCard key={c.id} c={c} />
```

- [ ] **Step 3: Run tests + build**

Run: `cd electron && pnpm test PRDetailPage && pnpm build`
Expected: PASS, build clean.

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/pages/PRDetailPage.tsx electron/tests/unit/PRDetailPage.test.tsx
git commit -m "feat(ui): severity-ordered overview comments with collapsed nits and gutter"
```

---

## Task 14: Renderer — severity marker on diff-view comment thread (prepared)

**Files:**
- Modify: `electron/src/renderer/components/CommentThread.tsx`
- Test: `electron/tests/unit/` (add a small test if a CommentThread test exists; otherwise a minimal new one)

> Context: `PRDetailPage` notes inline diff comments are "intentionally disabled
> this pass." This task adds the severity marker prop so the marker exists the
> moment inline threads are re-enabled — it does not re-enable them.

- [ ] **Step 1: Add optional `severity`/`vendor` props and render a dot marker**

```tsx
import { SEVERITY_COLOR, type Severity } from '../utils/severity';

interface CommentThreadProps {
  // ... existing props ...
  severity?: Severity;
  vendor?: string;
}

// In the thread header, before the author:
{severity && (
  <span
    data-testid="severity-marker"
    className={`inline-block w-2 h-2 rounded-full ${SEVERITY_COLOR[severity]}`}
    title={vendor ? `${severity} · ${vendor}` : severity}
    aria-label={`severity ${severity}`}
  />
)}
```

- [ ] **Step 2: Write a minimal failing test**

```tsx
it('renders a severity marker when severity is set', () => {
  render(<CommentThread /* ...required props... */ severity="critical" />);
  expect(screen.getByTestId('severity-marker')).toHaveClass('bg-error');
});
```

> Fill in `CommentThread`'s existing required props from its current signature.

- [ ] **Step 3: Run tests + build**

Run: `cd electron && pnpm test CommentThread && pnpm build`
Expected: PASS, build clean.

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/components/CommentThread.tsx electron/tests/unit/
git commit -m "feat(ui): severity marker on diff-view comment threads"
```

---

## Final verification

- [ ] **Step 1: Full Go suite + race detector**

Run: `cd go && go build ./... && go test ./... -race`
Expected: all pass, no races.

- [ ] **Step 2: Full Electron suite + build**

Run: `cd electron && pnpm test && pnpm build`
Expected: all pass, build clean.

- [ ] **Step 3: Manual smoke (optional, requires a real token)**

Use the @verify or @run skill: start `bistro-sync`, open a PR with CodeRabbit comments, confirm the sidebar counter shows severity counts and the overview leads with critical/warning comments. Edit `~/.config/bistro/rules/coderabbit.yml` (change a severity), reopen the PR, confirm the classification updates without restarting.

- [ ] **Step 4: Update the spec/roadmap status note (optional)**

Mark roadmap 3.1–3.4 as `done`/`partial` as appropriate in `docs/ROADMAP.md`.
