# Bistro UI Implementation — Design System & Screens

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the complete Bistro Electron UI based on the Swiss-style design system mockups (login, inbox, PR overview, diff view, command palette, settings) with full keyboard navigation integration.

**Architecture:**
The design system (Zero-Radius Architecture with Tonal Boundaries) will be implemented as a Tailwind config + CSS modules. Each screen maps to a React page/layout component. The keyboard bindings from Phase 3 will be wired into navigation and interactions. All components use Geist (UI) + JetBrains Mono (data) typography, material icons, and a carefully curated color palette with no border-radius.

**Tech Stack:** React 18, Tailwind CSS with custom theme, shadcn/ui components, Material Symbols icons, Geist + JetBrains Mono fonts, TanStack Query, neverthrow for errors.

---

## Design System Foundation

### Colors (Tailwind Theme Extension)
- **Primary:** `#004ac6` (Geist Blue)
- **Primary Container:** `#2563eb` (Accent Blue)
- **Surface Hierarchy:** `#ffffff` (lowest) → `#f8f9fa` (base) → `#e7e8e9` (high)
- **Outlines:** `#737686` (standard), `#c3c6d7` (variant)
- **Error:** `#ba1a1a`, **On Error:** `#ffffff`

### Typography
- **Geist (UI):** All headings, labels, body text
- **JetBrains Mono (Data):** PR numbers, timestamps, metrics, IDs
- **Material Symbols:** All icons (no emoji)

### Zero-Radius Rule
- **All border-radius = 0px** (square corners only)
- No rounded buttons, no rounded cards, no rounded inputs
- This is the signature aesthetic

### Tonal Boundaries (No Lines)
- **No 1px borders** for primary sectioning
- Use **background color shifts** instead: lighter surfaces on darker backgrounds
- Ghost borders (1px `outline_variant` @ 20% opacity) only for high-density tables

### Shadows (Ambient Only)
- **Floating elements:** `box-shadow: 0 10px 30px rgba(25,28,29,0.04)`
- Soft, diffuse, never dark/sharp

---

## File Structure

### 1. Design System & Theme (`src/renderer/styles/`)
- `tailwind.config.ts` — Tailwind theme with colors, fonts, border-radius overrides
- `design-tokens.css` — CSS variables for reusable color/spacing tokens
- `typography.css` — Font family setup, font weights, line heights

### 2. Layout Components (`src/renderer/layouts/`)
- `AuthLayout.tsx` — Centered card layout (login, onboarding)
- `MainLayout.tsx` — 3-pane layout (sidebar, list, detail) with keyboard support
- `SettingsLayout.tsx` — Full-width settings page

### 3. Pages (`src/renderer/pages/`)
- `LoginPage.tsx` — OAuth provider selection (GitHub, GitLab, Gitea)
- `OnboardingPage.tsx` — Initial sync configuration (if needed)
- `InboxPage.tsx` — PR inbox (list of all PRs)
- `PRDetailPage.tsx` — Single PR view with diff
- `SettingsPage.tsx` — Configuration options
- `CommandPalettePage.tsx` — Command search (`:` key)

### 4. Components (`src/renderer/components/`)
- `ProviderButton.tsx` — OAuth provider button with icon + keyboard hint
- `PRListItem.tsx` — Single PR row in table
- `PRListTable.tsx` — Table container (grid layout)
- `DiffViewer.tsx` — Syntax-highlighted diff with line numbers
- `CommentThread.tsx` — Inline comment + replies
- `SettingsGroup.tsx` — Settings section with toggles/inputs
- `CommandPalette.tsx` — Command search overlay
- `HelpOverlay.tsx` — Keyboard shortcuts reference

### 5. Utilities (`src/renderer/utils/`)
- `design-tokens.ts` — Exported color constants
- `tailwind-helpers.ts` — Utility functions for common Tailwind patterns

### 6. Hooks (`src/renderer/hooks/`)
- `useKeyboardNavigation.ts` — Integrate Phase 3 keyboard bindings with page-specific actions
- `useTheme.ts` — Dark/light mode support (future)

---

## Tasks

### Task 1: Tailwind Theme Configuration & Design Tokens

**Files:**
- Create: `electron/tailwind.config.ts`
- Create: `electron/src/renderer/styles/design-tokens.css`
- Create: `electron/src/renderer/utils/design-tokens.ts`
- Modify: `electron/src/renderer/styles/globals.css` (import design tokens)

- [ ] **Step 1: Write Tailwind config with custom theme**

