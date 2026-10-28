# Gate 3 — upstream source read (2026-10-27)

Split for the 5k-token file limit: [`source-trace.md`](source-trace.md) holds the source reference key (H1, …) and the transport T1/T2/T6 trace; [`s7-health.md`](s7-health.md) holds the S7 upstream-health figures and the keyboard issue table.

Research only: no app launch, key injection, shell benchmarks, or transport measurements were performed. Predictions below were written before any testing in this task. They are not S6 results and must not be substituted for the preregistered matrix in [ADR 0001](../../../0001-electron-vs-wails.md).

## PREDICTION — S6 against Electron

Assumption: same Bistro renderer, installed Hotkeys matcher and activation rules; ordinary Wails WKWebView; **no duplication of renderer hotkeys as native Wails KeyBindings**; native menu roles configured to match the Electron baseline. Changing these assumptions requires a new prediction, not retroactive editing after observations. Classes overlap; PASS means predicted within the ADR's class tolerance, not experimentally demonstrated parity. Injection and layout effects remain unmeasured.

| S6 class | Prediction | Source-based reason / falsifier |
|---|---|---|
| Plain outside text fields (0% failures) | **PASS**, moderate confidence | WKWebView forwards keyDown/keyUp into WebCore; logical key and physical code are both carried. Installed matcher accepts the same logical keys in either renderer. Falsify with any plain-key action/text mismatch, including Shift-only and navigation keys. [K1–K4, H1] |
| Cmd (0%) | **PASS**, low confidence, menu-dependent | WebKit's `performKeyEquivalent` explicitly offers modified keys to web content when the view is first responder, then resends unhandled keyDown through NSApp. It is **not** a blanket “menus eat all Cmd first” path. Wails native KeyBindings do preempt it when configured; duplicate accelerators or different Edit/Application menus would falsify this prediction. [K2, W2–W3] |
| Ctrl (0%) | **FAIL**, moderate confidence | Wails' own NSWindow override documents WKWebView silently consuming Ctrl+Tab; the override repairs only registered native KeyBindings, not renderer-only TanStack bindings. Standard `detail.nextFile` / `detail.prevFile` use Ctrl+Tab / Ctrl+Shift+Tab, making this a core-action risk. Ctrl editing selectors inside editable content are an additional parity risk. [W2, B1, K5] |
| Option on US (0%) | **FAIL**, low confidence | Emacs uses Alt+N (macOS dead-key territory) and other Option letters. WebKit interprets layout/IME commands before constructing keyboard events; Option-generated text and marked-text paths are not raw letters. Hotkeys has a physical fallback but not a guarantee that an actionable, non-composing event reaches it. One divergent Alt+N cell is enough to fail. [K3–K6, H1, B1] |
| Option on DE/AZERTY/Dvorak (≤5%, rebindable only) | **FAIL**, low confidence | Option's logical output/dead keys depend on layout; Hotkeys' compatibility fallback uses conventional physical positions, while ASCII/Latin mismatch rules selectively prohibit fallback. No source evidence establishes ≤5% divergence or a working rebind for every failing cell. Settings rebinding is a mitigation to verify, not proof of this gate. [K4, H1] |
| Multi-key sequences (0%) | **PASS**, low confidence | Sequence state/timeouts are renderer-owned and unchanged; each step uses the same matcher. Outside inputs the WebKit command queue does not itself consume arbitrary sequences. Emacs Ctrl prefixes, Option-prefixed sequences and IME transitions are the highest-risk falsifiers; overlapping Ctrl/Option class predictions do not exempt sequence cells. [H1–H2, B1–B2, K5] |
| Inside text fields, any modifier (0%) | **PASS**, low confidence for inactive IME | Bistro ignores normal global/sequence bindings in inputs, leaving native editing; local Cmd-or-Ctrl+Enter submits. WebKit executes collected AppKit selectors on editable content. Differences in Ctrl movement/deletion, dead-key insertion or menu roles falsify parity. IME-active cells are covered separately below. [B2–B3, K5–K6, W3] |
| IME active (0%; any action firing mid-composition fails in either shell) | **FAIL / baseline risk**, moderate confidence | Hotkeys suppresses printable/explicit-code matches while composing, not all named keys. Bistro's textarea-submit helper and CommentInput/CommentThread callers lack an `isComposing` check. Cmd/Ctrl+Enter or overlay Escape during marked text may fire in either renderer; identical unsafe firing still fails the ADR's explicit IME rule. WebKit has substantial IME ordering protection, but it cannot supply missing application guards. [H1–H2, B3, K6] |

