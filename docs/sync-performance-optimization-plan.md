# Sync performance optimization plan

Date: 2026-06-25

## Goal

Reduce first-time PR sync latency caused by cold git clone/fetch on macOS APFS while preserving Bistro's local-first, provider-portable sync model.

Current live baseline against `pingdotgg/t3code#3548`:

- Cold clone: ~50.1s
- Clone + diff: ~54.4s
- Full one-PR sync: ~53.9s
- SQLite write for fetched result: ~1.6ms

Whole-plan target outcomes:

- Cold clone/fetch: **~50.1s → <2s**
- Clone/fetch + diff: **~54.4s → <2s**
- Full one-PR sync: **~53.9s → <2s**

These are target goals, not hard promises. Recalibrate them after the in-memory prototype:

- If in-memory clone/fetch cannot realistically reach <2s, keep clone/fetch as the limiting factor and set clone+diff / full-sync goals relative to the measured clone floor.
- If in-memory clone/fetch reaches **<500ms**, tighten the clone+diff goal to approximately **<1s**, because diff generation then becomes the slowest meaningful element.
- Always track the slowest remaining phase separately. The full-sync goal should be roughly: `slowest git path + provider API + DB write + event overhead`, with DB write expected to remain negligible.

## Guardrails

- `https://github.com/pingdotgg/t3code` is a **read-only performance fixture**. Do not create PRs, comments, review replies, approvals, request-changes reviews, or any mutation there.
- Future mutation tests must target only `https://github.com/Thourum/testing`, and only README changes/comments.
- Do not repurpose `BISTRO_REPO_DIR`; users rely on it for real local development directories such as `~/dev` or `~/git`.
- GitHub raw diff, if implemented later, is only an opportunistic display accelerator. It must never be the authoritative durable sync path.

## Phase 1 — Timing instrumentation

Status: approved.

Add structured timing spans to ordinary sync logs so every slow run explains itself without ad-hoc profiling.

### Tasks

1. Add per-phase timings in `Orchestrator.Sync`:
   - list PRs
   - fetch details
   - write DB
   - emit completion
2. Add per-PR sub-step timings in `FetchPRDetails`:
   - comments
   - reviews
   - review threads
   - CI
   - clone/fetch
   - diff generation
   - anchor resolution
3. Ensure logs include `sync_id`, repo, PR number, base/head short SHAs, counts, and durations.
4. Keep log level reasonable: high-level timings at info, noisy per-step details at debug unless a threshold is exceeded.

### Acceptance

- A normal sync log clearly shows whether time is in GitHub API, clone/fetch, diff generation, DB write, or event delivery.
- No behavior changes.
- `go test ./...` passes.

## Phase 2 — Bare/no-checkout clone

Status: approved.

Current diff generation only reads commit/tree/blob objects. It does not need a checked-out worktree, so clone should avoid worktree checkout writes.

### Tasks

1. Change current clone path from non-bare to bare/no-checkout where compatible.
2. Verify `GenerateDiff` works against the resulting repository path.
3. Update tests if they assume a worktree exists.
4. Run live t3code benchmarks before/after.

### Acceptance

- `BenchmarkCloneColdMissT3Code` improves or stays neutral.
- `BenchmarkGenerateDiffT3CodeWarmClone` still passes.
- `BenchmarkCloneAndGenerateDiffT3Code` still returns non-empty diff files.
- `go test ./...` passes.

## Phase 3 — PR-specific narrow fetch

Status: approved when PRs are known.

Bistro already calls `ListPRs` before detail sync, so PR numbers are known before clone/diff work starts. Pass PR number into git fetch so Bistro fetches only the refs required for that PR instead of all branches and all PR heads.

go-git v5.17.2 supports this with `FetchOptions.RefSpecs`, `FetchOptions.Depth`, and `NoTags`. It does **not** support partial-clone filters such as `blob:none` in v5, so the immediate no-new-dependency optimization is shallow PR-specific fetch, not true partial clone.

### Tasks

1. Extend git clone/fetch API to accept optional PR number or a small fetch target struct.
2. Pass `provider.PR.Number` from `FetchPRDetails` into the git client.
3. Replace broad fetch specs:
   - `+refs/heads/*:refs/remotes/origin/*`
   - `+refs/pull/*/head:refs/remotes/origin/pr/*`
4. Fetch only PR-specific refs where possible, e.g.:
   - `refs/pull/<number>/head`
   - `refs/pull/<number>/merge` if useful for reachability
   - `refs/heads/<baseBranch>` for the base branch tip when the PR base SHA matches or is reachable at shallow depth
5. Keep a safe fallback to broad fetch if targeted fetch cannot prove both base/head commits are available.

### Acceptance

- Both exact requested SHAs are verified present before cache insert.
- `BenchmarkCloneColdMissT3Code` improves significantly or logs a fallback reason.
- Existing sync behavior remains correct for normal PRs and force-pushed PRs.
- `go test ./...` passes.

## Phase 4 — In-memory go-git prototype

Status: preferred major optimization; prototype implemented as diff-only path, **not production eligible yet**.

Avoid APFS writes by cloning/fetching into `memory.NewStorage()` for diff-only operations, then persist the computed diff to SQLite. Disk clone can remain as fallback and/or non-macOS path if benchmarks show disk is acceptable.

Research result: go-git v5.17.2 supports in-memory storage plus shallow PR-specific fetch, but not `--filter=blob:none` / treeless partial clone at the public `CloneOptions` / `FetchOptions` API. Therefore the first prototype should combine:

