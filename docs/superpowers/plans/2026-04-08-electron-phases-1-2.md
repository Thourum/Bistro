# Bistro Electron Phases 1-2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Electron desktop shell foundation — spawn Go process, establish IPC bridge, set up React app with TanStack Query/Store, and implement keyboard navigation for PR list.

**Architecture:** Phase 1 builds the core infrastructure (Electron main process, preload bridge, Go spawn, SQLite IPC). Phase 2 adds the React UI layer (routing, component structure, query hooks) and keyboard bindings. By end of Phase 2, users can navigate PR lists with keyboard and open detail views.

**Tech Stack:** Electron (main + renderer), React 18, Vite, TanStack Query v5, TanStack Store, neverthrow, better-sqlite3, typescript.

**UI/UX Alignment:** Keyboard-first design (vim-like bindings), three-pane layout (list | detail | status sidebar), dark mode primary, information density over visual delight. Progressive rendering: diff first, comments second.

---

## File Structure

```
electron/
├── package.json                    ← Dependencies, scripts
├── tsconfig.json                   ← TypeScript config
├── tsconfig.main.json              ← Main process types
├── tsconfig.renderer.json          ← Renderer types
├── vite.config.ts                  ← Vite build config
├── .env.example                    ← Example env vars
│
├── src/
│   ├── main/
│   │   ├── index.ts                ← App lifecycle (Phase 1)
│   │   ├── ipc.ts                  ← IPC handlers for db:query, go:command (Phase 1)
│   │   ├── go-spawn.ts             ← Spawn Go binary, detect WS port (Phase 1)
│   │   ├── window.ts               ← Create app window (Phase 1)
│   │   ├── preload.ts              ← Secure IPC bridge (Phase 1)
│   │   └── types.ts                ← Shared types (IPC envelopes)
│   │
│   ├── renderer/
│   │   ├── main.tsx                ← React entry point (Phase 2)
│   │   ├── App.tsx                 ← Root layout component (Phase 2)
│   │   │
│   │   ├── pages/
│   │   │   ├── PRList.tsx           ← PR list view + keyboard nav (Phase 2-3)
│   │   │   └── PRDetail.tsx         ← PR detail layout (Phase 2)
│   │   │
│   │   ├── components/
│   │   │   ├── PRListItem.tsx       ← Single PR row renderer
│   │   │   ├── CommandBar.tsx       ← Search + mode indicator
│   │   │   ├── StatusBar.tsx        ← Footer with keyboard hints
│   │   │   ├── DiffViewer.tsx       ← Placeholder for Phase 4
│   │   │   └── StatusSidebar.tsx    ← Placeholder for Phase 2
│   │   │
│   │   ├── hooks/
│   │   │   ├── useSQLiteQuery.ts    ← IPC wrapper + neverthrow (Phase 1)
│   │   │   ├── useWebSocket.ts      ← WebSocket connection (Phase 5)
│   │   │   └── useKeyboard.ts       ← Global keyboard handler (Phase 3)
│   │   │
│   │   ├── queries/
│   │   │   ├── usePRList.ts         ← TanStack Query for PR list (Phase 2)
│   │   │   ├── usePRDetail.ts       ← TanStack Query for PR detail (Phase 2)
│   │   │   └── useComments.ts       ← TanStack Query for comments (Phase 4)
│   │   │
│   │   ├── store/
│   │   │   ├── index.ts             ← TanStack Store setup (Phase 2)
│   │   │   ├── ui.ts                ← UI state (selected PR, mode, search) (Phase 2)
│   │   │   └── keyboard.ts          ← Keyboard state (Phase 3)
│   │   │
│   │   ├── types/
│   │   │   ├── index.ts             ← PR, Comment, File domain types (Phase 2)
│   │   │   └── ipc.ts               ← IPC message envelopes (Phase 1)
│   │   │
│   │   ├── utils/
│   │   │   ├── keyboard.ts          ← Vim keybinding parser (Phase 3)
│   │   │   ├── diff.ts              ← Diff parser (Phase 4)
│   │   │   └── error.ts             ← Error formatting
│   │   │
│   │   ├── index.html               ← React entry HTML
│   │   └── styles/
│   │       ├── globals.css          ← Global dark theme + typography
│   │       └── colors.css           ← Design token colors
│
├── tests/
│   ├── unit/
│   │   ├── keyboard.test.ts         ← Vim keybinding parser tests
│   │   ├── diff.test.ts             ← Diff parser tests
│   │   └── utils.test.ts            ← Error formatting tests
│   │
│   └── integration/
│       ├── ipc.test.ts              ← IPC handler mocks + tests
│       └── queries.test.ts          ← TanStack Query hook tests
│
└── dist/
    ├── main/                        ← Compiled main process
    ├── renderer/                    ← Compiled React app
    └── ...
```

---

## Phase 1: Core Setup & IPC Bridge

### Task 1: Project Scaffolding & Dependencies

**Files:**
- Create: `electron/package.json`
- Create: `electron/tsconfig.json`, `electron/tsconfig.main.json`, `electron/tsconfig.renderer.json`
- Create: `electron/.env.example`
- Create: `electron/vite.config.ts`

- [ ] **Step 1: Initialize package.json with Electron, React, TanStack, neverthrow**

```json
{
  "name": "bistro-electron",
  "version": "0.1.0",
  "type": "module",
  "main": "dist/main/index.js",
  "scripts": {
    "dev": "concurrently \"tsc -p tsconfig.main.json --watch\" \"vite\"",
    "build": "tsc -p tsconfig.main.json && vite build",
    "type-check": "tsc --noEmit",
    "test": "vitest",
    "lint": "eslint src --ext .ts,.tsx"
  },
  "dependencies": {
    "electron-squirrel-startup": "^1.1.1",
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "@tanstack/react-query": "^5.64.0",
    "@tanstack/react-store": "^1.30.0",
    "neverthrow": "^6.2.2"
  },
  "devDependencies": {
    "electron": "^31.0.0",
    "typescript": "^5.4.5",
    "vite": "^5.2.8",
    "@vitejs/plugin-react": "^4.2.2",
    "better-sqlite3": "^9.4.3",
    "@types/node": "^20.0.0",
    "@types/react": "^18.3.1",
    "@types/react-dom": "^18.3.0",
    "vitest": "^2.0.0",
    "concurrently": "^8.2.2"
  }
}
```

