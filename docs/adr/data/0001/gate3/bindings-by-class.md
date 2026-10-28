# Binding inventory (no key injection)

Reproduce: `node docs/adr/data/0001/gate3/bindings-by-class.mjs` from the repository root. Reads files as text; imports/evaluates no application or dependency code. Fixed source-table grammar is asserted; generated provider, PR-jump and lane entries are expanded using source constants. Counts are actionIds, not distinct chords or active-route counts. User overrides are absent.

| Class | vim | emacs | standard |
|---|---:|---:|---:|
| plain, no modifier (single step) | 83 | 38 | 80 |
| Cmd / Mod on macOS (any step) | 3 | 2 | 14 |
| Ctrl (any step) | 4 | 60 | 4 |
| Option / Alt (any step) | 1 | 24 | 1 |
| multi-key sequences | 18 | 36 | 10 |
| Shift-only single step (additional coverage) | 12 | 1 | 6 |
| All bound actionIds | 121 | 123 | 115 |

Classes overlap: Ctrl+Alt belongs to both Ctrl and Option; every modified sequence belongs to its modifier class and sequences. The plain row excludes Shift-only and sequences; both are still covered by S6. Option US and DE/AZERTY/Dvorak use the same Option inventory. Every bound actionId is a focus/IME test candidate, including expected non-firing cells. `global.quit.win` (Alt+F4) is in the source table for all styles but platform-disabled on macOS: subtract one from Option and total for the macOS-active inventory. Hidden arrow aliases are counted because they remain registered.

Inside comment textarea: normal global bindings/sequences use ignoreInputs; Escape (plus emacs C-g for active overlays) can bypass this. Local `TEXTAREA_SUBMIT_HOTKEY = Mod+Enter` is one additional binding in every style (function accepts Cmd **or Ctrl**+Enter), not in ACTION_CATALOG. Sandbox-only Shift+Space is not a comment-textarea binding. IME-active is a cross-cutting context, not a separate set of chords.

Full actionId/chord membership: [bindings-by-class.json](bindings-by-class.json).
