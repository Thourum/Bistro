# Comment Classification Foundation (Phase 3.1–3.4) — Design

> Scope: roadmap Phase 3 sub-features 3.1 (schema), 3.2 (bot identification),
> 3.3 (rules engine), 3.4 (severity heuristics). Bulk ops (3.5), dedup (3.6),
> slop detection (3.7), and BYOK (3.8) are explicitly out of scope and ship as
> their own spec → plan → implement cycles.

## Goal

When sync runs, every comment in every watched PR is classified along four axes
— `author_type`, `bot_vendor`, `severity`, `category` — by a YAML-driven,
user-customizable rules engine that runs in Go and persists results into the
existing SQLite cache. The renderer surfaces classification via a per-PR counter
in the sidebar, reorders the PR-overview comment list so critical comments
surface first (nits auto-collapsed), and marks inline threads in the diff view
with a severity color.

**The wedge.** Opening a PR with 47 CodeRabbit comments today means scrolling
past 40 nits to find the 1 critical. After this slice: the sidebar reads
"1 critical, 3 warnings, 12 nits," and the overview leads with the critical one.

> **⚠️ Amendment (2026-06-20) — depends on Phase 2.0; unified comments table.**
> The 2026-06-19 roadmap audit surfaced a cross-phase interaction
> (see `docs/superpowers/plans/2026-06-20-phase-2-decomposition.md`):
> - **Classification operates on the unified `comments` table** (a single table with
>   a `kind` discriminator `'issue' | 'review'`, introduced by Phase 2.0). The
>   classification columns and the `comment_classification_state` table apply
>   uniformly to every row regardless of `kind` — no separate path for inline
>   comments. Migration 013 here adds the classification columns to that one table.
> - **Build order: Phase 2.0 ships first.** Bot findings (CodeRabbit, Greptile,
>   Copilot, …) are posted mostly as **inline review comments**. Phase 2.0 adds the
>   inline fetch (`PullRequests.ListComments`) + the unified `comments` table; this
>   classification slice builds *on top of* 2.0, so inline bot findings already exist
>   in the cache when classification ships — full coverage from day one. Migration
>   numbering reflects this: Phase 2.0 = **012**, this classification slice = **013**.
>   (If the order ever flips and 3 ships before 2.0, classification would cover
>   issue-level comments only until 2.0 lands — not the planned path.)

## Non-goals (deferred to later slices)

- 3.5 bulk operations (`dN`, `dB`, `f c/w/n`), undo.
- 3.6 multi-bot dedup. `dedup_group_id` is **not** added this slice.
- 3.7 PR slop detection.
- 3.8 BYOK AI classification.
- Cross-file "jump to next critical" keybinding (Phase 2.4 territory).
- Per-comment "show why" tooltip (`classification_source` is stored; the tooltip
  is a trivial later add).
- Write-back of any classification/dismissal to the provider.

## Architecture

Classification lives entirely in Go and persists into SQLite. The renderer reads
classified data through the **existing** comments query path — no new WebSocket
events for the foundation slice.

### New package: `go/internal/classify/`

```
classify/
├── engine.go            # Match comments against loaded rules → ClassificationResult
├── rules.go             # YAML schema, parse, validate, compile+cache regexes
├── loader.go            # Load *.yml from rules dir (alphabetical), build fingerprint
├── seed.go              # First-launch seed + presence-gated update of built-ins
├── fingerprint.go       # sha256 over sorted (filename:content_hash) pairs
└── builtin/             # go:embed
    ├── coderabbit.yml
    ├── greptile.yml
    ├── copilot.yml
    ├── qodo.yml
    ├── cursor-bugbot.yml
    ├── claude-code.yml
    ├── codex.yml
    ├── sourcery.yml
    └── zz-fallback.yml  # Heuristics; sorts last → lowest precedence
```

Plus the matching `_test.go` files.

### Glue points (existing files touched)

