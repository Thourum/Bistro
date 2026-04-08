# Bistro UI Implementation — Execution Summary

**Status:** Plan complete and ready for execution
**Date:** 2026-04-08
**Component Approach:** shadcn/ui + Design System Overrides

---

## What's Ready

### ✅ Two Comprehensive Plans Created

**Location 1:** `/Users/samuelasvanyi/dev/bistro/docs/superpowers/plans/2026-04-08-ui-implementation-design-system.md`
- Main UI implementation plan with 9 complete tasks
- Each task has step-by-step instructions with code examples
- Covers all screens: login, inbox, PR detail, diff viewer, command palette, settings, help, filter

**Location 2:** `/Users/samuelasvanyi/dev/bistro/docs/superpowers/plans/2026-04-08-shadcn-ui-integration.md`
- Supplementary plan showing how to use shadcn/ui instead of custom components
- Pre-built component examples: Button, Dialog, Table, Tabs, Badge, DropdownMenu
- Global CSS overrides to enforce zero-radius design system
- Component code ready to copy-paste

### ✅ Design System Foundation

- **Tailwind config** with custom colors, fonts, border-radius overrides
- **Design tokens** (CSS variables + TypeScript exports)
- **Layout components** (AuthLayout, MainLayout, SettingsLayout)
- **Typography system** (Geist + JetBrains Mono)
- **Color palette** (clinical Swiss style, zero-radius, tonal boundaries)

### ✅ All 8 Screen Mockups Analyzed

From `/Users/samuelasvanyi/dev/bistro/electron/stitch_terminal_authentic_prd/`:
1. **bistro_login/** — OAuth provider buttons with keyboard hints
2. **pr_inbox_minimal_light/** — Table layout with state indicators
3. **pr_overview_minimal_refined/** — PR detail view
4. **diff_view_minimal_light/** — Syntax-highlighted diff
5. **command_palette/** — Command search overlay
6. **settings_minimal/** — Configuration tabs
7. **shortcut_guide/** — Help overlay
8. **bistro_systematic/DESIGN.md** — Complete design system documentation

---

## Design System Summary

### Colors (Tailwind Theme)
```
Primary:          #004ac6 (Blue)
Primary Container: #2563eb (Bright Blue)
Surface:          #f8f9fa (Lightest)
Surface variants: #ffffff → #edeeef → #e7e8e9 (Hierarchy)
Outline:          #737686 (Gray)
Error:            #ba1a1a (Red)
```

### Typography
- **Geist:** All UI text, headings, labels
- **JetBrains Mono:** PR numbers, timestamps, metrics, IDs
- **Material Symbols:** All icons (no emoji)

### Signature Aesthetic
- **Zero-radius:** All border-radius = 0px (no rounded corners)
- **Tonal boundaries:** Color shifts instead of 1px borders
- **Ambient shadows:** Soft diffuse only (0 10px 30px rgba(25,28,29,0.04))
- **No gradients:** Flat, clinical look (except button CTAs which have 45° gradient)

---

## Next Steps: Execution Options

### Option 1: Subagent-Driven Development (Recommended)
Use `/Users/samuelasvanyi/dev/bistro/docs/superpowers/plans/2026-04-08-ui-implementation-design-system.md`

- Fresh subagent per task
- Two-stage review (spec compliance + code quality)
- Expected timeline: 3-4 days
- Start: Task 1 (Design system foundation)

**Command:**
```
Use the plan at /Users/samuelasvanyi/dev/bistro/docs/superpowers/plans/2026-04-08-ui-implementation-design-system.md with shadcn/ui components per /Users/samuelasvanyi/dev/bistro/docs/superpowers/plans/2026-04-08-shadcn-ui-integration.md
```

### Option 2: Quick Manual Execution
Install shadcn/ui and copy-paste the component examples from the supplementary plan.

---

## Integration with Existing Code

### Keyboard Navigation (Phase 3)
✅ Already implemented: 25 bindings in `src/renderer/hooks/useKeyboard.ts`

**UI Tasks will wire these bindings:**
- `j/k` → PR list up/down
- `Enter` → Open PR detail
- `/` → Focus search
- `f` → Toggle filter menu
- `:` → Open command palette
- `?` → Show help overlay
- `esc` → Close overlays / go back

### TanStack Query (Phase 2)
✅ Already implemented: `usePRList` hook in `src/renderer/queries/usePRList.ts`

**UI Tasks will display:**
- PR list from `usePRList` query
- PR details from `usePRDetail` hook (to be created)
- Search filtering with FTS5

### Store (Phase 2)
✅ Already implemented: TanStack Store with state management

**UI Tasks will update:**
- `selectedPRIndex` (keyboard navigation)
- `mode` (list/detail/search/help/command)
- `searchQuery` (search input)
- `filterState` (filter toggles)

---

## Installation Commands Ready

```bash
cd /Users/samuelasvanyi/dev/bistro/electron

# 1. Setup shadcn/ui
npx shadcn-ui@latest init -d

# 2. Add components as tasks require them
npx shadcn-ui@latest add button
npx shadcn-ui@latest add card
npx shadcn-ui@latest add dialog
npx shadcn-ui@latest add table
npx shadcn-ui@latest add tabs
npx shadcn-ui@latest add badge
npx shadcn-ui@latest add dropdown-menu
npx shadcn-ui@latest add checkbox
npx shadcn-ui@latest add input
npx shadcn-ui@latest add scroll-area

# 3. Build
pnpm build

# 4. Type check
pnpm typecheck

# 5. Run tests
pnpm test
```

---

## Files to Create/Modify

### New Design System Files
```
electron/src/renderer/styles/
├── design-tokens.css      (CSS variables)
├── shadcn-overrides.css   (enforce zero-radius)
└── globals.css            (updated imports)

electron/src/renderer/utils/
├── design-tokens.ts       (TypeScript exports)
└── diff-parser.ts         (diff parsing utility)

electron/src/renderer/layouts/
├── AuthLayout.tsx
├── MainLayout.tsx
└── SettingsLayout.tsx

electron/tailwind.config.ts (custom theme)
```

### New Page & Component Files
```
electron/src/renderer/pages/
├── LoginPage.tsx
├── PRListPage.tsx          (refactored with new table)
└── SettingsPage.tsx

electron/src/renderer/components/
├── PRListTable.tsx         (shadcn/ui table)
├── DiffViewer.tsx
├── CommandPalette.tsx      (shadcn/ui dialog)
├── HelpOverlay.tsx         (shadcn/ui dialog)
├── FilterMenu.tsx          (shadcn/ui dropdown)
└── ProviderButton.tsx      (shadcn/ui button)
```

---

## Quality Gates

Each task includes:
- ✅ Step-by-step implementation instructions
- ✅ Complete code examples
- ✅ Build verification
- ✅ Type checking
- ✅ Test creation
- ✅ Git commit guidance

---

## Expected Outcome

After execution:
- ✅ All 8 screens matching mockups exactly
- ✅ Zero-radius design system enforced globally
- ✅ Keyboard navigation fully integrated (Phase 3)
- ✅ Type-safe React components with TypeScript
- ✅ Accessible components via shadcn/ui
- ✅ Build succeeds, type checks pass
- ✅ Ready for E2E testing

---

**Ready to begin? Start with Task 1 in the main plan.**
