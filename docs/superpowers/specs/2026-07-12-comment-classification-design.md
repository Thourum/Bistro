# Phase 3 — AI Comment Classification — Design

> Scope: roadmap Phase 3 sub-features 3.1 (schema), 3.2 (bot identification),
> 3.3 (rules engine), 3.4 (severity heuristics), 3.5 (bulk operations),
> 3.6 (multi-bot dedup), 3.8 (BYOK AI). **3.7 (slop detection) is deferred to
> the Team plan** — two of its five signals (commit messages, contributor
> account age) need data Bistro does not sync, and the feature is a better fit
> for server-side computation. The roadmap is updated alongside this spec.
>
> Supersedes `docs/superpowers/specs/2026-06-19-comment-classification-foundation-design.md`
> (removed 2026-07-11; in git history). That spec covered 3.1–3.4 only and
> predates migrations 014–025; its decisions are carried forward here, updated
> against the current codebase.

## Goal

Classify every PR comment by author type, bot vendor, severity, and category —
deterministically, inspectably, explainably. The user can always see *why* a
comment was tagged: the matching rule file, the classifier name, or the LLM
that judged it. Then make classification actionable: bulk-dismiss the noise,
merge duplicate bot findings, and optionally refine with a user-supplied LLM.

Every classification is local. The only network calls are the user's own
opt-in BYOK LLM calls, straight from the Go process to the provider.

## Build order

One pipeline, four increments — each shippable and testable alone:

1. **Foundation (3.1–3.4):** migration 026, `internal/classify` engine,
   bot ID, YAML rules, weighted heuristics, sidebar counter, severity-ordered
   overview, diff-view severity markers, "why" tooltip.
2. **Bulk ops (3.5):** migration 027 (`comment_dismissals`), `d`/`f`/`u`
   keystrokes, dismissal-scope setting with optional provider write-back.
3. **Dedup (3.6):** migration 028 (`dedup_group_id` + LLM cache table),
   writer post-pass, merged-finding rendering.
4. **BYOK (3.8):** classification-mode setting, `classify/llm` async tier,
   Settings section, keychain key storage, usage counter.

(Migration numbers are next-available at implementation time; 026–028 assumes
the current head of 025.)

## Architecture

### New packages

```
go/internal/classify/          — deterministic tier (foundation)
  rules.go        YAML schema + parser; regexes compiled at load
  loader.go       load rule dir alphabetically; Fingerprint()
  seed.go         go:embed built-ins → seed + presence-gated update
  engine.go       Classify(author, body, path) → Classification
  dedup.go        per-PR dedup post-pass (increment 3)
  builtin/        coderabbit.yml, greptile.yml, zz-fallback.yml
go/internal/classify/llm/      — BYOK tier (increment 4)
  client.go       OpenAI-compatible + Anthropic HTTP clients
  tier.go         post-commit per-PR fan-out, cache, usage accounting
```

### Glue points (existing files touched)

| File | Change |
|---|---|
| `go/internal/store/migrations/026..028_*.sql` | Schema below |
| `go/internal/sync/writer.go` | Classify each comment before persist (in-tx); dedup post-pass per PR (in-tx); kick LLM tier after commit |
| `go/internal/server/handler/comments.go` | `RefreshIfStale(prID)` before serving: re-classify comments whose stored fingerprint ≠ current |
| `go/cmd/bistro-sync/main.go` | Startup: `classify.SeedAndUpdate(rulesDir)`, construct engine, inject into writer + handler |
| `go/internal/actions/*` | Dismiss-with-write-back enqueues existing resolve/unresolve actions |
| `electron/src/renderer/components/StatusSidebar.tsx` | Per-PR classification counter |
| `electron/src/renderer/pages/PRDetailPage.tsx` | Severity sort, nit/info collapse, gutter stripe, dismissed section |
| `electron/src/renderer/components/CommentThread.tsx` | Severity marker, vendor tag, merged dedup rendering, why-tooltip |
| `electron/src/renderer/utils/keybindings.ts` | `d n`, `d b`, `f c/w/s/n/h/f`, `u` (see Keystrokes) |
| `electron/src/renderer/pages/SettingsPage.tsx` | "Comment Classification" section |
| Comment TS type | Optional classification + dismissal fields |

### Data flow — sync (deterministic tier, synchronous)

```
Phase 2 fetch (already parallel, 4-worker pool)
  → Phase 3 writer tx, per comment:
      bot ID → rules → heuristics → persist classification + fingerprint
    per PR, after its comments: dedup post-pass → dedup_group_id
  → tx commit
  → COMMENTS_READY / PR_LIST_READY (existing events)
```

