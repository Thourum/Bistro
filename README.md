# Bistro

**Keyboard-first, local-first GitHub pull request review.**

Bistro is a desktop app that syncs GitHub pull requests into a local inbox,
generates diffs on your machine, and keeps common review actions on the
keyboard. It is built for reviewers who need to move through many PRs without
living in browser tabs.

![PR inbox](electron/stitch_terminal_authentic_prd/pr_inbox_minimal_light/screen.png)

## Why Bistro?

Code review didn't get slower — pull requests got faster. AI tools open more
PRs, push more force-updates, and leave more bot comments than any web UI was
designed for. Bistro is built for the reviewer drowning in that stream:

- **Triage first.** Your inbox separates *Needs Action* from *Waiting on
  Author* and *FYI*, computed locally from who actually requested your review.
- **Runs on your machine.** A Go sync engine keeps a local SQLite
  cache warm — the UI never waits on the network to render. Your code and
  tokens never touch anyone's server, because there isn't one.
- **Keyboard everything.** `j`/`k` to move, `Enter` to open, `a a` to approve,
  `:` for the command palette, `?` for help. Every binding is remappable.
- **Local diffs.** PR branches are cloned and diffed locally with go-git, then
  rendered through a virtualized canvas for large reviews.
- **Works offline.** Approvals and comments queue locally when you're
  disconnected (or rate-limited) and replay automatically.

![PR overview](electron/stitch_terminal_authentic_prd/pr_overview_minimal_refined/screen.png)

## Core features

- PR inbox with filters (Open / Draft / All / Needs Action), search, and
  per-repo watch toggles
- Needs Action / Waiting on Author / FYI triage with live lane counters
- Full-screen diff view: file navigator, syntax highlighting, vim-style
  navigation (`h j k l`, `n`/`p` between files, `0`/`$` jumps)
- Review actions from the keyboard: approve (double-tap confirm), request
  changes, comment — with retry, backoff, and an offline queue
- Real-time updates over a local WebSocket: sync status, rate-limit state,
  and errors surface in the footer as they happen
- Command palette (`:`) and a help overlay (`?`) that always reflects your
  actual bindings — including your remaps

GitHub today; GitLab is next on the [roadmap](docs/ROADMAP.md).

## v0.1 install from source

Packaged installers are not available in v0.1. Run Bistro from source until the
release packaging work is complete.

### Prerequisites

- Go 1.26.x, matching `go/go.mod`
- Node 18+
- pnpm
- A GitHub OAuth app from <https://github.com/settings/developers>

### Setup

```bash
git clone https://github.com/Itterno/Bistro.git
cd Bistro/electron
pnpm install
cp .env.example .env.local
```

Fill `electron/.env.local` with the GitHub OAuth client id and secret:

```dotenv
GITHUB_CLIENT_ID=your_client_id
GITHUB_CLIENT_SECRET=your_client_secret
WS_PORT=0
GO_LOG_LEVEL=debug
```

Then build the Go sync engine:

```bash
pnpm build
```

See [DEVELOPMENT.md](DEVELOPMENT.md#configuration) for every supported local
environment variable.

## Run

```bash
cd electron && pnpm start
```

The app spawns the sync engine automatically, signs you in via GitHub OAuth
(your token lives in the OS keychain), and starts syncing the repos you watch.
Press `?` anytime to see what your keyboard can do.

## Keyboard shortcuts

Bistro's default bindings are documented on the docs site
([website/src/pages/docs/keybindings.mdx](website/src/pages/docs/keybindings.mdx)).
Inside the app, press `?` for the context-aware help overlay or `:` for the
command palette. Custom bindings are stored locally in `settings.json` and are
reflected in the footer legend and help overlay.

## Settings

Open settings with `⌘,` (or `:settings` from the command palette). Appearance,
sync behavior, and every keyboard binding are configurable; settings live in a
plain `settings.json` you can edit by hand — changes apply live. Rebinding a
key updates the footer legend and help overlay everywhere it appears.

## For developers

Architecture, build instructions, and debugging notes live in
[DEVELOPMENT.md](DEVELOPMENT.md). Contribution requirements, including DCO
sign-off, live in [CONTRIBUTING.md](CONTRIBUTING.md), with contributor
onboarding in [design/contribution-guide.md](design/contribution-guide.md). The
feature plan and project status live in [docs/ROADMAP.md](docs/ROADMAP.md).

## Privacy and feedback

Bistro is local-first, not cloud-hosted: the open-source app stores PR data in a
local SQLite cache, stores GitHub tokens in the OS keychain, generates diffs on
your machine, and talks directly to GitHub for sync and review actions. See the
privacy explainer on the docs site
([website/src/pages/docs/privacy.mdx](website/src/pages/docs/privacy.mdx)).

Launch feedback belongs in GitHub issues or discussions so it can be triaged into
bugs, docs fixes, and roadmap requests. See the feedback guide
([website/src/pages/docs/feedback.mdx](website/src/pages/docs/feedback.mdx))
before sharing logs or private repository context.

## Developed by

Built by **Samuel Asvanyi** at **Itterno ApS**.

## License

Bistro is licensed under the [GNU General Public License v3.0 only](LICENSE).

© 2026 Itterno ApS.
