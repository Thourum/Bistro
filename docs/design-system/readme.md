# Bistro Design System

**The keyboard-first code review client for the AI-PR era — distilled into a reusable design language.**

Bistro is a local-first desktop app (Electron + a Go sync engine) that syncs your GitHub pull requests into a blazing-fast inbox, generates diffs on your machine, and puts every action one keystroke away. This project is the design system extracted from that product: tokens, components, foundation specimens, and a full interactive UI kit.

> Built by **Samuel Asvanyi** at **Itterno ApS**. © 2026 Itterno ApS.

---

## Sources

This system was reconstructed from the materials the author provided. Keep these references; do not assume the reader has access.

- **Codebase** (`bistro/`, read-only mount) — the canonical source of truth.
  - `electron/src/renderer/` — the React renderer. The token system lives in `styles/globals.css` (`@theme` block) and `utils/design-tokens.ts`. Composed components (`PRListTable`, `CommandPalette`, `AppFooter`, `StatusSidebar`, `LoginPage`, `HelpOverlay`) define the real visual language. Note: the `components/ui/*` shadcn primitives are **unmodified defaults** and do NOT reflect the brand — ignore them.
  - `design/ui-ux-principles.md` — the interaction philosophy (keyboard-first, density over delight, scanability before aesthetics). Describes a dark default; the shipped/designed direction is the **Minimal Light** theme.
  - `website/PLAN.md` — marketing site plan (not yet built).
  - `go/`, `docs/` — sync engine + roadmap.
- **Figma: "Bistro.fig"** — 7 polished "Minimal Light" frames on Page-1: `Bistro-Login`, `PR-Inbox-Minimal-Light`, `PR-Overview-Minimal-Refined`, `Diff-View-Minimal-Light`, `Command-Palette`, `Settings-Minimal`, `Shortcut-Guide`. The file is screenshot-style (0 reusable components, 0 Figma Variable collections) — used as visual reference, not materialized. Fonts in-file: JetBrains Mono (215×), Geist (55×).

The figma carries **no component families or token collections** to materialize (it is a frames-only mock), so components here are authored from the codebase patterns and tokens are lifted verbatim from `globals.css`.

---

## What's in here (manifest)

- **`styles.css`** (root) — the one entry point consumers link. `@import`s only.
- **`tokens/`** — `colors.css`, `typography.css`, `spacing.css`, `radius.css` (radius + elevation), `fonts.css` (webfonts + Material Symbols), `base.css` (reset). All `--*` custom properties, light + dark (`.dark`) modes.
- **`components/`** — reusable React primitives, grouped:
  - `brand/` — BistroMark (the logo symbol), BistroWordmark, BistroLockup
  - `core/` — Button, IconButton, Kbd, Eyebrow, StatusDot, Badge, Avatar
  - `forms/` — Input, Checkbox, Switch, SegmentedControl
  - `layout/` — Panel, Toolbar, SystemBar
  - `review/` — PRRow, DiffLine, CommentCard  *(domain primitives)*
  - `feedback/` — CommandRow, Toast, LineFocusChip
- **`guidelines/`** — foundation specimen cards (Colors, Type, Spacing, Brand).
- **`ui_kits/bistro-app/`** — the full interactive client: login → inbox → PR overview → diff → settings, with command palette (`:`) and help (`?`).
- **`SKILL.md`** — Agent-Skill manifest for using this system in Claude Code.

Namespace for the compiled bundle: `window.BistroDesignSystem_8e22c3`.

---

## CONTENT FUNDAMENTALS — how Bistro writes

Bistro's voice is a **terminal operator's log**: terse, technical, lower-case in prose but SCREAMING_SNAKE in chrome. It never sells; it reports.

