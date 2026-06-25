# Sync performance investigation

Date: 2026-06-25

## Baseline reported by user

- Add one watched repo → sync → display PRs in Electron: **45.78 s** on macOS.
- The most likely hot path is local git clone/fetch/diff during detail sync, not SQLite or React rendering.

## Current sync flow

1. Electron sends `TOGGLE_WATCH_REPO`.
2. `server.App` triggers sync immediately.
3. `Orchestrator.Sync` runs:
   - `provider.ListPRs` once for the repo.
   - `FetchPRDetails` with `DefaultWorkers = 4`.
   - Per PR: comments, reviews, CI, and optionally `git.Clone` + `git.GenerateDiff`.
   - `Writer.WriteSyncResultsContext` persists PRs, patchsets, diffs, comments, and threads in one SQLite transaction.
4. Events are emitted over WebSocket and Electron invalidates PR queries.

## Benchmarks added

### Read-only live benchmarks

```bash
cd go
go test -run='^$' -bench='Benchmark(CloneCacheHitT3Code|CloneColdMissT3Code|GenerateDiffT3CodeWarmClone|CloneAndGenerateDiffT3Code|FetchPRDetailsT3CodeXXLPRLive|WriteSyncResultsT3CodeXXLPRLive|TypicalSyncT3CodeXXLPRLive)$' -benchtime=1x -count=1 ./internal/git ./internal/sync
```

Fixtures:

- Read-only repo: `https://github.com/pingdotgg/t3code`
- Base SHA: `a3422a9bb51d73724b9b665ae0ef1fb756f753d1`
- Head SHA: `c2776c233691ad1b6b9b891a500d9eac6c6586a2`
- PR fixture: `https://github.com/pingdotgg/t3code/pull/3548` (`size:XXL`)

Safety rule: benchmarks must never create PRs, comments, replies, approvals, request-changes reviews, or any other mutation against `pingdotgg/t3code`. Future mutation tests must use only the internal fixture repo `https://github.com/Thourum/testing`, and only README changes/comments.

Coverage:

- `go/internal/git/performance_test.go`
  - `BenchmarkCloneCacheHitT3Code`
  - `BenchmarkCloneColdMissT3Code`
  - `BenchmarkGenerateDiffT3CodeWarmClone`
  - `BenchmarkCloneAndGenerateDiffT3Code`
- `go/internal/sync/performance_test.go`
  - `BenchmarkFetchPRDetailsT3CodeXXLPRLive`
  - `BenchmarkWriteSyncResultsT3CodeXXLPRLive`
  - `BenchmarkTypicalSyncT3CodeXXLPRLive`

## Baseline benchmark results

Machine: Apple M3 Max, darwin/arm64.

```text
BenchmarkCloneCacheHitT3Code                    0.065 ms/op
BenchmarkCloneColdMissT3Code               50,120.527 ms/op
BenchmarkGenerateDiffT3CodeWarmClone        2,475.315 ms/op
BenchmarkCloneAndGenerateDiffT3Code        54,367.998 ms/op

BenchmarkFetchPRDetailsT3CodeXXLPRLive     52,051.512 ms/op
BenchmarkWriteSyncResultsT3CodeXXLPRLive        1.648 ms/op
BenchmarkTypicalSyncT3CodeXXLPRLive        53,860.158 ms/op
```

Interpretation:

- The live cold clone alone takes **~50.1 s**.
- The full current clone+diff path takes **~54.4 s**.
- The PR detail path takes **~52.1 s** and the full one-PR sync takes **~53.9 s**.
- Writing the fetched real PR result to SQLite takes only **~1.6 ms**.
- Current evidence confirms cold clone/fetch on macOS APFS is the dominant bottleneck.

## Research findings

- `NullVoxPopuli/disk-perf-git-and-pnpm` shows macOS APFS git clone times in the same range as Bistro's reported baseline: roughly 29–71 s on Apple Silicon APFS vs much faster Linux ext4/tmpfs results.
- `diffshub` avoids local git for initial display. It streams GitHub's `.diff`/`.patch` response and renders incrementally, which explains sub-second first display.
- go-git supports in-memory storage via `github.com/go-git/go-git/v5/storage/memory`, so a diff-only clone/fetch path does not need APFS writes.
- Bistro currently does a full `PlainCloneContext` with checkout, then fetches all branch heads and all PR heads. `GenerateDiff` only needs commit/tree/blob objects and does not need a checked-out worktree.

## Optimization options

| Option | Expected impact | Risk | Notes |
| --- | ---: | --- | --- |
| GitHub raw diff fast path | First diff/PR display potentially <1 s | Low | Use GitHub raw diff for initial UI; run local clone later for patchset/anchor semantics. |
| Bare clone (`isBare=true`) | 20–40% fewer APFS writes | Low | `GenerateDiff` opens commits/trees only; no worktree required. |
| Narrow refspec | 2–20× less fetch work on branch-heavy repos | Low–Medium | Current code fetches all branches and all PR heads. Prefer PR-specific refs when clone receives PR number. |
| In-memory go-git storage | Removes APFS clone writes | Medium | Needs a new diff path using `memory.NewStorage`; watch RAM use for large repos. |
| Configurable git cache dir | 2–5× possible on macOS, larger with RAM disk | Low | Add `BISTRO_GIT_CACHE_DIR`; enables `/tmp` or RAM disk experiments. |
| SQLite pragma tuning | 5–15% write improvement | Low | Helps but not likely main bottleneck. |
| More detailed timing spans | Diagnostic | Low | Add phase spans around list, per-PR clone, diff, DB write, and WS emit. |

## Recommended implementation order

Decision status after review:

1. **Approved — end-to-end timing spans.** Add structured timing so app logs expose list/comments/reviews/CI/clone/diff/DB/UI durations in ordinary runs.
2. **Needs caution — GitHub raw diff fast path.** This is useful for fast first display, but must be opportunistic only: never the sole path, never authoritative for durable sync, and always fall back to local git when unavailable, truncated, rate-limited, or provider behavior differs.
3. **Approved — bare/no-checkout local clone.** Current diff generation only needs objects/trees, not a worktree checkout.
4. **Approved when PRs are known — PR-specific narrow fetch.** Bistro already gets the PR list before detail sync, so after `ListPRs` we know PR numbers and can pass them into git fetch/clone to avoid fetching all PR refs and all branch heads.
5. **Preferred major optimization — in-memory go-git prototype.** This avoids APFS writes while preserving local-git semantics instead of relying on provider-specific raw diff APIs. On macOS this should become the primary optimization experiment; non-Apple platforms may keep the current disk path if benchmarks show it is acceptable.
6. **Denied — do not repurpose `BISTRO_REPO_DIR`.** It exists for users who want Bistro to use their own local dev/git directory. Do not change its semantics into a cache-dir/RAM-disk recommendation.

Revised implementation order:

1. Add timing instrumentation.
2. Change local clone to bare/no-checkout.
3. Pass PR number through sync → git clone/fetch and narrow fetch refspecs.
4. Prototype in-memory go-git clone/diff on macOS and compare against the live `t3code` benchmarks.
5. Consider GitHub raw diff only as a best-effort fast-display layer with strict fallback to local git.
