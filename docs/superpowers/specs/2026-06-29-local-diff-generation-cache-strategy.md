# Local Diff Generation and Cache Strategy

Issue: BIS-12
Owner: Diff Performance Engineer
Status: Defined; implementation split into follow-up issues.

## Product requirement

Bistro must show PR diffs from local state without making reviewers wait on GitHub page loads. The sync engine should therefore treat GitHub as metadata/ref input, but make clone, diff, parse, cache, invalidation, and large-PR degradation decisions locally and predictably.

## Current repo baseline

- `go/internal/sync/fetcher.go` computes diffs during `FetchPRDetails` when `includeDiff` is true. It clones `https://github.com/{owner}/{repo}.git`, generates a go-git tree patch, converts each file into `store.DiffFile`, and sets `DiffComputed=true` only on success.
- `go/internal/git/clone.go` performs a full `PlainCloneContext`, then fetches branches and PR refs. The process cache is keyed by `(repoURL, baseSHA, headSHA)` and validates the directory still exists before reuse.
- `go/internal/git/diff.go` uses `baseTree.Patch(headTree)`, renders each `FilePatch` into a unified patch string, counts additions/deletions, and labels files as `added`, `deleted`, or `modified`.
- `go/internal/sync/writer.go` persists PR row, patchset row, and `pr_diffs` under one savepoint. `DiffComputed=false` preserves last-known-good diffs; `DiffComputed=true` replaces all `pr_diffs`, including an empty set.
- `go/internal/store/diff_store.go` and writer replacement semantics already clear stale rows for deletes/renames.
- `go/internal/git/rangediff.go` has bounded, guarded system-git range-diff support with graceful degradation when unavailable.

## Target strategy

### 1. Cache layers

Use three explicit cache layers:

1. **Repo object cache**: one durable bare/mirror clone per provider/owner/repo under the existing OS cache root, not one full worktree per `(base, head)`. Fetch branches and PR refs into that mirror with per-repo locking. This avoids re-cloning large repositories for every force-push or PR.
2. **Patchset identity cache**: continue using `patchsets(pr_id, number, base_sha, head_sha)` as the invalidation boundary. Any base or head SHA change creates a new patchset. Same base/head is a no-op.
3. **Rendered diff cache**: `pr_diffs` stores the current patchset's renderable per-file patches. Keep `DiffComputed=false` semantics: failed generation must never erase the last usable diff.

Cache invalidation rules:

- New `(base_sha, head_sha)` => fetch refs, compute a new patchset, atomically replace `pr_diffs` only after diff success.
- Same `(base_sha, head_sha)` => reuse persisted `pr_diffs`; skip expensive diff generation when metadata sync has not changed the diff endpoints.
- Missing local repo directory => rebuild repo object cache; do not trust in-memory paths.
- Provider/repo URL change => new repo object cache key; do not reuse across providers.
- Cache pressure => evict least-recently-used repo mirrors by byte budget, never by deleting rows from SQLite.

### 2. Diff generation and parsing

Default path remains go-git tree diff between base and head commits. The next implementation pass should make the generated file model richer before UI/API layers rely on it:

- Persist file status explicitly (`added`, `modified`, `deleted`, `renamed`, `binary`, `too_large`).
- Persist `old_path` for renames and copies; current storage only has `file_path`, so renames appear as delete/add or modified-to-new-path depending on go-git output.
- Persist diff metadata (`is_binary`, `is_truncated`, `patch_bytes`, `hunk_count`) so the renderer can choose placeholders/virtualization without reparsing giant patch strings.
- Treat binary patches as first-class non-renderable file entries rather than failed diffs.

### 3. Large-PR behavior

Large PRs should degrade by file, not by entire PR:

- Keep a manifest row for every changed file even when patch text is omitted.
- Apply per-file and per-PR patch byte budgets before writing to SQLite.
- Store `too_large` / `truncated` metadata for files over budget and keep additions/deletions if available.
- Surface partial availability to the UI so keyboard navigation still works across the file list while large bodies load or remain summarized.

Recommended initial budgets for measurement, not hard product commitments:

- Warn log at diff generation > 500 ms or patch write > 500 ms.
- Truncate or omit individual patch bodies above 1 MiB.
- Mark PR as large above 500 files or 10 MiB total patch text.
- Keep the per-PR fetch timeout at 2 minutes until profiling proves a tighter bound.

### 4. Force-push and range-diff edge cases

Current patchset insertion handles observed base/head changes, but polling cannot reconstruct intermediate force-pushes that disappeared before sync. The strategy is:

- Preserve every observed patchset; never overwrite previous patchset identity.
- Use range-diff only as an enhancement between two observed patchsets, with the existing system-git degraded fallback.
- Do not promise recovery of unobserved intermediate heads unless a later provider timeline/commit-history feature supplies those SHAs.

### 5. Measurement gates

Implementation follow-ups should add deterministic checks for:

- cold repo cache fetch time, warm repo cache fetch time, diff generation time, diff persist time, patch bytes, file count, and truncation count;
- force-push cache miss/recompute correctness;
- missing clone directory rebuild;
- large binary file entry without PR-level failure;
- rename old/new path persistence;
- `DiffComputed=false` preserving existing diffs after generation failure.

## Follow-up implementation slices

1. Replace per-patchset full clones with a durable per-repo object cache plus locks and LRU budget.
2. Extend diff schema/model for status, old path, binary/truncated metadata, patch bytes, and hunk counts.
3. Add large-PR budget enforcement and partial diff manifest behavior.
4. Add diff profiling metrics and focused regression tests for cache invalidation, force-push, rename, binary, and failed-diff preservation.