- **Two registers, sharply split.**
  - *Product prose* (descriptions, settings help, marketing) is plain, confident, lower-case sentence case: "Review pull requests at the speed of thought." "Your code and tokens never touch anyone's server, because there isn't one." Calm, declarative, a little opinionated.
  - *System chrome* (labels, status, legends) is `UPPERCASE_MONO`, often with underscores and `//` separators: `SYSTEM_STATUS: OPERATIONAL // V.01`, `OPEN PULL REQUESTS`, `FILES_CHANGED (12)`, `LINE_FOCUS: 44`, `ACTIVE_DRAFT`, `TYPE_COMMENT_HERE…`, `IS:OPEN AUTHOR:ME`, `NEWEST_FIRST`. This is the "machine talking" voice.
- **You, not we.** Product copy addresses the developer directly: "Connect your platform to begin reviewing code." "Press `?` anytime to see what your keyboard can do." First person is reserved for inline review comments (human-to-human): "Good catch, will fix."
- **Keys are nouns.** Shortcuts are written inline as part of the sentence and always boxed: press `j`/`k` to move, `a a` to approve, `:` for the command palette. Multi-key combos use spaces (`g g`), modifiers use `+` or `⌘`.
- **Numbers are facts, never decoration.** `+842 / -156`, `45/45`, `2m ago`, `12 files`. Counts and diffs only; no vanity metrics.
- **Casing rules:** Title Case never appears. Headings are either large Geist sentence case ("Settings", a PR title) or UPPERCASE MONO eyebrows.
- **Punctuation:** `//` as a separator/comment marker, `:` for key/value, `/` for paths and breadcrumbs, `…` (ellipsis) for in-progress/placeholder, `→` for branch merges (`feature/x → main`).
- **No emoji. No exclamation.** Status is shown with colored squares + a word, never an emoji. Tone is unexcitable: "PR approved." not "Approved! 🎉".
- **Versioning is worn proudly:** `v4.0.2-stable`, `BISTRO // V.01`, `// TECHNICAL ATELIER © 2026` in the footer chrome.

---

## VISUAL FOUNDATIONS

The aesthetic is **technical-minimal / terminal-light**: a near-white drafting desk, hard square edges, one blue accent, and monospace everywhere a machine would speak. It reads like an IDE that went to design school.

- **Palette.** Near-white surfaces (`#f8f9fa` desk; `#ffffff`→`#e1e3e4` container ladder). One accent: **Bistro blue `#004ac6`** — used *functionally* (selection, links, active state, CTAs), never as decoration. Text is near-black `#191c1d`; metadata is gray `#737686`. Color is otherwise reserved for status: green `#22c55e` (pass), amber `#eab308` (running), red `#ba1a1a` (fail / changes requested). Diff gutters use GitHub-style green/red wash.
- **Typography.** Two families, maximum contrast: **Geist** for UI + display (big headings carry tight negative tracking, −0.03 to −0.05em) and **JetBrains Mono** for code, metadata, and all chrome (UPPERCASE, wide `0.15em` tracking). Dense scale: 9–12px mono chrome, 14px body, 24/48/60px display. The wordmark is Geist 800, uppercase, tight — purely typographic, no logomark.
- **Zero-radius architecture.** *Every* corner is square. `--radius-*` all resolve to `0px`. Buttons, cards, inputs, avatars, status dots, switches — all hard-edged. This is the single most identifying trait of the brand.
- **Borders & elevation.** Depth is built from **1px hairlines** (`--color-outline-variant #c3c6d7`) plus the surface-container ladder — not shadow. There is exactly **one** shadow in the system, `0 10px 30px rgba(25,28,29,0.04)`, used only to float overlays (command palette, toasts, menus). Everything else is flat.
- **Backgrounds.** Flat color only. No gradients, no images, no textures, no patterns. The command palette **blurs** the app behind it (20px backdrop blur over a translucent wash) rather than dimming it; modal dialogs use a `rgba(0,0,0,0.6)` scrim.
- **Layout.** Fixed structural chrome: a top Toolbar (56px, hairline underline) and a bottom **SystemBar** (32px) that's always present — left keyboard legend, center status, right `SYSTEM_STATUS`. Content scrolls between them. Three-pane review layout (list / detail / status rail). Generous content maxima (1152px settings column) but dense rows (12–16px padding, not 24–32).
- **Selection & focus.** The keyboard cursor is sacred and always visible: selected rows get a **2px primary left-accent** + a tinted fill (`surface-container-low`). Focus rings are 2px primary, offset, never removed.
- **Cursor.** Interactive rows use `cursor: crosshair` — a deliberate technical tell that this is a precision tool, not a webpage.
- **Hover.** Subtle and instant: background steps one rung up the container ladder; the left-accent border appears in primary. Primary buttons darken to `#003ea8`. Danger buttons invert to a red fill.
- **Press / active.** No shrink, no bounce. State changes are color-only.
- **Motion.** Minimal and functional only. \~120ms color transitions; the running status square pulses (opacity 1↔0.35, 1.4s); 140ms switch thumb slide. No spinners-as-celebration, no parallax, no entrance choreography. "Keep focus on content, reduce cognitive load."
- **Status signals.** Always a hard **square** (never a circle/dot) + a word. `w-3 h-3` in lists, `w-2 h-2` in the system bar.
- **Imagery.** None. The product ships no illustrations or photography; the "imagery" is code, diffs, and chrome. Avatars are square initial tiles, usually the Bistro-blue identity color.