Classification is RE2 regex over text already in memory — microseconds per
comment. It runs synchronously and serially inside the existing single write
transaction; SQLite has one writer, so there is nothing to parallelize here.
Every comment is classified by the time it is queryable: no async pass for the
deterministic tier, no unclassified flicker.

### Data flow — LLM tier (async, post-commit)

```
tx commit
  → if mode ∈ {rules+llm, llm-all}: one goroutine per synced PR
      select comments needing LLM (mode-dependent)
      → llm_classification_cache lookup; hits applied immediately
      → misses: errgroup.SetLimit(N) fan-out (N=4 default; higher for
        localhost Ollama), per-comment timeout ~30s
      → each result: single-row UPDATE via store + cache INSERT
      → batch done: COMMENTS_READY re-fires for that PR
```

Sync never waits on the network. The UI shows the deterministic result
instantly; LLM refinements arrive seconds later and upgrade in place. This
mirrors the concurrency shape sync already has (worker-pool fetch, serial
write) rather than inventing a queue.

### Data flow — PR open (rules may have changed)

```
renderer requests comments
  → handler RefreshIfStale(prID):
      for each comment whose stored fingerprint != loader.Fingerprint():
        re-run deterministic tier; UPDATE comment + state row
      if mode ∈ {rules+llm, llm-all}: kick the same async LLM tier for this
        PR (identical code path as post-sync; cache makes repeats cheap)
  → return comments
```

Coarse fingerprint: sha256 over rule-dir file names+contents **plus the
classification mode string**. Any rule edit or mode change invalidates lazily
on next PR open. No fsnotify, no watcher, no timer — cold until the user
looks. Mode changes are not retroactive across the whole DB; switching to an
LLM mode upgrades a PR's comments the next time that PR is opened (or synced),
via the LLM kick above.

## Data model

### Migration 026 — classification columns + state (3.1)

```sql
ALTER TABLE comments ADD COLUMN author_type TEXT;            -- 'human'|'bot'|'unknown'
ALTER TABLE comments ADD COLUMN bot_vendor TEXT;             -- 'coderabbit'|'greptile'|…
ALTER TABLE comments ADD COLUMN severity TEXT;               -- 'critical'|'warning'|'suggestion'|'nit'|'info'
ALTER TABLE comments ADD COLUMN category TEXT;               -- 'security'|'correctness'|'performance'|'style'|'readability'|'docs'
ALTER TABLE comments ADD COLUMN classification_source TEXT;  -- 'rule:<file>:<name>'|'byok:<provider>:<model>'|'unclassified'

CREATE TABLE comment_classification_state (
  comment_id  TEXT PRIMARY KEY,
  fingerprint TEXT NOT NULL,
  FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
);
```

No CHECK constraints: the Go engine is the single writer and validates values;
the listed enums are the contract. The fingerprint lives in a side table so
the hot `comments` SELECT never widens. Both comment kinds (`issue` and
`review`, per migration 012) are classified.

### Migration 027 — local dismissals (bulk ops)

Per-account, following the migration-022 namespacing pattern:

```sql
CREATE TABLE comment_dismissals (
  comment_id       TEXT NOT NULL,
  account_provider TEXT NOT NULL DEFAULT 'github',
  account_login    TEXT NOT NULL DEFAULT '',
  dismissed_at     TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  PRIMARY KEY (comment_id, account_provider, account_login),
  FOREIGN KEY(comment_id) REFERENCES comments(id) ON DELETE CASCADE
);
```

Undo = DELETE the row. The local row is always the UI's source of truth.

### Migration 028 — dedup + LLM cache

```sql
ALTER TABLE comments ADD COLUMN dedup_group_id TEXT;

CREATE TABLE llm_classification_cache (
  body_hash  TEXT PRIMARY KEY,   -- sha256 of normalized comment body
  severity   TEXT NOT NULL,
  category   TEXT,
  provider   TEXT NOT NULL,      -- 'anthropic'|'openai'|'openrouter'|'ollama'
  model      TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
```

`dedup_group_id` = hash of the sorted member comment IDs → idempotent across
re-syncs.

### Rules YAML schema (3.3)

```yaml
name: CodeRabbit
vendor: coderabbit
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
    weight: 0.8                 # weighted classifiers tally per severity; max wins
```

Per classifier: `name`; optional `author` / `path` / `body_pattern`
(AND-combined); an outcome that is either a direct `severity` (+ optional
`category`) or a `severity_map` keyed on a named capture group; optional
`weight` marking it as a heuristic.

