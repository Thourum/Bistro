# Gate 3 — source reference key and transport trace

Part of the Gate 3 read; see [README.md](README.md). Reference IDs (H1, …) cited in the README resolve here; excerpts are in `source-evidence.txt`.

## Source reference key

Upstream links below are immutable pins; local Bistro links refer to this checkout's existing files.

- **K1**: WebKit [`WebViewImpl.mm:6819–6854`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/UIProcess/mac/WebViewImpl.mm#L6819-L6854), keyUp/keyDown.
- **K2**: [`WebViewImpl.mm:6782–6816`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/UIProcess/mac/WebViewImpl.mm#L6782-L6816), performKeyEquivalent; [`5870–5886`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/UIProcess/mac/WebViewImpl.mm#L5870-L5886), NSApp resend.
- **K3**: [`WebViewImpl.mm:5915–5940`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/UIProcess/mac/WebViewImpl.mm#L5915-L5940), collectKeyboardLayoutCommandsForEvent / interpretKeyEvents.
- **K4**: [`PlatformEventFactoryMac.mm:259–349`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebCore/platform/mac/PlatformEventFactoryMac.mm#L259-L349), logical key / physical code; [`WebEventFactory.mm:324–339`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/Shared/mac/WebEventFactory.mm#L324-L339), event serialization.
- **K5**: [`WebViewImpl.mm:6085–6100`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/UIProcess/mac/WebViewImpl.mm#L6085-L6100), doCommandBySelector; [`WebPageMac.mm:270–360`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/WebProcess/WebPage/mac/WebPageMac.mm#L270-L360), selector/editor execution.
- **K6**: [`WebViewImpl.mm:5942–6079`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/UIProcess/mac/WebViewImpl.mm#L5942-L6079), asynchronous IME ordering / composition handling; [`6560–6635`](https://github.com/WebKit/WebKit/blob/e34989160bcd4d4f2cad8fe879118d0402fff434/Source/WebKit/UIProcess/mac/WebViewImpl.mm#L6560-L6635), setMarkedText.
- **W1**: Wails [`webview_window_darwin.go:139–210`](https://github.com/wailsapp/wails/blob/7b11001b92b712210b27d8cb53430e83f67ad8e4/v3/pkg/application/webview_window_darwin.go#L139-L210), ordinary WKWebView creation.
- **W2**: [`webview_window_darwin.m:45–106`](https://github.com/wailsapp/wails/blob/7b11001b92b712210b27d8cb53430e83f67ad8e4/v3/pkg/application/webview_window_darwin.m#L45-L106), NSWindow accelerators/performKeyEquivalent.
- **W3**: [`menuitem_darwin.go:28–44`](https://github.com/wailsapp/wails/blob/7b11001b92b712210b27d8cb53430e83f67ad8e4/v3/pkg/application/menuitem_darwin.go#L28-L44), responder-chain selector targets; [`318–324`](https://github.com/wailsapp/wails/blob/7b11001b92b712210b27d8cb53430e83f67ad8e4/v3/pkg/application/menuitem_darwin.go#L318-L324), native equivalents; [`roles.go:115–136`](https://github.com/wailsapp/wails/blob/7b11001b92b712210b27d8cb53430e83f67ad8e4/v3/pkg/application/roles.go#L115-L136), Edit role menu.
- **H1**: installed `node_modules/@tanstack/hotkeys/dist/_match.js:7–44` and `_keyboard-event.js:5–42`, version **0.11.0** (matching source captured in `source-evidence.txt`).
- **H2**: installed `hotkey-manager.js:263–285`, `sequence-manager.js:252–280`; no blanket composition rejection beyond matcher. Bistro `keybindingActivation.ts:185` avoids tagging composition as sequence continuation.
- **B1**: `electron/src/renderer/utils/keymaps.ts:31–301`; [binding inventory](bindings-by-class.md).
- **B2**: `electron/src/renderer/utils/keybindingActivation.ts:95–118`, `hooks/useAppHotkeys.ts:174–215` (ignoreInputs and registration).
- **B3**: `electron/src/renderer/utils/keybindings.ts:48–56`, `components/CommentInput.tsx:27–38`, `components/CommentThread.tsx:205–211` (local textarea submit).

## Transport T1 / T2 / T6 (source only; no measurements)

- **T1 — binding calls.**
  - Path: the runtime JS does a `fetch` POST of JSON to the app's asset handler (chunked above 512 KiB). The WKURLSchemeHandler in the UI process hands it to Go over cgo (`webviewRequests`, buffer 256, `application.go:413`). Each request runs in its own goroutine (`714–715`) via the queueing dispatcher (`assetserver_webview.go:75–100`). The response is `json.Marshal`ed (`transport_http.go:370–388`).
  - `messageprocessor_call.go:126–131` marshals the args and the result **on every call**, only to log them at debug level, whatever the log level.
  - The WS baseline also does JSON plus a cross-process hop, so a ≥2× p50 win is not evident from source.
  - Prediction: **T1 uncertain, leaning FAIL** on `p50 ≤ 0.5× WS`.
- **T2 — push events.**
  - Each event becomes an `ExecJS` / evaluateJavaScript chained on `__eq` (`webview_window.go:1430–1478`). Large payloads go by reference through `/wails/eventpayload/` fetch (TTL store, 64 MiB cap).
  - `eventQueueCapacity = 64` applies backpressure to goroutine emitters only. The UI-thread drain still makes one JS evaluation, and one IPC round, per event (`1480–1587`). There is no batching.
  - Prediction for a 1000-event burst: **T2 leaning FAIL**, low confidence.
- **T6 — origin restriction.**
  - Outside server mode, `checkRuntimeRequest` always returns true (`transport_http_notserver.go:7–12`). The only explicit origin check, `authorizeRuntimeOrigin`, applies to server mode.
  - What is left: the asset server rejects a host other than `ExpectedWebViewHost` unless the URI starts with `wails://` (`assetserver_webview.go:202–211`); `wails:`-prefixed messages from child frames are ignored (`application.go:864–876`); and macOS has no `decidePolicyForNavigationAction`, so top-level navigation is not restricted.
  - Whether a foreign origin can reach the `wails://` handler depends on WebKit scheme and CORS policy that was not traced.
  - Result: **T6 not demonstrated.** It needs an explicit test: a cross-origin fetch to the runtime endpoint plus a navigation test. It must not be recorded as a pass.

