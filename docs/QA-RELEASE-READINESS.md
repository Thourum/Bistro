# Bistro QA and release-readiness plan

Owner: QA & Release Engineer
Issues: BIS-14, BIS-36
Last updated: 2026-06-29

This plan defines the minimum evidence required to call a Bistro desktop build release-ready. It favors small proving checks over broad expensive runs, but escalates to full regression when installer, sync, database, or keyboard workflow risk changes.

## Release principles

- **Local-first stays true:** no release may require a Paperclip/Bistro server for review flows; PR content, repo clones, SQLite cache, queued actions, and tokens remain local-only.
- **Keyboard-first is a release gate:** core review flows must be completeable without a mouse, and help/remap surfaces must match actual bindings.
- **Evidence over assertion:** every signoff records command output, app version/build, OS, fixture PR(s), and pass/fail notes.
- **Risk-based scope:** smoke every release candidate; run focused regressions for changed subsystems; run the full checklist for public/beta distribution.

## Release gates

| Gate | Required evidence | Exit criteria |
|---|---|---|
| 0. Scope and blockers | Release scope, commit/version, known roadmap blockers, open v1 questions | No unowned ship blockers; public release explicitly blocked until telemetry/crash/update decisions and dependency license review are resolved |
| 1. Static and automated checks | Go and Electron command matrix below | Required commands pass or have named, accepted exceptions |
| 2. Local-first smoke | Fresh DB, keychain auth, localhost WS, sync, local diff generation, offline queue | App renders cached data without network after initial sync; write actions queue offline and replay safely |
| 3. Keyboard accessibility smoke | Inbox → PR → diff → action flow using keyboard only | No mouse required; focus is visible; `?` and footer reflect remaps; destructive actions require confirmation |
| 4. Fixture PR regression | Fixture PR matrix below | Core rendering, comments, CI, force-push, large diff, and markdown cases match expected behavior |
| 5. Installer/package risk | Package artifact opens, spawns/kills engine, persists settings, handles upgrade from previous DB | No orphaned engine, DB migration failure, missing native dependency, or unsigned/unexplained security prompt beyond accepted distribution policy |
| 6. Signoff | Completed evidence log and release notes QA section | QA disposition is `pass`, `pass_with_known_issues`, or `block` with named owner/action |

## Automated command matrix

Run from a clean checkout unless the release manager explicitly approves a narrower proof.

### CI workflow matrix

The required PR and `master` branch guardrail lives in `.github/workflows/ci.yml`. It is also manually runnable with `workflow_dispatch` when a release candidate needs fresh hosted evidence.

| CI job | Hosted runner | Commands covered | Release gate covered | Required before merge |
|---|---|---|---|---|
| Go vet and tests | `ubuntu-latest` | `go vet ./...`; `go test -race ./...` under `dbus-run-session` with keyring dependencies | Static checks; sync/store/concurrency regression smoke | Yes |
| Electron typecheck, lint, and tests | `ubuntu-latest` | `pnpm install --frozen-lockfile`; `pnpm type-check`; `pnpm lint`; `pnpm test` | Renderer/main process static checks; keyboard workflow/unit regression smoke | Yes |

CI is not a substitute for installer/package, offline queue, fixture PR, or keyboard-only manual release gates below. Those remain explicit QA signoff evidence for release candidates.

### Mandatory for every release candidate

```bash
cd go && go vet ./...
cd go && go test ./...
cd electron && pnpm type-check
cd electron && pnpm lint
cd electron && pnpm test
cd electron && pnpm package
```

### Mandatory before public/beta distribution or when concurrency/sync/store code changes

```bash
cd go && go test -race ./...
```

### Mandatory when installer/update behavior changes

```bash
cd electron && pnpm release:macos
cd electron && pnpm release:win
```

For update-path packaging, `release:macos` must produce both the DMG installer
and the ZIP artifact consumed by Squirrel.Mac / `update-electron-app`.

Record the exact command, OS/architecture, duration, and result. Prefer rerunning only the failed/focused command after a fix, then rerun the relevant gate before signoff.

## Smoke checklist

### Fresh install / first launch

- Launch from packaged app with an empty user data directory and no existing `bistro-sync` process.
- Confirm Electron waits for `bistro-sync` ready JSON after migrations and connects to the reported random WS port.
- Complete GitHub OAuth with expected scopes for enabled actions; token lands in OS keychain, not repo files or app logs.
- Watch one repo and confirm inbox fills from local SQLite-backed sync.
- Quit app and confirm the engine exits via stdin close; relaunch without DB lock errors.

### Local-first and offline behavior

- After sync, disable network and reopen a synced PR from cache.
- Confirm local diff rendering does not require network once clones/diffs are cached.
- Queue an approve/comment/request-changes action offline; verify pending state is visible and no duplicate action is created.
- Restore network; verify replay succeeds once and the queue drains.

### Keyboard review path

