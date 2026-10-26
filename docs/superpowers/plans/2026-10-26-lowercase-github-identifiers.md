# Lowercase GitHub identifiers

GitHub (and GitLab) owner/repo names are case-insensitive, so the engine stores
them in one case: lowercase. Previously repo rows and PR IDs kept GitHub's
casing on some paths and lowercase on others, producing duplicate repo rows
(`github:Thourum/testing` vs `github:thourum/testing`) and broken lookups.

Gitea is case-sensitive; there is no Gitea provider yet. The lowercase rule
lives at the GitHub provider boundary plus `git.Identity.ID()`, so a future
Gitea provider must opt out there.

## Already done on this branch (uncommitted)

- `go/internal/sync/writer_rows.go`, `fetcher.go`: `prs.repo_id` / expertise
  lookup use `git.Identity{...}.ID()`.
- `go/internal/store/migrations/00036_fold_repo_id_case.sql` + test
  `migration_036_test.go`: folds mixed-case `repos` rows, lowercases
  `prs.repo_id` and expertise `repo_id`s. Migration 036 is NOT shipped, so
  extend it in place (rename allowed) instead of adding 037.
- `electron/src/renderer/utils/expertise.ts` `repoIdForPR` lowercases.

## Tasks

1. **Provider boundary (Go).** In `go/internal/provider/github/`, lowercase
   owner and repo wherever domain values are minted: `formatPRID`,
   `normalizePR` (`Owner`, `Repo`, `ID`), `repos.go` (`ListUserRepos` Owner /
   Name / FullName), `review_queue.go` (`provider.Repo`). GitHub API calls
   accept lowercase, so `parsePRID` round-trips are fine. Then any remaining
   `repos.owner`/`repos.repo` writers (`refreshRepoList`, review-queue handler,
   `repolocation_clone.go`, `UpsertLocal`) store lowercase. Grep for every other
   place owner/repo strings enter the DB or events and confirm they come from
   these minted values.
2. **Migration 036 (extend).** Additionally:
   - lowercase `repos.owner`, `repos.repo` (drop the "keep GitHub casing" rule
     and its comment);
   - lowercase `prs.id`, `prs.owner`, `prs.repo`;
   - lowercase every `pr_id` column: comments, files, action_queue, pr_diffs,
     review_threads, patchsets, patchset_files, last_reviewed_sha,
     files_viewed, pr_read_state (verify the list against migrations);
   - lowercase `pr_id` inside `action_queue.payload` JSON (`json_set`), and
     any `sync_cursors.scope` that embeds owner/repo (check its format first);
   - FKs are enforced (`_foreign_keys=on`, no ON UPDATE CASCADE): use
     `PRAGMA defer_foreign_keys = ON;` at the top (goose runs the file in a tx).
   - Collisions: for each table, `UPDATE OR IGNORE ... SET x = lower(x)` then
     `DELETE` rows still mixed-case (lowercase row wins). Order parents before
     children so ON DELETE CASCADE from deleted duplicate `prs` rows only hits
     their own children.
3. **Migration test.** Extend `migration_036_test.go`: seed a mixed-case PR
   with a comment, patchset + patchset_files, read state, an action_queue row
   with a payload, and a colliding lowercase duplicate PR; assert everything is
   lowercase, children follow, FK check passes (`PRAGMA foreign_key_check`
   returns no rows), duplicate collapsed.
4. **Tests that assert casing.** Update Go tests expecting GitHub casing in PR
   IDs / owner / repo; add one focused test that `normalizePR`/`formatPRID`
   lowercase (no per-function suites).
5. **Electron audit.** Find any renderer code comparing PR or repo
   owner/repo/ID against strings with other casing (routes, deep links,
   events, `CacheUsageCard` `repoIdentity`, onboarding queue `repo_id`s,
   viewer comparisons). Fix only real mismatches. No React component tests.
6. **Docs.** Update `CLAUDE.md` Conventions: repo IDs, PR IDs and stored
   owner/repo are lowercase (GitHub is case-insensitive; Gitea providers would
   need to opt out at the provider boundary / `git.Identity.ID()`).

## Verify

```bash
cd go && go vet ./... && go test -race ./...
cd electron && pnpm type-check && pnpm lint && pnpm test
```