Extract from mockups' Tailwind config and create `electron/tailwind.config.ts`:
```typescript
import type { Config } from 'tailwindcss';

export default {
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        'surface-container-highest': '#e1e3e4',
        'surface-container-high': '#e7e8e9',
        'surface-container': '#edeeef',
        'surface-container-low': '#f3f4f5',
        'surface-container-lowest': '#ffffff',
        'surface': '#f8f9fa',
        'background': '#f8f9fa',
        'primary': '#004ac6',
        'primary-container': '#2563eb',
        'on-primary': '#ffffff',
        'outline': '#737686',
        'outline-variant': '#c3c6d7',
        'on-surface': '#191c1d',
        'on-surface-variant': '#434655',
        'error': '#ba1a1a',
        'on-error': '#ffffff',
      },
      borderRadius: {
        'none': '0px',
        'DEFAULT': '0px',
        'lg': '0px',
        'xl': '0px',
        'full': '0px',
      },
      fontFamily: {
        'sans': ['Geist', 'sans-serif'],
        'mono': ['JetBrains Mono', 'monospace'],
      },
      boxShadow: {
        'ambient': '0 10px 30px rgba(25, 28, 29, 0.04)',
      },
    },
  },
  plugins: [],
} satisfies Config;
```

- [ ] **Step 2: Create design tokens CSS**

Create `electron/src/renderer/styles/design-tokens.css`:
```css
:root {
  /* Colors */
  --color-primary: #004ac6;
  --color-primary-container: #2563eb;
  --color-surface: #f8f9fa;
  --color-surface-container-lowest: #ffffff;
  --color-surface-container-low: #f3f4f5;
  --color-surface-container: #edeeef;
  --color-surface-container-high: #e7e8e9;
  --color-surface-container-highest: #e1e3e4;
  --color-outline: #737686;
  --color-outline-variant: #c3c6d7;
  --color-on-surface: #191c1d;
  --color-on-surface-variant: #434655;
  --color-error: #ba1a1a;
  --color-on-error: #ffffff;

  /* Typography */
  --font-sans: 'Geist', sans-serif;
  --font-mono: 'JetBrains Mono', monospace;

  /* Spacing */
  --spacing-xs: 4px;
  --spacing-sm: 8px;
  --spacing-md: 16px;
  --spacing-lg: 24px;
  --spacing-xl: 32px;
  --spacing-2xl: 48px;

  /* Shadows */
  --shadow-ambient: 0 10px 30px rgba(25, 28, 29, 0.04);
}
```

- [ ] **Step 3: Create TypeScript design tokens export**

Create `electron/src/renderer/utils/design-tokens.ts`:
```typescript
export const COLORS = {
  primary: '#004ac6',
  primaryContainer: '#2563eb',
  surface: '#f8f9fa',
  surfaceContainerLowest: '#ffffff',
  surfaceContainerLow: '#f3f4f5',
  surfaceContainer: '#edeeef',
  surfaceContainerHigh: '#e7e8e9',
  surfaceContainerHighest: '#e1e3e4',
  outline: '#737686',
  outlineVariant: '#c3c6d7',
  onSurface: '#191c1d',
  onSurfaceVariant: '#434655',
  error: '#ba1a1a',
  onError: '#ffffff',
} as const;

export const FONTS = {
  sans: "'Geist', sans-serif",
  mono: "'JetBrains Mono', monospace",
} as const;

export const SHADOWS = {
  ambient: '0 10px 30px rgba(25, 28, 29, 0.04)',
} as const;
```

- [ ] **Step 4: Import fonts and design tokens in globals.css**

Update `electron/src/renderer/styles/globals.css`:
```css
@import url('https://fonts.googleapis.com/css2?family=Geist:wght@100..900&family=JetBrains+Mono:wght@100..800&display=swap');
@import url('https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined:wght,FILL@100..700,0..1&display=swap');

@import './design-tokens.css';

body {
  font-family: var(--font-sans);
  background-color: var(--color-surface);
  color: var(--color-on-surface);
}

/* Material Symbols default */
.material-symbols-outlined {
  font-family: 'Material Symbols Outlined';
  font-variation-settings: 'FILL' 0, 'wght' 400, 'GRAD' 0, 'opsz' 24;
}
```

- [ ] **Step 5: Run build to verify Tailwind config loads**

```bash
cd electron && pnpm build
```

Expected: No errors, tailwind processes successfully.

- [ ] **Step 6: Commit**