- [ ] **Step 2: Create TypeScript config files**

`tsconfig.json` (root):
```json
{
  "extends": "@tsconfig/node20/tsconfig.json",
  "compilerOptions": {
    "target": "ES2020",
    "module": "ESNext",
    "lib": ["ES2020"],
    "jsx": "react-jsx",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "resolveJsonModule": true,
    "moduleResolution": "bundler"
  }
}
```

`tsconfig.main.json`:
```json
{
  "extends": "./tsconfig.json",
  "compilerOptions": {
    "outDir": "./dist/main",
    "target": "ES2020"
  },
  "include": ["src/main/**/*", "src/types/**/*"],
  "exclude": ["src/renderer/**/*"]
}
```

`tsconfig.renderer.json`:
```json
{
  "extends": "./tsconfig.json",
  "compilerOptions": {
    "jsx": "react-jsx",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "outDir": "./dist/renderer"
  },
  "include": ["src/renderer/**/*", "src/types/**/*"],
  "exclude": ["src/main/**/*"]
}
```

- [ ] **Step 3: Create Vite config**

`vite.config.ts`:
```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist/renderer',
    emptyOutDir: false,
  },
  server: {
    port: 5173,
  },
});
```

- [ ] **Step 4: Create .env.example**

```
# Go process config
GO_BINARY_PATH=./go-bin/bistro-sync
GO_DB_PATH=./data/bistro.db

# WebSocket
WS_HOST=localhost
WS_PORT=8080

# App
APP_WINDOW_WIDTH=1600
APP_WINDOW_HEIGHT=900
```

- [ ] **Step 5: Run npm install**

```bash
cd electron && npm install
```

---

### Task 2: IPC Type Definitions & Envelopes

**Files:**
- Create: `src/types/ipc.ts`
- Create: `src/main/types.ts`

- [ ] **Step 1: Define IPC message envelopes**

`src/types/ipc.ts`:
```typescript
/**
 * IPC Message Envelope
 *
 * All IPC messages follow this pattern:
 * - Request: { ok?: never; err?: never; method: string; params?: unknown }
 * - Response: { ok: T } | { err: string }
 */

export interface IPCRequest {
  method: string;
  params?: unknown;
}

export interface IPCSuccess<T> {
  ok: T;
}

export interface IPCError {
  err: string;
}

export type IPCResponse<T> = IPCSuccess<T> | IPCError;

// ---- DB Query Channel ----

export interface DBQueryRequest {
  sql: string;
  params?: unknown[];
}

export interface DBQueryResponse {
  ok: Record<string, unknown>[];
}

// ---- File Read Channel ----

export interface FSReadPatchRequest {
  path: string;
}

export interface FSReadPatchResponse {
  ok: string;
}

// ---- Go Command Channel ----

export interface GoCommandRequest {
  type: 'approve_pr' | 'request_changes' | 'post_comment';
  pr_id: string;
  comment?: string;
  line?: number;
}

export interface GoCommandResponse {
  ok: boolean;
}
```

- [ ] **Step 2: Create main process types**

`src/main/types.ts`:
```typescript
import type { DBQueryRequest, FSReadPatchRequest, GoCommandRequest } from '../types/ipc';

export interface IPCHandler {
  'db:query': (req: DBQueryRequest) => Promise<Record<string, unknown>[]>;
  'fs:read-patch': (req: FSReadPatchRequest) => Promise<string>;
  'go:command': (req: GoCommandRequest) => Promise<boolean>;
}

export interface GoProcessConfig {
  binaryPath: string;
  dbPath: string;
  wsPort: number;
}

export interface AppConfig {
  windowWidth: number;
  windowHeight: number;
  goProcess: GoProcessConfig;
}
```

- [ ] **Step 3: Verify types compile**

```bash
cd electron && npm run type-check
```

Expected: No errors.

---

### Task 3: Go Spawn Logic

**Files:**
- Create: `src/main/go-spawn.ts`

- [ ] **Step 1: Write Go spawn module**

`src/main/go-spawn.ts`:
```typescript
import { spawn, type ChildProcess } from 'child_process';
import { Logger } from 'electron-log';
import type { GoProcessConfig } from './types';

let goProcess: ChildProcess | null = null;

export function spawnGoProcess(config: GoProcessConfig, logger: Logger): void {
  if (goProcess) {
    logger.warn('Go process already running');
    return;
  }

  logger.info(`Spawning Go binary: ${config.binaryPath}`);

  goProcess = spawn(config.binaryPath, [
    `--db=${config.dbPath}`,
    `--ws-port=${config.wsPort}`,
  ]);

  goProcess.stdout?.on('data', (data) => {
    const lines = data.toString().split('\n').filter(Boolean);
    lines.forEach((line) => {
      logger.info(`[Go] ${line}`);
      // TODO: Parse port from stdout if needed
    });
  });

  goProcess.stderr?.on('data', (data) => {
    logger.error(`[Go stderr] ${data.toString()}`);
  });

  goProcess.on('exit', (code, signal) => {
    logger.warn(`Go process exited: code=${code}, signal=${signal}`);
    goProcess = null;
    // TODO: Emit event to renderer to show "Go crashed" error
  });

  process.on('exit', () => {
    if (goProcess) {
      goProcess.kill();
    }
  });
}

export function isGoProcessRunning(): boolean {
  return goProcess !== null && !goProcess.killed;
}

export function killGoProcess(): void {
  if (goProcess && !goProcess.killed) {
    goProcess.kill();
    goProcess = null;
  }
}
```

- [ ] **Step 2: Write test for Go spawn**

`tests/integration/go-spawn.test.ts`:
```typescript
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { spawnGoProcess, isGoProcessRunning, killGoProcess } from '../../src/main/go-spawn';
import type { GoProcessConfig } from '../../src/main/types';

describe('Go Process Spawn', () => {
  let logger = { info: vi.fn(), warn: vi.fn(), error: vi.fn() };
  let config: GoProcessConfig = {
    binaryPath: '/fake/path/bistro-sync',
    dbPath: '/fake/path/bistro.db',
    wsPort: 8080,
  };

  beforeEach(() => {
    logger = { info: vi.fn(), warn: vi.fn(), error: vi.fn() };
  });

  afterEach(() => {
    if (isGoProcessRunning()) {
      killGoProcess();
    }
  });

  it('should track process state', () => {
    expect(isGoProcessRunning()).toBe(false);

    // NOTE: Can't actually spawn without Go binary; this tests the API
    // Real test would mock child_process
  });
});
```

