# Bistro

**The keyboard-first code review client for the AI-PR era.**

Review pull requests at the speed of thought — no browser tabs, no mouse, no
waiting. Bistro is a local-first desktop app that syncs your GitHub pull
requests into a blazing-fast inbox, generates diffs on your machine, and puts
every action one keystroke away.

![PR inbox](electron/stitch_terminal_authentic_prd/pr_inbox_minimal_light/screen.png)

## Why Bistro?

Code review didn't get slower — pull requests got faster. AI tools open more
PRs, push more force-updates, and leave more bot comments than any web UI was
designed for. Bistro is built for the reviewer drowning in that stream:

- **Triage first.** Your inbox separates *Needs Action* from *Waiting on
  Author* and *FYI*, computed locally from who actually requested your review.
- **Runs entirely on your machine.** A Go sync engine keeps a local SQLite
  cache warm — the UI never waits on the network to render. Your code and
  tokens never touch anyone's server, because there isn't one.
- **Keyboard everything.** `j`/`k` to move, `Enter` to open, `a a` to approve,
  `:` for the command palette, `?` for help. Every binding is remappable.
- **Diffs without limits.** PR branches are cloned and diffed locally with
  go-git, then rendered through a virtualized canvas — 100-file PRs scroll
  like 5-file PRs.
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

## Install

> Packaged installers (DMG / MSI / AppImage) land with v1.0. Until then,
> Bistro runs from source:

```bash
git clone https://github.com/Itterno/Bistro.git && cd Bistro/electron
pnpm install
pnpm build                                     # builds the Go sync engine
```

You'll need Go 1.21+, Node 18+, and pnpm. Create a GitHub OAuth app
(github.com/settings/developers) and put its credentials in
`electron/.env.local` — see [DEVELOPMENT.md](DEVELOPMENT.md#configuration).

## Run

```bash
cd electron && pnpm start
```

The app spawns the sync engine automatically, signs you in via GitHub OAuth
(your token lives in the OS keychain), and starts syncing the repos you watch.
Press `?` anytime to see what your keyboard can do.

## Settings

Open settings with `⌘,` (or `:settings` from the command palette). Appearance,
sync behavior, and every keyboard binding are configurable; settings live in a
plain `settings.json` you can edit by hand — changes apply live. Rebinding a
key updates the footer legend and help overlay everywhere it appears.

## For developers

Architecture, build instructions, debugging, and contribution notes live in
[DEVELOPMENT.md](DEVELOPMENT.md). The feature plan and project status live in
[docs/ROADMAP.md](docs/ROADMAP.md).

## Developed by

Built by **Samuel Asvanyi** at **Itterno ApS**.

## License

© 2026 Itterno ApS. The open-source license will be finalized before the
public v1.0 release (see the [roadmap's open questions](docs/ROADMAP.md)).