```bash
git add electron/tailwind.config.ts electron/src/renderer/styles/design-tokens.css electron/src/renderer/utils/design-tokens.ts electron/src/renderer/styles/globals.css
git commit -m "feat: add design system foundation (colors, fonts, tokens)"
```

---

### Task 2: Layout Components (AuthLayout, MainLayout, SettingsLayout)

**Files:**
- Create: `electron/src/renderer/layouts/AuthLayout.tsx`
- Create: `electron/src/renderer/layouts/MainLayout.tsx`
- Create: `electron/src/renderer/layouts/SettingsLayout.tsx`
- Create: `electron/src/renderer/layouts/index.ts`

- [ ] **Step 1: Create AuthLayout**

Create `electron/src/renderer/layouts/AuthLayout.tsx`:
```typescript
import React from 'react';

interface AuthLayoutProps {
  children: React.ReactNode;
  title?: string;
}

export function AuthLayout({ children, title = 'BISTRO' }: AuthLayoutProps) {
  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center px-6">
      {/* Aesthetic background gradients */}
      <div className="fixed inset-0 -z-10 overflow-hidden opacity-30 pointer-events-none">
        <div className="absolute top-[10%] left-[5%] w-[40%] h-[1px] bg-gradient-to-r from-transparent via-outline-variant/20 to-transparent"></div>
        <div className="absolute top-0 right-[20%] w-[1px] h-screen bg-gradient-to-b from-transparent via-outline-variant/20 to-transparent"></div>
      </div>

      <main className="w-full max-w-md">
        {children}
      </main>

      {/* Footer */}
      <footer className="fixed bottom-0 w-full border-t border-surface-container-high bg-surface-container-low flex items-center justify-between px-4 h-8 z-50">
        <div className="flex items-center gap-4">
          <span className="font-mono text-[10px] uppercase tracking-widest text-on-surface font-bold">BISTRO // TECHNICAL ATELIER</span>
          <span className="font-mono text-[10px] uppercase tracking-widest text-outline">© 2024</span>
        </div>
        <nav className="flex gap-4">
          <a href="#" className="font-mono text-[10px] uppercase tracking-widest text-outline hover:text-on-surface transition-colors">Documentation</a>
          <a href="#" className="font-mono text-[10px] uppercase tracking-widest text-outline hover:text-on-surface transition-colors">Support</a>
        </nav>
      </footer>
    </div>
  );
}
```

- [ ] **Step 2: Create MainLayout**

Create `electron/src/renderer/layouts/MainLayout.tsx`:
```typescript
import React, { useEffect } from 'react';
import { useKeyboard } from '../hooks/useKeyboard';

interface MainLayoutProps {
  sidebar?: React.ReactNode;
  content: React.ReactNode;
  detail?: React.ReactNode;
}

export function MainLayout({ sidebar, content, detail }: MainLayoutProps) {
  useKeyboard(); // Integrate Phase 3 keyboard navigation

  return (
    <div className="flex h-screen bg-surface">
      {/* Sidebar */}
      {sidebar && (
        <aside className="w-60 border-r border-surface-container-high bg-surface-container-lowest flex flex-col">
          {sidebar}
        </aside>
      )}

      {/* Main Content */}
      <main className="flex-1 flex flex-col overflow-hidden bg-surface">
        {content}
      </main>

      {/* Detail Pane (optional) */}
      {detail && (
        <aside className="w-80 border-l border-surface-container-high bg-surface-container-lowest flex flex-col overflow-auto">
          {detail}
        </aside>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Create SettingsLayout**

Create `electron/src/renderer/layouts/SettingsLayout.tsx`:
```typescript
import React from 'react';

interface SettingsLayoutProps {
  title: string;
  description?: string;
  children: React.ReactNode;
}

export function SettingsLayout({ title, description, children }: SettingsLayoutProps) {
  return (
    <div className="min-h-screen bg-surface flex flex-col">
      <header className="border-b border-surface-container-high bg-surface-container-lowest p-8">
        <h1 className="text-4xl font-black tracking-tighter text-on-surface mb-2">{title}</h1>
        {description && <p className="text-on-surface-variant text-sm">{description}</p>}
      </header>
      <main className="flex-1 p-8 overflow-auto">
        <div className="max-w-4xl">
          {children}
        </div>
      </main>
    </div>
  );
}
```

- [ ] **Step 4: Create layouts index**

Create `electron/src/renderer/layouts/index.ts`:
```typescript
export { AuthLayout } from './AuthLayout';
export { MainLayout } from './MainLayout';
export { SettingsLayout } from './SettingsLayout';
```

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/layouts/
git commit -m "feat: add layout components (auth, main, settings)"
```

