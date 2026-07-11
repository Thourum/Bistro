# Phase 3 — Comment Classification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Classify every PR comment (author type, bot vendor, severity, category) with an inspectable local rules engine, then make it actionable: bulk dismiss, multi-bot dedup, optional BYOK LLM refinement.

**Architecture:** Deterministic tier (bot ID → YAML rules → weighted heuristics) runs synchronously inside the existing sync write transaction in `go/internal/sync/writer.go`; a lazy fingerprint check re-classifies on PR open. LLM tier (increment 4) runs async post-commit, one goroutine per PR with bounded fan-out. Go owns all SQLite writes; the renderer talks WS commands.

**Tech Stack:** Go 1.26 (`gopkg.in/yaml.v3`, `github.com/bmatcuk/doublestar/v4` — both new deps), SQLite migrations 026–028, React 19 + TanStack + Vitest, neverthrow (no try/catch in committed TS).

**Spec:** `docs/superpowers/specs/2026-07-12-comment-classification-design.md` — read it first. Its DDL, YAML schema, resolution order, dedup algorithm, and wire contracts are normative; this plan operationalizes them.

**Ground rules for every task:**
- TDD: failing test → minimal code → pass → commit. Table-driven Go tests, `-race`.
- Verify Go: `cd go && go vet ./... && go test -race ./...`
- Verify Electron: `cd electron && pnpm type-check && pnpm lint && pnpm test`
- Commits: conventional format, end body with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- Module path is `github.com/itterno/bistro` (from `go/go.mod`). All Go paths below are relative to `go/`.
- Never write `try/catch` in committed TS — use neverthrow `Result`/`ResultAsync` (see `electron/src/renderer/hooks/useSQLiteQuery.ts` for the house style).

---

## File structure (what exists at the end)

```
go/internal/classify/
  rules.go          YAML schema + parser; regexes compiled at load
  rules_test.go
  loader.go         LoadDir (alphabetical) + Fingerprint
  loader_test.go
  seed.go           go:embed built-ins → SeedAndUpdate (presence-gated)
  seed_test.go
  engine.go         Classify(author, body, path) → Classification
  engine_test.go
  dedup.go          per-PR dedup post-pass (increment 3)
  dedup_test.go
  corpus_test.go    ≥70% heuristic accuracy over testdata corpus
  builtin/coderabbit.yml, greptile.yml, zz-fallback.yml
  testdata/corpus.yml
go/internal/classify/llm/          (increment 4)
  client.go         OpenAI-compatible + Anthropic HTTP clients
  client_test.go
  tier.go           post-commit per-PR fan-out + cache + usage
  tier_test.go
go/internal/store/migrations/
  026_add_comment_classification.sql
  027_add_comment_dismissals.sql
  028_add_dedup_and_llm_cache.sql
go/internal/store/dismissals.go + dismissals_test.go
go/internal/server/handler/dismiss.go + dismiss_test.go
go/internal/server/handler/classification_settings.go + _test.go
electron/src/renderer/utils/classification.ts + classification.test.ts   (pure sort/filter/aggregate)
electron/src/renderer/hooks/useDismissals.ts                             (undo stack + WS commands)
```

Modified: `go/internal/sync/writer.go`, `go/internal/server/handler/comments.go`, `go/internal/server/app.go`, `go/cmd/bistro-sync/main.go`, `electron/src/types/settings.ts`, `electron/src/renderer/types/index.ts`, `electron/src/renderer/hooks/useWebSocket.ts`, `electron/src/renderer/utils/keybindings.ts`, `electron/src/renderer/pages/PRDetailPage.tsx`, `electron/src/renderer/pages/SettingsPage.tsx`, `electron/src/renderer/components/StatusSidebar.tsx`, `electron/src/renderer/components/CommentThread.tsx`.

---

# Increment 1 — Foundation (3.1–3.4)

### Task 1: Migration 026 — classification columns + state table

**Files:**
- Create: `go/internal/store/migrations/026_add_comment_classification.sql`
- Test: `go/internal/store/migration_classification_test.go`

- [ ] **Step 1: Write the failing test**

Look at an existing migration test in `go/internal/store/` for the open-fresh-DB-and-migrate helper pattern, then:

```go
package store

import "testing"

func TestMigration026CommentClassification(t *testing.T) {
	db := openTestDB(t) // reuse the existing test helper that runs all migrations

	// New columns exist on comments.
	for _, col := range []string{"author_type", "bot_vendor", "severity", "category", "classification_source"} {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('comments') WHERE name = ?`, col,
		).Scan(&count)
		if err != nil || count != 1 {
			t.Errorf("comments.%s missing (err=%v count=%d)", col, err, count)
		}
	}

	// State side-table exists with the right shape.
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('comment_classification_state') WHERE name IN ('comment_id','fingerprint')`,
	).Scan(&n); err != nil || n != 2 {
		t.Errorf("comment_classification_state shape wrong (err=%v n=%d)", err, n)
	}
}
```

If there is no `openTestDB`-style helper, copy the setup from the newest migration test file (025-era) instead of inventing one.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd go && go test -race ./internal/store/ -run TestMigration026`
Expected: FAIL — columns missing.

- [ ] **Step 3: Write the migration** — exactly the spec DDL:

```sql
-- 026_add_comment_classification.sql
-- Phase 3.1: classification columns + fingerprint side-table.
-- No CHECK constraints: the Go engine is the single writer and validates values.
ALTER TABLE comments ADD COLUMN author_type TEXT;
ALTER TABLE comments ADD COLUMN bot_vendor TEXT;
ALTER TABLE comments ADD COLUMN severity TEXT;
ALTER TABLE comments ADD COLUMN category TEXT;
ALTER TABLE comments ADD COLUMN classification_source TEXT;

CREATE TABLE comment_classification_state (
  comment_id  TEXT PRIMARY KEY,
  fingerprint TEXT NOT NULL,
  FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
);
```

- [ ] **Step 4: Run tests** — `cd go && go test -race ./internal/store/` → PASS (all, not just the new one — migrations are embedded via `//go:embed migrations/*.sql` in `migration_runner.go`, no registration needed).

- [ ] **Step 5: Commit** — `feat(store): migration 026 — comment classification columns + state table`

### Task 2: `classify` package — YAML schema + parser (`rules.go`)

**Files:**
- Create: `go/internal/classify/rules.go`, `go/internal/classify/rules_test.go`
- Modify: `go/go.mod` (add `gopkg.in/yaml.v3`, `github.com/bmatcuk/doublestar/v4`)