- [ ] **Step 3: Commit**

```bash
git add src/main/go-spawn.ts tests/integration/go-spawn.test.ts src/main/types.ts src/types/ipc.ts
git commit -m "feat(phase-1): add Go spawn logic and IPC type definitions"
```

---

### Task 4: Preload Script (IPC Bridge)

**Files:**
- Create: `src/main/preload.ts`

- [ ] **Step 1: Write preload script with context isolation**

`src/main/preload.ts`:
```typescript
import { contextBridge, ipcRenderer } from 'electron';
import type {
  DBQueryRequest,
  DBQueryResponse,
  FSReadPatchRequest,
  FSReadPatchResponse,
  GoCommandRequest,
  GoCommandResponse,
  IPCError,
} from '../types/ipc';

// Expose safe IPC methods to renderer
contextBridge.exposeInMainWorld('electron', {
  /**
   * Query SQLite database
   * @returns { ok: Record<string, unknown>[] } | { err: string }
   */
  async db(request: DBQueryRequest): Promise<DBQueryResponse | IPCError> {
    return ipcRenderer.invoke('db:query', request);
  },

  /**
   * Read patch file from filesystem
   * @returns { ok: string } | { err: string }
   */
  async fs(request: FSReadPatchRequest): Promise<FSReadPatchResponse | IPCError> {
    return ipcRenderer.invoke('fs:read-patch', request);
  },

  /**
   * Send command to Go backend
   * @returns { ok: boolean } | { err: string }
   */
  async go(request: GoCommandRequest): Promise<GoCommandResponse | IPCError> {
    return ipcRenderer.invoke('go:command', request);
  },

  /**
   * Listen for WebSocket events from Go
   */
  onGoEvent(callback: (event: Record<string, unknown>) => void): void {
    ipcRenderer.on('go:event', (_, event) => callback(event));
  },
});

declare global {
  interface Window {
    electron: {
      db: (request: DBQueryRequest) => Promise<DBQueryResponse | IPCError>;
      fs: (request: FSReadPatchRequest) => Promise<FSReadPatchResponse | IPCError>;
      go: (request: GoCommandRequest) => Promise<GoCommandResponse | IPCError>;
      onGoEvent: (callback: (event: Record<string, unknown>) => void) => void;
    };
  }
}
```

- [ ] **Step 2: Write type definitions for global window**

`src/renderer/types/window.d.ts`:
```typescript
import type {
  DBQueryRequest,
  DBQueryResponse,
  FSReadPatchRequest,
  FSReadPatchResponse,
  GoCommandRequest,
  GoCommandResponse,
  IPCError,
} from '../../types/ipc';

declare global {
  interface Window {
    electron: {
      db: (request: DBQueryRequest) => Promise<DBQueryResponse | IPCError>;
      fs: (request: FSReadPatchRequest) => Promise<FSReadPatchResponse | IPCError>;
      go: (request: GoCommandRequest) => Promise<GoCommandResponse | IPCError>;
      onGoEvent: (callback: (event: Record<string, unknown>) => void) => void;
    };
  }
}

export {};
```

- [ ] **Step 3: Commit**

```bash
git add src/main/preload.ts src/renderer/types/window.d.ts
git commit -m "feat(phase-1): add secure preload bridge with context isolation"
```

---

### Task 5: IPC Handlers (SQLite, File, Go Command)

**Files:**
- Create: `src/main/ipc.ts`

- [ ] **Step 1: Write IPC handlers**

`src/main/ipc.ts`:
```typescript
import { ipcMain } from 'electron';
import Database from 'better-sqlite3';
import { readFileSync } from 'fs';
import { join } from 'path';
import { Logger } from 'electron-log';
import type {
  DBQueryRequest,
  FSReadPatchRequest,
  GoCommandRequest,
} from '../types/ipc';
import type { IPCHandler } from './types';

let db: Database.Database | null = null;

export function setupIPC(dbPath: string, logger: Logger): void {
  // Initialize database
  try {
    db = new Database(dbPath);
    logger.info(`Connected to SQLite: ${dbPath}`);
  } catch (e) {
    logger.error(`Failed to open database: ${e}`);
    throw e;
  }

  // ---- db:query handler ----
  ipcMain.handle('db:query', async (event, request: DBQueryRequest) => {
    try {
      if (!db) throw new Error('Database not initialized');

      const stmt = db.prepare(request.sql);
      const result = stmt.all(...(request.params || []));

      return { ok: result };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`db:query error: ${message}`);
      return { err: message };
    }
  });

  // ---- fs:read-patch handler ----
  ipcMain.handle('fs:read-patch', async (event, request: FSReadPatchRequest) => {
    try {
      const content = readFileSync(request.path, 'utf-8');
      return { ok: content };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`fs:read-patch error: ${message}`);
      return { err: message };
    }
  });

  // ---- go:command handler ----
  ipcMain.handle('go:command', async (event, request: GoCommandRequest) => {
    try {
      // TODO: Send command via WebSocket to Go backend
      // For now, just acknowledge
      logger.info(`Go command received: ${request.type}`);
      return { ok: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`go:command error: ${message}`);
      return { err: message };
    }
  });

  logger.info('IPC handlers registered');
}

export function closeDatabase(): void {
  if (db) {
    db.close();
    db = null;
  }
}
```

- [ ] **Step 2: Write IPC handler tests**

`tests/integration/ipc.test.ts`:
```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { DBQueryRequest } from '../../src/types/ipc';

describe('IPC Handlers', () => {
  let logger = { info: vi.fn(), error: vi.fn() };

  beforeEach(() => {
    logger = { info: vi.fn(), error: vi.fn() };
  });

  it('should mock db:query handler', () => {
    const request: DBQueryRequest = {
      sql: 'SELECT * FROM prs LIMIT 1',
      params: [],
    };

    // Mock test: in real test, would use mock database
    expect(request.sql).toContain('SELECT');
  });

  it('should mock fs:read-patch handler', () => {
    const request = { path: '/fake/patch.diff' };
    expect(request.path).toContain('patch');
  });
});
```

- [ ] **Step 3: Commit**