---

### Task 3: Login Page with OAuth Buttons

**Files:**
- Create: `electron/src/renderer/pages/LoginPage.tsx`
- Create: `electron/src/renderer/components/ProviderButton.tsx`
- Modify: `electron/src/renderer/App.tsx` (route to login)

- [ ] **Step 1: Create ProviderButton component**

Create `electron/src/renderer/components/ProviderButton.tsx`:
```typescript
import React from 'react';

export interface ProviderButtonProps {
  provider: 'github' | 'gitlab' | 'gitea';
  icon: string;
  label: string;
  keyHint: string;
  onClick: () => void;
}

export function ProviderButton({ provider, icon, label, keyHint, onClick }: ProviderButtonProps) {
  return (
    <button
      onClick={onClick}
      className="w-full h-14 flex items-center justify-between px-5 bg-surface-container-lowest border border-outline hover:bg-surface-container-highest transition-colors group"
    >
      <div className="flex items-center gap-4">
        <span className="material-symbols-outlined text-on-surface">{icon}</span>
        <span className="font-sans font-bold text-sm tracking-tight text-on-surface">{label}</span>
      </div>
      <div className="font-mono text-[10px] text-outline flex items-center gap-2">
        <span className="px-1.5 py-0.5 border border-outline-variant/50 bg-surface-container-low">{keyHint}</span>
        <span className="uppercase opacity-60">Key Hint</span>
      </div>
    </button>
  );
}
```

- [ ] **Step 2: Create LoginPage**

Create `electron/src/renderer/pages/LoginPage.tsx`:
```typescript
import React from 'react';
import { AuthLayout } from '../layouts/AuthLayout';
import { ProviderButton } from '../components/ProviderButton';

export function LoginPage() {
  const handleProviderClick = (provider: 'github' | 'gitlab' | 'gitea') => {
    console.log(`Authenticating with ${provider}`);
    // TODO: Dispatch OAuth flow via IPC
  };

  return (
    <AuthLayout>
      <div className="bg-surface-container-lowest border border-outline-variant/40 p-12 shadow-ambient">
        <header className="mb-12">
          <h1 className="text-4xl font-extrabold tracking-tighter text-on-background mb-4 uppercase">BISTRO</h1>
          <p className="text-on-surface-variant text-sm font-medium tracking-tight">Connect your platform to begin reviewing code</p>
        </header>

        <div className="space-y-4">
          <ProviderButton
            provider="github"
            icon="terminal"
            label="GitHub"
            keyHint="G"
            onClick={() => handleProviderClick('github')}
          />
          <ProviderButton
            provider="gitlab"
            icon="source"
            label="GitLab"
            keyHint="L"
            onClick={() => handleProviderClick('gitlab')}
          />
          <ProviderButton
            provider="gitea"
            icon="coffee"
            label="Gitea"
            keyHint="T"
            onClick={() => handleProviderClick('gitea')}
          />
        </div>

        {/* Asymmetric data decoration */}
        <div className="mt-16 flex items-end justify-between">
          <div className="font-mono text-[10px] text-outline-variant flex flex-col">
            <span className="uppercase tracking-widest mb-1">Status: Ready</span>
            <span>v4.0.2-stable</span>
          </div>
          <div className="w-24 h-[1px] bg-outline-variant/30"></div>
        </div>
      </div>
    </AuthLayout>
  );
}
```

- [ ] **Step 3: Update App.tsx to show LoginPage**

Modify `electron/src/renderer/App.tsx`:
```typescript
import { LoginPage } from './pages/LoginPage';

export function AppContent() {
  const isAuthenticated = false; // TODO: Check auth state

  if (!isAuthenticated) {
    return <LoginPage />;
  }

  return (
    // Existing main app layout
    <>...</>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/pages/LoginPage.tsx electron/src/renderer/components/ProviderButton.tsx electron/src/renderer/App.tsx
git commit -m "feat: add login page with OAuth provider buttons"
```

---

### Task 4: PR List Page & Table Component

**Files:**
- Modify: `electron/src/renderer/pages/PRList.tsx` (add design system styling)
- Create: `electron/src/renderer/components/PRListTable.tsx`
- Modify: `electron/src/renderer/components/PRListItem.tsx` (update styling)