- Use only keyboard: move inbox selection (`j`/`k`), open PR (`Enter`), move files/hunks (`n`/`p`, `[`/`]`), scroll (`h`/`j`/`k`/`l`), open command palette (`:`), open help (`?`), approve with double-tap (`a a`).
- Verify focus ring/active row is visible in light and dark themes.
- Remap at least one binding in settings; verify footer legend and help overlay update immediately.
- Confirm destructive/remote-write actions cannot fire from accidental single-key presses.

### Markdown/security rendering

- Open a PR with tables, TypeScript and diff fences, unlabeled fences, nested `<details>`, links, images, and a literal `<script>` payload.
- Confirm links open externally, code/diff highlighting is readable in light and dark mode, and script content is inert.

## Fixture PR matrix

Maintain these as GitHub fixture PRs in a QA-owned repository, plus recorded/local fixtures where API calls should not be live.

| Fixture | Purpose | Must cover |
|---|---|---|
| `small-clean` | Baseline review | 1-2 files, CI pass, approve/comment path |
| `large-ai-pr` | AI-era volume | 100+ files, generated/vendor paths, virtualized diff performance |
| `force-pushed` | Patchset/rebase risk | Updated head SHA, stale comments, diff refresh |
| `review-comments` | Comment/thread sync | PR-level comments, inline review comments, bot comments, unresolved/resolved thread data when available |
| `offline-actions` | Queue idempotency | Network/rate-limit failure, retry, replay once |
| `markdown-hostile` | Renderer/security | GFM table, code fences, nested details, external link, inert script text |
| `multi-repo-triage` | Inbox classification | Needs Action, Waiting on Author, FYI, draft/merged filters, search |
| `installer-upgrade` | Migration/update | Previous-version DB, settings remap, keychain token, existing clone cache |

Each fixture records: repository URL, PR number, required GitHub scopes, expected lane/status, expected file/comment counts, and last refreshed date. Do not depend on private production PRs for release signoff.

## Installer and update risk checklist

- Package includes the Go `bistro-sync` binary for the target platform and can execute it from the app bundle/package path.
- Native modules and SQLite/keychain dependencies load on a clean machine.
- GitHub release publishing uses `electron/forge.config.ts`'s GitHub publisher with `GITHUB_TOKEN`; releases stay draft until QA signs off, then are published for `update.electronjs.org`.
- Auto-update checks run only in packaged apps and use public GitHub Releases through `update-electron-app`; macOS first install still uses the DMG, but update delivery requires the matching ZIP artifact.
- macOS update signoff requires Developer ID signing/notarization; unsigned ZIPs can build but Squirrel.Mac will not reliably apply them.
- App data paths are stable across upgrades; old SQLite migrations apply once and are idempotent.
- A second launch does not create competing engine instances for the same DB.
- Logs do not contain OAuth secrets, tokens, PR body content beyond expected local diagnostics, or repo clone credentials.
- Public release remains blocked until the remaining roadmap open questions are resolved: `@pierre/diffs` license, telemetry stance, crash reporter, and signing/notarization secrets. The FOSS code license is resolved as GPLv3-only with DCO sign-off.

## Accessibility and keyboard QA

- Every interactive control in the review path has an accessible name, visible focus state, and keyboard activation path.
- Modal/dialog focus is trapped and returns to the invoking element on close.
- Help overlay and command palette are reachable without a mouse and searchable enough to recover from remaps.
- Color is not the only signal for CI/review state; badges or labels carry state text.
- Test at default size and a narrow desktop width; no required action is clipped or mouse-only.

## Signoff template

Use this template in the issue/release thread.

```markdown
## QA signoff — Bistro desktop <version/build>

Disposition: pass | pass_with_known_issues | block
Build: <commit SHA, package artifact, OS/arch>
Scope: <release candidate / beta / focused regression>

Automated checks:
- go vet ./...: PASS/FAIL <link/log>
- go test ./...: PASS/FAIL <link/log>
- go test -race ./...: PASS/FAIL/SKIPPED <reason>
- pnpm type-check: PASS/FAIL <link/log>
- pnpm lint: PASS/FAIL <link/log>
- pnpm test: PASS/FAIL <link/log>
- pnpm package/release:*: PASS/FAIL/SKIPPED <reason>

Manual smoke:
- Fresh launch/auth/sync: PASS/FAIL
- Local-first offline cache: PASS/FAIL
- Offline queue replay: PASS/FAIL
- Keyboard-only review path: PASS/FAIL
- Markdown/security rendering: PASS/FAIL
- Installer/update: PASS/FAIL/SKIPPED <reason>

Fixture PRs:
- <fixture>: PASS/FAIL <PR URL, notes>

Known issues accepted:
- <issue/link, owner, acceptance reason>

Blockers:
- <owner/action needed>
```

## Escalation rules

- Mark release **blocked** for data loss, token leakage, DB migration failure, duplicate remote writes, broken app launch, inaccessible keyboard-only core flow, or public-release licensing uncertainty.
- Mark **pass_with_known_issues** only for documented non-core defects with owner, severity, and user-visible release note or mitigation.
- Create child issues for fixture creation, installer automation, or accessibility fixes that exceed one heartbeat; do not leave release-readiness work as an unowned checklist.