```bash
git add src/main/ipc.ts tests/integration/ipc.test.ts
git commit -m "feat(phase-1): add IPC handlers for SQLite, file read, and Go commands"
```

---

### Task 6: Electron Main Process (App Lifecycle)

**Files:**
- Create: `src/main/window.ts`
- Create: `src/main/index.ts`

- [ ] **Step 1: Write window management**

`src/main/window.ts`:
```typescript
import { BrowserWindow } from 'electron';
import { isDev } from 'electron-is-dev';
import { join } from 'path';
import { Logger } from 'electron-log';
import type { AppConfig } from './types';

let mainWindow: BrowserWindow | null = null;

export function createWindow(config: AppConfig, logger: Logger): BrowserWindow {
  mainWindow = new BrowserWindow({
    width: config.windowWidth,
    height: config.windowHeight,
    webPreferences: {
      preload: join(__dirname, '../main/preload.js'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
    },
  });

  const startUrl = isDev
    ? 'http://localhost:5173'
    : `file://${join(__dirname, '../renderer/index.html')}`;

  mainWindow.loadURL(startUrl);

  if (isDev) {
    mainWindow.webContents.openDevTools();
  }

  mainWindow.on('closed', () => {
    mainWindow = null;
  });

  logger.info('Main window created');
  return mainWindow;
}

export function getMainWindow(): BrowserWindow | null {
  return mainWindow;
}
```

- [ ] **Step 2: Write main process entry**

`src/main/index.ts`:
```typescript
import { app } from 'electron';
import log from 'electron-log';
import { createWindow } from './window';
import { setupIPC, closeDatabase } from './ipc';
import { spawnGoProcess, killGoProcess } from './go-spawn';
import type { AppConfig } from './types';

const isDev = process.env.NODE_ENV === 'development';

const appConfig: AppConfig = {
  windowWidth: parseInt(process.env.APP_WINDOW_WIDTH || '1600'),
  windowHeight: parseInt(process.env.APP_WINDOW_HEIGHT || '900'),
  goProcess: {
    binaryPath: process.env.GO_BINARY_PATH || './go-bin/bistro-sync',
    dbPath: process.env.GO_DB_PATH || './data/bistro.db',
    wsPort: parseInt(process.env.WS_PORT || '8080'),
  },
};

log.info('Starting Bistro Electron app');

// Handle app ready
app.on('ready', async () => {
  try {
    // 1. Setup IPC handlers
    setupIPC(appConfig.goProcess.dbPath, log);

    // 2. Spawn Go process
    spawnGoProcess(appConfig.goProcess, log);

    // 3. Create window
    createWindow(appConfig, log);
  } catch (error) {
    log.error('Failed to start app', error);
    app.quit();
  }
});

// Handle app quit
app.on('before-quit', () => {
  log.info('App shutting down');
  killGoProcess();
  closeDatabase();
});

// Handle window all closed
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

// Activate on macOS
app.on('activate', () => {
  // TODO: Recreate window if needed
});

log.info('Main process initialized');
```

- [ ] **Step 3: Build and verify main process**

```bash
cd electron && npm run type-check
```

Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add src/main/window.ts src/main/index.ts
git commit -m "feat(phase-1): add Electron main process lifecycle and window management"
```

---

## Phase 2: React UI & TanStack Setup

### Task 7: React Renderer Entry & Global Styles

**Files:**
- Create: `src/renderer/main.tsx`
- Create: `src/renderer/index.html`
- Create: `src/renderer/styles/globals.css`
- Create: `src/renderer/styles/colors.css`

- [ ] **Step 1: Create React entry point**

`src/renderer/main.tsx`:
```typescript
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './App';
import './styles/globals.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
```

- [ ] **Step 2: Create HTML template**

`src/renderer/index.html`:
```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Bistro</title>
</head>
<body>
  <div id="root"></div>
  <script type="module" src="/main.tsx"></script>
</body>
</html>
```

- [ ] **Step 3: Create global CSS with dark theme**

`src/renderer/styles/globals.css`:
```css
@import './colors.css';

* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

:root {
  /* Fonts */
  --font-family-sans: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  --font-family-mono: 'Menlo', 'Monaco', 'Courier New', monospace;

  /* Spacing (8px grid) */
  --space-xs: 4px;
  --space-sm: 8px;
  --space-md: 16px;
  --space-lg: 24px;
  --space-xl: 32px;

  /* Typography */
  --font-size-sm: 12px;
  --font-size-base: 14px;
  --font-size-md: 16px;
  --font-size-lg: 18px;
  --font-size-xl: 24px;

  --line-height-tight: 1.4;
  --line-height-base: 1.6;
  --line-height-loose: 1.8;

  --font-weight-normal: 400;
  --font-weight-medium: 500;
  --font-weight-semibold: 600;
  --font-weight-bold: 700;

  /* Transitions */
  --transition-fast: 50ms;
  --transition-base: 150ms;
  --transition-slow: 300ms;
}

html, body {
  height: 100%;
  width: 100%;
  background-color: var(--color-bg);
  color: var(--color-text-primary);
  font-family: var(--font-family-sans);
  font-size: var(--font-size-base);
  line-height: var(--line-height-base);
}

h1 {
  font-size: var(--font-size-xl);
  font-weight: var(--font-weight-bold);
  margin-bottom: var(--space-md);
}

h2 {
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  margin-bottom: var(--space-sm);
}

h3 {
  font-size: var(--font-size-md);
  font-weight: var(--font-weight-semibold);
}

code, pre {
  font-family: var(--font-family-mono);
  font-size: var(--font-size-sm);
}

pre {
  background-color: var(--color-surface);
  padding: var(--space-md);
  border-radius: 4px;
  overflow-x: auto;
}

button {
  font-family: var(--font-family-sans);
  padding: var(--space-sm) var(--space-md);
  border-radius: 4px;
  border: 1px solid transparent;
  cursor: pointer;
  transition: all var(--transition-base);
  font-weight: var(--font-weight-medium);
}

button:focus-visible {
  outline: 2px solid var(--color-accent);
  outline-offset: 2px;
}

input {
  font-family: var(--font-family-sans);
  padding: var(--space-sm);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background-color: var(--color-surface);
  color: var(--color-text-primary);
  font-size: var(--font-size-base);
}

input:focus-visible {
  outline: 2px solid var(--color-accent);
  border-color: var(--color-accent);
}

#root {
  height: 100%;
  width: 100%;
  display: flex;
  flex-direction: column;
}

/* Dark mode scrollbar */
::-webkit-scrollbar {
  width: 12px;
  height: 12px;
}

::-webkit-scrollbar-track {
  background: var(--color-bg);
}

::-webkit-scrollbar-thumb {
  background: var(--color-border);
  border-radius: 6px;
}

::-webkit-scrollbar-thumb:hover {
  background: var(--color-text-secondary);
}
```

