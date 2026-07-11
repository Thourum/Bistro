# Repo Location, Dev Folder Scan, and Cache Management Design

Date: 2026-07-11

## Goal

Bistro should reuse repositories users already have in their developer projects folder, instead of always cloning into the default managed cache path. This makes PR fetches, diffs, and syncs faster and more intuitive while preserving the safety guarantee that Bistro never touches a user's checked-out branch or working tree.

The feature also supports adding a repository by URL, choosing whether to clone it into the configured dev folder or Bistro's managed cache, and managing cache storage from Settings.

## Non-goals

- No filesystem watcher. Bistro scans the configured dev folder at app startup and on explicit troubleshooting actions only.
- No checkout into the user's current branch or worktree.
- No updates to user branches, tags, or working tree files.
- No repository search UI for the command bar; add-by-URL is sufficient.
- No deletion of local/dev-folder repositories from cache actions.
- No GitHub Enterprise or non-GitHub provider support in v1 unless existing provider support is already present.

## Existing context

- Go owns SQLite writes.
- Existing `repos` table was added in `go/internal/store/migrations/008_add_repos.sql` and is linked from `prs.repo_id`.
- Current managed clone path is resolved in `go/internal/git/paths.go`: `BISTRO_REPO_DIR` first, otherwise `os.UserCacheDir()/bistro/repos`.
- Git diffs are computed from Git objects, so Bistro can reuse local repository objects without checking out branches.
- Electron already persists app settings in `userData/settings.json`; this remains the authoritative settings file for user-facing preferences.

## Settings ownership and API contract

The configured dev folder is an Electron app setting, persisted in the existing settings service as a new `repositories.devFolderPath` field. Electron is the authoritative owner of this preference because onboarding and Settings already write user preferences there.

Go remains the authoritative owner of repository location state in SQLite. Electron sends the configured path to Go; Go validates it, scans it, and writes discovered repo rows.

Required contracts:

- On engine ready, Electron sends `repo.scanDevFolder` with `{ path, reason: "startup" }` when `repositories.devFolderPath` is set.
- On Settings change, Electron first sends `repo.validateDevFolder` with the candidate path.
- If validation succeeds, Electron persists the new path and sends `repo.scanDevFolder` with `{ path, reason: "settings-change" }`.
- If validation fails, Electron leaves the previous persisted path unchanged and shows the validation error.
- A Settings troubleshooting action may send `{ path, reason: "manual" }` for the currently persisted path.
- Go returns scan progress and completion events with discovered count, healthy count, degraded count, skipped count, partial flag, and optional error.

Path validation:

- Path must exist, be a directory, and be readable.
- Path is canonicalized before use.
- Filesystem root, home directory, and common system directories are rejected with a clear error; the user must choose a child projects folder.

## Data model

Add a new migration after `020_add_comment_diff_hunk.sql` to extend the existing `repos` table:

- `local_path TEXT` — absolute path to the repository when Bistro knows one.
- `source TEXT` — `local`, `cache`, or `NULL` when not present on disk.
- `last_used_at DATETIME` — updated when the repo is used for clone, fetch, diff, or PR sync.
- `location_status TEXT` — `ok`, `missing`, `remote_mismatch`, `permission_denied`, or `NULL`.
- `location_error_code TEXT` — machine-readable latest location error.
- `location_error TEXT` — human-readable latest location error for UI display/log context.

`size_bytes` is not persisted. Cache size is computed on demand so it cannot go stale.

Repo identity and uniqueness:

- The existing `repos.id` primary key (`provider:owner/repo`) is the identity. Providers enforce owner/repo uniqueness, so no new constraint is needed.
- Scan and add-by-URL both upsert by this identity: if a row exists, update its location columns; never create a duplicate row. `prs.repo_id` continues to point at the same row.
- If the same owner/repo exists on two providers, they are distinct identities (different `provider:` prefix) and that is accepted as-is.

## Remote identity normalization

Remote parsing must normalize repository identity consistently for scans, add-by-URL, and validation.

Accepted v1 GitHub forms:

- `https://github.com/owner/repo`
- `https://github.com/owner/repo.git`
- `git@github.com:owner/repo.git`
- `ssh://git@github.com/owner/repo.git`