- [ ] **Step 1: Create PRListTable component**

Create `electron/src/renderer/components/PRListTable.tsx`:
```typescript
import React from 'react';
import type { PR } from '../types';
import { PRListItem } from './PRListItem';

interface PRListTableProps {
  prs: PR[];
  selectedIndex: number;
  onSelectPR: (index: number) => void;
}

export function PRListTable({ prs, selectedIndex, onSelectPR }: PRListTableProps) {
  return (
    <div className="flex-1 overflow-auto">
      {/* Table header */}
      <div className="grid grid-cols-[24px_60px_1fr_120px_100px] items-center gap-4 px-4 py-3 border-b border-surface-container-high bg-surface-container-low sticky top-0">
        <div className="font-mono text-[11px] uppercase tracking-widest text-outline-variant"></div>
        <div className="font-mono text-[11px] uppercase tracking-widest text-outline-variant">State</div>
        <div className="font-mono text-[11px] uppercase tracking-widest text-outline-variant">Title</div>
        <div className="font-mono text-[11px] uppercase tracking-widest text-outline-variant text-right">Updated</div>
        <div className="font-mono text-[11px] uppercase tracking-widest text-outline-variant text-right">Author</div>
      </div>

      {/* Table rows */}
      {prs.map((pr, idx) => (
        <PRListItem
          key={pr.id}
          pr={pr}
          isSelected={idx === selectedIndex}
          onClick={() => onSelectPR(idx)}
        />
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Update PRListItem component**

Update `electron/src/renderer/components/PRListItem.tsx` to use design system:
```typescript
import React from 'react';
import type { PR } from '../types';

interface PRListItemProps {
  pr: PR;
  isSelected?: boolean;
  onClick?: () => void;
}

export function PRListItem({ pr, isSelected = false, onClick }: PRListItemProps) {
  const stateColor = pr.state === 'open' ? 'bg-green-500' : pr.state === 'closed' ? 'bg-red-500' : 'bg-purple-500';

  return (
    <div
      onClick={onClick}
      className={`grid grid-cols-[24px_60px_1fr_120px_100px] items-center gap-4 px-4 py-3 border-b border-surface-container-high hover:bg-surface-container-low transition-colors cursor-pointer ${
        isSelected ? 'border-l-[3px] border-l-primary bg-surface-container-low' : ''
      }`}
    >
      {/* State indicator */}
      <div className={`w-3 h-3 ${stateColor}`}></div>

      {/* State label */}
      <div className="font-mono text-[11px] text-outline uppercase">{pr.state}</div>

      {/* PR title */}
      <div className="font-medium text-[14px] text-on-surface truncate">{pr.title}</div>

      {/* Updated date */}
      <div className="font-mono text-[11px] text-outline-variant text-right">{new Date(pr.updated_at).toLocaleDateString()}</div>

      {/* Author */}
      <div className="font-mono text-[11px] text-primary text-right">{pr.author}</div>
    </div>
  );
}
```

- [ ] **Step 3: Update PRList page**

Update `electron/src/renderer/pages/PRList.tsx`:
```typescript
import React, { useCallback } from 'react';
import { usePRList } from '../queries/usePRList';
import { PRListTable } from '../components/PRListTable';
import { CommandBar } from '../components/CommandBar';
import { StatusBar } from '../components/StatusBar';
import { useStore } from '../hooks/useStore';