### Resolution

1. Load files alphabetically; heuristics ship as `zz-fallback.yml` so they
   load last.
2. Run every matching classifier across all loaded files.
3. Direct-severity classifiers: **last match wins** (`coderabbit.company.yml`
   overrides `coderabbit.yml` by loading later).
4. Weighted classifiers: tally weight per severity; max-weight severity wins.
5. A direct match outranks any weighted result.
6. `classification_source` records the winner: `rule:<file>:<classifier>`.
7. No match → `severity = NULL`, `classification_source = 'unclassified'`.

No confidence score in v1 (deferred until real conflicts are observed).

### Heuristics — `zz-fallback.yml` starter content (3.4)

Heuristics are not a separate code path: they are ordinary weighted
classifiers in one built-in file that loads last. Starter set (tuned against
the corpus before v1.0; the file is the documentation the roadmap asks for,
alongside `docs/classification.md`):

```yaml
name: Heuristics
classifiers:
  - name: nit_words
    body_pattern: '(?i)(nit:|nitpick|consider|you could|minor\b|stylistic|preference|personal taste|would be cleaner)'
    severity: nit
    weight: 0.4
  - name: security_words
    body_pattern: '(?i)(sql injection|xss|credential|secret|password|auth bypass|race condition|memory leak)'
    severity: critical
    category: security
    weight: 0.7
  - name: security_paths
    path: "{**/auth/**,**/security/**,**/payments/**}"
    body_pattern: '(?i)(vulnerab|inject|leak|bypass)'
    severity: critical
    category: security
    weight: 0.8
  - name: docs_style
    path: "{**/*.md,**/*.rst}"
    severity: suggestion
    category: docs
    weight: 0.3
```

Weighted results tally per severity, max wins; any direct vendor-rule match
outranks all of these. Category from the winning weighted classifier is
carried along when present.

### Bot identification (3.2)

- `author_type`: author matches any rule file's `match.author` → `bot` with
  that file's `vendor`; else login ends in `[bot]` (or matches the engine's
  generic bot heuristic) → `bot` + `bot_vendor = 'unknown'`; else `human`.
- Built-in vendors at foundation: CodeRabbit, Greptile. The roadmap's fuller
  list (Copilot, Qodo, Cursor BugBot, Claude Code, Codex, Sourcery) is added
  as one YAML file each, as real comment samples are collected — `go:embed`
  picks up whatever is in `builtin/`.

### Rule directory lifecycle

- Built-ins `go:embed`-ed into the binary.
- First launch: seeded to `~/.config/bistro/rules/`.
- App upgrade: **presence-gated update** — overwrite a built-in that is still
  present, respect deletions (a deleted `coderabbit.yml` stays deleted), never
  touch user-created files.
- Users drop their own YAML in the same dir; alphabetical load order is the
  override mechanism.
- No runtime fetch, no hot-reload daemon; staleness is handled by the
  fingerprint on PR open.

### Dedup (3.6)

Per-PR post-pass in the writer, after all comments are classified, before
commit:

- **Participants:** comments with `author_type = 'bot'` and a non-NULL
  `file_path`. Display line = `COALESCE(anchored_line, line_number)`;
  comments with no line at all don't participate. Human comments never group.
- **Candidate pairs:** same `file_path`, display lines within ±3.
- **Similarity:** trigram Jaccard over normalized bodies (lowercased,
  whitespace-collapsed, markdown/code fences stripped), threshold > 0.7.
- **Grouping:** matching pairs form an undirected graph; each connected
  component of size ≥ 2 becomes a group with
  `dedup_group_id = sha256(sorted member comment IDs)` — so re-running dedup
  on the same comments is idempotent.
- Group severity = max of members (severity rank as defined in Renderer).
- Pure Go, no dependencies.

### Classification mode (3.8)

One setting, four values:

| Value | Behavior |
|---|---|
| `rules` (default) | Deterministic tier only |
| `rules+llm` | LLM classifies only what the deterministic tier left `unclassified` |
| `llm-all` | Every comment goes to the LLM; rules result kept until the LLM answer lands |
| `server` | **Hidden stub** — enum value + switch arm exist; not selectable in the UI. Reserved for the Team plan (classification fetched from Bistro servers) |

BYOK config: provider (Anthropic / OpenAI / OpenRouter / Ollama / custom
OpenAI-compatible endpoint), model, API key. The key is stored in the OS
keychain via the existing `internal/keychain` package (`bistro.byok.<provider>`),
delivered to Go over a WS message, never persisted in settings JSON or SQLite,
never echoed back to the renderer. Mode/provider changes apply live via a
settings-changed WS message — no restart.

