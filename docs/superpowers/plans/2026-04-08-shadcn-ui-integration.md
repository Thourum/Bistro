# Bistro UI with shadcn/ui Components

> **Supplementary Plan:** Adapt the main UI implementation plan to use **shadcn/ui** components instead of custom components.

**Goal:** Leverage shadcn/ui for rapid, production-ready component development while maintaining the Swiss-style design system (zero-radius, tonal boundaries, Geist + Mono typography).

**Advantage of shadcn/ui:**
- Pre-built, accessible components (Button, Input, Card, Dialog, Tabs, etc.)
- Tailwind-based (matches our design system perfectly)
- Copy-paste components (not a package) — full control over styling
- Can override defaults to enforce zero-radius, custom colors, etc.

---

## Setup: Install and Configure shadcn/ui

### Task 0.1: Install shadcn/ui CLI

```bash
cd /Users/samuelasvanyi/dev/bistro/electron
npx shadcn-ui@latest init -d
```

When prompted:
- **Framework:** React
- **Style:** New York (or Default)
- **Base color:** Slate
- **CSS variables:** Yes

This creates:
- `src/renderer/components/ui/` — shadcn/ui components
- `lib/utils.ts` — clsx + cn utility

### Task 0.2: Override shadcn/ui Defaults for Zero-Radius Design

Create `electron/src/renderer/styles/shadcn-overrides.css` to enforce zero-radius globally:

```css
/* Override all shadcn/ui components to use 0px border-radius */
[class*="rounded"] {
  border-radius: 0 !important;
}

/* Override button styles */
.button,
[class*="btn"],
[class*="button"] {
  border-radius: 0;
}

/* Override card styles */
[class*="card"] {
  border-radius: 0;
  background-color: var(--color-surface-container-lowest);
  border: 1px solid var(--color-outline-variant);
}

/* Override input styles */
input,
textarea,
select,
[class*="input"] {
  border-radius: 0;
  border: 1px solid var(--color-outline-variant);
}

/* Override dialog/modal styles */
[class*="dialog"],
[class*="modal"],
[class*="popover"] {
  border-radius: 0;
}

/* Use Geist for UI text, Mono for data */
body {
  --font-sans: 'Geist', sans-serif;
  --font-mono: 'JetBrains Mono', monospace;
}
```

Import in `electron/src/renderer/styles/globals.css`:
```css
@import './shadcn-overrides.css';
```

---

## Component Mapping: Replace Custom with shadcn/ui

### Components to Use from shadcn/ui

| Feature | shadcn/ui Component | Custom File | Status |
|---------|-------------------|------------|--------|
| Buttons | `Button` | ProviderButton.tsx | Replace |
| Input fields | `Input` | (LoginPage form) | Use |
| Cards | `Card` | (PR list items) | Adapt |
| Dialogs | `Dialog` | CommandPalette | Replace |
| Tabs | `Tabs` | (Settings groups) | Use |
| Tables | `DataTable` | PRListTable | Use |
| Badges | `Badge` | (PR state indicators) | Use |
| Dropdowns | `DropdownMenu` | (Filter menu) | Use |
| Modals | `Dialog` | (Help overlay) | Use |

---

## Updated Task List (Using shadcn/ui)

### Task 1: Tailwind Theme + shadcn/ui Setup (SAME)

Steps 1-6 remain identical. The Tailwind config and design tokens work with shadcn/ui out of the box.

---

### Task 2: Update Components to Use shadcn/ui

**Replace:** Custom `ProviderButton.tsx` → shadcn/ui `Button`

```typescript
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';

interface ProviderButtonProps {
  provider: 'github' | 'gitlab' | 'gitea';
  icon: string;
  label: string;
  keyHint: string;
  onClick: () => void;
}

export function ProviderButton({ provider, icon, label, keyHint, onClick }: ProviderButtonProps) {
  return (
    <Button
      onClick={onClick}
      variant="outline"
      className="w-full h-14 justify-between px-5"
    >
      <div className="flex items-center gap-4">
        <span className="material-symbols-outlined">{icon}</span>
        <span className="font-bold text-sm">{label}</span>
      </div>
      <Badge variant="outline" className="font-mono text-[10px]">
        {keyHint}
      </Badge>
    </Button>
  );
}
```

---

### Task 3: LoginPage with shadcn/ui

