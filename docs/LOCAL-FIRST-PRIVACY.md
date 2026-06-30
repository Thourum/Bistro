# Local-first privacy notes for v0.1

Bistro is a local-first desktop app for GitHub pull request review. In v0.1, the
open-source review path has no required Bistro-operated server: the Go sync
engine talks to GitHub, stores review data on your machine, generates diffs
locally, and serves the Electron UI over localhost.

This is a product explainer, not a formal security audit. The detailed internal
source document is [`design/security-model.md`](../design/security-model.md).

## What stays local

- **PR cache:** PR metadata, comments, status, repository names, and local review
  state are stored in a local SQLite database.
- **Diff generation:** PR branches are cloned and diffed on your machine with
  `go-git`; Bistro does not send repository contents to a Bistro cloud service.
- **Settings:** keyboard bindings, appearance, and sync preferences are local
  settings.
- **GitHub token storage:** OAuth tokens are stored in the OS keychain, not in
  Bistro's SQLite cache.
- **Queued review actions:** comments, approvals, and request-changes actions can
  queue locally when offline or rate-limited, then replay to GitHub when network
  access is available.

## What leaves your machine

- **GitHub API calls:** Bistro syncs PR data from GitHub and sends review actions
  back to GitHub. GitHub remains the source of truth.
- **OAuth flow:** sign-in happens through GitHub OAuth. Bistro receives a token
  from GitHub after you authorize the app.
- **Repository fetches:** local diff generation still requires network access to
  GitHub or your configured Git remote when Bistro fetches branches that are not
  already present locally.
- **Optional update checks:** packaged builds do not check for updates by default.
  Builds started with `BISTRO_ENABLE_AUTO_UPDATES=1` can check GitHub releases via
  Electron's public update service; `BISTRO_DISABLE_AUTO_UPDATES=1` disables this
  path even when packaged.
- **Optional telemetry (BIS-41):** usage telemetry is disabled until you enable
  Settings → Privacy → Usage Telemetry. When enabled, Bistro stores an anonymous
  install ID plus allowlisted product events under Electron `userData/telemetry/`
  and can POST them to `BISTRO_TELEMETRY_ENDPOINT` when that endpoint is
  configured. The allowlist is limited to `app_started`, `settings_updated`,
  `review_action_requested`, and `fs_patch_read` with booleans/enums/app version
  only. It excludes repository names, usernames, file paths, PR titles/bodies,
  code, diff text, comments, tokens, URLs, branch names, and Git SHAs.
  Crash reporting uses the same explicit consent gate; Bistro does not initialize
  or call crash telemetry before Usage Telemetry is enabled.
- **Optional user-configured providers:** if future builds let you configure other
  services, those integrations should be documented where they are added.

Bistro's privacy claim is not that data never leaves your computer. The claim is
that the open-source review path does not require a Bistro-hosted server between
you and GitHub.

## Tokens and credentials

- GitHub tokens are stored in the OS keychain, not in the SQLite database.
- Development OAuth credentials belong in ignored local environment files such as
  `electron/.env.local`.
- Keep OAuth app client secrets out of screenshots, logs, issues, and pull
  requests.
- Do not paste OAuth tokens, private repository contents, or unredacted logs into
  GitHub issues.

## Local storage

Bistro treats SQLite as a local cache. The database may contain repository names,
PR titles, comments, diff content, and local review state. It should not contain
OAuth tokens, personal access tokens, GitHub passwords, or SSH keys.

Because the cache can include private repository context, use normal workstation
protections: a separate OS account, disk encryption such as FileVault, BitLocker,
or LUKS, and careful log sharing when reporting bugs.

## Local process boundary

The app runs as two local processes:

1. The Go sync engine owns GitHub sync, local SQLite writes, local diff
   generation, and queued review actions.
2. The Electron UI talks to the engine over localhost and renders the inbox, diff,
   keyboard actions, and sync status.

The localhost channel is intended for same-machine communication. It is not a
public API and should not be exposed to the network.

## Logs and feedback

Logs are for debugging startup, sync, rate limits, and review-action replay. They
should not include OAuth tokens or full comment bodies, but still review and redact
logs before sharing them in public issues.

When reporting a bug, include the Bistro version or commit SHA, operating system,
and a redacted log excerpt around the failure. Do not include private repository
contents unless you intentionally choose to share them.

For report templates and the feedback path, see [`docs/FEEDBACK.md`](FEEDBACK.md).

## Current limits

- Bistro is GitHub-first today; other providers are roadmap work unless a current
  release says otherwise.
- Packaged installers are pending until release packaging is ready; source-run
  setup is the public path for now.
- Local-first does not protect against a compromised OS, stolen unlocked laptop,
  or compromised GitHub account. Use OS disk encryption and GitHub account
  protections such as 2FA.