Rules:

- Lowercase provider/host and identity keys for matching.
- Trim trailing `.git`.
- Reject URLs without exactly owner and repo path parts.
- Prefer `origin` during scans. If `origin` is absent, use the first supported GitHub remote.
- For validation against an expected repo, accept any remote that matches the expected identity.
- Store/display canonical `provider:owner/repo`; display may use provider API casing later.
- Fork/upstream ambiguity is resolved by matching the expected identity, not by guessing intent.

## Local repository safety model

Dev-folder reuse is opt-in during onboarding or Settings. The UI must disclose that Bistro may fetch missing PR objects into the local repository's Git object database and create/update only Bistro-owned refs under `refs/bistro/*`.

Choosing a dev folder also grants consent for Bistro to register discovered GitHub repos in that folder and include healthy ones in normal PR syncing. The onboarding and Settings copy must say this clearly.

Bistro must never:

- checkout commits or branches;
- update `HEAD`;
- update user branches, tags, or remote-tracking refs;
- edit, delete, or stage working tree files;
- run destructive git operations against local repos.

Fetches into local repos must use explicit refspecs that write only to `refs/bistro/<safe-repo-id>/*`, with tag following disabled. No wildcard refspec may write to `refs/heads/*`, `refs/remotes/*`, or `refs/tags/*`. `<safe-repo-id>` is a short hash (e.g. first 16 hex chars of SHA-256) of the canonical `provider:owner/repo` identity, which is deterministic, path-safe, and cannot escape the ref namespace regardless of repo naming.

If the user does not accept this access model, they can skip dev-folder reuse or move individual repos to Bistro's managed cache.

## Repo path resolution

When the Go engine needs repository objects for a PR:

1. Resolve the repo row by provider/owner/repo.
2. Acquire a per-repo operation lock before fetch, diff, recovery, prune, or delete.
3. If `source='local'` and `local_path` exists:
   - Open the repository with go-git.
   - Validate its remote still matches the expected repo.
   - Fetch required PR/base/head commits into Bistro-owned refs under `refs/bistro/*`.
   - Never checkout, never update user branches, and never modify the worktree.
   - Update `last_used_at` and clear location errors.
4. If the local path is missing, inaccessible, or has a remote mismatch:
   - Keep the repo row and PR history.
   - Set `location_status`, `location_error_code`, and `location_error`.
   - Return a structured location error to the UI.
   - Do not silently fall back to cache. The user must choose a recovery action.
5. If no local repo is configured for the repo, use today's managed cache clone path.
6. Cache clones update the repo row with `source='cache'`, `local_path=<cache path>`, and `last_used_at=now`.

The existing `BISTRO_REPO_DIR` behavior remains the cache-root override.

## Onboarding and startup scan

Onboarding adds an optional dev/projects folder step. Users may choose a folder or skip and keep cache-only behavior.

On app startup, Go scans the configured dev folder in the background:

- Scan immediate children plus one nested level for `.git` directories.
- Maximum default scan budget: 500 candidate directories or 30 seconds, whichever comes first.
- If the scan hits a limit, return `partial=true` with skipped count and show a non-blocking Settings notice. Partial results are still usable, but add-by-URL must run a targeted check before cloning.
- Canonicalize paths before storing or comparing them.
- Do not follow symlinked directories by default.
- Ignore bare repos and submodules in v1 unless they are normal worktrees with a readable remote.
- Read supported remotes and normalize them to repo identities such as `github:owner/repo`.
- Register new discoveries as `source='local'`, `local_path=<repo path>`, `location_status='ok'`.
- Compare existing local repo rows with the filesystem:
  - path still exists and remote matches: mark healthy;
  - path missing: mark `missing`;
  - remote no longer matches: mark `remote_mismatch`;
  - permissions fail: mark `permission_denied`.
- Healthy discovered repos are included in normal PR syncing because choosing the dev folder is explicit consent.

No watcher is used. If a user adds or moves repos while Bistro is open, the next startup scan reconciles the state. Settings may include a "Scan now" troubleshooting action, but it is not the primary flow.

