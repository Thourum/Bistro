# Bistro Contributor Guide

**Purpose:** give new contributors the shortest reliable path from clone to a useful first change.

**Audience:** open-source contributors and Bistro team members onboarding to the repository.

Bistro is a keyboard-first, local-first desktop app for GitHub pull request review. The app has no Bistro-hosted server: a Go sync engine talks to GitHub, stores a local SQLite cache, generates diffs locally with go-git, and serves the Electron + React UI over a localhost WebSocket.

## Start here

1. Read the product overview in [`README.md`](../README.md).
2. Read contribution requirements in [`CONTRIBUTING.md`](../CONTRIBUTING.md), including the DCO sign-off requirement.
3. Set up and run the app with [`DEVELOPMENT.md`](../DEVELOPMENT.md).
4. Use [`docs/go-project-overview.md`](../docs/go-project-overview.md) when changing the sync engine or WebSocket flow.
5. Use [`docs/ROADMAP.md`](../docs/ROADMAP.md) to understand current project priorities and known limits.
6. Use [`design/keyboard-bindings.md`](keyboard-bindings.md) when touching review navigation, command palette behavior, or keyboard help.
7. Use [`docs/FEEDBACK.md`](../docs/FEEDBACK.md) and [`docs/LOCAL-FIRST-PRIVACY.md`](../docs/LOCAL-FIRST-PRIVACY.md) when updating launch, privacy, or user-reporting copy.

## Prerequisites

- Go 1.26.x, matching [`go/go.mod`](../go/go.mod).
- Node 18+.
- pnpm. Run pnpm commands from `electron/`; the lockfile is `electron/pnpm-lock.yaml`.
- A GitHub OAuth app for local development. Copy `electron/.env.example` to `electron/.env.local` and fill in the GitHub client values before testing authenticated sync.

## Run Bistro from source

```bash
git clone https://github.com/Thourum/Bistro.git
cd Bistro/electron
pnpm install
pnpm build
pnpm start
```

`pnpm start` launches Electron Forge. In normal development, Electron starts the Go sync engine for you. If you need engine logs or a debugger, build and run it separately:

```bash
cd go
go build -o bistro-sync ./cmd/bistro-sync
./bistro-sync
```

Then run Electron with `BISTRO_SKIP_GO_SPAWN=1` as described in [`DEVELOPMENT.md`](../DEVELOPMENT.md#quick-start).

## Repository map

```text
bistro/
├── go/              Go sync engine: GitHub sync, local SQLite cache, local diff generation, WebSocket API
├── electron/        Electron main process and React renderer
├── docs/            roadmap, release readiness, architecture notes, implementation plans
├── design/          product/design references, keyboard bindings, contribution guide
└── website/         website planning and launch-copy work
```

## License and DCO

Bistro is licensed under the [GNU General Public License v3.0 only](../LICENSE).
Contributions use the Developer Certificate of Origin, not a CLA. Sign every
commit with `git commit -s`; the full certification text is in
[`DCO.txt`](../DCO.txt).

## Contribution principles

- Keep Bistro local-first. Do not add a required hosted service for the open-source app.
- Preserve keyboard-first review. Every core review action needs a keyboard path, visible focus, and updated shortcut documentation.
- Keep GitHub tokens and local repository data on the user's machine. Secrets belong in the OS keychain or ignored local env files, never in committed docs or code.
- Treat SQLite as a local cache. The Go engine owns SQLite writes; the UI talks through the WebSocket/API layer.
- Prefer specific, testable language in docs. Explain mechanisms such as local cache, local diffs, and offline queueing instead of broad speed claims.

## Before opening a pull request

Choose the smallest verification that matches your change:

```bash
# Go engine changes
cd go && go vet ./... && go test -race ./...

# Electron UI changes
cd electron && pnpm type-check && pnpm lint && pnpm test

# Docs-only changes
git diff --check
```

For UI or review-flow changes, also run the app and verify the keyboard path manually: inbox navigation, opening a PR, moving through files/hunks, command palette, help overlay, and the action you changed.

## Good first contributions

- Fix stale setup, roadmap, or keyboard-reference docs.
- Improve screenshots or docs that explain the local-first review flow.
- Add focused tests around a small Go package or Electron component.
- Pick a bounded issue labeled for docs, onboarding, UI polish, or backend cleanup.

Avoid large architecture changes as a first PR. If a change affects sync semantics, local storage, authentication, offline actions, or provider boundaries, open an issue or design note first.

## Reporting bugs and feedback

Use [`docs/FEEDBACK.md`](../docs/FEEDBACK.md) for the canonical feedback path and
the GitHub issue templates for structured reports.

Include enough detail for a maintainer to reproduce the issue:

- OS and architecture.
- Bistro commit or app version.
- GitHub provider scope and whether the issue happens before or after OAuth.
- Steps to reproduce, expected behavior, and actual behavior.
- Relevant terminal logs from Electron or `bistro-sync`.
- For performance reports: PR size, file count, and which view or action felt slow.

Docs feedback should name the page, what was confusing, and the wording or example that would have helped.

## Keep these docs current

When you change commands, repo structure, runtime versions, keyboard bindings, or contribution expectations, update this guide and the canonical source it points to in the same change.