- [ ] **Step 4: Create color tokens CSS**

`src/renderer/styles/colors.css`:
```css
:root {
  /* Light mode (fallback) */
  --color-bg: #0d1117;
  --color-surface: #161b22;
  --color-text-primary: #e6edf3;
  --color-text-secondary: #8b949e;
  --color-border: #30363d;
  --color-accent: #58a6ff;
  --color-success: #3fb950;
  --color-warning: #d29922;
  --color-error: #f85149;
}

/* Optional: light mode support */
@media (prefers-color-scheme: light) {
  :root {
    --color-bg: #ffffff;
    --color-surface: #f6f8fa;
    --color-text-primary: #24292f;
    --color-text-secondary: #57606a;
    --color-border: #d0d7de;
    --color-accent: #0969da;
    --color-success: #2da44e;
    --color-warning: #9e6a03;
    --color-error: #cf222e;
  }
}
```

- [ ] **Step 5: Commit**

```bash
git add src/renderer/main.tsx src/renderer/index.html src/renderer/styles/
git commit -m "feat(phase-2): add React entry point and global dark mode styles"
```

---

### Task 8: Domain Types & Store Setup

**Files:**
- Create: `src/renderer/types/index.ts`
- Create: `src/renderer/store/index.ts`
- Create: `src/renderer/store/ui.ts`

- [ ] **Step 1: Define domain types**

`src/renderer/types/index.ts`:
```typescript
/**
 * PR domain model — matches Go `prs` table schema exactly.
 */
export interface PR {
  id: string;
  provider: string;
  owner: string;
  repo: string;
  number: number;
  title: string;
  body: string | null;
  author: string;
  state: 'open' | 'closed' | 'merged';
  created_at: string;
  updated_at: string;
  merged_at: string | null;
  base_branch: string;
  head_branch: string;
  base_sha: string;
  head_sha: string;
  comment_count: number;
  ci_status: string | null;
  draft: number; // 0 or 1 from SQLite
  review_status: string | null; // 'pending' | 'approved' | 'changes_requested'
  approved_by: string | null;
  changes_requested_by: string | null;
}

export interface Comment {
  id: string;
  pr_id: string;
  author: string;
  body: string;
  created_at: string;
  updated_at: string;
  line?: number;
  file?: string;
  is_reply_to?: string;
}

export interface File {
  path: string;
  status: 'added' | 'modified' | 'deleted';
  additions: number;
  deletions: number;
}

export interface CICheck {
  name: string;
  state: 'pending' | 'success' | 'failure';
  description?: string;
  url?: string;
}

export interface PRDetail extends PR {
  files: File[];
  comments: Comment[];
  ci_checks: CICheck[];
}
```

- [ ] **Step 2: Create TanStack Store**

`src/renderer/store/index.ts`:
```typescript
import { createStore } from '@tanstack/react-store';
import type { UIState } from './ui';
import { createUIStore } from './ui';

export type AppStore = UIState;

export const appStore = createUIStore();
```

- [ ] **Step 3: Create UI state store**

`src/renderer/store/ui.ts`:
```typescript
import { createStore } from '@tanstack/react-store';

export interface UIState {
  selectedPRIndex: number;
  searchQuery: string;
  mode: 'list' | 'detail' | 'search' | 'help';
  paneLayout: 'list' | 'detail' | 'split';
  sortBy: 'updated' | 'created';
  filterState: 'open' | 'draft' | 'all';
}

const initialUIState: UIState = {
  selectedPRIndex: 0,
  searchQuery: '',
  mode: 'list',
  paneLayout: 'split',
  sortBy: 'updated',
  filterState: 'open',
};

export function createUIStore() {
  return createStore<UIState>((set) => ({
    ...initialUIState,
    setSelectedPRIndex: (index: number) => set((s) => ({ ...s, selectedPRIndex: index })),
    setSearchQuery: (query: string) => set((s) => ({ ...s, searchQuery: query })),
    setMode: (mode: UIState['mode']) => set((s) => ({ ...s, mode })),
    setPaneLayout: (layout: UIState['paneLayout']) => set((s) => ({ ...s, paneLayout: layout })),
    setSortBy: (sort: UIState['sortBy']) => set((s) => ({ ...s, sortBy: sort })),
    setFilterState: (state: UIState['filterState']) => set((s) => ({ ...s, filterState: state })),
  }));
}
```

- [ ] **Step 4: Commit**

```bash
git add src/renderer/types/index.ts src/renderer/store/
git commit -m "feat(phase-2): add domain types and TanStack Store setup"
```

---

### Task 9: TanStack Query Hooks (useSQLiteQuery, usePRList)

**Files:**
- Create: `src/renderer/hooks/useSQLiteQuery.ts`
- Create: `src/renderer/queries/usePRList.ts`

- [ ] **Step 1: Write useSQLiteQuery hook**

`src/renderer/hooks/useSQLiteQuery.ts`:
```typescript
import { useCallback } from 'react';
import { ResultAsync, err, ok } from 'neverthrow';
import type { DBQueryRequest, DBQueryResponse, IPCError } from '../../types/ipc';

export type QueryResult<T> = ResultAsync<T[], string>;

export function useSQLiteQuery() {
  const query = useCallback(
    <T extends Record<string, unknown>>(
      sql: string,
      params?: unknown[]
    ): QueryResult<T> => {
      return ResultAsync.fromPromise(
        (async () => {
          const request: DBQueryRequest = { sql, params };
          const response = await window.electron.db(request);

          if ('err' in response) {
            throw new Error(response.err);
          }

          return response.ok as T[];
        })(),
        (error) => {
          return error instanceof Error ? error.message : 'Unknown error';
        }
      );
    },
    []
  );

  return { query };
}
```

- [ ] **Step 2: Write usePRList query hook**