- `memory.NewStorage()`
- `git.Init(...)` / `CreateRemote(...)`
- `remote.FetchContext(...)`
- `Depth: 1` initially, with fallback depth escalation if exact base/head SHAs are unavailable
- `Tags: git.NoTags`
- PR-specific refs only

True partial clone can be reconsidered when go-git v6 partial clone support is stable.

### Tasks

1. Add a new internal git path that uses go-git memory storage.
2. Refactor diff generation so it can operate from a `*git.Repository` or object reader, not only a filesystem path.
3. Implement in-memory shallow PR-specific fetch for the t3code base/head benchmark.
4. Add memory-specific benchmarks:
   - in-memory PR-specific fetch
   - in-memory PR-specific fetch + diff
   - compare against current disk clone/fetch + diff
5. Add guardrails:
   - context timeout
   - max diff size / max memory budget if practical
   - fallback to disk path on unsupported cases
6. Decide platform policy after measurement:
   - macOS: prefer memory if materially faster
   - Linux/Windows: choose based on benchmark results and memory cost

### Acceptance

- In-memory path returns the same file set and line counts as disk path for t3code fixture, within expected formatting differences if any.
- It materially improves `BenchmarkCloneAndGenerateDiffT3Code` on macOS.
- It has a safe fallback to existing disk behavior.
- `go test ./...` passes.

### Prototype result

Directional single-iteration live benchmark results on Apple M3 Max:

```text
BenchmarkCloneColdMissT3Code                    ~45.6s   broad disk clone of range fixture
BenchmarkCloneAndGenerateDiffT3Code             ~46.1s   broad disk clone + diff of range fixture
BenchmarkCloneForPRTargetedColdMissT3Code        ~5.2s   disk targeted shallow fetch of PR fixture
BenchmarkCloneForPRTargetedAndGenerateDiffT3Code ~5.2s   disk targeted shallow fetch + diff of PR fixture
BenchmarkGenerateDiffForPRInMemoryT3Code         ~4.3s   in-memory targeted fetch + diff of PR fixture
```

Conclusion: targeted fetch delivers the largest current improvement (~9× vs broad clone across the t3code live fixtures). In-memory avoids APFS writes but does not yet meet the <2s goal, implying the remaining floor is likely network transfer and go-git object/diff processing. The in-memory path remains prototype/diff-only because production anchor resolution still needs a local repository path.

Production-sync validation after these phases still measured the full t3code one-PR sync at roughly **50.5s**. That means the current anchor-safe production path is not yet meeting the whole-plan target: for the XXL PR fixture it can still fall back to the broad clone when shallow targeted fetch cannot satisfy required anchor or historical patchset SHAs. Further work should focus on either avoiding that fallback for anchoring, materializing memory/disk only when required, or introducing a separate fast-display diff path.

Prototype limitations before production use:

- no fallback/depth escalation inside `GenerateDiffForPRInMemory`;
- no explicit memory budget or diff-size budget;
- no local path for anchor resolution;
- no production integration in `FetchPRDetails`.

Therefore Phase 4 acceptance is **not complete** for production. The current value is benchmark support and a validated experimental path for future work.

## Phase 5 — Optional GitHub raw diff fast display

Status: caution / later.

Use GitHub raw diff only as a best-effort first-display accelerator. This does not replace local git sync.

### Conditions before implementation

- Local git path has timing instrumentation and at least one optimization shipped.
- UI/data model can distinguish raw-provider diff from locally-computed diff.
- Fallback behavior is explicit for downtime, rate limiting, truncation, and provider differences.

### Acceptance

- Raw diff failure never blocks durable local sync.
- Local git diff eventually replaces or validates raw-provider diff.
- Non-GitHub providers are not forced into GitHub-specific assumptions.

## Benchmark commands

Run before and after each implementation phase:

```bash
cd go
go test -run='^$' -bench='Benchmark(CloneCacheHitT3Code|CloneColdMissT3Code|GenerateDiffT3CodeWarmClone|CloneAndGenerateDiffT3Code)$' -benchtime=1x -count=1 ./internal/git
go test -run='^$' -bench='Benchmark(CloneForPRTargetedColdMissT3Code|CloneForPRTargetedAndGenerateDiffT3Code|GenerateDiffForPRInMemoryT3Code)$' -benchtime=1x -count=1 ./internal/git
go test -run='^$' -bench='Benchmark(FetchPRDetailsT3CodeXXLPRLive|WriteSyncResultsT3CodeXXLPRLive|TypicalSyncT3CodeXXLPRLive)$' -benchtime=1x -count=1 ./internal/sync
go test ./...
```

## Definition of done

The optimization work is successful when:

- Live t3code one-PR sync is materially below the current ~54s baseline, with the primary target of **<2s** unless measured in-memory limits force a documented recalibration.
- Cold clone/fetch and clone/fetch+diff are benchmarked independently so the remaining bottleneck is explicit.
- Logs clearly identify remaining bottlenecks.
- Read-only benchmark guardrails are preserved.
- SQLite remains the durable cache for fetched PR metadata and diffs.
- Provider-specific fast paths, if any, are fallback-only and do not compromise local-first behavior.

## Not pursuing now

- **go-git partial clone filters (`blob:none`, `tree:0`)**: unavailable in go-git v5.17.2 public clone/fetch options. v6 has experimental filter support, but full partial-clone behavior is not stable enough to adopt yet.
- **Native `git` CLI hot path**: supports filters, but adds an external runtime dependency and packaging variability.
- **libgit2/git2go**: CGo and distribution complexity are not justified before exhausting go-git memory + shallow PR-specific fetch.
