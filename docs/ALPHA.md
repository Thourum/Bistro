# Bistro alpha

## Scope

- macOS 12 or later, Apple Silicon (arm64) only.
- Alpha means: expect rough edges and data resets. Bistro is not yet a stable v1.0.

## Installing

Releases are drafts first; a maintainer publishes them on GitHub after the smoke test passes.

1. Download the `.dmg` (or `.zip`) from the GitHub release and verify it against `SHA256SUMS`:
   `shasum -a 256 -c SHA256SUMS`
2. Drag `Bistro.app` to `/Applications`.
3. If the build is unsigned (the release notes say so), macOS blocks the first launch. Either right-click the app, choose **Open**, and confirm, or run:
   `xattr -dr com.apple.quarantine /Applications/Bistro.app`

On first use Bistro asks the macOS keychain for access to your GitHub token. It is stored in the login keychain under the `bistro-sync` service.

## Updates

Auto-updates are off by default. To enable them for a build, set `BISTRO_ENABLE_AUTO_UPDATES=1`. Otherwise install each alpha by hand from the release page.

## Data and resets

- Data lives in `~/Library/Application Support/Bistro/`, including the SQLite cache and the `logs/` folder.
- Alpha builds may change the schema or wipe the cache without a migration path. Expect to delete `~/Library/Application Support/Bistro/` to recover from a bad build. The cache holds no data that is not on GitHub, but local settings and rules are lost.

## Logs and reporting

- Engine log: `~/Library/Application Support/Bistro/logs/bistro-sync.log`.
- App (Electron) log: `~/Library/Logs/Bistro/` (electron-log default; confirm the file name on your build).
- Report bugs at https://github.com/Itterno/Bistro/issues. Attach the relevant log excerpt and your macOS version. Remove tokens and repo names you do not want public.

## Telemetry

Usage telemetry and crash reporting are off by default. They turn on only when you opt in from Settings. Settings lists what is sent. No PR content, repo names, or usernames are sent.

## For maintainers

Build, smoke test, and publish steps are in `CLAUDE.md` under "Release". The release workflow is `.github/workflows/release.yml`; it runs on `v*` tags and creates a draft pre-release.