| File | Change |
|---|---|
| `go/internal/store/migrations/013_add_comment_classification.sql` | New columns on `comments` + `comment_classification_state` table |
| `go/internal/sync/writer.go` | Classify each comment before persist; store classification + current fingerprint (synchronous) |
| `go/internal/server/handler/comments.go` | Before returning comments, run `RefreshIfStale(prID)` — re-classify comments whose stored fingerprint ≠ current |
| `go/cmd/bistro-sync/main.go` | On startup: `classify.SeedAndUpdate(rulesDir)` once, then construct the engine |
| `electron/src/renderer/components/StatusSidebar.tsx` | Per-PR counter component |
| `electron/src/renderer/pages/PRDetailPage.tsx` | Overview: sort by severity, auto-collapse nit/info, gutter stripe |
| `electron/src/renderer/components/CommentThread.tsx` | Diff-view inline thread: severity marker + vendor tag |
| `electron/src/renderer/pages/DiffPage.tsx` | Confirm/extend the show/hide-inline-comments toggle |
| Comment TS type (location TBD at impl) | Add 5 optional classification fields |

### Data flow — freshly synced PR (synchronous classify)

```
sync fetches comment
  → classify.Engine.Classify(comment) + current RulesFingerprint
  → writer persists comment + {author_type, bot_vendor, severity, category,
                                classification_source} + fingerprint
  → existing COMMENTS_READY / PR_LIST_READY events fire
  → renderer rerenders list + sidebar counter
```

Classification is regex over text already in memory (microseconds/comment), so
it runs **synchronously** in the ingest path. Every comment in SQLite is
classified by the time it is queryable. No async pass, no "unclassified flicker".

### Data flow — user opens a PR (rules may have changed)

```
renderer requests comments for PR
  → handler RefreshIfStale(prID):
      current := loader.Fingerprint()
      for each comment whose stored fingerprint != current:
        re-run engine; UPDATE comments SET ...; UPDATE state SET fingerprint
  → return comments
```

Coarse fingerprint (see Data Model): any rule-file change invalidates all
comments on next open. The first open after an edit pays a small re-classify
pass; subsequent opens are no-ops. **No fsnotify, no background watcher, no
timer.** The system stays cold until the user looks.

`RefreshIfStale` re-classifies in the same synchronous regex-over-memory path as
ingest, so its cost scales linearly with the PR's comment count and is bounded by
the same per-comment budget. For the foundation slice this is fine even on large
PRs (a few thousand regex matches). If a pathologically large PR ever makes the
first-open-after-edit pass noticeable, the upgrade path is to classify in the
background and let the renderer show `unclassified` until done — explicitly out of
scope now.

Acceptance for reload: *editing a YAML and reopening a PR shows updated
classifications without restarting the app.*

## Data Model

### Migration `013_add_comment_classification.sql`

```sql
ALTER TABLE comments ADD COLUMN author_type TEXT;            -- 'human'|'bot'|'unknown'
ALTER TABLE comments ADD COLUMN bot_vendor TEXT;             -- 'coderabbit'|'greptile'|...
ALTER TABLE comments ADD COLUMN severity TEXT;               -- 'critical'|'warning'|'suggestion'|'nit'|'info'
ALTER TABLE comments ADD COLUMN category TEXT;               -- 'security'|'correctness'|'performance'|'style'|'readability'|'docs'
ALTER TABLE comments ADD COLUMN classification_source TEXT;  -- 'rule:<file>:<name>'|'fallback'|'unclassified'

CREATE TABLE comment_classification_state (
  comment_id  TEXT PRIMARY KEY,
  fingerprint TEXT NOT NULL,
  FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
);
```

No `CHECK` constraints on the new enum columns. Values are validated in the Go
engine (the rules schema parser is the single writer); a DB-level constraint would
duplicate that and make adding a severity/category a two-place migration. The
listed enum values are the contract; the engine never writes anything else.

**Cut from roadmap 3.1:**
- `dedup_group_id` — only used by 3.6; adding it now is a speculative migration.
  Lands with the 3.6 slice.
- `classified_at` on the state table — nothing reads a timestamp this slice; the
  fingerprint alone answers "stale?".

