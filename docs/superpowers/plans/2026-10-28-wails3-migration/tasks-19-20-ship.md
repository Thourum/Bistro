# Tasks 19–20: CI, release, smoke test, docs

Read first: [README](README.md) and the spec §4 and Done criteria. Before you edit either workflow, read the current `.github/workflows/ci.yml` and `release.yml`.

### Task 19: CI, release workflow, `scripts/smoke.sh`

**Files:**
- Create: `scripts/smoke.sh`
- Modify: `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `Taskfile.yml` (only if packaging names need fixing)

**Interfaces:**
- Consumes:
  - the Taskfile tasks `generate`, `package:darwin`, `package:windows` and `package:linux`, with the asset names from Task 14;
  - the binary's `--smoke` flag;
  - the `BISTRO_VERSION`, `GITHUB_CLIENT_ID` and `SENTRY_DSN` build env.
- Produces:
  - green CI jobs `go`, `frontend` and `drift`;
  - a release workflow that uploads `bistro-darwin-arm64.zip`, `bistro-darwin-arm64-installer.dmg`, `bistro-windows-amd64.zip`, `bistro-windows-amd64-installer.exe`, `bistro-linux-amd64.tar.gz` and `SHA256SUMS` to a draft pre-release.

- [ ] **Step 1: `scripts/smoke.sh`.**
  ```bash
  #!/usr/bin/env bash
  # Launch a packaged Bistro with --smoke. The app exits 0 once migrations ran and
  # the window finished its first load; non-zero on error or after 60 s.
  # Usage: scripts/smoke.sh <path-to-binary-or-.app>
  set -euo pipefail
  target="$1"
  if [[ -d "$target" && "$target" == *.app ]]; then
    target="$target/Contents/MacOS/$(defaults read "$PWD/$target/Contents/Info" CFBundleExecutable)"
  fi
  runner=()
  if [[ "$(uname -s)" == Linux ]]; then runner=(xvfb-run -a); fi
  # --smoke uses its own temporary data dir and never syncs (Task 14).
  timeout 90 "${runner[@]}" "$target" --smoke
  echo "smoke ok"
  ```
  - The app isolates its own data in smoke mode. The outer `timeout 90` is a guard above the app's own 60 s.
  - On Windows the job runs it under Git Bash with `bin/bistro.exe`. If `timeout` is missing there, use a PowerShell step with `Start-Process -Wait -Timeout` instead, and say so in the commit.
  - `chmod +x scripts/smoke.sh`.
- [ ] **Step 2: `ci.yml`.**
  - **Go job.** At the repo root, extend the existing apt step with the Wails Linux deps: `libgtk-4-dev libwebkitgtk-6.0-dev`. Confirm the exact package names against `wails3 doctor` or the v3.0.0 docs. Before `go vet`, add `mkdir -p frontend/dist && echo '<!doctype html>' > frontend/dist/index.html`. Keep the dbus/gnome-keyring wrapper around `go test -race ./...`.
  - **Frontend job.** `working-directory: frontend`; `pnpm install --frozen-lockfile`, `pnpm type-check`, `pnpm lint`, `pnpm test`.
  - **New `drift` job.** Set up Go, pnpm and Node with the same pins. Run `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0`, install the frontend deps, then run `wails3 task generate` and `git diff --exit-code frontend/bindings internal/store/db`. It needs the same apt deps as the Go job.
  - Keep `permissions: contents: read`, the pinned action SHAs and `persist-credentials: false`, exactly as the current file does.
- [ ] **Step 3: `release.yml`.**
  - **`gates`.** `govulncheck ./...` at the root (it needs the apt deps and the `frontend/dist` placeholder), `pnpm audit --prod --audit-level=critical` in `frontend/`, and gitleaks unchanged.
  - **Version check.** At the start of each build job, assert that the tag matches `v[0-9]+.[0-9]+.[0-9]+(-…)?` and export `BISTRO_VERSION=${GITHUB_REF_NAME#v}`. The `electron/package.json` comparison goes away (decision 13).
  - **`build` matrix.** Use `include:` entries:
    - `{os: macos-15, task: package:darwin, smoke: bin/Bistro.app}`
    - `{os: windows-latest, task: package:windows, smoke: bin/bistro.exe}`
    - `{os: ubuntu-latest, task: package:linux, smoke: bin/bistro}`

    For each entry: set up Go, pnpm and Node; install the CLI; on Linux install the apt deps and `xvfb`. Then run the existing macOS signing and notarization steps only when `matrix.os == 'macos-15'` and the `MACOS_*`/`APPLE_*` secrets exist. That logic is unchanged; the signing identity is passed to the Wails darwin sign task. Then run `wails3 task ${{ matrix.task }}` with `GITHUB_CLIENT_ID=${{ secrets.BISTRO_GITHUB_CLIENT_ID }}`, `SENTRY_DSN=${{ secrets.SENTRY_DSN }}` and `BISTRO_VERSION`. Run `scripts/smoke.sh ${{ matrix.smoke }}`. Write `sha256sum` lines for this platform's assets to `sums-<os>.txt` (`shasum -a 256` on macOS). Finally, `actions/upload-artifact` the assets plus the sums file.
  - **`release`** (`needs: build`, ubuntu). `actions/download-artifact` all of them, `cat sums-*.txt > SHA256SUMS`, and check that all six expected files are present. Then run `gh release create "$GITHUB_REF_NAME" --verify-tag --draft --prerelease --title "Bistro $GITHUB_REF_NAME" <assets> SHA256SUMS`. Release notes: keep the current alpha text, but name all three platforms and point unsigned macOS users to `docs/ALPHA.md`.
  - **`SHA256SUMS` format.** Every line must be `<hex>  <basename>`, the `sha256sum` format the updater's `ChecksumAsset` parses. Run `basename` on the paths before hashing.
- [ ] **Step 4: Verify locally (macOS).**
  ```bash
  BISTRO_VERSION=0.0.0-local wails3 task package:darwin && ls bin/
  scripts/smoke.sh bin/Bistro.app
  unzip -l bin/bistro-darwin-arm64.zip | awk 'NR>3 {print $4}' | cut -d/ -f1 | sort -u   # expect exactly: Bistro.app
  hdiutil imageinfo bin/bistro-darwin-arm64-installer.dmg >/dev/null && du -m bin/bistro-darwin-arm64-installer.dmg
  ```
  - Expected: `smoke ok`, one top-level entry, and a DMG of ≤ 40.5 MB. Report the size.
  - Lint the YAML with `actionlint` if it is installed; otherwise say it was not run.
  - The Windows and Linux jobs can only be verified in CI. Say so.
- [ ] **Step 5: Commit.** `git add -A && git commit -m "ci: build, smoke-test and release Wails bundles for macOS, Windows and Linux"`

### Task 20: Docs and the ADR 0001 amendment

**Files:**
- Modify: `CLAUDE.md`, `AGENTS.md`, `README.md`, `DEVELOPMENT.md`, `docs/go-project-overview.md`, `docs/ALPHA.md`, `docs/adr/0001-electron-vs-wails.md`, `frontend/CLAUDE.md`, `.gitignore` (only if a doc names a path that is missing)
- Do not touch: `website/` and `docs/adr/data/`, both left untouched by spec. Also leave the old plans/specs alone, as historical notes.

- [ ] **Step 1: Root `CLAUDE.md`.**
  - **Intro.** Bistro is one Wails v3 process. The OAuth note stays.
  - **Architecture.** Replace the two-process section with: `main.go` (Wails app); `internal/engine` (sync engine, no WS); `internal/services` (one service per domain, typed methods, `ServiceError` codes, `guard`); `internal/shell` (settings, editor, telemetry, updater); `frontend/` (renderer) with the committed generated `frontend/bindings/`. Describe the lifecycle: single-instance through Wails, `OnShutdown` stops the engine, and `--smoke`.
  - **Package paths.** Rewrite every `go/internal/...` path and every `internal/server` mention to `internal/...` / `internal/engine`.
  - **Conventions.** Replace the "Validate external data with zod at the edge" bullet and the WebSocket transport bullet with:
    - Engine calls go through `call()` / `callWrite()` in `frontend/src/services/engine/call.ts` with the generated bindings (`@bindings/services`). Never call a binding directly from a component.
    - Faults carry `code`/`reason` context. Branch on `faultReason`, never on message text.
    - Push events are registered in `internal/services/events.go` and handled in the exhaustive table `frontend/src/services/events/index.ts`.
    - zod is only for genuinely external input: URL search params, bench collector, DOM events, and the RuntimeError cause in `call.ts`.
    - A new service method needs `wails3 task generate`, with the bindings committed.
    - Update the "Review drafts" bullet: `submit_review` becomes `Threads.SubmitAction` with action `submit_review`.
    - Update the onboarding bullet's engine-side paths, which now live in `internal/services/{review_queue,dev_folder_scan,repos}.go`.
    - Bench: the `?bench_keys=1` → `callWrite` guard.
  - **How to work.**
    - Commands: `wails3 task dev`, `wails3 task generate`, `wails3 task build`, `wails3 task package:<os>`.
    - Verify: `go vet ./... && go test -race ./...` at the root, and `cd frontend && pnpm type-check && pnpm lint && pnpm test`.
    - sqlc: run from the root, or use `wails3 task generate`.
    - Config: `BISTRO_DATA_DIR`, `GITHUB_CLIENT_ID`/`SECRET`, `BISTRO_LOG_LEVEL`.
    - Pre-commit hook: mention the `frontend/bindings/` skip if Task 14 added it.
    - The ADR bench section stays, noting that the harness keeps its Electron paths as a historical record.
  - **Tech stack.** Add Wails v3.0.0, `@wailsio/runtime`, sentry-go and posthog-go. Drop Electron Forge and gorilla/websocket. The pnpm workspace is `frontend/` + `website/`.
  - **Release.** Point to the new workflow, the asset names and `scripts/smoke.sh`.
- [ ] **Step 2: The other docs.** Make `AGENTS.md`, `README.md`, `DEVELOPMENT.md`, `docs/go-project-overview.md`, `docs/ALPHA.md` and `frontend/CLAUDE.md` describe the same layout and commands. Reference `CLAUDE.md` instead of duplicating it.
  - `go-project-overview.md` drops `internal/ws` and `cmd/bistro-sync`, and adds `engine`, `services` and `shell`.
  - `ALPHA.md` covers install per platform, `xattr` for unsigned macOS builds, and auto-update behaviour (the built-in window, Skip/Remind).
  - `rg -n "electron/|go/internal|bistro-sync|WebSocket|ws://|8080|pnpm start" CLAUDE.md AGENTS.md README.md DEVELOPMENT.md docs/go-project-overview.md docs/ALPHA.md frontend/CLAUDE.md`. Every remaining hit must be intentional: historical, or the OAuth loopback port.
- [ ] **Step 3: ADR amendment.** Append to `docs/adr/0001-electron-vs-wails.md`:
  ```markdown
  ## Amendment — 2026-10-28

  The verdict above stays **no-go** under its pre-registered rule. The owner nevertheless chose to migrate on the experimental branch `experiment/wails3` (implementation on `experiment/wails3-migration`), because Electron's memory and the localhost WebSocket remained the performance blockers after the React work. Design: [Wails v3 migration design](../superpowers/specs/2026-10-28-wails3-migration-design.md). The S3 (idle footprint ≤ 173 MiB) and S5 (DMG ≤ 40.5 MB) limits are re-checked on the packaged app as the migration's done criteria. The known risks in the design's "Known risks" section are accepted for the experiment, not resolved.
  ```
  Do not edit any thresholds or earlier text in the ADR.
- [ ] **Step 4: Verify.** Check every relative link you added or changed resolves (`ls` each target). Re-run `go vet ./... && go test -race ./...` and the frontend checks to confirm the docs-only change broke nothing.
- [ ] **Step 5: Commit.** `git add -A && git commit -m "docs: describe the Wails v3 layout and amend ADR 0001"`
