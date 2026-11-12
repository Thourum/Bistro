# Release workflow fails on a version tag

`.github/workflows/release.yml` runs only on `v*` tags, so its gates and packaging steps are not exercised by PR CI. Failures seen while cutting `v0.0.1` (2026-11), in run order:

| Step | Symptom | Cause | Resolution |
|---|---|---|---|
| Verify release inputs | `secret BISTRO_GITHUB_CLIENT_ID is required` | Stale hard requirement; `main.go` embeds the public OAuth client ID | Secret is an optional override |
| Scan Go call graph | `govulncheck` exit 3, standard-library findings | `go` directive pinned an old patch release; new advisories can fail a tag days after a clean run with no code change | Raise the `go` line in `go.mod` to the latest patch; bump flagged modules |
| Audit production Node dependencies | Critical advisory in a transitive package | A dependency pins the vulnerable version exactly | Upgrade the parent within `minimumReleaseAge`, then a version-scoped `overrides` entry in `pnpm-workspace.yaml` |
| Scan git history for secrets | `gitleaks` findings in `docs/adr/data/` | Public API dumps (40-hex IDs) and keybinding `"keys"` fields | Path allowlist in `.gitleaks.toml`; never rewrite that evidence |
| Build and package (macOS) | `diskutil … Unknown option '--volumeName'` | `diskutil image` exists only on newer macOS than the `macos-15` runner | `hdiutil create -volname … -srcfolder … -format UDZO` |
| Smoke (Linux) | `GNU coreutils timeout is required` | Ubuntu 26.04 ships uutils coreutils | `smoke.sh` accepts GNU or uutils |
| Smoke (Linux) | Exit 2 after GTK portal timeouts | See [packaged-app-fails-to-start.md](packaged-app-fails-to-start.md#wails-smoke-waits-for-runtime-readiness) | `GDK_DEBUG=no-portals` in Linux smoke |

Before tagging, reproduce the gates locally: `govulncheck ./...` on the `go.mod` toolchain, `pnpm audit --prod --audit-level=critical` from `frontend/`, and `gitleaks git --config .gitleaks.toml --log-opts="--remotes=origin --tags" .`. A failed tag produced no release, so it can be moved to the fixed commit (`git push origin :refs/tags/vX` then re-push).