### Logo

The mark is a **3D card** (a terminal window lifting off the page) holding a Geist **"B"** with Square corners and an angular drop shadow to the bottom right. The **wordmark** is the established Geist 800 uppercase "BISTRO" (tight tracking) — the mark's B is the same letterform, so mark and wordmark agree. Lock them together with `BistroLockup` (horizontal for bars, stacked for splash). The mark file is `assets/bistro-mark.svg` and also serves as the favicon/app icon. Variants: `default` (white card), `inverse` (dark card, for dark backgrounds), `mono` (single-color). Never set the wordmark in monospace.

---

## ICONOGRAPHY

- **Material Symbols Outlined** is the icon system, inherited from the app's Google-Stitch origins and loaded from the Google Fonts CDN (see `tokens/fonts.css`). Use the `.material-symbols-outlined` helper class with a ligature name: `<span class="material-symbols-outlined">search</span>`. Default axes: `FILL 0, wght 400, GRAD 0, opsz 24` (outline weight, unfilled).
- **Common glyphs in use:** `search`, `settings`, `sync` / `autorenew`, `filter_alt`, `keyboard`, `rebase`, `description`, `person`, `check`, `restart_alt`, `terminal` (GitHub), `folder_open` (GitLab), `local_cafe` (Gitea), `keyboard_arrow_up/down`, `keyboard_command_key`.
- **No emoji, ever.** Status uses colored squares + a label, not emoji or unicode symbols — with two deliberate exceptions used as *typographic* marks in chrome: `↩` (enter/return) and `→` (branch merge). These are set in JetBrains Mono, not an icon font.
- **No hand-drawn SVG.** Icons come from the Material Symbols font; the brand has no bespoke icon set and no pictorial logo. Provider "logos" on the login screen are Material Symbols, not brand marks.
- **CDN substitution flag:** Material Symbols loads from Google Fonts (matching the app exactly). For a fully offline build, self-host the variable font.

---

## Font substitution note

Geist and JetBrains Mono load from the **Google Fonts CDN** (`tokens/fonts.css`). The original app bundles no font binaries — it relies on the same CDN plus system fallbacks. If you need an offline build, swap the `@import`s for self-hosted woff2 files and add `@font-face` rules. **Flag to the user:** confirm whether self-hosted Geist / JetBrains Mono binaries should be added.

---

## Using the system

Consumers link one file and read components off the namespace:

```html
<link rel="stylesheet" href="styles.css" />
<script src="_ds_bundle.js"></script>
<script>
  const { Button, PRRow, SystemBar } = window.BistroDesignSystem_8e22c3;
</script>
```

Every component is square, mono-forward, and self-contained (it references CSS custom properties only). See each component's `.prompt.md` for usage, and the Design System tab for live specimens.