export function PRListPage() {
  const { data: prs = [] } = usePRList();
  const store = useStore();
  const selectedIndex = store.getState().selectedPRIndex ?? 0;

  const handleSelectPR = useCallback((index: number) => {
    store.setState((s) => ({ ...s, selectedPRIndex: index }));
  }, [store]);

  return (
    <>
      <CommandBar />
      <PRListTable prs={prs} selectedIndex={selectedIndex} onSelectPR={handleSelectPR} />
      <StatusBar />
    </>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/components/PRListTable.tsx electron/src/renderer/components/PRListItem.tsx electron/src/renderer/pages/PRList.tsx
git commit -m "feat: style PR list table with design system"
```

---

### Task 5: Command Palette Component

**Files:**
- Create: `electron/src/renderer/components/CommandPalette.tsx`
- Create: `electron/src/renderer/components/CommandPalette.css`

- [ ] **Step 1: Create CommandPalette component**

Create `electron/src/renderer/components/CommandPalette.tsx`:
```typescript
import React, { useState, useEffect } from 'react';
import { useStore } from '../hooks/useStore';
import './CommandPalette.css';

export function CommandPalette() {
  const store = useStore();
  const mode = store.getState().mode;
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<string[]>([]);

  const isOpen = mode === 'command';

  useEffect(() => {
    if (isOpen) {
      const input = document.getElementById('command-input') as HTMLInputElement;
      input?.focus();
    }
  }, [isOpen]);

  const handleClose = () => {
    store.setState((s) => ({ ...s, mode: 'list' }));
    setQuery('');
  };

  const handleSearch = (value: string) => {
    setQuery(value);
    // Filter available commands based on query
    const allCommands = ['help', 'settings', 'clear', 'focus list', 'focus detail'];
    setResults(allCommands.filter((cmd) => cmd.includes(value.toLowerCase())));
  };

  if (!isOpen) return null;

  return (
    <div className="command-palette" onClick={handleClose}>
      <div className="command-palette-content" onClick={(e) => e.stopPropagation()}>
        <div className="bg-surface-container-lowest border border-outline shadow-ambient">
          <input
            id="command-input"
            type="text"
            placeholder=":"
            value={query}
            onChange={(e) => handleSearch(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') handleClose();
            }}
            className="w-full px-4 py-3 font-mono text-on-surface bg-surface-container-lowest border-b border-outline-variant/20 focus:outline-none"
          />
          <div className="divide-y divide-surface-container-high max-h-64 overflow-auto">
            {results.map((cmd) => (
              <div key={cmd} className="px-4 py-3 hover:bg-surface-container-low cursor-pointer font-mono text-sm text-on-surface">
                {cmd}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create CommandPalette CSS**

Create `electron/src/renderer/components/CommandPalette.css`:
```css
.command-palette {
  position: fixed;
  inset: 0;
  background-color: rgba(25, 28, 29, 0.4);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 20vh;
  z-index: 1000;
  backdrop-filter: blur(4px);
}

.command-palette-content {
  width: 100%;
  max-width: 500px;
  animation: slideDown 0.2s ease-out;
}

@keyframes slideDown {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
```

- [ ] **Step 3: Wire CommandPalette into App**

Update `electron/src/renderer/App.tsx` to include CommandPalette:
```typescript
import { CommandPalette } from './components/CommandPalette';

export function AppContent() {
  return (
    <>
      <MainLayout sidebar={<Sidebar />} content={<PRListPage />} />
      <CommandPalette />
      <HelpOverlay />
    </>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/components/CommandPalette.tsx electron/src/renderer/components/CommandPalette.css electron/src/renderer/App.tsx
git commit -m "feat: add command palette with glassmorphism design"
```

---

### Task 6: Diff Viewer with Syntax Highlighting

**Files:**
- Create: `electron/src/renderer/components/DiffViewer.tsx`
- Create: `electron/src/renderer/utils/diff-parser.ts`

- [ ] **Step 1: Create diff parser utility**

Create `electron/src/renderer/utils/diff-parser.ts`:
```typescript
export interface ParsedDiffLine {
  type: 'add' | 'remove' | 'context';
  content: string;
  lineNum: number;
}

export interface ParsedDiffFile {
  file: string;
  lines: ParsedDiffLine[];
}

export function parseDiff(diffText: string): ParsedDiffFile[] {
  const files: ParsedDiffFile[] = [];
  let currentFile: ParsedDiffFile | null = null;
  let lineNum = 0;

  const lines = diffText.split('\n');
  for (const line of lines) {
    if (line.startsWith('---') || line.startsWith('+++')) {
      const filename = line.slice(4).split('\t')[0];
      if (!currentFile || currentFile.file !== filename) {
        currentFile = { file: filename, lines: [] };
        files.push(currentFile);
        lineNum = 0;
      }
    } else if (line.startsWith('+')) {
      currentFile?.lines.push({
        type: 'add',
        content: line.slice(1),
        lineNum: ++lineNum,
      });
    } else if (line.startsWith('-')) {
      currentFile?.lines.push({
        type: 'remove',
        content: line.slice(1),
        lineNum: ++lineNum,
      });
    } else if (!line.startsWith('\\')) {
      currentFile?.lines.push({
        type: 'context',
        content: line.slice(1),
        lineNum: ++lineNum,
      });
    }
  }

  return files;
}
```

- [ ] **Step 2: Create DiffViewer component**

Create `electron/src/renderer/components/DiffViewer.tsx`:
```typescript
import React, { useEffect, useState } from 'react';
import { parseDiff, type ParsedDiffFile } from '../utils/diff-parser';

interface DiffViewerProps {
  diff: string;
}

export function DiffViewer({ diff }: DiffViewerProps) {
  const [files, setFiles] = useState<ParsedDiffFile[]>([]);

  useEffect(() => {
    setFiles(parseDiff(diff));
  }, [diff]);

  return (
    <div className="flex-1 overflow-auto bg-surface-container-lowest font-mono text-sm">
      {files.map((file) => (
        <div key={file.file} className="mb-8">
          {/* File header */}
          <div className="sticky top-0 bg-surface-container-low px-4 py-2 border-b border-outline-variant/20">
            <div className="font-mono text-[11px] text-outline font-bold">{file.file}</div>
          </div>

          {/* Diff lines */}
          <div className="divide-y divide-surface-container-high">
            {file.lines.map((line, idx) => (
              <div
                key={`${file.file}-${idx}`}
                className={`flex ${
                  line.type === 'add'
                    ? 'bg-green-50 text-green-900'
                    : line.type === 'remove'
                      ? 'bg-red-50 text-red-900'
                      : 'hover:bg-surface-container-low'
                } py-1`}
              >
                <div className="w-12 text-right pr-4 text-outline-variant select-none text-[10px]">{line.lineNum}</div>
                <div className="w-6 text-center select-none text-[12px] font-bold">{line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' '}</div>
                <div className="flex-1 px-4 whitespace-pre-wrap break-words">{line.content}</div>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add electron/src/renderer/components/DiffViewer.tsx electron/src/renderer/utils/diff-parser.ts
git commit -m "feat: add diff viewer with syntax highlighting and line numbers"
```

---

### Task 7: Settings Page

**Files:**
- Create: `electron/src/renderer/pages/SettingsPage.tsx`
- Create: `electron/src/renderer/components/SettingsGroup.tsx`

- [ ] **Step 1: Create SettingsGroup component**

Create `electron/src/renderer/components/SettingsGroup.tsx`:
```typescript
import React from 'react';

interface SettingsGroupProps {
  title: string;
  description?: string;
  children: React.ReactNode;
}

export function SettingsGroup({ title, description, children }: SettingsGroupProps) {
  return (
    <div className="mb-8 pb-8 border-b border-surface-container-high last:border-b-0">
      <h3 className="text-lg font-bold text-on-surface mb-2">{title}</h3>
      {description && <p className="text-sm text-on-surface-variant mb-4">{description}</p>}
      <div className="space-y-4">{children}</div>
    </div>
  );
}
```

- [ ] **Step 2: Create SettingsPage**

Create `electron/src/renderer/pages/SettingsPage.tsx`:
```typescript
import React from 'react';
import { SettingsLayout } from '../layouts/SettingsLayout';
import { SettingsGroup } from '../components/SettingsGroup';

export function SettingsPage() {
  return (
    <SettingsLayout
      title="Settings"
      description="Configure Bistro to your preferences"
    >
      <SettingsGroup title="Appearance" description="Customize how Bistro looks">
        <label className="flex items-center gap-4 cursor-pointer">
          <input type="checkbox" className="w-5 h-5 border border-outline" defaultChecked />
          <span className="text-on-surface">Dark mode</span>
        </label>
        <label className="flex items-center gap-4 cursor-pointer">
          <input type="checkbox" className="w-5 h-5 border border-outline" />
          <span className="text-on-surface">Show file tree</span>
        </label>
      </SettingsGroup>

      <SettingsGroup title="Sync" description="Control how Bistro syncs with your repositories">
        <label className="flex items-center gap-4 cursor-pointer">
          <input type="checkbox" className="w-5 h-5 border border-outline" defaultChecked />
          <span className="text-on-surface">Auto-sync every minute</span>
        </label>
        <label className="flex items-center gap-4 cursor-pointer">
          <input type="checkbox" className="w-5 h-5 border border-outline" defaultChecked />
          <span className="text-on-surface">Notify on new PRs</span>
        </label>
      </SettingsGroup>

      <SettingsGroup title="Keyboard" description="Manage keyboard shortcuts">
        <button className="px-4 py-2 bg-primary text-on-primary font-sans font-bold text-sm hover:bg-primary-container transition-colors">
          View All Shortcuts
        </button>
      </SettingsGroup>
    </SettingsLayout>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add electron/src/renderer/pages/SettingsPage.tsx electron/src/renderer/components/SettingsGroup.tsx
git commit -m "feat: add settings page with configuration groups"
```

---

### Task 8: Integrate with Phase 3 Keyboard Navigation

**Files:**
- Modify: `electron/src/renderer/hooks/useKeyboard.ts` (add page-specific actions)
- Create: `electron/src/renderer/hooks/useKeyboardNavigation.ts` (high-level binding handler)

- [ ] **Step 1: Create useKeyboardNavigation hook**

Create `electron/src/renderer/hooks/useKeyboardNavigation.ts`:
```typescript
import { useEffect } from 'react';
import { useStore } from './useStore';

export function useKeyboardNavigation() {
  const store = useStore();

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const state = store.getState();

      // Navigate between pages
      if (event.key === 's' && !event.ctrlKey && !event.metaKey) {
        // Open Settings (Phase 3 extension)
        store.setState((s) => ({ ...s, currentPage: 'settings' }));
      }

      // Handle mode-specific actions
      if (state.mode === 'list') {
        if (event.key === 'Enter') {
          // Open selected PR
          store.setState((s) => ({ ...s, mode: 'detail' }));
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [store]);
}
```

- [ ] **Step 2: Update App to use useKeyboardNavigation**

Modify `electron/src/renderer/App.tsx`:
```typescript
import { useKeyboardNavigation } from './hooks/useKeyboardNavigation';

export function AppContent() {
  useKeyboardNavigation(); // Use the high-level keyboard hook

  // Existing code...
}
```

- [ ] **Step 3: Commit**

```bash
git add electron/src/renderer/hooks/useKeyboardNavigation.ts electron/src/renderer/App.tsx
git commit -m "feat: integrate keyboard navigation with page switching"
```

---

### Task 9: Build, Type Check & E2E Tests

**Files:**
- Create: `tests/e2e/ui-navigation.spec.ts`

- [ ] **Step 1: Build and type check**

```bash
cd electron && pnpm build 2>&1
```

Expected: No errors, build succeeds.

- [ ] **Step 2: Create E2E test for login flow**

Create `tests/e2e/ui-navigation.spec.ts`:
```typescript
import { test, expect } from '@playwright/test';

test('login page renders with OAuth buttons', async ({ page }) => {
  await page.goto('http://localhost:3000');

  // Check login page title
  await expect(page.locator('text=BISTRO')).toBeVisible();

  // Check OAuth buttons
  await expect(page.locator('button:has-text("GitHub")')).toBeVisible();
  await expect(page.locator('button:has-text("GitLab")')).toBeVisible();
  await expect(page.locator('button:has-text("Gitea")')).toBeVisible();
});

test('keyboard hint displays correctly', async ({ page }) => {
  await page.goto('http://localhost:3000');

  // Check for keyboard hints
  await expect(page.locator('text=G')).toBeVisible(); // GitHub hint
  await expect(page.locator('text=L')).toBeVisible(); // GitLab hint
  await expect(page.locator('text=T')).toBeVisible(); // Gitea hint
});
```

- [ ] **Step 3: Run type check**

```bash
cd electron && pnpm typecheck
```

Expected: No type errors.

- [ ] **Step 4: Commit**

```bash
git add tests/e2e/ui-navigation.spec.ts
git commit -m "test: add E2E tests for UI navigation and login flow"
```

---

## Summary

**Acceptance Criteria:**
- [ ] All 8 screens implemented per mockups
- [ ] Design system (colors, fonts, zero-radius) applied consistently
- [ ] Phase 3 keyboard bindings integrated
- [ ] All components use Geist (UI) + JetBrains Mono (data)
- [ ] No border-radius (0px everywhere)
- [ ] Tonal boundaries instead of 1px borders
- [ ] Tests cover critical user flows
- [ ] Build succeeds with no type errors

**Key Files Created:**
- Design system: `tailwind.config.ts`, `design-tokens.css`, `design-tokens.ts`
- Layouts: `AuthLayout.tsx`, `MainLayout.tsx`, `SettingsLayout.tsx`
- Pages: `LoginPage.tsx`, `PRListPage.tsx`, `SettingsPage.tsx`
- Components: `ProviderButton.tsx`, `PRListTable.tsx`, `DiffViewer.tsx`, `CommandPalette.tsx`, `SettingsGroup.tsx`
- Utilities: `diff-parser.ts`

**Next Steps (Phase 4+):**
- Implement full diff viewer with comment threading
- Add WebSocket integration for real-time PR updates
- Implement PR actions (approve, comment, request changes)
- Add dark mode support via CSS custom properties