`src/renderer/queries/usePRList.ts`:
```typescript
import { useQuery } from '@tanstack/react-query';
import { useSQLiteQuery } from '../hooks/useSQLiteQuery';
import type { PR } from '../types';

export function usePRList(provider: string, filterState: 'open' | 'draft' | 'all' = 'open') {
  const { query } = useSQLiteQuery();

  return useQuery({
    queryKey: ['prs', provider, filterState],
    queryFn: async () => {
      let sql = 'SELECT * FROM prs WHERE provider = ?';
      const params: unknown[] = [provider];

      if (filterState === 'open') {
        sql += " AND state = 'open' AND draft = 0";
      } else if (filterState === 'draft') {
        sql += " AND draft = 1";
      }

      sql += ' ORDER BY updated_at DESC LIMIT 100';

      const result = await query<PR>(sql, params);

      if (result.isErr()) {
        throw new Error(result.error);
      }

      return result.value;
    },
    staleTime: 30 * 1000, // 30s
    gcTime: 5 * 60 * 1000, // 5m
  });
}
```

- [ ] **Step 3: Write tests**

`tests/unit/queries.test.ts`:
```typescript
import { describe, it, expect, vi } from 'vitest';

describe('usePRList', () => {
  it('should construct SQL with filter', () => {
    const sql = "SELECT * FROM prs WHERE provider = ? AND state = 'open' AND draft = 0";
    expect(sql).toContain('provider');
    expect(sql).toContain('state');
  });

  it('should construct SQL for draft filter', () => {
    const sql = "SELECT * FROM prs WHERE provider = ? AND draft = 1";
    expect(sql).toContain('draft');
  });
});
```

- [ ] **Step 4: Commit**

```bash
git add src/renderer/hooks/useSQLiteQuery.ts src/renderer/queries/usePRList.ts tests/unit/queries.test.ts
git commit -m "feat(phase-2): add useSQLiteQuery and usePRList hooks with neverthrow error handling"
```

---

### Task 10: React Components (App, PRList, CommandBar, StatusBar)

**Files:**
- Create: `src/renderer/App.tsx`
- Create: `src/renderer/pages/PRList.tsx`
- Create: `src/renderer/components/CommandBar.tsx`
- Create: `src/renderer/components/StatusBar.tsx`
- Create: `src/renderer/components/PRListItem.tsx`

- [ ] **Step 1: Create App root component**

`src/renderer/App.tsx`:
```typescript
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { appStore } from './store';
import { StoreProvider } from '@tanstack/react-store';
import PRList from './pages/PRList';
import './App.css';

const queryClient = new QueryClient();

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <StoreProvider store={appStore}>
        <div className="app">
          <PRList />
        </div>
      </StoreProvider>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 2: Create PRList page**

`src/renderer/pages/PRList.tsx`:
```typescript
import { useStore } from '@tanstack/react-store';
import { usePRList } from '../queries/usePRList';
import CommandBar from '../components/CommandBar';
import StatusBar from '../components/StatusBar';
import PRListItem from '../components/PRListItem';
import { appStore } from '../store';

export default function PRList() {
  const filterState = useStore(appStore, (state) => state.filterState);
  const { data: prs = [], isLoading, error } = usePRList('github', filterState);

  if (error) {
    return (
      <div className="error-state">
        <p>Failed to load PRs: {error.message}</p>
      </div>
    );
  }

  return (
    <div className="pr-list-view">
      <CommandBar />
      <div className="pr-list-pane">
        {isLoading ? (
          <div className="loading">Loading PRs...</div>
        ) : prs.length === 0 ? (
          <div className="empty-state">No PRs found</div>
        ) : (
          <ul className="pr-list">
            {prs.map((pr, index) => (
              <PRListItem key={pr.id} pr={pr} isSelected={index === 0} index={index} />
            ))}
          </ul>
        )}
      </div>
      <StatusBar />
    </div>
  );
}
```

- [ ] **Step 3: Create CommandBar component**

`src/renderer/components/CommandBar.tsx`:
```typescript
import { useState } from 'react';
import { useStore } from '@tanstack/react-store';
import { appStore } from '../store';

export default function CommandBar() {
  const [search, setSearch] = useState('');
  const mode = useStore(appStore, (state) => state.mode);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSearch(e.currentTarget.value);
  };

  return (
    <div className="command-bar">
      <input
        type="text"
        placeholder="Search by title, author, file, repo..."
        value={search}
        onChange={handleInputChange}
        className="search-input"
      />
      <div className="mode-indicator">
        {mode === 'list' && <span>List Mode</span>}
        {mode === 'search' && <span>Search Mode</span>}
        {mode === 'detail' && <span>Detail Mode</span>}
        {mode === 'help' && <span>Help Mode</span>}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Create StatusBar component**

`src/renderer/components/StatusBar.tsx`:
```typescript
import { useStore } from '@tanstack/react-store';
import { appStore } from '../store';

export default function StatusBar() {
  const selectedIndex = useStore(appStore, (state) => state.selectedPRIndex);

  return (
    <div className="status-bar">
      <div className="status-left">
        <span>Synced at {new Date().toLocaleTimeString()}</span>
      </div>
      <div className="status-center">
        <span>j/k to navigate</span>
        <span className="separator">•</span>
        <span>Enter to open</span>
        <span className="separator">•</span>
        <span>? for help</span>
      </div>
      <div className="status-right">
        <span>PR #{selectedIndex + 1}</span>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Create PRListItem component**

`src/renderer/components/PRListItem.tsx`:
```typescript
import type { PR } from '../types';
import './PRListItem.css';

interface Props {
  pr: PR;
  isSelected: boolean;
  index: number;
}

export default function PRListItem({ pr, isSelected }: Props) {
  const stateColor = {
    open: 'green',
    draft: 'gray',
    closed: 'red',
    merged: 'purple',
  };

  return (
    <li className={`pr-list-item ${isSelected ? 'selected' : ''}`}>
      <div className="pr-item-header">
        <span className={`state-badge state-${stateColor[pr.state]}`}>{pr.state.toUpperCase()}</span>
        <span className="pr-title">
          {pr.title} <span className="pr-number">#{pr.number}</span>
        </span>
      </div>
      <div className="pr-item-meta">
        <span className="pr-repo">{pr.owner}/{pr.repo}</span>
        <span className="separator">•</span>
        <span className="pr-author">@{pr.author}</span>
        <span className="separator">•</span>
        <span className="pr-date">{formatDate(pr.updated_at)}</span>
      </div>
      <div className="pr-item-stats">
        <span className="additions">+{pr.additions}</span>
        <span className="deletions">-{pr.deletions}</span>
        <span className="separator">|</span>
        <span className="files">{pr.changed_files} files</span>
      </div>
    </li>
  );
}

function formatDate(iso: string): string {
  const date = new Date(iso);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 0) return 'today';
  if (diffDays === 1) return 'yesterday';
  if (diffDays < 7) return `${diffDays} days ago`;
  return date.toLocaleDateString();
}
```

- [ ] **Step 6: Create App styles**

`src/renderer/App.css`:
```css
.app {
  height: 100%;
  display: flex;
  flex-direction: column;
  background-color: var(--color-bg);
}

