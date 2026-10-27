# Duplicate migration versions after a branch merge

## Symptom

Startup fails with `create goose provider: found duplicate migration version 36`.
Renumbering alone can hide the failure while leaving mixed-case repository IDs
and unreachable review drafts in an existing feature-branch database.

## Cause

Independent branches used version 36 for different changes: upstream repository
identity folding and feature review drafts. Goose records numeric versions, not
file names. An existing feature database therefore reports upstream 36 applied
without having run it. Table-based schema reconciliation cannot prove a data-only
normalization ran, including after Goose metadata is lost.

## Resolution

Keep upstream `00036_fold_repo_id_case.sql`; review drafts live in
`00037_add_review_drafts.sql`. The runner reconciles only through schema version
35. When legacy drafts exist without applied version 37 or a newer version, it
atomically removes only ambiguous version-36 tracking rows before Goose runs.
Versions 36 and 37 can then replay safely. Migration 37 canonicalizes only GitHub
draft PR IDs; it preserves draft IDs, account logins, user text, paths, positions,
commit SHAs and timestamps, even when the PR cache was evicted.

## Tested histories and recovery

`go/internal/store/review_drafts_test.go` covers upstream-36 upgrades,
feature-36 upgrades, lost metadata, repeated startup, distinct account/canonical
drafts, cache-evicted drafts, non-GitHub IDs and untouched newer tracking rows.
Injected failures cover both the repository-fold transaction and draft migration
37 after 36 commits. Goose rolls back the failing migration; retry replays the
idempotent fold and finishes 37 without losing user text. Metadata repair is one
atomic statement, not a transaction spanning both migrations.

Verify with the authoritative commands in [../CLAUDE.md](../CLAUDE.md).