Cache: `body_hash` lookup before any call — the same comment text is never
classified twice, across PRs and sessions. Cache hits apply regardless of the
currently configured provider/model (the answer is the point, not who gave
it); `classification_source` is written as `byok:<provider>:<model>` from the
**cache row**, so provenance always reflects the model that actually judged
the text. Token usage is accumulated per session and pushed via a
`BYOK_USAGE` event for the Settings counter ("This session: 14,230 tokens,
est. $0.18").

### Settings & wire contract

Non-secret settings persist in the existing Electron settings store under
`settings.classification = { mode, dismissalScope, provider, model,
endpoint }`. Electron pushes them to Go via a new
`SET_CLASSIFICATION_SETTINGS` WS message — once on engine connection
(bootstrap) and on every change; Go acks with
`CLASSIFICATION_SETTINGS_UPDATED`. The API key never enters settings JSON:
`SET_BYOK_KEY { provider, key }` / `DELETE_BYOK_KEY { provider }` store and
remove it via `internal/keychain` (`bistro.byok.<provider>`), and the key is
never echoed back to the renderer (Settings shows presence only). Go emits
`BYOK_AUTH_FAILED` on auth errors and `BYOK_USAGE` after each LLM batch.

## Dismissals (3.5)

**Dismissal scope setting** (per account): `local` (default) or
`local+provider`.

Go owns all SQLite writes, so dismissal is a WS command pair, not a renderer
write:

- `DISMISS_COMMENTS { pr_id, comment_ids[] }` → Go resolves the current
  account (existing `current_user` table), inserts the `comment_dismissals`
  rows in one tx, and returns `DISMISS_RESULT { batch_id, dismissed_ids[],
  resolved_thread_ids[] }`. When scope is `local+provider`, Go additionally
  enqueues one `ResolveThread` action per **resolvable thread whose stored
  state is known and unresolved** (`review_threads.state_known=1, resolved=0`)
  among the dismissed comments' threads; those thread IDs are what
  `resolved_thread_ids` reports. Issue comments and unresolvable/already-
  resolved threads silently stay local-only.
- `UNDISMISS_COMMENTS { comment_ids[], unresolve_thread_ids[] }` → deletes the
  rows and enqueues `UnresolveThread` for exactly the listed threads.
- After either command, the existing `COMMENTS_READY` event re-fires for the
  PR; provider write-back rides the offline action queue (retry for free).

Undo (`u`): the renderer keeps a session-scoped stack of `DISMISS_RESULT`
payloads; one `u` sends `UNDISMISS_COMMENTS` with the top entry's
`dismissed_ids` + `resolved_thread_ids` — so only threads this batch actually
resolved get unresolved, never threads that were already resolved beforehand.
The stack does not survive app restart; the dismissals (SQLite rows) do.

## Renderer surface

### Sidebar counter (`StatusSidebar.tsx`)

Per-open-PR aggregate near the CI block:

```
Comments
  Human 4 · Critical 1 · Warning 3 · Nit 12
  CodeRabbit 23 · Greptile 8 · Other 2
```

`useMemo` over comments already in the store. No new IPC, no aggregation
endpoint.

### PR overview (`PRDetailPage.tsx`)

- Sort: severity rank DESC (critical → info, unclassified last), then
  `created_at` ASC.
- `nit` / `info` auto-collapse to header-only; click to expand.
- 2px left gutter stripe colored by severity — `data-severity` attribute +
  CSS map, no JS.
- Dismissed comments: collapsed under a "Dismissed (N)" section at the bottom.

### Diff view (`CommentThread.tsx`)

- Severity marker + vendor tag on each thread header.
- A dedup group renders once, as a merged finding anchored at its first
  member: "Flagged by CodeRabbit + Greptile (2 sources)", each source
  expandable inside. Other members do not render separately.
- "Why" tooltip on the severity badge: humanized `classification_source` —
  "Rule: coderabbit.yml / emoji_severity" or "AI: anthropic (claude-haiku-4-5)".

### Keystrokes (pure config in `keybindings.ts`)

| Keys | Action |
|---|---|
| `d n` | Dismiss all nits (+info) in this PR |
| `d b` | Dismiss all comments from one bot (vendor picker if >1 present) |
| `f c` / `f w` / `f s` / `f n` | Filter comments to ≥ critical / warning / suggestion / nit |
| `f h` | Human comments only |
| `f f` | Clear comment filters |
| `u` | Undo last dismissal batch |

`d` and `f` are sequence prefixes (existing pattern: `g u`). Implementation
must verify no conflict with the existing `u` binding (`keybindings.ts:963`)
and the `d`/`f` single-key namespace on the affected routes.

### Settings (`SettingsPage.tsx`)

"Comment Classification" section:

- Mode radio: Rules engine / Rules + AI fallback / AI for everything
  (`server` exists in the type, renders no option).
- Dismissal scope: Local only / Also resolve on GitHub.
- When an AI mode is active: provider dropdown, model field, API-key input,
  session token/cost counter (fed by `BYOK_USAGE`).

## Error handling

Go:

- **Malformed YAML / bad regex** → caught at load (regexes compiled once);
  file skipped with a clear log line; remaining files load. One bad rule never
  kills classification or sync.
- **Rules dir unwritable** (seed fails) → fall back to embedded built-ins in
  memory; classification still runs.
- **Engine error on a comment** → persist `unclassified`, continue.
  Classification is enrichment, never a gate.
- **Backtracking** → non-issue: Go `regexp` is RE2. Do not introduce a
  backtracking engine later.
- **LLM failure** (timeout ~30s/comment, network, 5xx) → keep the
  deterministic result, log, no user-facing error.
- **LLM 429** → abandon the batch this pass; next sync retries naturally.
- **Invalid API key** → one `BYOK_AUTH_FAILED` event (Settings badges the key
  field); tier disables itself until the key changes.
- **Shutdown** → per-PR context cancellation stops in-flight LLM work cleanly.
- **Write-back failure** → the local dismissal stands; the resolve action
  retries via the existing queue.

TypeScript: neverthrow `Result` for fallible paths (settings save, key
submission) per repo convention. Filter/sort/collapse/count logic is pure over
in-memory data.

## Testing

Table-driven, `go test -race`, per repo standard:

- **`classify`**: parsing (valid / malformed / bad regex); resolution
  (alphabetical override, direct-beats-weighted, weight tally); bot ID
  (vendor lists, `[bot]` fallback, human); fingerprint staleness (incl. mode
  string); seed lifecycle (fresh dir, deleted built-in stays deleted, user
  file untouched, upgrade overwrites present built-in).
- **Corpus**: `testdata/` fixture set (~50 real bot/human comments from
  public OSS PRs) asserting the ≥70% heuristic-accuracy bar — regressions
  fail CI.
- **Writer integration**: synced comments come out classified; dedup groups
  form and are idempotent; engine error → comment persists unclassified.
- **LLM tier** (mock provider): cache hit skips the call; fan-out bounded;
  failure degrades; cancellation clean; cache round-trip.
- **Benchmark**: 50-comment PR through built-ins; sanity bar well under
  50ms.
- **Renderer (Vitest)**: severity sort/collapse; counter aggregation; merged
  dedup rendering; dismissal + undo stack; filter keystrokes; settings mode
  switching.
- **Reload acceptance**: edit a YAML → reopen PR → classifications update,
  no restart.

## Acceptance

1. Every synced comment carries `author_type`; known-vendor bot comments
   carry `bot_vendor`; classification adds <50ms/PR to sync.
2. Rules ship for CodeRabbit + Greptile + fallback heuristics; user YAML in
   `~/.config/bistro/rules/` overrides built-ins alphabetically; edits apply
   on next PR open without restart; deleting a built-in permanently disables
   that vendor; upgrades never touch user files.
3. Heuristics ≥70% severity accuracy on the checked-in corpus.
4. Overview opens most-severe-first, nits collapsed; sidebar counts correct;
   severity tooltip names the rule (or model) that fired.
5. `d n` / `d b` bulk-dismiss; `u` reverses the batch; dismissals are
   per-account and survive restart; with `local+provider` enabled,
   resolvable threads also resolve on GitHub via the action queue.
6. Duplicate bot comments (same file, ±3 lines, similarity >0.7) render as
   one merged finding with per-source expansion; group severity = max.
7. BYOK: three visible modes + hidden `server` stub; key in OS keychain
   only; same-text-never-reclassified cache; session token/cost counter;
   LLM outage degrades silently to rules.
8. Roadmap updated: 3.7 deferred to Team plan.

## Non-goals

- 3.7 slop detection (Team plan).
- `server` classification mode behavior (stub only; Team plan).
- Confidence scores on rules (deferred until conflicts are observed).
- Retroactive whole-DB re-classification on mode change (lazy per PR open).
- LLM-assisted dedup (deterministic trigram only in v1).
- Cross-file "jump to next critical" navigation.
- Hot-reload daemon / fsnotify for rule files.