Changing the dev folder needs no special reconciliation: local repo rows whose paths no longer exist are marked `missing` by the normal health checks, gray out in the UI, and the user recovers them via the standard broken-repo recovery actions (new path, move to cache, or remove).

## Command-bar add repository

The command bar includes "Add repository by URL".

Flow:

1. User pastes a GitHub repository URL.
2. Go normalizes the URL into provider/owner/repo.
3. Before cloning, Bistro checks scan results and performs a targeted dev-folder check for `<dev-folder>/<owner>/<repo>` plus any known scan matches. This avoids duplicating an existing repo after a partial or stale startup scan. Repos nested deeper than the scan depth may be missed and re-cloned to the dev-folder root; this is accepted v1 behavior.
4. If found, register or update the repo as `source='local'`.
5. If not found, prompt for destination:
   - **Clone into dev folder** — default when a dev folder is configured.
   - **Use Bistro cache** — default when no dev folder is configured; managed and prunable.
6. Derive the dev-folder destination as `<dev-folder>/<owner>/<repo>` by default to avoid same-name collisions across owners.
7. If the destination exists:
   - if it is the matching repo, register it as local;
   - if it is non-empty or a different remote, show a conflict prompt and do not overwrite.
8. Validate the remote after clone.
9. Register the repo and start PR sync.

Clone cancellation, private-repo auth failures, permission errors, and URL validation failures return structured errors.

Partial clone cleanup:

- Cache destination: remove the partial cache directory if Bistro created it and it remains inside the canonical cache root.
- Dev-folder destination: remove only directories Bistro created for this operation and only if they contain no user-created files beyond the partial clone. Otherwise leave the directory and show cleanup guidance.

Local clones are user-owned. Cache clones are Bistro-managed.

## Broken repo recovery

When a local repo is missing, renamed, removed, inaccessible, or remote-mismatched:

- The repo and PR history remain visible but degraded.
- UI grays out affected repo/PR entries.
- Sync, fetch, and diff operations show a clear message such as: `Repo owner/name could not sync: folder not found`.
- The repo detail/recovery UI offers:
  1. **Choose new path** — user selects a repo folder; Bistro validates the `.git` remote matches before updating `local_path`.
  2. **Move to Bistro cache** — after a warning, Bistro clones/fetches into the cache, sets `source='cache'`, and clears the location error only once the cache fetch succeeds. Queued actions stay linked to the repo/PRs throughout.
  3. **Remove repo and PR history** — after strong confirmation, delete repo registration and associated data in one transaction.

Destructive remove contract:

- Before deletion, show a summary count of PRs, comments, review threads, patchsets, files, diffs, viewed-file rows, last-reviewed rows, and queued actions that will be removed.
- Delete in a single transaction.
- Delete `action_queue` rows for affected `pr_id`s explicitly.
- Delete affected `prs`; existing cascades remove `comments`, `files`, `pr_diffs`, and `review_threads` where FKs are enforced.
- Explicitly delete `patchsets`, `last_reviewed_sha`, and `files_viewed` rows for affected PRs if they are not covered by existing FKs.
- If `source='cache'`, delete the managed cache folder only when its canonical path is under the cache root and matches the expected Bistro-owned directory. Never delete `source='local'` dev-folder repos.
- Delete the `repos` row last.
- Roll back DB changes on any DB error. Filesystem deletion failures are reported and leave the repo row degraded instead of pretending cleanup succeeded.

The confirmation copy must say that PR review history is lost.

## Cache settings UI

Settings includes a cache usage card for Bistro-managed repos only (`source='cache'`). It does not show or delete dev-folder repos.

The card shows:

- A horizontal progress bar with total cache size shown on the right.
- The bar has two segments:
  - blue: active cached repos used within the last 30 days;
  - gray: stale cached repos not used for more than 30 days.
- A "Prune" action labelled for repos older than 30 days.
- A secondary gray "Clear all cached repos" action with confirmation.
- An expandable accordion listing cached repos with columns:
  - repo identity (`owner/repo`, or equivalent provider namespace);
  - size;
  - last used;
  - remove action, e.g. a trash icon, for deleting one cached repo from disk.