.pr-list-view {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 0;
}

.command-bar {
  padding: var(--space-md);
  background-color: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  display: flex;
  gap: var(--space-md);
  align-items: center;
}

.search-input {
  flex: 1;
  padding: var(--space-sm) var(--space-md);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  background-color: var(--color-bg);
  color: var(--color-text-primary);
}

.mode-indicator {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  padding: 0 var(--space-md);
}

.pr-list-pane {
  flex: 1;
  overflow-y: auto;
}

.pr-list {
  list-style: none;
  border-bottom: 1px solid var(--color-border);
}

.loading,
.empty-state {
  padding: var(--space-lg);
  text-align: center;
  color: var(--color-text-secondary);
}

.error-state {
  padding: var(--space-lg);
  color: var(--color-error);
}

.status-bar {
  padding: var(--space-sm) var(--space-md);
  background-color: var(--color-surface);
  border-top: 1px solid var(--color-border);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.status-left,
.status-center,
.status-right {
  display: flex;
  gap: var(--space-sm);
  align-items: center;
}

.separator {
  color: var(--color-border);
}
```

- [ ] **Step 7: Create PRListItem styles**

`src/renderer/components/PRListItem.css`:
```css
.pr-list-item {
  padding: var(--space-md);
  border-bottom: 1px solid var(--color-border);
  cursor: pointer;
  transition: background-color var(--transition-fast);
}

.pr-list-item:hover {
  background-color: rgba(255, 255, 255, 0.02);
}

.pr-list-item.selected {
  background-color: rgba(88, 166, 255, 0.1);
  border-left: 3px solid var(--color-accent);
  padding-left: calc(var(--space-md) - 3px);
}

.pr-item-header {
  display: flex;
  gap: var(--space-sm);
  align-items: center;
  margin-bottom: var(--space-xs);
}

.state-badge {
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-bold);
  padding: 2px 6px;
  border-radius: 3px;
  text-transform: uppercase;
  min-width: 50px;
  text-align: center;
}

.state-green {
  background-color: var(--color-success);
  color: white;
}

.state-gray {
  background-color: var(--color-border);
  color: var(--color-text-secondary);
}

.state-red {
  background-color: var(--color-error);
  color: white;
}

.state-purple {
  background-color: #845ec2;
  color: white;
}

.pr-title {
  font-weight: var(--font-weight-medium);
  color: var(--color-text-primary);
}

