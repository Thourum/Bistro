# Gate 4 — throwaway Wails spike

Smoke evidence only (n=1, synthetic fixtures only, 60 Hz external display). S7 already failed; this does not change the shell no-go. No production Go or Electron sources changed. `harness/run-wails.mjs` is the explicitly requested shared-harness exception to the gate4-only scope.

## Pin and build

Own module: `bistro-adr.local/wails-spike`, `github.com/wailsapp/wails/v3 v3.0.0`. `go mod download -json github.com/wailsapp/wails/v3@v3.0.0` resolved `Origin.Hash=7b11001b92b712210b27d8cb53430e83f67ad8e4`; sums are in `wails-spike/go.sum`. No CLI installation was needed: stdlib bundle assembly plus hdiutil suffices for this unsigned evidence host.

From `wails-spike/`:

```sh
go mod download
go build -tags production -ldflags='-s -w' -o build/BistroWails.app/Contents/MacOS/wails-spike .
cp Info.plist build/BistroWails.app/Contents/Info.plist
mkdir -p build/BistroWails.app/Contents/Resources
cp -R ../../gate2/extracted-smoke/.vite/renderer/main_window build/BistroWails.app/Contents/Resources/renderer
cp /tmp/bistro-smoke-seed/bistro-sync build/BistroWails.app/Contents/MacOS/bistro-sync
hdiutil create -volname BistroWails -srcfolder build/BistroWails.app -format ULMO -ov build/BistroWails.dmg
```

ULMO matches `electron/forge.config.ts`'s MakerDMG. Bundle inventory includes the exact slim renderer and standalone seed engine, although the spike does not launch that engine. Unsigned, no updater/icons/notarization/production integration: not a shippable Wails size comparison. Host-only size was 8,900 KiB allocated / 2,959,399-byte DMG; complete evidence bundle is 70,624 KiB allocated / 28,038,196-byte DMG. Final bundle assembled after smoke; executable unchanged.

## Run

From repository root, acquire the shared port lock before any engine phase; never kill an unowned engine. Keep window frontmost/visible. Release lock even on failure.

```sh
mkdir /tmp/bistro-adr-8080.lock
node docs/adr/data/0001/harness/run-wails.mjs \
  --app docs/adr/data/0001/gate4/wails-spike/build/BistroWails.app \
  --dmg docs/adr/data/0001/gate4/wails-spike/build/BistroWails.dmg \
  --renderer docs/adr/data/0001/gate2/extracted-smoke/.vite/renderer/main_window \
  --seed /tmp/bistro-smoke-seed --out /tmp/wails-smoke --runs 1
rmdir /tmp/bistro-adr-8080.lock
```

Omit `--runs` for the pre-registered 30 startup / 5 idle and post-diff memory / 10 frames per fixture counts; **not run here**. Driver reuses `run-webview.mjs`, `browser-common.mjs`, production renderer collector/statistics, localhost serving, fresh DB per phase and 5-second memory settle. Explicit private WKWebView PID SPIs attribute host + engine + WebContent + GPU + Networking rather than launchd ancestry. Shared allocation totals can double-count. Raw footprint outputs are ignored; per-process sample/process-tree records are retained.

## Origin finding

`origin-repro/run-1/engine.log` proves rejection:

```
origin=wails://localhost error="websocket: request origin not allowed by Upgrader.CheckOrigin"
```

`origin-repro/host.log` records the asset page `wails://localhost/`. Reproduce with `node docs/adr/data/0001/gate4/origin-repro.mjs --host <bundle>/Contents/MacOS/wails-spike --renderer <built renderer> --seed /tmp/bistro-smoke-seed --out <new directory>` under the same lock. This matters only if the WebSocket stays: transport options (a) and (c) would need `wails://localhost` added to the engine's `isLocalOrigin` allowlist, which is a small change; don't accept custom schemes broadly. With option (b), Wails bindings and events, the renderer has no WebSocket and this finding doesn't apply. The cost moves to re-plumbing the transport: 24 message handlers, 68 push events, request IDs and cancellation (Gate 0.4), plus the Gate 3 T1/T2/T6 risks. Measurement instead loads `http://localhost:<port>` from the same `serve-renderer.mjs` as Gate 2.3; engine remains unchanged.

## Smoke results

| Fixture | Frames p50/p95/p99 ms | Late share (measured 17 ms refresh × 1.5) |
|---|---|---|
| 1,000 files | 17 / 30 / 35 | 10.5% |
| One 20,000-line changed file | 17 / 18 / 18 | 0% |

Startup spawn → `pr_list_rendered` collector receipt: **963.97 ms**, prestarted engine, warm/no purge; not comparable to Electron hyperfine launch-to-exit. Each fixture has 1,200 frames, one run. At the separate 120 Hz target budget, late shares are 86.5% / 100%; this is a 60 Hz monitor, not a 120 Hz run.

Physical footprint MiB:

| Phase | Host | Engine | WebContent | GPU | Networking | Sum |
|---|---:|---:|---:|---:|---:|---:|
| Idle | 37.27 | 8.28 | 78.28 | 14.02 | 8.14 | 145.99 |
| Post-diff 1,000 files | 35.47 | 18.88 | 332.38 | 51.92 | 8.25 | 446.90 |
| Post-diff one file | 36.45 | 15.22 | 419.75 | 19.44 | 8.30 | 499.16 |

All retained smoke `host-events.json` show 1600×900, key=true, visible=true, inspectable=true. `xctrace` available at `/Applications/Xcode.app/Contents/Developer/usr/bin/xctrace`; no profiling run. Web Inspector enabled via native `isInspectable`, not just `DevToolsEnabled`.

## Wails API surprises / limits

- `WebviewWindowOptions.JS` executes after navigation, too late to seed `bistro-settings-v1` before React initialization. A blank Wails bootstrap navigation installs a document-start WKUserScript, then loads the unmodified renderer. Startup includes bootstrap cost.
- `window.SetMenu` is a no-op on macOS; use `app.Menu.Set`. Explicit AppMenu/EditMenu roles installed; no duplicate native renderer hotkeys. Edit-role command parity is **not tested** here.
- Wails activates regular apps and shows the window on navigation. Initial sizing yielded 1916×2098; constrain native content size and view frame to 1600×900. No keystroke injection occurred.
- Wails default signal handling did not reliably terminate the original host phase; explicit SIGTERM/SIGINT → `app.Quit` makes harness process-group cleanup complete. `failed-viewport-smoke/` and `failed-boolean-smoke/` preserve implementation failures, not valid measurements; `failed-port-smoke/` records an occupied-port refusal.
- Private PID SPI instrumentation is evidence-only, not an API migration recommendation. Native key/occlusion notifications feed the shared `run-webview.mjs` guard, which requires a key+visible window before measuring and invalidates the phase on later loss. Full profiling, real fixtures (owner re-auth needed), 120 Hz, macOS 13 and keyboard parity remain unmeasured.

Verification: `go vet ./...` and `go test -race ./...` passed inside spike (no Go test files); both new scripts passed `node --check`. Requested `node --test docs/adr/data/0001/harness/*.test.mjs` initially had 8 passes / 1 failure because concurrently edited `run-keys.mjs` declared `openNative` twice. Per coordinator, reran `harness.test.mjs` + `browser-harness.test.mjs`: **8/8 passed**. Code review found no critical/high issues; fixed its origin-repro false-success check (must see actual rejection, not merely a path containing “origin”).