The managed cache root is the existing default: `BISTRO_REPO_DIR` when set, otherwise `os.UserCacheDir()/bistro/repos`. Cache clones are laid out as `<cache-root>/<owner>/<repo>` (existing clones at legacy flat paths are still recognized by their registered `local_path`).

"Clear all cached repos" means all Bistro-owned directories under the canonical managed cache root, including registered cache repos and safe orphan cache folders that match Bistro's expected owner/repo directory scheme. It does not delete anything outside the cache root and does not delete dev-folder repos.

Cache prune, clear-all, and per-repo trash actions:

1. Acquire the same per-repo operation lock used by sync/fetch/diff.
2. Re-read the repo row after the lock is acquired.
3. Canonicalize/evaluate the target path immediately before deletion.
4. Refuse deletion if the path is a symlink, resolves outside the managed cache root, or does not match a Bistro-owned expected directory name.
5. Delete only managed cache folders where `source='cache'` and `local_path` is inside the managed cache root, plus safe orphan cache folders for clear-all.
6. Never follow symlinks during removal.
7. Preserve repo registrations, PRs, comments, reviews, patchsets, and queued actions for cache prune/clear/trash.
8. After successful deletion of a registered cache repo, clear `local_path`, clear cache-related location state, and set `source=NULL` so the repo can be re-cached when needed.

If deletion fails, Bistro leaves the DB row unchanged, shows a generic UI error, and logs detailed context including repo identity, path, and OS error.

## Structured errors

Backend location operations return structured errors with:

- repo identity;
- operation (`scan`, `fetch`, `diff`, `clone`, `prune`, `recover`);
- machine code (`path_missing`, `remote_mismatch`, `permission_denied`, `clone_failed`, `delete_failed`, etc.);
- human-readable message;
- optional details safe for logs.

The UI maps states from machine codes, not human text. Human text is display/log context only. The exhaustive code list and per-operation payload fields are defined during implementation planning, not in this spec.

## Testing plan

Go tests:

- Migration adds repo-location columns without breaking existing rows.
- Dev-folder setting validation rejects root, home, missing, and unreadable paths.
- Remote normalization covers HTTPS, SSH, `.git` suffixes, missing owner/repo, multiple remotes, and case handling.
- Startup scan discovers GitHub remotes in a shallow dev-folder layout and auto-registers them after dev-folder consent.
- Startup scan canonicalizes paths, skips symlinks by default, and reports partial scans at 500 candidates or 30 seconds.
- Startup scan marks missing paths, permission failures, and remote mismatches.
- Local repo path fetches required refs only into `refs/bistro/*` with tag following disabled.
- Local repo fetch tests assert `HEAD`, user branches, tags, remote-tracking refs, index, and worktree remain unchanged.
- Command-bar add URL performs targeted dev-folder checks even after partial/stale scans.
- Add URL handles owner/name destination paths, destination conflicts, missing dev folder, invalid URL, auth failure, cancellation, and partial clone cleanup.
- Recovery validates remotes before accepting a replacement path.
- Prune waits for active sync/diff locks before deleting cache filesystem data.
- Prune deletes only `source='cache'` filesystem data older than 30 days and only under the cache root.
- Clear-all deletes registered cache repos and safe orphan cache folders, refuses symlinks and paths outside the cache root, and never follows symlinks.
- Prune preserves PR history and clears cache location fields after successful deletion.
- Failed prune leaves DB rows unchanged and logs the detailed error.
- Destructive remove deletes repo-linked PR/review/action metadata transactionally after confirmation, deletes only managed cache folders for cache repos, never deletes local repos, and rolls back DB changes on DB failure.

Electron tests:

- Onboarding stores the configured dev folder and can skip it.
- Settings validates before persisting a changed dev folder.
- On engine ready, Electron sends the configured dev folder scan request.
- Settings renders startup scan status, partial-scan notices, cache progress segments, and cached repo rows.
- Degraded repos are grayed out and expose recovery actions.
- Add-by-URL destination prompt defaults to dev folder when configured, otherwise cache.

## Open implementation notes

- Keep the scan service and repo-location service separate from UI concerns.
- Keep folder scanning bounded and cancellable so startup is not blocked.
- Prefer explicit repository/location service boundaries over expanding clone logic with unrelated UI concerns.