These are deliberately falsifiable forecasts. An upstream comment about Ctrl+Tab is not an observed Bistro failure; Option/IME risks are not measurements. No percentage of failing cells is inferred from source code.

## Pins and reproduction

| Upstream | Pin | Notes |
|---|---|---|
| Wails | tag `v3.0.0` = `7b11001b92b712210b27d8cb53430e83f67ad8e4` | Published 2026-10-26T11:15:13Z, not a prerelease ([health-pin.json](health-pin.json)). Health counts use the `master` snapshot `c40713d9523ec24a90aac28aa9cf9faac2ab384f`. |
| WebKit | `e34989160bcd4d4f2cad8fe879118d0402fff434` | WebKit `main`, committed 2026-10-27T12:02:05Z ([health-webkit-pin.json](health-webkit-pin.json)). **This is not the WebKit that ships with macOS.** System WebKit on the macOS 13 floor and on current macOS is an older Safari branch; the key paths read here are long-standing, but line-level behaviour can differ. |
| @tanstack/hotkeys | 0.11.0, installed in repo-root `node_modules` | Excerpts in [source-evidence.txt](source-evidence.txt). |

Clones live outside the repo (`~/.cache/bistro-adr/`); nothing from them was executed.

```sh
git clone --depth 1 --branch v3.0.0 https://github.com/wailsapp/wails ~/.cache/bistro-adr/wails-v3.0.0
git -C ~/.cache/bistro-adr/wails-v3.0.0 rev-parse HEAD   # 7b11001b92b712210b27d8cb53430e83f67ad8e4
git clone --filter=blob:none --no-checkout https://github.com/WebKit/WebKit ~/.cache/bistro-adr/webkit-e34989160bcd
cd ~/.cache/bistro-adr/webkit-e34989160bcd && git sparse-checkout set --no-cone \
  Source/WebKit/UIProcess/mac/WebViewImpl.mm Source/WebKit/UIProcess/API/Cocoa/WKWebView.mm \
  Source/WebKit/UIProcess/API/mac/WKWebViewMac.mm Source/WebKit/UIProcess/API/mac/WKView.mm \
  Source/WebKit/Shared/mac/WebEventFactory.mm Source/WebKit/Shared/mac/NativeWebKeyboardEventMac.mm \
  Source/WebKit/WebProcess/WebPage/mac/WebPageMac.mm Source/WebKit/WebProcess/WebPage/WebPage.cpp \
  Source/WebKit/UIProcess/mac/WebPageProxyMac.mm Source/WebCore/platform/mac/PlatformEventFactoryMac.mm \
  Source/WebCore/platform/mac/PlatformKeyboardEventMac.mm Source/WebCore/platform/mac/KeyEventMac.mm \
  Source/WebCore/editing/mac/EditorMac.mm
git checkout e34989160bcd4d4f2cad8fe879118d0402fff434
sh docs/adr/data/0001/gate3/health.sh          # from the Bistro repo root; read-only gh calls
node docs/adr/data/0001/gate3/bindings-by-class.mjs
```

## macOS keyboard path (source trace)

Cocoa order, per Apple's [Handling Key Events](https://developer.apple.com/library/archive/documentation/Cocoa/Conceptual/EventOverview/HandlingKeyEvents/HandlingKeyEvents.html): `NSApplication sendEvent:` sends events with modifier flags to the key window's `performKeyEquivalent:`. The window offers them down the view hierarchy. **Only if no view returns YES** does NSApp offer them to the main menu. Everything else goes to the first responder's `keyDown:`, where text views call `interpretKeyEvents:`. The text system maps keys through the key-bindings dictionary to `doCommandBySelector:` selectors (`moveUp:`, `deleteBackward:`, Emacs-style Ctrl editing) or to `insertText:` / marked text.