```typescript
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { AuthLayout } from '../layouts/AuthLayout';
import { ProviderButton } from '../components/ProviderButton';

export function LoginPage() {
  const handleProviderClick = (provider: 'github' | 'gitlab' | 'gitea') => {
    console.log(`Authenticating with ${provider}`);
  };

  return (
    <AuthLayout>
      <Card className="p-12">
        <header className="mb-12">
          <h1 className="text-4xl font-extrabold tracking-tighter text-on-surface mb-4 uppercase">BISTRO</h1>
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

        <div className="mt-16 flex items-end justify-between">
          <div className="font-mono text-[10px] text-outline-variant flex flex-col">
            <span className="uppercase tracking-widest mb-1">Status: Ready</span>
            <span>v4.0.2-stable</span>
          </div>
          <div className="w-24 h-[1px] bg-outline-variant/30"></div>
        </div>
      </Card>
    </AuthLayout>
  );
}
```

---

### Task 4: Command Palette with shadcn/ui Dialog

Replace custom CommandPalette with shadcn/ui `Dialog`:

```typescript
import React, { useState } from 'react';
import { useStore } from '../hooks/useStore';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';

export function CommandPalette() {
  const store = useStore();
  const mode = store.getState().mode;
  const isOpen = mode === 'command';
  const [query, setQuery] = useState('');

  const handleClose = () => {
    store.setState((s) => ({ ...s, mode: 'list' }));
    setQuery('');
  };

  const commands = ['help', 'settings', 'clear', 'focus list', 'focus detail'].filter((cmd) =>
    cmd.includes(query.toLowerCase())
  );

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="w-[500px]">
        <DialogHeader>
          <DialogTitle>Command Palette</DialogTitle>
        </DialogHeader>
        <Input
          type="text"
          placeholder=":"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === 'Escape' && handleClose()}
          className="font-mono"
          autoFocus
        />
        <div className="divide-y max-h-64 overflow-auto">
          {commands.map((cmd) => (
            <div key={cmd} className="px-4 py-3 hover:bg-surface-container-low cursor-pointer font-mono text-sm">
              {cmd}
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
```

---

### Task 5: Settings Page with shadcn/ui Tabs

Replace custom SettingsGroup with shadcn/ui `Tabs`:

```typescript
import React from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Checkbox } from '@/components/ui/checkbox';
import { Button } from '@/components/ui/button';
import { SettingsLayout } from '../layouts/SettingsLayout';

export function SettingsPage() {
  return (
    <SettingsLayout title="Settings" description="Configure Bistro to your preferences">
      <Tabs defaultValue="appearance" className="w-full">
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="appearance">Appearance</TabsTrigger>
          <TabsTrigger value="sync">Sync</TabsTrigger>
          <TabsTrigger value="keyboard">Keyboard</TabsTrigger>
        </TabsList>

        <TabsContent value="appearance" className="space-y-4">
          <div className="flex items-center space-x-4">
            <Checkbox id="dark-mode" defaultChecked />
            <label htmlFor="dark-mode" className="text-sm cursor-pointer">
              Dark mode
            </label>
          </div>
          <div className="flex items-center space-x-4">
            <Checkbox id="file-tree" />
            <label htmlFor="file-tree" className="text-sm cursor-pointer">
              Show file tree
            </label>
          </div>
        </TabsContent>

        <TabsContent value="sync" className="space-y-4">
          <div className="flex items-center space-x-4">
            <Checkbox id="auto-sync" defaultChecked />
            <label htmlFor="auto-sync" className="text-sm cursor-pointer">
              Auto-sync every minute
            </label>
          </div>
          <div className="flex items-center space-x-4">
            <Checkbox id="notifications" defaultChecked />
            <label htmlFor="notifications" className="text-sm cursor-pointer">
              Notify on new PRs
            </label>
          </div>
        </TabsContent>

        <TabsContent value="keyboard">
          <Button variant="default">View All Shortcuts</Button>
        </TabsContent>
      </Tabs>
    </SettingsLayout>
  );
}
```

---

### Task 6: PR List Table with shadcn/ui

Use shadcn/ui `DataTable` component for PR list:

