# ADR 0001: shared rules for the Gate 2, Gate 4 and Gate 5 tasks

Read first: `docs/adr/0001-electron-vs-wails.md` (thresholds are pre-registered and frozen at `30a6d7f`; do not edit them), `docs/adr/data/0001/harness/README.md`, `docs/adr/data/0001/gate3/README.md` (S6 prediction, pins: Wails v3.0.0 = `7b11001b92b712210b27d8cb53430e83f67ad8e4`).

## State
- The shell verdict is already **no-go**: S7 failed at Gate 3. The owner chose to run the remaining gates anyway, as evidence. So measure and report faithfully, and don't tune anything to pass.
- Measurement builds (from committed sources):
  - `electron/out/current/Bistro-darwin-arm64/Bistro.app`
  - `electron/out/slim/Bistro-darwin-arm64/Bistro.app`
- The synthetic-only seed is at `/tmp/bistro-smoke-seed`.
- The two real PR fixtures are **not seeded**: the engine's keychain token returns 401 until the owner signs in again. Synthetic fixtures only for now.
- An extracted renderer from slim is at `docs/adr/data/0001/gate2/extracted-smoke/`, created with `@electron/asar` from the repo-root `node_modules`. It's gitignored and can be regenerated.
- Tools:
  - Playwright 1.63.0 is installed globally (`npm root -g`); browsers chromium-1248 and webkit-2370 are cached.
  - **No macOS 13 VM:** the owner declined to boot the downloaded tart image. Do not use tart.
  - hyperfine and footprint are installed.
- The display is a 60 Hz external monitor.

## Rules
- **Smoke runs only (n=1), and label them that way.** Other agents are building at the same time, so the machine isn't quiet. The coordinator schedules full-count measurement runs separately. Build drivers so a full run is one command.
- Every number comes from a command, with raw output written under `docs/adr/data/0001/<gate>/`. Report n and spread; compare distributions, not means.
- **Commits:**
  - Make per-task commits in conventional format (`docs:`, `feat:`, `test:` …). No `--no-verify`.
  - The pre-commit hook blocks any staged file over 20,000 bytes. Keep authored files under that size by splitting them.
  - Raw dumps over the limit stay uncommitted. List them in your report; the owner decides.
  - Don't edit `.githooks`, permission settings, CLAUDE.md/AGENTS.md rules, or the frozen thresholds.
- **If a permission is denied,** stop that piece of work and report exactly what was denied and why it's needed. Don't work around it, and don't ask the coordinator to run it for you.
- Never print secrets or tokens. Don't copy `electron/.env.local`. GitHub access is read-only.
- Install nothing globally except what's listed above, plus Go module downloads into the normal module cache. Per-worktree npm installs only where your task allows it.
- **Code style:**
  - Code in `electron/src` follows AGENTS.md: `@thourum/fault` with no try/catch, zod at the edges, Biome. Check it with `cd electron && pnpm type-check && pnpm lint && pnpm test`.
  - Harness `.mjs` scripts follow the existing harness style.
  - Add a test only when it would fail if real logic broke.
- Don't run anything that injects keystrokes or switches input sources on the host unless your task says so. If it does, follow that task's safety guard.
- **Final report:** files created, commits, what was verified, n=1 smoke numbers, blockers, and anything that needs the owner. Keep it concise.
