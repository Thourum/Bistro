# BIS-11 — GitHub sync and local data model validation

## Disposition

Current Bistro has a viable repo-scoped sync/data-model path, but the next implementation work should close three bounded gaps before treating GitHub sync as safe for broad local-first use: first-run auth, synced-vs-local state account scoping, and idempotent replay of GitHub write actions.

## Current supported path

- Auth: OAuth URL/callback helpers live in `go/internal/auth/*`; tokens are stored in OS keychain via `go/internal/keychain/keychain.go`; `github.NewClient(token)` wires REST + GraphQL clients with redacted auth logging.
- Sync orchestration: `go/internal/sync/orchestrator.go` requires an explicit `{provider, owner, repo}` sync request, fetches PRs via `provider.ListPRs`, fetches per-PR details through `FetchPRDetails`, and writes through `Writer.WriteSyncResultsContext`.
- GitHub provider coverage: current provider methods cover repo PR listing/get, issue comments, review submissions, review comments/threads, CI status, and review actions.
- SQLite persistence: migrations define GitHub-synced/cache tables for `prs`, `comments`, `files`, `repos`, `review_threads`, `patchsets`, `diffs`, `current_user`, and `rate_limit_state`.
- Local-only state is intentionally separate in `last_reviewed_sha`, `files_viewed`, `action_queue`, local `diffs`, and locally derived `patchsets`.
- Write safety: sync writes run in a transaction with per-PR savepoints for critical PR/patchset/diff writes; comments and review threads are best-effort so one bad comment does not poison the whole PR sync.

## Missing pieces / risks

1. First-run auth is not fully closed: `Client.Authenticate()` is still a stub, so the app depends on the callback flow or an existing keychain token.
2. Sync is repo-scoped, not inbox-discovery-scoped. The BIS-4 target (`involves:USERNAME` discovery + hydration) is not yet the primary path.
3. No durable sync cursor/ETag model is visible beyond rate-limit state; incremental sync still needs endpoint-level cursors/conditional request storage if we want efficient large inbox sync.
4. Local-only tables are scoped mostly by `pr_id` and `login`, without a durable provider/account namespace; sign-out/re-auth or future multi-account/GHE support can leave stale local state mixed with a new account.
5. `action_queue` deduplicates local enqueue IDs, but replay after a GitHub-side success followed by local crash can still duplicate non-idempotent comments/reviews unless replay has a GitHub-side idempotency marker/reconciliation check.
6. Migration rollback/backfill risks: later migrations rebuild/extend `patchsets`, `last_reviewed_sha`, and comment anchoring columns. New account namespace/cursor migrations should use additive columns + backfills and preserve existing local-only review state.

## Follow-up implementation issues to create

1. Close first-run GitHub auth path and unauthenticated startup reporting.
2. Add account/provider namespace and sign-out/re-auth handling for local-only state.
3. Add durable sync cursors/ETags and inbox discovery hydration path.
4. Harden offline action replay idempotency for comments/reviews/thread actions.

## Verification run

Targeted Go tests should cover auth, provider normalization, sync writer/orchestrator, action queue, and local-only state stores.