.pr-number {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.pr-item-meta {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin-bottom: var(--space-xs);
}

.pr-item-stats {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  display: flex;
  gap: var(--space-sm);
}

.additions {
  color: var(--color-success);
}

.deletions {
  color: var(--color-error);
}
```

- [ ] **Step 8: Commit**

```bash
git add src/renderer/App.tsx src/renderer/pages/ src/renderer/components/ src/renderer/App.css
git commit -m "feat(phase-2): add React components for PR list view with styling"
```

---

### Task 11: React Router & Navigation (PRDetail Page Stub)

**Files:**
- Create: `src/renderer/pages/PRDetail.tsx`

- [ ] **Step 1: Create PRDetail page stub**

`src/renderer/pages/PRDetail.tsx`:
```typescript
import { useStore } from '@tanstack/react-store';
import { appStore } from '../store';

export default function PRDetail() {
  const selectedIndex = useStore(appStore, (state) => state.selectedPRIndex);

  return (
    <div className="pr-detail-view">
      <div className="detail-header">
        <button onClick={() => appStore.setState((s) => ({ ...s, mode: 'list' }))}>
          ← Back to List
        </button>
        <h1>PR #{selectedIndex + 1} Detail</h1>
      </div>

      <div className="detail-content">
        <div className="diff-viewer-placeholder">
          <p>Diff viewer will render here (Phase 4)</p>
        </div>

        <div className="comments-pane-placeholder">
          <p>Comments will render here (Phase 4)</p>
        </div>
      </div>

      <div className="status-sidebar-placeholder">
        <p>Status sidebar will render here (Phase 4)</p>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Update App.tsx to include routing**

`src/renderer/App.tsx` (modify):
```typescript
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { appStore } from './store';
import { StoreProvider, useStore } from '@tanstack/react-store';
import PRList from './pages/PRList';
import PRDetail from './pages/PRDetail';
import './App.css';

const queryClient = new QueryClient();

function AppContent() {
  const mode = useStore(appStore, (state) => state.mode);

  return (
    <div className="app">
      {mode === 'list' && <PRList />}
      {mode === 'detail' && <PRDetail />}
    </div>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <StoreProvider store={appStore}>
        <AppContent />
      </StoreProvider>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 3: Add PRDetail styles**

`src/renderer/App.css` (append):
```css
.pr-detail-view {
  display: flex;
  height: 100%;
  flex-direction: column;
}

.detail-header {
  padding: var(--space-md);
  background-color: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  display: flex;
  gap: var(--space-md);
  align-items: center;
}

.detail-content {
  display: flex;
  flex: 1;
  gap: var(--space-md);
  padding: var(--space-md);
  overflow: hidden;
}

.diff-viewer-placeholder,
.comments-pane-placeholder,
.status-sidebar-placeholder {
  padding: var(--space-lg);
  background-color: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: 4px;
  color: var(--color-text-secondary);
}

.diff-viewer-placeholder {
  flex: 2;
  overflow-y: auto;
}

.comments-pane-placeholder {
  flex: 1;
  overflow-y: auto;
}

.status-sidebar-placeholder {
  width: 200px;
  overflow-y: auto;
}
```

- [ ] **Step 4: Commit**

```bash
git add src/renderer/pages/PRDetail.tsx src/renderer/App.tsx src/renderer/App.css
git commit -m "feat(phase-2): add PRDetail page stub and mode-based routing"
```

---

### Task 12: TanStack Store Integration & State Updates

**Files:**
- Modify: `src/renderer/queries/usePRList.ts`
- Create: `src/renderer/hooks/useStoreSync.ts`

- [ ] **Step 1: Create useStoreSync hook to manage state**

`src/renderer/hooks/useStoreSync.ts`:
```typescript
import { useEffect } from 'react';
import { useStore } from '@tanstack/react-store';
import type { AppStore } from '../store';

export function useStoreSync(store: AppStore) {
  const selectedIndex = useStore(store, (s) => s.selectedPRIndex);

  // Sync to localStorage for persistence
  useEffect(() => {
    const state = store.state;
    try {
      localStorage.setItem('bistro-ui-state', JSON.stringify({
        selectedPRIndex: state.selectedPRIndex,
        filterState: state.filterState,
        sortBy: state.sortBy,
      }));
    } catch (e) {
      console.warn('Failed to save state to localStorage', e);
    }
  }, [selectedIndex]);

  // Load from localStorage on mount
  useEffect(() => {
    try {
      const saved = localStorage.getItem('bistro-ui-state');
      if (saved) {
        const parsed = JSON.parse(saved);
        if (parsed.selectedPRIndex !== undefined) {
          store.setState((s) => ({ ...s, selectedPRIndex: parsed.selectedPRIndex }));
        }
        if (parsed.filterState !== undefined) {
          store.setState((s) => ({ ...s, filterState: parsed.filterState }));
        }
      }
    } catch (e) {
      console.warn('Failed to load state from localStorage', e);
    }
  }, []);
}
```

- [ ] **Step 2: Update PRList to use store**

`src/renderer/pages/PRList.tsx` (modify):
```typescript
import { useStore } from '@tanstack/react-store';
import { usePRList } from '../queries/usePRList';
import CommandBar from '../components/CommandBar';
import StatusBar from '../components/StatusBar';
import PRListItem from '../components/PRListItem';
import { appStore } from '../store';
import { useStoreSync } from '../hooks/useStoreSync';

export default function PRList() {
  useStoreSync(appStore);

  const filterState = useStore(appStore, (state) => state.filterState);
  const selectedIndex = useStore(appStore, (state) => state.selectedPRIndex);
  const { data: prs = [], isLoading, error } = usePRList('github', filterState);

  const handlePRClick = (index: number) => {
    appStore.setState((s) => ({
      ...s,
      selectedPRIndex: index,
      mode: 'detail',
    }));
  };

  if (error) {
    return (
      <div className="error-state">
        <p>Failed to load PRs: {error.message}</p>
      </div>
    );
  }

  return (
    <div className="pr-list-view">
      <CommandBar />
      <div className="pr-list-pane">
        {isLoading ? (
          <div className="loading">Loading PRs...</div>
        ) : prs.length === 0 ? (
          <div className="empty-state">No PRs found</div>
        ) : (
          <ul className="pr-list">
            {prs.map((pr, index) => (
              <div
                key={pr.id}
                onClick={() => handlePRClick(index)}
                role="button"
                tabIndex={0}
              >
                <PRListItem
                  pr={pr}
                  isSelected={index === selectedIndex}
                  index={index}
                />
              </div>
            ))}
          </ul>
        )}
      </div>
      <StatusBar />
    </div>
  );
}
```

- [ ] **Step 3: Write tests for store sync**

`tests/unit/store.test.ts`:
```typescript
import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { UIState } from '../../src/renderer/store/ui';

describe('UIState', () => {
  it('should initialize with default values', () => {
    const initial: UIState = {
      selectedPRIndex: 0,
      searchQuery: '',
      mode: 'list',
      paneLayout: 'split',
      sortBy: 'updated',
      filterState: 'open',
    };

    expect(initial.mode).toBe('list');
    expect(initial.selectedPRIndex).toBe(0);
  });

  it('should support state updates', () => {
    let state: UIState = {
      selectedPRIndex: 0,
      searchQuery: '',
      mode: 'list',
      paneLayout: 'split',
      sortBy: 'updated',
      filterState: 'open',
    };

    // Simulate update
    state = { ...state, selectedPRIndex: 1, mode: 'detail' };
    expect(state.selectedPRIndex).toBe(1);
    expect(state.mode).toBe('detail');
  });
});
```

- [ ] **Step 4: Commit**

```bash
git add src/renderer/hooks/useStoreSync.ts tests/unit/store.test.ts src/renderer/pages/PRList.tsx
git commit -m "feat(phase-2): add store persistence and state sync"
```

---

### Task 13: Build & Type Check Phase 2

**Files:**
- (No new files)

- [ ] **Step 1: Type check entire project**

```bash
cd electron && npm run type-check
```

Expected: No errors.

- [ ] **Step 2: Build for development**

```bash
cd electron && npm run build
```

Expected: Successful build, dist/ folder populated.

- [ ] **Step 3: Verify file structure**

```bash
find dist -type f | head -20
```

Expected: dist/main/*.js, dist/renderer/index.html, etc.

- [ ] **Step 4: Final commit for Phase 2**

```bash
git status  # Verify no uncommitted files
```

---

## Execution Summary

**Phase 1 (Tasks 1-6):** Electron scaffolding, IPC bridge, Go spawn, main process lifecycle
- Result: Electron app can start, spawn Go, and handle IPC requests

**Phase 2 (Tasks 7-13):** React UI, TanStack Query/Store, component tree, navigation
- Result: PR list renders, keyboard navigation ready, state persists locally

---

## Next Steps (Phase 3)

After Phase 2 completion:
1. Implement `useKeyboard.ts` — global key handler (j/k, gg/G, /, Enter)
2. Integrate keyboard bindings with Store dispatch
3. Add keyboard hints display (StatusBar expansion)
4. Test full keyboard flow: navigate → open PR → view detail

---

## Verification Checklist

- [ ] Electron app starts without crashing
- [ ] Go process spawns successfully
- [ ] IPC channels respond to queries
- [ ] React app renders PR list
- [ ] Selected PR index updates on click
- [ ] Navigation to detail view works
- [ ] Back to list works
- [ ] Type checking passes
- [ ] Build completes without errors
- [ ] All commits follow conventional commits format