```typescript
import React from 'react';
import { usePRList } from '../queries/usePRList';
import { useStore } from '../hooks/useStore';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';

export function PRListTable() {
  const { data: prs = [] } = usePRList();
  const store = useStore();
  const selectedIndex = store.getState().selectedPRIndex ?? 0;

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-[60px]">State</TableHead>
          <TableHead>Title</TableHead>
          <TableHead className="text-right w-[120px]">Updated</TableHead>
          <TableHead className="text-right w-[100px]">Author</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {prs.map((pr, idx) => (
          <TableRow
            key={pr.id}
            className={selectedIndex === idx ? 'bg-surface-container-low border-l-[3px] border-l-primary' : ''}
          >
            <TableCell>
              <Badge
                variant={pr.state === 'open' ? 'default' : pr.state === 'closed' ? 'destructive' : 'secondary'}
              >
                {pr.state}
              </Badge>
            </TableCell>
            <TableCell className="font-medium">{pr.title}</TableCell>
            <TableCell className="text-right font-mono text-[11px]">
              {new Date(pr.updated_at).toLocaleDateString()}
            </TableCell>
            <TableCell className="text-right font-mono text-[11px] text-primary">{pr.author}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
```

---

### Task 7: Filter Menu with shadcn/ui DropdownMenu

```typescript
import React from 'react';
import { useStore } from '../hooks/useStore';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Button } from '@/components/ui/button';

export function FilterMenu() {
  const store = useStore();
  const filterState = store.getState().filterState;

  const handleToggle = (filter: 'open' | 'draft' | 'merged') => {
    store.setState((s) => ({
      ...s,
      filterState: {
        ...s.filterState,
        [filter]: !s.filterState[filter],
      },
    }));
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm">
          Filter (f)
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuCheckboxItem
          checked={filterState.open}
          onCheckedChange={() => handleToggle('open')}
        >
          Open (o)
        </DropdownMenuCheckboxItem>
        <DropdownMenuCheckboxItem
          checked={filterState.draft}
          onCheckedChange={() => handleToggle('draft')}
        >
          Draft (d)
        </DropdownMenuCheckboxItem>
        <DropdownMenuCheckboxItem
          checked={filterState.merged}
          onCheckedChange={() => handleToggle('merged')}
        >
          Merged (m)
        </DropdownMenuCheckboxItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
```

---

### Task 8: Help Overlay with shadcn/ui Dialog

```typescript
import React from 'react';
import { useStore } from '../hooks/useStore';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { ScrollArea } from '@/components/ui/scroll-area';

export function HelpOverlay() {
  const store = useStore();
  const mode = store.getState().mode;
  const isOpen = mode === 'help';

  const handleClose = () => {
    store.setState((s) => ({ ...s, mode: 'list' }));
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl max-h-[80vh]">
        <DialogHeader>
          <DialogTitle>Keyboard Shortcuts</DialogTitle>
        </DialogHeader>
        <ScrollArea className="h-full pr-4">
          <div className="space-y-4">
            <section>
              <h3 className="font-bold text-sm text-on-surface-variant uppercase tracking-widest">Navigation</h3>
              <div className="space-y-2 mt-2 text-sm">
                <div className="flex justify-between">
                  <span className="font-mono">j/k</span>
                  <span>Move up/down</span>
                </div>
                <div className="flex justify-between">
                  <span className="font-mono">gg</span>
                  <span>Jump to top</span>
                </div>
                <div className="flex justify-between">
                  <span className="font-mono">G</span>
                  <span>Jump to bottom</span>
                </div>
              </div>
            </section>
            {/* More sections... */}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
```

---

## Summary: shadcn/ui Benefits

✅ **Pre-built components** — Button, Card, Dialog, Table, Tabs, Badge, etc. → reduces custom CSS
✅ **Full Tailwind control** — Components are copy-pasted (not NPM packages) → override any styling
✅ **Zero-radius enforcement** — Global CSS override in `shadcn-overrides.css`
✅ **Accessibility built-in** — All components WCAG compliant
✅ **TypeScript support** — Full type safety
✅ **Consistency** — Component library ensures design system adherence

## Installation Commands

```bash
cd /Users/samuelasvanyi/dev/bistro/electron

# Install shadcn/ui CLI
npx shadcn-ui@latest init -d

# Add components as needed
npx shadcn-ui@latest add button
npx shadcn-ui@latest add input
npx shadcn-ui@latest add card
npx shadcn-ui@latest add dialog
npx shadcn-ui@latest add tabs
npx shadcn-ui@latest add table
npx shadcn-ui@latest add badge
npx shadcn-ui@latest add dropdown-menu
npx shadcn-ui@latest add checkbox
npx shadcn-ui@latest add scroll-area
```

## Next: Execute Main Plan with shadcn/ui Components

Use the main UI implementation plan (`2026-04-08-ui-implementation-design-system.md`) but replace all custom component files with the shadcn/ui versions shown above. This dramatically accelerates development while maintaining design consistency.
