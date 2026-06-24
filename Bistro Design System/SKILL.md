---
name: bistro-design
description: Use this skill to generate well-branded interfaces and assets for Bistro, the keyboard-first code review client, either for production or throwaway prototypes/mocks/etc. Contains essential design guidelines, colors, type, fonts, assets, and UI kit components for prototyping.
user-invocable: true
---

Read the `readme.md` file within this skill, and explore the other available
files. It is the design guide and manifest — CONTENT FUNDAMENTALS, VISUAL
FOUNDATIONS, and ICONOGRAPHY sections cover voice, the technical-minimal /
terminal-light aesthetic, the zero-radius rule, and the Material Symbols icon
system.

Key facts to internalize before designing:
- **Zero-radius architecture** — every corner is square. No exceptions.
- **Two fonts, max contrast** — Geist (UI/display, tight tracking) + JetBrains
  Mono (code + all UPPERCASE chrome, wide tracking).
- **One accent** — Bistro blue `#004ac6`, used functionally only. Color is
  otherwise reserved for status (green/amber/red squares).
- **Flat** — depth from 1px hairlines + the surface ladder, not shadow; exactly
  one ambient shadow exists, for overlays. No gradients, images, or emoji.
- **Keyboard-first** — keys are boxed `<kbd>`, every action has a binding, the
  bottom SystemBar and the `cursor: crosshair` on rows are core furniture.

Where things live:
- `styles.css` — the single CSS entry point (link this). Tokens under `tokens/`.
- `components/<group>/` — reusable React primitives (`.jsx` + `.d.ts` +
  `.prompt.md`). Read the `.prompt.md` for each component's usage.
- `guidelines/` — foundation specimen cards.
- `ui_kits/bistro-app/` — full interactive client recreation (login, inbox, PR
  overview, diff, settings, command palette, help).

How to work:
- If creating visual artifacts (slides, mocks, throwaway prototypes), copy assets
  out and create static HTML files for the user to view. Link `styles.css`, load
  the compiled bundle, and read components from `window.BistroDesignSystem_8e22c3`.
- If working on production code, copy assets and read the rules here to become an
  expert in designing with this brand. The original codebase
  (`electron/src/renderer/`) is the canonical reference.
- Icons: use Material Symbols Outlined (ligature names). Never hand-draw SVG
  icons or use emoji.

If the user invokes this skill without any other guidance, ask them what they
want to build or design, ask a few clarifying questions, and act as an expert
Bistro designer who outputs HTML artifacts _or_ production code, depending on the
need.