**Why a separate state table** rather than a `fingerprint` column on `comments`:
the fingerprint is internal bookkeeping, `comments` is served on every
renderer query. Keeping it out of the hot row avoids widening every comment
SELECT with a field the UI never reads. One JOIN, only in the refresh path.

### Fingerprint (coarse)

`sha256` over the sorted list of `<filename>:<sha256(content)>` for every `*.yml`
in the rules dir. Stable across reorderings; changes iff any file content
changes. Any rule change → new fingerprint → every comment stale on next open.

### Rules YAML schema

```yaml
name: CodeRabbit
match:                          # which comments this file owns
  author: ["coderabbitai[bot]", "coderabbit-ai[bot]"]

classifiers:
  - name: emoji_severity
    body_pattern: '^(?P<emoji>🔒|⚠️|🛠️|💡)'
    severity_map: { "🔒": critical, "⚠️": warning, "🛠️": suggestion, "💡": nit }

  - name: nitpick_section
    body_pattern: '<details>.*?Nitpick'
    severity: nit

  - name: security_in_auth      # multi-condition (heuristics use this form)
    path: "**/auth/**"
    body_pattern: '(SQL injection|XSS|credential|secret)'
    severity: critical
    category: security
    weight: 0.8                 # weighted classifiers sum per severity; max wins
```

Per classifier: `name`; optional `author` / `path` / `body_pattern`
(AND-combined); an outcome that is either a direct `severity` (+ optional
`category`) or a `severity_map` keyed on a named capture group; optional `weight`
for fallback heuristics.

### Resolution

1. Load files alphabetically.
2. Run every matching classifier across all loaded files.
3. **Direct-severity** classifiers: last match wins (so `coderabbit.company.yml`,
   loading after `coderabbit.yml`, overrides it).
4. **Weighted** classifiers (heuristics): tally weight per severity, max-weight
   severity wins.
5. A direct match outranks any weighted result.
6. `classification_source` records the winner: `rule:<file>:<classifier>`.
7. No match → `severity = NULL`, `classification_source = 'unclassified'`.

## Rule Directory Lifecycle

`classify.SeedAndUpdate(rulesDir)` runs once at startup.

```
os.MkdirAll(rulesDir)
dirIsEmpty := no *.yml present

for each embedded builtin F (e.g. "coderabbit.yml"):
    target := rulesDir/F
    if dirIsEmpty:        write F            # first launch → seed all
    else if exists(target): overwrite F      # present → user uses this vendor → update
    else:                 skip               # absent → user deleted it → respect
```

Contract:
- **First launch** (dir empty) → all built-ins written.
- **Upgrade, file present** → overwritten with new built-in version. User edits to
  built-in files are lost on update (documented). Escape hatch: name a custom
  file `coderabbit.company.yml` — it never matches an embedded filename, so it is
  never touched.
- **Upgrade, file absent** → stays gone. Vendor permanently disabled.
- **Restore a deleted built-in**: `touch coderabbit.yml` then relaunch — present →
  overwritten with the default. (Empty or non-empty: present always overwrites.)
- **Custom files** → never touched.

The `dirIsEmpty` gate is the only subtlety: without it, deleting *every* built-in
would re-seed everything on next launch, breaking "disable by deletion". Once the
dir holds any `.yml`, we are in update-mode where absent built-ins stay absent.

No `--no-seed` flag, no `:reset-rules` command this slice (relaunch restores
present built-ins; `touch` restores deleted ones). Add the palette command later
if users need it.

## Renderer Surface

Two distinct views — they present the same classification data differently.

### PR overview (`PRDetailPage.tsx`) — description + comment timeline, no code

- **Sort:** severity rank DESC (critical → info, unclassified last), then
  `created_at` ASC.
- **Auto-collapse:** `nit` and `info` render header-only; click to expand.
- **Gutter:** 2px left stripe colored by severity. CSS only — `data-severity`
  attribute + stylesheet map, no JS.

### Diff view (`DiffPage.tsx` / `CommentThread.tsx`) — code + inline threads

- Inline threads render as today; confirm/extend the existing show/hide-inline
  toggle.
