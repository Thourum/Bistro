# Packaged app fails to start (crash, quit, or white window)

The packaged `Bistro.app` behaves differently from `pnpm start`. The Forge Vite plugin packs only `.vite/` into `app.asar`: no `node_modules`, no `assets/`. Anything the main process loads at runtime has to be bundled by Vite or shipped as a Forge `extraResource`.

## Diagnose

- Electron log: `~/Library/Logs/Bistro/main.log`. Engine log: `~/Library/Application Support/Bistro/logs/bistro-sync.log`.
- See what was actually packed: `npx @electron/asar list out/Bistro-darwin-arm64/Bistro.app/Contents/Resources/app.asar | grep -v node_modules`
- Run the packed main process with full errors: extract the asar (`npx @electron/asar extract … /tmp/bx`) and run `node_modules/electron/dist/Electron.app/Contents/MacOS/Electron /tmp/bx`.
- `scripts/smoke-macos.sh out/Bistro-darwin-arm64/Bistro.app` checks the engine, the `/ws` upgrade, and the `Renderer loaded` log line.

## Known causes

| Symptom | Cause | Fix |
|---|---|---|
| `Cannot find module 'electron-log'` / `'wait-on'` | Dependency marked external in `vite.main.config.ts`, or loaded via `createRequire`, so Vite didn't bundle it | Keep rollup `external` to `['electron']`; use static `import` (add a `.d.ts` for untyped packages, e.g. `src/main/wait-on.d.ts`) |
| Engine ready, then `Failed to load image … app.asar/assets/icon.png` and quit | `assets/` isn't in the asar | Ship it via `extraResource` and read it from `process.resourcesPath` when `app.isPackaged` (`src/main/window.ts`) |
| White window, devtools shows `net::FILE_NOT_FOUND` for `.vite/renderer/main_window/index.html` | Forge 8's Vite plugin passes a relative renderer `outDir`; Vite resolves it against `root: src/renderer`, so the build lands in `src/renderer/.vite/` and isn't packed | Pin `build.outDir` in `vite.renderer.config.ts` |
| Forge 8: `"main" … now emits .vite/build/index.cjs` | Forge 8 emits `.cjs` main/preload bundles | `"main": ".vite/build/index.cjs"` in `package.json`, `preload.cjs` in `window.ts` |

`pnpm type-check` doesn't cover `forge.config.ts`; Forge config breakage (e.g. the `PublisherGitHub` rename in Forge 8) only shows up in `pnpm run release:macos`.