- [ ] **Step 1: Add deps** — `cd go && go get gopkg.in/yaml.v3 github.com/bmatcuk/doublestar/v4 && go mod tidy`. (Check `go.mod` first; add only what's missing.)

- [ ] **Step 2: Write the failing tests**

```go
package classify

import "testing"

func TestParseRuleFile(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, rf *RuleFile)
	}{
		{
			name: "valid vendor file with severity_map and direct classifiers",
			yaml: `
name: CodeRabbit
vendor: coderabbit
match:
  author: ["coderabbitai[bot]"]
classifiers:
  - name: emoji_severity
    body_pattern: '^(?P<emoji>A|B)'
    severity_map: { "A": critical, "B": nit }
  - name: nitpick_section
    body_pattern: 'Nitpick'
    severity: nit
`,
			check: func(t *testing.T, rf *RuleFile) {
				if rf.Vendor != "coderabbit" || len(rf.Classifiers) != 2 {
					t.Fatalf("parse shape: %+v", rf)
				}
				if rf.Classifiers[0].SeverityMap["A"] != "critical" {
					t.Errorf("severity_map not parsed")
				}
			},
		},
		{
			name: "weighted heuristic with path glob",
			yaml: `
name: Heuristics
classifiers:
  - name: docs_style
    path: "{**/*.md,**/*.rst}"
    severity: suggestion
    category: docs
    weight: 0.3
`,
			check: func(t *testing.T, rf *RuleFile) {
				if rf.Classifiers[0].Weight != 0.3 {
					t.Errorf("weight not parsed")
				}
			},
		},
		{name: "malformed yaml", yaml: `name: [broken`, wantErr: true},
		{name: "bad regex", yaml: "name: X\nclassifiers:\n  - name: bad\n    body_pattern: '('\n    severity: nit\n", wantErr: true},
		{name: "invalid severity value", yaml: "name: X\nclassifiers:\n  - name: bad\n    body_pattern: 'x'\n    severity: catastrophic\n", wantErr: true},
		{name: "classifier with no outcome", yaml: "name: X\nclassifiers:\n  - name: bad\n    body_pattern: 'x'\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rf, err := Parse([]byte(tt.yaml), tt.name+".yml")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.check != nil && err == nil {
				tt.check(t, rf)
			}
		})
	}
}
```

- [ ] **Step 3: Run** — `go test -race ./internal/classify/ -run TestParseRuleFile` → FAIL (package doesn't exist).

- [ ] **Step 4: Implement `rules.go`**

```go
// Package classify implements Bistro's local comment-classification engine:
// bot identification, YAML rule files, and weighted heuristics. Deterministic,
// offline, inspectable — classification_source always names what fired.
package classify

import (
	"fmt"
	"regexp"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"
)

// Severity values, ranked. ValidSeverities is the write contract for the
// comments.severity column (migration 026).
var ValidSeverities = map[string]int{
	"critical": 5, "warning": 4, "suggestion": 3, "nit": 2, "info": 1,
}

var ValidCategories = map[string]bool{
	"security": true, "correctness": true, "performance": true,
	"style": true, "readability": true, "docs": true,
}

// Classifier is one rule inside a RuleFile. Conditions (Author, Path,
// BodyPattern) are AND-combined; the outcome is either a direct Severity
// (+ optional Category) or a SeverityMap keyed on a named capture group.
// A non-zero Weight marks it as a heuristic (tallied, not last-match-wins).
type Classifier struct {
	Name        string            `yaml:"name"`
	Author      string            `yaml:"author"`
	Path        string            `yaml:"path"`
	BodyPattern string            `yaml:"body_pattern"`
	Severity    string            `yaml:"severity"`
	Category    string            `yaml:"category"`
	SeverityMap map[string]string `yaml:"severity_map"`
	Weight      float64           `yaml:"weight"`

	bodyRe *regexp.Regexp // compiled at parse; nil when BodyPattern empty
}

type Match struct {
	Author []string `yaml:"author"`
}

// RuleFile is one parsed YAML rule file. FileName is the base name it was
// loaded from — recorded in classification_source as rule:<file>:<classifier>.
type RuleFile struct {
	Name        string       `yaml:"name"`
	Vendor      string       `yaml:"vendor"`
	Match       Match        `yaml:"match"`
	Classifiers []Classifier `yaml:"classifiers"`
	FileName    string       `yaml:"-"`
}

// Parse decodes and validates one rule file. Regexes are compiled here so a
// bad pattern fails the whole file at load time, never during sync.
func Parse(data []byte, fileName string) (*RuleFile, error) {
	var rf RuleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("%s: %w", fileName, err)
	}
	rf.FileName = fileName
	for i := range rf.Classifiers {
		c := &rf.Classifiers[i]
		if c.Name == "" {
			return nil, fmt.Errorf("%s: classifier %d has no name", fileName, i)
		}
		if c.BodyPattern != "" {
			re, err := regexp.Compile(c.BodyPattern)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: bad body_pattern: %w", fileName, c.Name, err)
			}
			c.bodyRe = re
		}
		if c.Path != "" {
			if !doublestar.ValidatePattern(c.Path) {
				return nil, fmt.Errorf("%s/%s: bad path glob %q", fileName, c.Name, c.Path)
			}
		}
		if c.Severity == "" && len(c.SeverityMap) == 0 {
			return nil, fmt.Errorf("%s/%s: no severity or severity_map", fileName, c.Name)
		}
		if c.Severity != "" {
			if _, ok := ValidSeverities[c.Severity]; !ok {
				return nil, fmt.Errorf("%s/%s: invalid severity %q", fileName, c.Name, c.Severity)
			}
		}
		for k, v := range c.SeverityMap {
			if _, ok := ValidSeverities[v]; !ok {
				return nil, fmt.Errorf("%s/%s: severity_map[%s]: invalid severity %q", fileName, c.Name, k, v)
			}
		}
		if c.Category != "" && !ValidCategories[c.Category] {
			return nil, fmt.Errorf("%s/%s: invalid category %q", fileName, c.Name, c.Category)
		}
	}
	return &rf, nil
}
```

- [ ] **Step 5: Run** → PASS. `go vet ./...` clean.
- [ ] **Step 6: Commit** — `feat(classify): YAML rule schema + validating parser`

### Task 3: `loader.go` — alphabetical dir load + fingerprint

**Files:**
- Create: `go/internal/classify/loader.go`, `go/internal/classify/loader_test.go`

- [ ] **Step 1: Failing tests** — table-driven over a `t.TempDir()` populated with YAML files:
  - `TestLoadDirAlphabetical`: write `b.yml`, `a.yml`; assert `Loaded.Files` order is `[a.yml, b.yml]`.
  - `TestLoadDirSkipsBadFile`: one valid + one malformed file → valid file loads, error is collected (loader returns `(*Loaded, []error)`), classification never blocked.
  - `TestLoadDirIgnoresNonYAML`: `README.md` in dir is ignored (only `.yml`/`.yaml`).
  - `TestFingerprintChangesOnEdit`: fingerprint over (dir contents + mode string); editing a file changes it; same input is stable across calls.
  - `TestFingerprintIncludesMode`: same dir, mode `"rules"` vs `"rules+llm"` → different fingerprints.

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement**

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

// Loaded is an immutable snapshot of a rule directory.
type Loaded struct {
	Files       []*RuleFile // alphabetical by file name — later overrides earlier
	Fingerprint string      // sha256 over file names+contents+mode; see spec "Data flow — PR open"
}

// LoadDir reads every *.yml / *.yaml in dir alphabetically. A file that fails
// to parse is skipped and reported in errs; the rest still load — one bad rule
// never kills classification. mode participates in the fingerprint so a
// classification-mode change invalidates lazily like a rule edit.
func LoadDir(dir, mode string) (*Loaded, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return &Loaded{Fingerprint: fingerprint(nil, nil, mode)}, []error{fmt.Errorf("read rules dir: %w", err)}
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".yml" || ext == ".yaml" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var errs []error
	loaded := &Loaded{}
	var contents [][]byte
	var loadedNames []string
	for _, name := range names {
		data, rerr := os.ReadFile(filepath.Join(dir, name))
		if rerr != nil {
			errs = append(errs, rerr)
			continue
		}
		rf, perr := Parse(data, name)
		if perr != nil {
			errs = append(errs, perr)
			continue
		}
		loaded.Files = append(loaded.Files, rf)
		loadedNames = append(loadedNames, name)
		contents = append(contents, data)
	}
	loaded.Fingerprint = fingerprint(loadedNames, contents, mode)
	return loaded, errs
}

func fingerprint(names []string, contents [][]byte, mode string) string {
	h := sha256.New()
	for i, n := range names {
		h.Write([]byte(n))
		h.Write([]byte{0})
		h.Write(contents[i])
		h.Write([]byte{0})
	}
	h.Write([]byte(mode))
	return hex.EncodeToString(h.Sum(nil))
}
```

- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5: Commit** — `feat(classify): rule directory loader with coarse fingerprint`

### Task 4: `seed.go` — embedded built-ins, presence-gated update

**Files:**
- Create: `go/internal/classify/seed.go`, `go/internal/classify/seed_test.go`, `go/internal/classify/builtin/coderabbit.yml`, `builtin/greptile.yml`, `builtin/zz-fallback.yml`

- [ ] **Step 1: Write the three built-in YAML files.** `zz-fallback.yml` is EXACTLY the spec's "Heuristics — starter content" block (nit_words 0.4 / security_words 0.7 / security_paths 0.8 / docs_style 0.3). `coderabbit.yml` per the spec's schema example (emoji_severity map 🔒→critical ⚠️→warning 🛠️→suggestion 💡→nit; nitpick_section `<details>.*?Nitpick` → nit; authors `coderabbitai[bot]`, `coderabbit-ai[bot]`). `greptile.yml`: vendor `greptile`, author `greptile-apps[bot]`, one classifier `greptile_confidence` matching `(?i)\b(logic|bug|error|incorrect)\b` → warning until real samples land.

- [ ] **Step 2: Failing tests** (all against `t.TempDir()`):
  - `TestSeedFreshDir`: empty dir → all three files appear.
  - `TestSeedRespectsDeletion`: seed, delete `coderabbit.yml`, re-run → stays deleted.
  - `TestSeedOverwritesPresent`: seed, corrupt `greptile.yml` on disk, re-run → restored to embedded content.
  - `TestSeedNeverTouchesUserFiles`: seed, add `my-rules.yml`, re-run → byte-identical.
  - `TestSeedUnwritableDirFallsBack`: dir with `0o500` perms → `SeedAndUpdate` returns error, but `Builtins()` still parses the embedded files (in-memory fallback path).

- [ ] **Step 3: Run → FAIL.**

- [ ] **Step 4: Implement**

```go
package classify

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed builtin/*.yml
var builtinFS embed.FS

// SeedAndUpdate materializes embedded rule files into dir.
// Fresh dir: write everything. Existing dir: overwrite only built-ins that
// are still PRESENT (upgrade path) — a deleted built-in stays deleted, and
// user-created files are never touched.
func SeedAndUpdate(dir string) error {
	entries, _ := builtinFS.ReadDir("builtin")
	fresh := false
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create rules dir: %w", err)
		}
		fresh = true
	}
	for _, e := range entries {
		dst := filepath.Join(dir, e.Name())
		if !fresh {
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				continue // deleted by the user — respect it
			}
		}
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("seed %s: %w", e.Name(), err)
		}
	}
	return nil
}

// Builtins parses the embedded rule files directly — the in-memory fallback
// when the rules dir is unusable. Embedded files are compile-time trusted;
// a parse error here is a programmer bug surfaced in tests.
func Builtins() ([]*RuleFile, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	var out []*RuleFile
	for _, e := range entries { // ReadDir is already sorted by name
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, err
		}
		rf, err := Parse(data, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, rf)
	}
	return out, nil
}
```

Edge case the test will catch: `TestSeedFreshDir` requires writing all files when the dir was just created, but `TestSeedRespectsDeletion` requires skipping absent files when it wasn't — that's what the `fresh` flag does.

- [ ] **Step 5: Run → PASS.** (Skip the perms test on Windows if CI ever runs there — guard with `runtime.GOOS`.)
- [ ] **Step 6: Commit** — `feat(classify): embedded built-in rules with presence-gated seeding`

### Task 5: `engine.go` — Classify with the spec's resolution order

**Files:**
- Create: `go/internal/classify/engine.go`, `go/internal/classify/engine_test.go`

- [ ] **Step 1: Failing tests** — table-driven `TestClassify` over an engine built from inline-parsed rule files. Cases (each maps to a spec "Resolution" clause):
  - Bot ID: author in a file's `match.author` → `AuthorType=bot`, `BotVendor=coderabbit`; author `random-thing[bot]` with no vendor match → `bot`/`unknown`; plain login → `human`.
  - `severity_map` on named capture group: body `"🔒 SQL injection"` under coderabbit-style map → critical, source `rule:coderabbit.yml:emoji_severity`.
  - Direct last-match-wins: two files (`a.yml` then `z.yml`) whose direct classifiers both match → z's severity and source win.
  - Weighted tally: two weight-0.4 `nit` matches vs one weight-0.7 `critical` match → nit wins (0.8 > 0.7); source names the LAST contributing nit classifier.
  - Direct beats weighted: direct `warning` match + weighted `critical` tally → warning.
  - Category carried from winning weighted classifier.
  - `author`/`path`/`body_pattern` AND-combine: path-gated classifier does not fire when path is nil.
  - No match → `Severity=""`, `Source="unclassified"`, but bot ID still applies.
  - Vendor file scoping: a file with `match.author` set only runs its classifiers for those authors; `zz-fallback.yml` (no `match.author`) runs for everyone.

```go
type Classification struct {
	AuthorType string // "human" | "bot" | "unknown"
	BotVendor  string // "" for humans; "unknown" for unmatched bots
	Severity   string // "" = unclassified
	Category   string
	Source     string // rule:<file>:<classifier> | unclassified
}
```

- [ ] **Step 2: Run → FAIL.**

- [ ] **Step 3: Implement `engine.go`.** Shape:

```go
package classify

import (
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Engine classifies one comment against a Loaded rule set. Immutable after
// construction; safe for concurrent use (RE2 regexps are goroutine-safe).
type Engine struct {
	loaded *Loaded
}

func NewEngine(loaded *Loaded) *Engine { return &Engine{loaded: loaded} }

func (e *Engine) Fingerprint() string { return e.loaded.Fingerprint }

// Classify implements the spec's resolution order:
// files alphabetical → run all matching classifiers → direct last-match-wins,
// weighted tally per severity (max wins) → direct outranks weighted.
// path may be empty (issue comments). Never returns an error: no match is
// Source="unclassified", per "classification is enrichment, never a gate".
func (e *Engine) Classify(author, body, path string) Classification {
	c := Classification{AuthorType: "human", Source: "unclassified"}

	// ── Bot identification (3.2) ──
	for _, rf := range e.loaded.Files {
		for _, a := range rf.Match.Author {
			if a == author {
				c.AuthorType, c.BotVendor = "bot", rf.Vendor
			}
		}
	}
	if c.AuthorType == "human" && strings.HasSuffix(author, "[bot]") {
		c.AuthorType, c.BotVendor = "bot", "unknown"
	}

	// ── Severity resolution (3.3 + 3.4) ──
	var direct *Classification            // last direct match
	weights := map[string]float64{}       // severity → tallied weight
	weightMeta := map[string]Classification{} // severity → last contributor

	for _, rf := range e.loaded.Files {
		if len(rf.Match.Author) > 0 && !contains(rf.Match.Author, author) {
			continue // vendor file owns only its authors
		}
		for i := range rf.Classifiers {
			cl := &rf.Classifiers[i]
			sev, cat, ok := evalClassifier(cl, author, body, path)
			if !ok {
				continue
			}
			result := Classification{
				Severity: sev, Category: cat,
				Source: "rule:" + rf.FileName + ":" + cl.Name,
			}
			if cl.Weight > 0 {
				weights[sev] += cl.Weight
				weightMeta[sev] = result
			} else {
				direct = &result // last match wins
			}
		}
	}

	switch {
	case direct != nil:
		c.Severity, c.Category, c.Source = direct.Severity, direct.Category, direct.Source
	case len(weights) > 0:
		best := maxWeightSeverity(weights) // ties broken by higher severity rank
		m := weightMeta[best]
		c.Severity, c.Category, c.Source = best, m.Category, m.Source
	}
	return c
}
```

`evalClassifier` AND-combines: `author` (exact), `path` (`doublestar.Match`; a path condition on an empty path = no match), `body_pattern` (compiled RE2). For `severity_map`, resolve the first non-empty named capture group's value through the map; unmapped capture → no match. `maxWeightSeverity` breaks weight ties by `ValidSeverities` rank (higher severity wins — deterministic).

- [ ] **Step 4: Run → PASS.** `go vet ./...` clean.
- [ ] **Step 5: Commit** — `feat(classify): classification engine with layered resolution`

### Task 6: Corpus fixture + accuracy gate + benchmark

**Files:**
- Create: `go/internal/classify/testdata/corpus.yml`, `go/internal/classify/corpus_test.go`

- [ ] **Step 1: Build the corpus.** ~50 entries in one YAML fixture: `{author, body, path, want_severity}`. Source real comment text from public OSS PRs reviewed by CodeRabbit/Greptile (search github.com PRs; paraphrase-free bodies, trimmed). Include ≥10 human comments (want empty severity or whatever a rule legitimately assigns), ≥10 nits, ≥5 security-flavored, ≥5 docs-path. This corpus is the tuning target for `zz-fallback.yml` — expect to iterate weights/patterns here.

- [ ] **Step 2: Write the test** (fails until corpus accuracy is reached):

```go
func TestCorpusHeuristicAccuracy(t *testing.T) {
	// Engine from Builtins() only — exactly what ships.
	files, err := Builtins()
	if err != nil { t.Fatal(err) }
	eng := NewEngine(&Loaded{Files: files})

	corpus := loadCorpus(t) // yaml.Unmarshal testdata/corpus.yml
	correct := 0
	for _, c := range corpus {
		got := eng.Classify(c.Author, c.Body, c.Path)
		if got.Severity == c.WantSeverity {
			correct++
		} else {
			t.Logf("MISS %q: got %q want %q (src %s)", truncate(c.Body, 60), got.Severity, c.WantSeverity, got.Source)
		}
	}
	acc := float64(correct) / float64(len(corpus))
	if acc < 0.70 {
		t.Errorf("heuristic accuracy %.0f%% < 70%% (%d/%d)", acc*100, correct, len(corpus))
	}
}

func BenchmarkClassify50(b *testing.B) { /* 50 corpus comments per iteration */ }
```

- [ ] **Step 3: Run, tune `zz-fallback.yml` patterns/weights until ≥70%.** Tuning built-in YAML is the intended workflow, not cheating — the corpus is the regression gate the spec demands.
- [ ] **Step 4: Run benchmark** — `go test -bench Classify -run xxx ./internal/classify/`. Expected: 50 comments well under 50ms (RE2 over short strings ≈ µs; if this fails something is broken, not slow).
- [ ] **Step 5: Commit** — `test(classify): 50-comment corpus with 70% accuracy gate + benchmark`

### Task 7: Writer integration — classify during sync

**Files:**
- Modify: `go/internal/sync/writer.go` (writeComment `:431`, WriteSyncResultsContext `:86`)
- Test: `go/internal/sync/writer_classify_test.go`

- [ ] **Step 1: Read `writer.go` fully before editing** (it was summarized out of context). Find: `Writer` struct (~`:20`), `writeComment` (~`:431`) and its INSERT ... ON CONFLICT column list, the per-PR result loop with savepoints.

- [ ] **Step 2: Failing test** — reuse the writer test setup pattern already in `go/internal/sync/writer_test.go`:
  - `TestWriterClassifiesComments`: write a sync result with a `coderabbitai[bot]` nit comment and a human comment → rows come back with `author_type`/`severity`/`classification_source` populated and a `comment_classification_state` row carrying the engine fingerprint.
  - `TestWriterNilEngineSkipsClassification`: writer without engine (all existing tests) → columns stay NULL, nothing breaks.
  - `TestWriterEngineErrorPersistsUnclassified`: not applicable — `Classify` can't error; instead assert a no-match comment persists `classification_source='unclassified'`.

- [ ] **Step 3: Implement.**
  - Add `engine *classify.Engine` to `Writer` + option method `WithClassifier(e *classify.Engine) *Writer` (mirrors existing `WithEmitter`).
  - In `writeComment`: when `w.engine != nil`, call `w.engine.Classify(comment.Author, comment.Body, commentPath(comment))` and extend the INSERT/UPDATE column list with the five columns (NULLs via the existing `nullableString` helper for empty severity/category/vendor).
  - After the comment INSERT, upsert the fingerprint: `INSERT INTO comment_classification_state (comment_id, fingerprint) VALUES (?, ?) ON CONFLICT(comment_id) DO UPDATE SET fingerprint = excluded.fingerprint`.
  - Comments stay inside the existing per-PR flow (they're outside the savepoint-guarded critical writes — keep it that way).

- [ ] **Step 4: Run `go test -race ./internal/sync/`** → all pass, including pre-existing writer tests (nil-engine path must be transparent).
- [ ] **Step 5: Commit** — `feat(sync): classify comments in the write path`

### Task 8: RefreshIfStale — lazy re-classification on PR open

**Files:**
- Modify: `go/internal/server/handler/comments.go` (handler at `:66`), SELECT list gains the five new columns
- Create: `go/internal/server/handler/refresh.go` (or inline in comments.go if <100 lines)
- Test: extend `go/internal/server/handler/comments_test.go`

- [ ] **Step 1: Read `comments.go` fully.** The handler is `NewRequestCommentsHandler(db prLister, log)` responding `COMMENTS_READY` with `commentRow` structs.

- [ ] **Step 2: Failing tests:**
  - `TestCommentsReadyIncludesClassification`: seeded classified comment → response rows carry `author_type`, `bot_vendor`, `severity`, `category`, `classification_source`, `dismissed` (JSON omitempty pointers, matching the existing `commentRow` style).
  - `TestRefreshIfStaleReclassifies`: seed a comment with fingerprint `"old"` while the injected engine has fingerprint `"new"` → after REQUEST_COMMENTS, the row is re-classified and the state row updated.
  - `TestRefreshIfStaleNoopWhenCurrent`: matching fingerprints → comment row untouched (assert unchanged `classification_source`).

- [ ] **Step 3: Implement.** Handler construction gains the engine (nil = skip refresh, current behavior). Before the SELECT: `UPDATE`-loop over `SELECT c.id, c.author, c.body, c.file_path FROM comments c LEFT JOIN comment_classification_state s ON s.comment_id=c.id WHERE c.pr_id=? AND (s.fingerprint IS NULL OR s.fingerprint != ?)` — re-run `Classify`, UPDATE the comment + upsert state. Single small tx; Go owns writes so this lives Go-side, never renderer-side. Wire the engine through `app.go` where the handler is registered.

- [ ] **Step 4: Run handler tests → PASS.**
- [ ] **Step 5: Commit** — `feat(server): lazy re-classification on PR open via fingerprint staleness`

### Task 9: Wiring — main.go / app.go + `--rules-dir` flag

**Files:**
- Modify: `go/cmd/bistro-sync/main.go`, `go/internal/server/app.go`
- Test: `go/internal/classify/` already covers behavior; add one `app_test.go` case only if App construction is already test-covered (follow the existing pattern — don't invent an App test harness for this).

- [ ] **Step 1: Add flag** in main.go: `rulesDir := flag.String("rules-dir", "", "Rule files directory (default: <UserConfigDir>/bistro/rules)")`. Resolve the default in main.go before App construction:

```go
dir := *rulesDir
if dir == "" {
	cfg, err := os.UserConfigDir()
	if err != nil {
		log.Warn("no user config dir; classification uses embedded built-ins only", "err", err)
	} else {
		dir = filepath.Join(cfg, "bistro", "rules")
	}
}
```

Pass the resolved `dir` into App the same way `--db-path` flows today (follow that flag's plumbing). Note: config dir, not the `os.UserCacheDir()` used for repos at `app.go:183` — rules are user-editable config. An empty `dir` (no config dir available) skips seeding and goes straight to the `Builtins()` fallback in Step 2.
- [ ] **Step 2: Startup sequence** (in App construction, near where the orchestrator/writer are built):

```go
if err := classify.SeedAndUpdate(rulesDir); err != nil {
	log.Warn("rules dir unusable, using embedded built-ins", "err", err)
}
loaded, errs := classify.LoadDir(rulesDir, mode) // mode from classification settings; "rules" until SET_CLASSIFICATION_SETTINGS arrives
for _, e := range errs { log.Warn("rule file skipped", "err", e) }
if len(loaded.Files) == 0 {
	if files, berr := classify.Builtins(); berr == nil {
		loaded = &classify.Loaded{Files: files, Fingerprint: "builtin:" + mode}
	}
}
engine := classify.NewEngine(loaded)
```

Inject into writer (`WithClassifier`) and comments handler. The engine reference must be swappable when mode changes (Task 21) — hold it behind a small `atomic.Pointer[classify.Engine]` or a mutex-guarded getter on App, and pass the getter, not the raw pointer.

- [ ] **Step 3: Full verify** — `cd go && go vet ./... && go test -race ./...` → all green. Launch `go run cmd/bistro-sync/main.go` once; confirm `~/.config/bistro/rules/` (macOS: `~/Library/Application Support/bistro/rules/`) is seeded with 3 files.
- [ ] **Step 4: Commit** — `feat: wire classification engine into startup, sync, and comment serving`

### Task 10: Comment type + renderer utils (sort / collapse / aggregate)

**Files:**
- Modify: `electron/src/renderer/types/index.ts` (Comment interface at `:33`)
- Create: `electron/src/renderer/utils/classification.ts`, `electron/src/renderer/utils/classification.test.ts`

- [ ] **Step 1: Extend the Comment type** — optional fields, matching Go's JSON omitempty:

```ts
// Classification (Phase 3, migration 026) — absent on unclassified/legacy rows.
author_type?: 'human' | 'bot' | 'unknown';
bot_vendor?: string;
severity?: 'critical' | 'warning' | 'suggestion' | 'nit' | 'info';
category?: string;
classification_source?: string;
dismissed?: boolean;
dedup_group_id?: string;
```

- [ ] **Step 2: Failing Vitest tests** for pure helpers (AAA style, descriptive names):
  - `sortBySeverity` — critical→info, unclassified last, `created_at` ASC within rank.
  - `isAutoCollapsed` — true for nit/info only.
  - `aggregateCounts` — returns `{human, bySeverity, byVendor}` for the sidebar; vendors beyond known ones bucket as `other`.
  - `humanizeSource` — `rule:coderabbit.yml:emoji_severity` → `"Rule: coderabbit.yml / emoji_severity"`; `byok:anthropic:claude-haiku-4-5` → `"AI: anthropic (claude-haiku-4-5)"`; `unclassified` → `"Not classified"`.
  - `filterBySeverity` — min-severity filter (`f c/w/s/n`), plus `humanOnly`.

- [ ] **Step 3: Implement `classification.ts`** — pure functions, no React, immutable (sort copies). `SEVERITY_RANK` constant mirrors Go's `ValidSeverities`.
- [ ] **Step 4: Run** — `cd electron && pnpm test classification && pnpm type-check` → PASS.
- [ ] **Step 5: Commit** — `feat(renderer): classification types + pure sort/filter/aggregate utils`

### Task 11: Renderer surface — sidebar counter, overview sort/collapse, markers, tooltip

**Files:**
- Modify: `electron/src/renderer/components/StatusSidebar.tsx`, `electron/src/renderer/pages/PRDetailPage.tsx`, `electron/src/renderer/components/CommentThread.tsx`

- [ ] **Step 1: Read the three components** to find where comments arrive (React Query via `useWebSocket.ts` `COMMENTS_READY` at `:464`-ish region) and current render order.
- [ ] **Step 2: Failing component tests** (Vitest, existing render-test pattern): sidebar shows `Human N · Critical N …` line from `aggregateCounts`; PRDetailPage renders comments in severity order with nit/info collapsed to header; CommentThread header shows severity badge + vendor tag; badge has `title`/tooltip from `humanizeSource`.
- [ ] **Step 3: Implement.**
  - Sidebar: `useMemo(() => aggregateCounts(comments), [comments])`; render near the CI block.
  - PRDetailPage: pipe comments through `sortBySeverity`; collapsed state per comment id (expand on click); dismissed → bottom "Dismissed (N)" section (renders empty until increment 2).
  - CommentThread: `data-severity={comment.severity ?? 'none'}` on the container + CSS `[data-severity='critical'] { border-left: 2px solid … }` map using existing theme tokens; severity badge + vendor tag + tooltip.
- [ ] **Step 4: Full verify** — `pnpm type-check && pnpm lint && pnpm test` → green.
- [ ] **Step 5: Manual acceptance** — `pnpm start`, open a synced PR with bot comments: counter, ordering, stripes, tooltip. Then edit a YAML rule, reopen the PR, confirm classifications update without restart (acceptance #2).
- [ ] **Step 6: Commit** — `feat(renderer): classification UI — counter, severity ordering, markers, why-tooltip`

**Increment 1 done — ship gate:** acceptance criteria 1–4 verifiable. Run both full verify suites.

# Increment 2 — Bulk ops (3.5)

### Task 12: Migration 027 — `comment_dismissals`

**Files:**
- Create: `go/internal/store/migrations/027_add_comment_dismissals.sql`
- Test: extend `go/internal/store/migration_classification_test.go`

- [ ] **Step 1: Failing test** — `TestMigration027CommentDismissals`: table exists with composite PK `(comment_id, account_provider, account_login)`; inserting the same triple twice fails; deleting a comment cascades.
- [ ] **Step 2: Migration** — exactly the spec DDL (per-account, migration-022 namespacing pattern):

```sql
-- 027_add_comment_dismissals.sql
-- Phase 3.5: local dismissals, namespaced per account (pattern of migration 022).
CREATE TABLE comment_dismissals (
  comment_id       TEXT NOT NULL,
  account_provider TEXT NOT NULL DEFAULT 'github',
  account_login    TEXT NOT NULL DEFAULT '',
  dismissed_at     TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  PRIMARY KEY (comment_id, account_provider, account_login),
  FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
);
```

- [ ] **Step 3: Run → PASS. Commit** — `feat(store): migration 027 — per-account comment dismissals`

### Task 13: Dismissals store API

**Files:**
- Create: `go/internal/store/dismissals.go`, `go/internal/store/dismissals_test.go`

- [ ] **Step 1: Failing tests:** `Dismiss(ctx, commentIDs, provider, login)` inserts rows in one tx (idempotent via `INSERT OR IGNORE`); `Undismiss(ctx, commentIDs, provider, login)` deletes; `DismissedIDs(ctx, prID, provider, login)` returns the set for serving; unknown comment id in `Dismiss` fails FK → whole tx rolls back with a clear error.
- [ ] **Step 2: Implement** following the `ActionQueue` struct style (`store/action_queue.go`: struct holding `*sql.DB`, ctx-taking methods, wrapped errors).
- [ ] **Step 3: Run → PASS. Commit** — `feat(store): dismissals persistence API`

### Task 14: WS handlers — DISMISS_COMMENTS / UNDISMISS_COMMENTS

**Files:**
- Create: `go/internal/server/handler/dismiss.go`, `go/internal/server/handler/dismiss_test.go`
- Modify: `go/internal/server/app.go` (register handlers — same pattern as `RegisterHandler("REQUEST_COMMENTS", …)`)

- [ ] **Step 1: Read** `handler/comments_test.go` for the handler-test harness, and `actions/executor.go:252-255` + `actions/handlers.go` for how `ResolveThreadCommand` actions are built/enqueued.

- [ ] **Step 2: Failing tests:**
  - `TestDismissLocalOnly`: scope `local` → rows inserted, `DISMISS_RESULT{batch_id, dismissed_ids, resolved_thread_ids: []}`, nothing enqueued.
  - `TestDismissWithProviderWriteback`: scope `local+provider`, dismissed comments span three threads: one `state_known=1,resolved=0` (→ ResolveThread enqueued + id reported), one `resolved=1` (skipped), one `state_known=0` (skipped). `resolved_thread_ids` lists exactly the first.
  - `TestDismissIssueCommentsStayLocal`: comment with no thread → dismissed locally, no action.
  - `TestUndismiss`: deletes rows; enqueues `UnresolveThread` for exactly `unresolve_thread_ids` from the request — never inferred.
  - `TestDismissResolvesCurrentAccount`: rows carry provider/login from the `current_user` table (migration 022).
  - After both commands: `COMMENTS_READY` re-fires for the PR (assert via the emitter/handler harness).

- [ ] **Step 3: Implement.** Payloads per spec: `DISMISS_COMMENTS {pr_id, comment_ids[]}` / `UNDISMISS_COMMENTS {comment_ids[], unresolve_thread_ids[]}`. `batch_id` = `crypto/rand` UUID-ish hex (no new dep). Scope read from live classification settings (Task 20 supplies them; until then handler takes a scope getter defaulting `local`). Resolvable-thread query: `SELECT DISTINCT t.id FROM comments c JOIN review_threads t ON t.id=c.thread_id WHERE c.id IN (…) AND t.state_known=1 AND t.resolved=0`.

- [ ] **Step 4: Run `go test -race ./internal/server/...` → PASS. Commit** — `feat(server): dismiss/undismiss WS commands with optional provider write-back`

### Task 15: Serve `dismissed` on comments; renderer dismissed section

**Files:**
- Modify: `go/internal/server/handler/comments.go` (LEFT JOIN `comment_dismissals` for the current account → `dismissed` field), `electron/src/renderer/pages/PRDetailPage.tsx`
- Test: extend comments handler test + PRDetailPage test

- [ ] **Step 1: Failing test Go-side** — dismissed comment serves `dismissed: true` for the dismissing account, absent for another account.
- [ ] **Step 2: Implement join + field.**
- [ ] **Step 3: Renderer** — the "Dismissed (N)" section from Task 11 now gets real data; test it renders dismissed comments collapsed at bottom.
- [ ] **Step 4: Verify both suites. Commit** — `feat: surface per-account dismissed state end-to-end`

### Task 16: Renderer — dismissal hook, undo stack, keystrokes, filters

**Files:**
- Create: `electron/src/renderer/hooks/useDismissals.ts`
- Modify: `electron/src/renderer/utils/keybindings.ts`, `electron/src/renderer/hooks/useWebSocket.ts` (DISMISS_RESULT case), `electron/src/renderer/pages/PRDetailPage.tsx`
- Test: `useDismissals` hook test + keybinding tests (existing pattern)

- [ ] **Step 1: Check the keymap first.** Read `keybindings.ts` around `:963` (existing `u`) and grep for existing `d`/`f` bindings on `detail`/`diff` routes. The spec requires resolving conflicts explicitly: if `u` is taken on these routes, scope-split by route/overlay activation (the `activation: {routes, overlays}` mechanism exists for exactly this); if a single-key `d` or `f` exists on the same routes, the sequence prefix would shadow it — document the resolution in the commit message. **Do not silently rebind existing actions.**
- [ ] **Step 2: Failing tests:**
  - `useDismissals`: `dismissNits(comments)` sends `DISMISS_COMMENTS` with all nit+info ids; `dismissBot(vendor)` filters by vendor; DISMISS_RESULT pushes onto a session stack; `undo()` pops and sends `UNDISMISS_COMMENTS{comment_ids: top.dismissed_ids, unresolve_thread_ids: top.resolved_thread_ids}`; empty stack → no-op. Immutable stack updates (spread, not push).
  - Keybindings: `['d','n']`, `['d','b']`, `['f','c']`, `['f','w']`, `['f','s']`, `['f','n']`, `['f','h']`, `['f','f']`, `u` registered on `detail` + `diff` routes with correct descriptions (sequence syntax per the `['g','u']` example at `:638`).
- [ ] **Step 3: Implement.** WS send via the existing socket send path (`useActionDispatch.ts` style); neverthrow on the send result. `d b` with >1 vendor present opens a picker overlay (reuse the existing overlay/palette machinery — check `OverlayContext`); with exactly 1, dismisses directly. Filters set a store value consumed by PRDetailPage through `filterBySeverity`.
- [ ] **Step 4: Verify: `pnpm type-check && pnpm lint && pnpm test` → green.**
- [ ] **Step 5: Manual acceptance** — `d n` dismisses nits, `u` restores, dismissals survive app restart (SQLite), undo stack doesn't (session).
- [ ] **Step 6: Commit** — `feat(renderer): bulk dismiss/filter keystrokes with undo stack`

### Task 17: Dismissal-scope setting (UI + persistence + push to Go)

**Files:**
- Modify: `electron/src/types/settings.ts`, `electron/src/renderer/pages/SettingsPage.tsx`
- Test: settings normalization test + SettingsPage render test

- [ ] **Step 1: Failing tests** — `DEFAULT_SETTINGS.classification` is `{mode: 'rules', dismissalScope: 'local', provider: 'anthropic', model: '', endpoint: ''}`; `normalise()` fills it for old settings files (the merge in `main/settings.ts:29` handles missing sections — verify with a test, don't assume). SettingsPage renders a "Comment Classification" section with the scope radio.
- [ ] **Step 2: Implement:**

```ts
export type ClassificationMode = 'rules' | 'rules+llm' | 'llm-all' | 'server'; // 'server' = hidden Team-plan stub
export type DismissalScope = 'local' | 'local+provider';

export interface ClassificationSettings {
  mode: ClassificationMode;
  dismissalScope: DismissalScope;
  provider: 'anthropic' | 'openai' | 'openrouter' | 'ollama' | 'custom';
  model: string;
  endpoint: string; // custom/ollama only
}
```

Add to `AppSettings` + `DEFAULT_SETTINGS`. **Keep `SETTINGS_SCHEMA_VERSION = 1`** — the deep-merge normalise makes adding a section non-breaking; bump only if the merge test disproves this. SettingsPage: new `<Section id="classification" title="Comment Classification">` in the SECTIONS array — scope radio only for now (mode radio lands in Task 22).
- [ ] **Step 3: Push to Go** — on settings change AND on WS (re)connect, send `SET_CLASSIFICATION_SETTINGS {mode, dismissal_scope, provider, model, endpoint}` (bootstrap-on-connect per the spec wire contract; Go-side handler is Task 20 — until then Go logs unknown message type, which is fine to leave for two tasks **only if** unknown WS types are non-fatal — verify in `ws` server code first; if they error, land a stub ack handler in this task instead).
- [ ] **Step 4: Verify + commit** — `feat(settings): classification settings section with dismissal scope`

**Increment 2 done — ship gate:** acceptance #5. Full verify both suites; manual: dismiss with `local+provider` on a real PR → thread resolves on GitHub via action queue.

# Increment 3 — Dedup (3.6)

### Task 18: Migration 028 + `dedup.go`

**Files:**
- Create: `go/internal/store/migrations/028_add_dedup_and_llm_cache.sql`, `go/internal/classify/dedup.go`, `go/internal/classify/dedup_test.go`

- [ ] **Step 1: Migration test + DDL** (spec-exact):

```sql
-- 028_add_dedup_and_llm_cache.sql
-- No extra indexes by design: cache lookups hit the body_hash PRIMARY KEY,
-- and dedup_group_id is only read on rows already narrowed by pr_id.
ALTER TABLE comments ADD COLUMN dedup_group_id TEXT;

CREATE TABLE llm_classification_cache (
  body_hash  TEXT PRIMARY KEY,
  severity   TEXT NOT NULL,
  category   TEXT,
  provider   TEXT NOT NULL,
  model      TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
```

- [ ] **Step 2: Failing dedup tests** (pure function, no DB):

```go
// DedupInput is what the writer hands the post-pass per comment.
type DedupInput struct {
	ID           string
	AuthorType   string
	FilePath     *string
	AnchoredLine *int
	LineNumber   *int
	Body         string
}
// Dedup returns commentID → groupID for every comment in a group of size ≥ 2.
func Dedup(comments []DedupInput) map[string]string
```

Cases: two bots, same file, lines 10 & 12, near-identical bodies → same group id = `sha256(sorted ids)`; lines 10 & 14 (>±3) → no group; similarity ≤ 0.7 → no group; human author excluded; nil FilePath excluded; both line fields nil excluded; display line prefers AnchoredLine (`COALESCE` semantics); transitive chain A~B~C (A·C dissimilar) → one 3-member connected component; **idempotence**: same input twice → identical ids; markdown/code-fence stripping affects similarity (fenced code ignored).

- [ ] **Step 3: Implement** — normalize (lowercase, strip fenced blocks ``` … ```, strip `*_#>` markdown chars, collapse whitespace), 3-gram sets, Jaccard, pair candidates by (file, |Δline| ≤ 3), union-find for components, group id = hex sha256 of sorted member ids joined by `\n`. Pure Go, stdlib only.
  `// ponytail: O(n²) pair scan per PR — fine at PR comment counts; bucket by file+line window if a 1000-comment PR ever shows up.`
- [ ] **Step 4: Run → PASS. Commit** — `feat(classify): trigram-Jaccard dedup post-pass + migration 028`

### Task 19: Writer dedup integration + merged rendering

**Files:**
- Modify: `go/internal/sync/writer.go` (per-PR post-pass, in-tx, after comments), `go/internal/server/handler/comments.go` (serve `dedup_group_id`), `electron/src/renderer/components/CommentThread.tsx`, `electron/src/renderer/pages/PRDetailPage.tsx`
- Test: writer test + component test

- [ ] **Step 1: Failing writer test** — sync a PR with two similar bot comments → both rows share a `dedup_group_id`; re-sync → same id (idempotent); dissimilar → NULL.
- [ ] **Step 2: Implement** — after a PR's comments are written, collect that PR's comment rows (the just-written data is in memory in `PRFetchResult`; query only if anchor lines are DB-computed), run `classify.Dedup`, `UPDATE comments SET dedup_group_id = ? WHERE id = ?` for members and `… SET dedup_group_id = NULL` for former members no longer grouped. Same tx.
- [ ] **Step 3: Renderer** — group comments by `dedup_group_id` before rendering: a group renders once at its first member's anchor as "Flagged by CodeRabbit + Greptile (2 sources)" with per-source expandable bodies; group severity badge = max member severity (`SEVERITY_RANK`). Other members don't render separately. Test: 2-group renders one merged block; expanding shows both bodies.
- [ ] **Step 4: Verify both suites; manual on a real multi-bot PR. Commit** — `feat: multi-bot dedup end-to-end — grouped writes, merged rendering`

**Increment 3 done — ship gate:** acceptance #6.

# Increment 4 — BYOK (3.8)

### Task 20: Go-side classification settings + BYOK key handlers

**Files:**
- Create: `go/internal/server/handler/classification_settings.go`, `go/internal/server/handler/classification_settings_test.go`
- Modify: `go/internal/server/app.go` (register `SET_CLASSIFICATION_SETTINGS`, `SET_BYOK_KEY`, `DELETE_BYOK_KEY`; hold live settings)

- [ ] **Step 1: Failing tests:**
  - `SET_CLASSIFICATION_SETTINGS {mode, dismissal_scope, provider, model, endpoint}` → stored on App (thread-safe — the dismiss handler and writer read it concurrently), ack `CLASSIFICATION_SETTINGS_UPDATED`; invalid mode value → error response, settings unchanged.
  - Mode change → engine getter now returns an engine whose fingerprint reflects the new mode (reload `LoadDir(dir, newMode)` on change) — this is what makes mode flips lazily re-classify via Task 8.
  - `SET_BYOK_KEY {provider, key}` → `keychain.NewTokenManager()`-style storage under `bistro.byok.<provider>` (read `internal/keychain/keychain.go` — `Store/Get/Delete(service,key)`; construct with the byok service prefix). Key **never** appears in any response or event payload.
  - `DELETE_BYOK_KEY {provider}` → removed.
  - Keychain tests: follow whatever seam existing keychain-dependent tests use (fake keyring vs skip-on-CI) — check `internal/keychain` tests first.
- [ ] **Step 2: Implement.** Settings struct + mutex on App; dismiss handler (Task 14) and writer swap their placeholder getters to this. `server` mode value accepted and stored (stub) — the LLM tier treats it as `rules`.
- [ ] **Step 3: Run → PASS. Commit** — `feat(server): classification settings + BYOK keychain handlers`

### Task 21: `classify/llm` — clients + async tier

**Files:**
- Create: `go/internal/classify/llm/client.go`, `client_test.go`, `tier.go`, `tier_test.go`

- [ ] **Step 1: Failing client tests** (against `httptest.Server`, never live APIs):
  - OpenAI-compatible client (`openai`/`openrouter`/`ollama`/`custom` — one client, different base URLs): builds chat-completions request with a strict JSON-only system prompt, parses `{"severity":"nit","category":"style"}` from the response; invalid severity in reply → error (never persisted).
  - Anthropic client: messages API shape, same contract.
  - 401 → typed `ErrAuth`; 429 → typed `ErrRateLimited`; timeout via ctx.
  - Both return `(severity, category, usageTokens, error)`.
- [ ] **Step 2: Implement `client.go`.** ~30s per-call timeout via `context.WithTimeout` (rule: always ctx). Prompt: system message pinning the taxonomy + "respond with JSON only", user message = comment body (truncated ~4000 chars). Secret handling per Go security rule: key arrives as an argument from keychain, never logged.
- [ ] **Step 3: Failing tier tests** (mock client injected via small interface):
  - `rules+llm` mode selects only `classification_source='unclassified'` comments; `llm-all` selects all; `rules`/`server` → no-op.
  - Cache hit (`llm_classification_cache` by sha256 of normalized body) skips the client, applies cached severity, writes `classification_source = 'byok:<provider>:<model>'` **from the cache row** (provenance = who actually judged it).
  - Miss → client call, single-row UPDATE + cache INSERT.
  - Fan-out bounded: with `errgroup.SetLimit(4)` and 10 comments, ≤4 concurrent (track with atomic counter in mock).
  - `ErrAuth` → tier disables itself + emits `BYOK_AUTH_FAILED` once; re-enabled when key changes.
  - `ErrRateLimited` → abandon batch silently.
  - Client failure on one comment → others proceed; failed comment keeps deterministic result.
  - Context cancellation → clean stop, no goroutine leak (`-race` + a drain assertion).
  - Batch completion → `COMMENTS_READY` re-fires for the PR; `BYOK_USAGE {session_tokens, est_cost}` emitted.
  - **Re-fire contract:** `COMMENTS_READY` is today a *response* to `REQUEST_COMMENTS` carrying the full `commentRow` list. The tier's re-fire must be a server-initiated *push* with the identical payload shape (`{type: "COMMENTS_READY", pr_id, comments: [...]}`), built by the same query the handler uses — extract that query into a shared function rather than duplicating it. The renderer's existing `COMMENTS_READY` case in `useWebSocket.ts` then updates the query cache with no renderer change needed; Task 22 verifies this rather than inventing a new event.
- [ ] **Step 4: Implement `tier.go`.** `Tier.KickPR(ctx, prID)` spawns one goroutine per PR; per-PR ctx derived from app shutdown ctx. Usage accumulates in the tier (atomic), cost = tokens × a per-model table (rough constants, clearly labeled estimates). Writes go through single-row `UPDATE comments SET severity=?, category=?, classification_source=? WHERE id=?` + cache upsert — Go owns writes; no long tx.
- [ ] **Step 5: Run `go test -race ./internal/classify/...` → PASS. Commit** — `feat(classify/llm): BYOK clients and bounded async classification tier`

### Task 22: Wire LLM tier — post-sync kick, PR-open kick, Settings UI

**Files:**
- Modify: `go/internal/sync/writer.go` (post-commit hook), `go/internal/server/handler/comments.go` (RefreshIfStale kicks tier), `go/internal/server/app.go` (construct tier), `electron/src/renderer/pages/SettingsPage.tsx`, `electron/src/renderer/hooks/useWebSocket.ts` (BYOK_USAGE, BYOK_AUTH_FAILED)
- Test: extend writer/handler tests; SettingsPage test

- [ ] **Step 1: Failing tests:**
  - Writer with tier + LLM mode: after `tx.Commit()` (post-commit events block at `:262-303`), `KickPR` called once per synced PR; `rules` mode → never called.
  - RefreshIfStale in LLM mode kicks the tier for the opened PR (spec "identical code path as post-sync").
  - Async UI refresh: after a tier batch completes, the pushed `COMMENTS_READY` (payload contract in Task 21) reaches the renderer and the comment list re-renders with upgraded severities — assert the `useWebSocket` handler processes a pushed (non-request-response) `COMMENTS_READY` correctly; if it assumes request/response pairing, fix that here.
  - SettingsPage: mode radio shows exactly 3 options (Rules engine / Rules + AI fallback / AI for everything) — `server` never renders; AI mode selected → provider dropdown, model field, key input (masked, presence-only, submitted via `SET_BYOK_KEY`, never stored in settings JSON), session token/cost line updating from `BYOK_USAGE`; `BYOK_AUTH_FAILED` badges the key field.
- [ ] **Step 2: Implement.** Key input flow with neverthrow `ResultAsync` on the WS round-trip; input cleared after successful ack.
- [ ] **Step 3: Full verify** — both suites green.
- [ ] **Step 4: Manual acceptance** — Ollama or a real key: switch to `rules+llm`, open a PR with unclassified comments → refinements arrive async, tooltip says `AI: <provider> (<model>)`, usage counter ticks, same text on another PR = cache hit (no call), kill network → silent degradation to rules.
- [ ] **Step 5: Commit** — `feat: BYOK LLM tier wired end-to-end with Settings UI`

### Task 23: Docs + final acceptance sweep

**Files:**
- Create: user-facing doc under `website/src/pages/docs/` (CLAUDE.md: that's where user-facing docs live — match the existing page format there)
- Modify: `docs/ROADMAP.md` (tick Phase 3 boxes), `CLAUDE.md` only if commands/conventions changed (they didn't — skip unless something did)

- [ ] **Step 1: Write the classification docs page** — user-facing: how classification works, rules dir location + YAML schema by example, override mechanics (alphabetical, last wins, delete-to-disable), heuristics list (this + `zz-fallback.yml` = the documented heuristic library the roadmap asks for), keystroke table, BYOK setup + privacy note (key in OS keychain; only opt-in LLM calls leave the machine).
- [ ] **Step 2: Walk all 8 acceptance criteria** from the spec, each verified by a named test or a manual step performed now. Fix anything that fails before ticking.
- [ ] **Step 3: Update ROADMAP** Phase 3 checkboxes (3.7 already marked deferred).
- [ ] **Step 4: Full verify both suites one last time. Commit** — `docs: classification user guide + roadmap Phase 3 completion`

---

## Execution notes

- **Task order is dependency order.** Within an increment, tasks are sequential. Increments 2 and 3 both depend only on 1; 4 depends on 1 (and touches files from 2's settings work — do 2 first as numbered).
- **When a line number from this plan doesn't match:** the codebase moved; grep for the symbol instead (`WriteSyncResultsContext`, `NewRequestCommentsHandler`, `SETTINGS_SCHEMA_VERSION`). Never edit blind.
- **Read before modify.** Tasks that touch `writer.go`, `comments.go`, `keybindings.ts`, `SettingsPage.tsx` start with a full read of that file — their internal patterns (savepoints, `commentRow`, activation descriptors, SECTIONS) are the template.
- **No try/catch in TS. neverthrow everywhere fallible.** No CHECK constraints in SQL (Go validates). RE2 only — never introduce a backtracking regex engine.



