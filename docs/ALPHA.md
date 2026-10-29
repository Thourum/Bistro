# Bistro alpha

## Scope

Bistro is a single-process Wails v3 desktop app. Release assets target macOS
Apple Silicon (arm64), Windows amd64, and Linux amd64. Native webviews are
WKWebView, WebView2, and WebKitGTK respectively. Alpha means rough edges and
possible data resets; Bistro is not yet stable v1.0.

## Installing

Releases are drafted by CI; the maintainer publishes them after smoke checks.
Download from [GitHub releases](https://github.com/Thourum/Bistro/releases).
Use only assets from a release you trust and verify their SHA-256 hashes against
that release's `SHA256SUMS` before opening them.

### macOS

1. Download `bistro-darwin-arm64-installer.dmg` (or the app archive
   `bistro-darwin-arm64.zip`) and `SHA256SUMS`.
2. Compare `shasum -a 256 <downloaded-file>` with the matching checksum entry.
   If all release assets are present, `shasum -a 256 -c SHA256SUMS` verifies all.
3. Open the DMG and drag `Bistro.app` into `/Applications`, or unpack the ZIP
   and move the app there.
4. If release notes identify the build as unsigned and macOS blocks it, use
   right-click → **Open** and confirm. After checking the source and checksum,
   you can also remove quarantine:

   ```bash
   xattr -dr com.apple.quarantine /Applications/Bistro.app
   ```

On first sign-in macOS may request keychain access. The token remains in the
login keychain; its retained credential service name is historically
`bistro-sync`, not a running helper process.

### Windows

Download `bistro-windows-amd64-installer.exe` and `SHA256SUMS`. Check the hash
with PowerShell `Get-FileHash <downloaded-file> -Algorithm SHA256` against the
matching entry, then run the installer. It includes a WebView2 bootstrapper.
The `bistro-windows-amd64.zip` archive is also available; extract it before
running `bistro.exe`, and ensure WebView2 is installed. Unsigned alpha builds
may trigger Windows reputation warnings; check release provenance first.

### Linux

Download `bistro-linux-amd64.tar.gz` and `SHA256SUMS`, compare
`sha256sum <downloaded-file>` with its entry, then extract and run `bistro`.
This is an application archive, not a system package; GTK 4, WebKitGTK 6.0,
and a working Secret Service/keyring are required. Build prerequisites and the
Ubuntu package list are in [DEVELOPMENT.md](../DEVELOPMENT.md#prerequisites).

## Updates

Versioned release builds check `Thourum/Bistro` GitHub releases on launch and
every six hours, including pre-releases. The built-in Wails update window
shows the available update and offers **Skip Version** or **Remind Me Later**,
as well as installation. Downloads are verified against `SHA256SUMS`.

Updates use `bistro-darwin-arm64.zip`, `bistro-windows-amd64.zip`, or
`bistro-linux-amd64.tar.gz`; `*-installer.*` assets are for first installation,
not updater payloads. Dev/untagged builds (`dev` or empty version) do not update.
You can always install a newer alpha manually from its release page.

## Data and resets

Default state directories are:

- macOS: `~/Library/Application Support/Bistro/`
- Windows: `%AppData%\Bistro\`
- Linux: `$XDG_CONFIG_HOME/Bistro/`, or `~/.config/Bistro/`

`BISTRO_DATA_DIR` overrides the directory. It holds `sync.db`, `settings.json`,
and `logs/bistro.log`; tokens remain in the OS keychain. Classification rules
use the separate `os.UserConfigDir()/bistro/rules` directory (lowercase `bistro`),
not the data-directory override. Source development uses `.dev-data/` for app state.

The Wails migration does not migrate old settings. Existing DB/keychain data
is reused only if paths and service names resolve identically. Alpha versions
may require cache resets. Back up the data directory before a reset: drafts,
queued offline actions, local settings, and custom rules may not be on GitHub
and can be lost if you delete it. Viewed marks from before patch-fingerprint
support may need to be marked again.

## Logs and reporting

Logs are in `<data-dir>/logs/bistro.log` and stderr. Report problems at
[GitHub issues](https://github.com/Thourum/Bistro/issues), including the app
version and platform. Inspect logs and redact private repository details and
credentials before sharing. Telemetry/crash reporting honors the privacy
setting; consult the [privacy documentation](../website/src/pages/docs/privacy.mdx)
for user-facing details.

## For maintainers

[CLAUDE.md](../CLAUDE.md#release) and
[DEVELOPMENT.md](../DEVELOPMENT.md#packaging) describe current commands.
`.github/workflows/release.yml` builds native bundles, runs `scripts/smoke.sh`,
creates `SHA256SUMS`, and drafts the pre-release on `v*` tags. Signing and
notarization are conditional on configured secrets. Do not publish or push
release tags without explicit permission. Historical ADR measurements and
harness paths in `docs/adr/data/` remain unchanged.