- Each shown thread gets a small severity dot/tag on its header. Same color map
  as the gutter.
- No reordering here — threads are anchored to lines; line order is the file's.

### Sidebar counter (`StatusSidebar.tsx`)

Aggregate for the open PR, near the CI block:

```
Comments
  Human 4 · Critical 1 · Warning 3 · Nit 12
  CodeRabbit 23 · Greptile 8 · Other 2
```

Counts computed client-side via `useMemo` over the comments already in the store
(group by `severity` and `bot_vendor`). No new query, no new IPC, no aggregation
endpoint until the list is too big to count client-side (it won't be).

### Severity color map (shared constant)

```
critical → red    warning → amber   suggestion → blue
nit → grey        info → grey       unclassified → none
```

### Type change

Extend the `Comment` TS type with five optional fields: `authorType`,
`botVendor`, `severity`, `category`, `classificationSource`.

## Error Handling

Go:
- **Malformed user YAML** → log file + line, **skip that file**, keep loading the
  rest. One bad rule must never kill classification or sync. Not surfaced to the
  UI yet (add a "rules health" settings panel later if needed).
- **Bad regex** → caught at *load* (compile once, cache `*regexp.Regexp`); the
  file is skipped with a clear log line.
- **Classification never blocks sync** — engine error on a comment → persist
  `unclassified`, continue. Classification is enrichment, not a gate.
- **Rules dir unwritable** (seed fails) → log, fall back to the embedded built-in
  set held in memory so classification still runs.
- **Regex catastrophic backtracking** → non-issue: Go `regexp` is RE2, no
  backtracking. Do not introduce a backtracking engine later.

TypeScript:
- Use **neverthrow** (`Result`/`ok`/`err`) for fallible paths — no try/catch,
  matching Go's explicit-error style and existing convention
  (`renderer/hooks/useSQLiteQuery.ts`). Most renderer changes here are pure
  (sort/collapse/count over in-memory data) and don't fail; the convention
  applies if/when an IPC call is added.

## Testing

Go (load-bearing):
- `rules_test.go` — parse valid; reject malformed (bad regex, unknown field,
  missing outcome). Bad regex fails at load, not match time.
- `engine_test.go` — table-driven matrix: emoji→severity map, author match,
  multi-condition (path+body), weighted-sum max-wins, last-match-wins across
  files, direct beats weighted, unmatched → unclassified. This is the corpus the
  roadmap's 70% accuracy target leans on.
- `loader_test.go` — alphabetical order; `coderabbit.company.yml` overrides
  `coderabbit.yml`; fingerprint changes iff content changes; stable across
  reorderings.
- `seed_test.go` — first-launch seeds all; present overwritten; deleted stays
  gone; custom untouched; empty-dir gate behavior documented.
- Refresh path — stale fingerprint re-classifies; matching fingerprint is a
  no-op (assert engine not invoked).
- **Benchmark** — classify a realistic 50-comment PR against the full built-in
  rule set; assert well under the 50ms/PR budget. The guard against an
  accidentally quadratic engine.

Renderer (light):
- Sidebar counter `useMemo` grouping over a fixture.
- Sort + collapse: critical sorts above nit; nit renders collapsed.
- No E2E this slice (pure Go logic + CSS reorder; unit coverage both sides is
  enough). Add E2E when 3.5 introduces stateful user actions.

## Acceptance

- Classification runs during sync without measurably slowing it (<50ms/PR
  typical).
- Built-in rules ship starter coverage: CodeRabbit emoji severity, Greptile
  severity field, generic `[bot]` author detection, fallback heuristics file.
- Deleting a built-in YAML permanently disables that vendor's classification.
- Dropping in `coderabbit.company.yml` overrides CodeRabbit without touching the
  built-in.
- App update overwrites a present `coderabbit.yml`; doesn't recreate a deleted
  one; doesn't touch `coderabbit.company.yml`.
- Editing a YAML and reopening a PR shows updated classifications without an app
  restart.
- PR overview opens with the most severe comments first, nits collapsed.
- Sidebar shows accurate per-PR counts.