1. **Wails NSWindow (W2).** `performKeyEquivalent:` first calls `processWindowKeyEquivalent`. It returns YES only for a registered Wails KeyBinding/accelerator, so the renderer never sees that key. Otherwise it calls `super`, which follows the normal Cocoa path above. The source comment says the override exists because WKWebView silently consumes Ctrl+Tab. The fix covers native KeyBindings only.
2. **WKWebView (K2).** `WKWebViewMac.mm:623–645` forwards `doCommandBySelector`, `performKeyEquivalent`, `keyUp` and `keyDown` to `WebViewImpl`. `WebViewImpl::performKeyEquivalent` offers modified keys to WebCore when the web view is first responder, returns YES, and has an Escape special case. If the page does not `preventDefault`, the event is resent through NSApp (`5870–5886`), so main-menu equivalents run **after** the page. Cmd shortcuts therefore reach JS before menus, unless a Wails KeyBinding matches first. Electron/Chromium also lets the page see Cmd before the menu, so the Cmd class depends on the menus matching (W3).
3. **Text system (K3, K5).** Before building the DOM event, WebKit runs `interpretKeyEvents` to collect AppKit commands. In editable content it executes them as editor commands; outside editable content they are mostly no-ops. This is where Ctrl editing (Ctrl+A/E/K/N/P…) can diverge from Chromium in text fields.
4. **DOM KeyboardEvent (K4).** `key` comes from `characters`. When Ctrl is down it comes from `charactersIgnoringModifiers` instead. An empty dead-key result becomes `"Dead"`. `code` comes from the physical key code. Option+letter therefore yields the layout's Option character (US Option+N → `"Dead"`), not `"n"`. Hotkeys' physical-code fallback is the only route to an `Alt+N` match (H1).
5. **IME (K6).** WebKit queues key events behind marked-text changes, and `isComposing` / `key: "Process"` reach the page. Hotkeys treats `Process` / `Unidentified` / `isComposing` as composing. It refuses printable and code-based matches while composing, but named keys like Enter and Escape can still match (H1–H2).
6. **Wails menus (W3).** Role items such as Undo, Redo, Cut, Copy, Paste and Select All target `nil`, so they travel the responder chain into WKWebView's editing commands. Custom items target their own handler. Key equivalents are built from US virtual-key names (`accelerator_darwin.go:26–104`). Upstream #4918 (v2) shows that clipboard shortcuts die in inputs without an Edit menu. The workaround is the explicit Edit role menu (`roles.go:115–136`).

Not traced: Fn/Globe (`flagsChanged` filters Fn, `6860–6870`; see #6033), and how input sources behave inside the WebContent process. No source line proves what happens on any specific layout; that is Gate 2's job.

## Binding inventory

[bindings-by-class.md](bindings-by-class.md) / [bindings-by-class.json](bindings-by-class.json) (generated by `bindings-by-class.mjs`, text parsing only):

| Class | vim | emacs | standard |
|---|---:|---:|---:|
| Plain | 83 | 38 | 80 |
| Cmd | 3 | 2 | 14 |
| Ctrl | 4 | 60 | 4 |
| Option | 1 | 24 | 1 |
| Sequences | 18 | 36 | 10 |
| Shift-only | 12 | 1 | 6 |
| Total bound | 121 | 123 | 115 |

Classes overlap. Subtract `global.quit.win` (Alt+F4) on macOS (`HelpOverlay.tsx:110` selects the platform's quit). Add the textarea-local `Mod+Enter` submit in every style.

## Limitations

- Source reading only. Every S6 / T verdict above is a prediction. Gate 2 measurements replace it.
- The WebKit pin is today's `main`, not the system WebKit on any supported macOS. Chromium was not traced; Electron behaviour is taken as the baseline the ADR measures against.
- The analysis assumes Bistro registers no native Wails KeyBindings. Any native duplication changes the Cmd and Ctrl predictions.
- The S7 response metric is sensitive to how automated responders are classified. The direction (FAIL) is stable; the magnitude is not.
- Issue searches use GitHub search over titles and bodies (`issues-*.json`). Relevant issues phrased differently may be missed.
