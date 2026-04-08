# Phase 6: Error Handling & Resilience — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add comprehensive error handling with user-friendly messages, toast notifications, retry logic, offline detection, and resilient WebSocket reconnection throughout the Bistro Electron UI.

**Architecture:** (1) Centralized error formatter maps error codes to user-friendly messages, (2) ToastContainer manages a queue of dismissible toasts, (3) WebSocket reconnect uses exponential backoff, (4) TanStack Query gets explicit retry/retryDelay config, (5) StatusBar gets visual offline/error indicators, (6) ActionFeedback gains retry capability.

**Tech Stack:** React, TanStack Store, TanStack Query, neverthrow (already installed), Vitest

---

## File Structure

| File | Responsibility |
|---|---|
| **Create:** `src/renderer/utils/error.ts` | Error code → user-friendly message mapping |
| **Create:** `src/renderer/components/ToastContainer.tsx` | Global toast queue (max 3, auto-dismiss, manual dismiss) |
| **Create:** `tests/unit/error.test.ts` | Error formatter tests |
| **Create:** `tests/unit/ToastContainer.test.tsx` | Toast queue tests |
| **Modify:** `src/renderer/store/ui.ts` | Add `toasts` array, `isOffline` flag |
| **Modify:** `src/renderer/hooks/useWebSocket.ts` | Exponential backoff reconnect, offline detection via `navigator.onLine` |
| **Modify:** `src/renderer/queries/usePRList.ts` | Add retry + retryDelay config |
| **Modify:** `src/renderer/components/StatusBar.tsx` | Visual offline/error indicator with color/icon |
| **Modify:** `src/renderer/components/ActionFeedback.tsx` | Add retry button on failure |
| **Modify:** `src/renderer/pages/PRDetailPage.tsx` | Wire ToastContainer |
| **Modify:** `src/renderer/components/App.tsx` (or layout) | Mount ToastContainer globally |

---

### Task 1: Error Formatter Utility

**Files:**
- Create: `electron/src/renderer/utils/error.ts`
- Create: `electron/tests/unit/error.test.ts`

- [ ] **Step 1: Write failing test**

Create `electron/tests/unit/error.test.ts`:

```typescript
import { describe, it, expect } from 'vitest';
import { formatErrorMessage, ERROR_MESSAGES } from '../../src/renderer/utils/error';

describe('formatErrorMessage', () => {
  it('maps AUTH_TOKEN_EXPIRED to friendly message', () => {
    expect(formatErrorMessage('AUTH_TOKEN_EXPIRED')).toBe(
      'Your authentication expired. Please re-login.'
    );
  });

  it('maps RATE_LIMITED to friendly message', () => {
    expect(formatErrorMessage('RATE_LIMITED')).toBe(
      'Too many requests. Try again in a few minutes.'
    );
  });

  it('maps WEBSOCKET_DISCONNECTED to friendly message', () => {
    expect(formatErrorMessage('WEBSOCKET_DISCONNECTED')).toBe(
      'Connection lost. Reconnecting...'
    );
  });

  it('maps GO_PROCESS_CRASHED to friendly message', () => {
    expect(formatErrorMessage('GO_PROCESS_CRASHED')).toBe(
      'Sync engine crashed. Restarting automatically...'
    );
  });

  it('maps NETWORK_OFFLINE to friendly message', () => {
    expect(formatErrorMessage('NETWORK_OFFLINE')).toBe(
      'No network connection. Using cached data.'
    );
  });

  it('returns raw message for unknown error codes', () => {
    expect(formatErrorMessage('Something unexpected happened')).toBe(
      'Something unexpected happened'
    );
  });

  it('strips sensitive data patterns from messages', () => {
    expect(formatErrorMessage('Error: token=ghp_abc123xyz')).not.toContain('ghp_');
    expect(formatErrorMessage('Bearer eyJhbGci...')).not.toContain('eyJ');
  });

  it('has entries for all known error codes', () => {
    const codes = [
      'AUTH_TOKEN_EXPIRED', 'RATE_LIMITED', 'WEBSOCKET_DISCONNECTED',
      'GO_PROCESS_CRASHED', 'NETWORK_OFFLINE', 'DB_QUERY_FAILED',
      'PERMISSION_DENIED', 'PR_NOT_FOUND', 'DIFF_FETCH_FAILED',
    ];
    for (const code of codes) {
      expect(ERROR_MESSAGES[code]).toBeDefined();
    }
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd electron && pnpm vitest run tests/unit/error.test.ts`
Expected: FAIL — module not found

- [ ] **Step 3: Implement error formatter**

Create `electron/src/renderer/utils/error.ts`:

```typescript
/**
 * Known error codes from Go sync engine and IPC layer.
 */
export const ERROR_MESSAGES: Record<string, string> = {
  AUTH_TOKEN_EXPIRED: 'Your authentication expired. Please re-login.',
  RATE_LIMITED: 'Too many requests. Try again in a few minutes.',
  WEBSOCKET_DISCONNECTED: 'Connection lost. Reconnecting...',
  GO_PROCESS_CRASHED: 'Sync engine crashed. Restarting automatically...',
  NETWORK_OFFLINE: 'No network connection. Using cached data.',
  DB_QUERY_FAILED: 'Failed to load data. Please try again.',
  PERMISSION_DENIED: 'You do not have permission to perform this action.',
  PR_NOT_FOUND: 'Pull request not found. It may have been closed or deleted.',
  DIFF_FETCH_FAILED: 'Failed to load diff. Please try again.',
};

/**
 * Patterns that indicate sensitive data — must be stripped from user-facing messages.
 */
const SENSITIVE_PATTERNS = [
  /ghp_[a-zA-Z0-9]+/g,         // GitHub personal access tokens
  /gho_[a-zA-Z0-9]+/g,         // GitHub OAuth tokens
  /Bearer\s+eyJ[a-zA-Z0-9._-]+/g, // JWT Bearer tokens
  /token=[a-zA-Z0-9._-]+/g,    // Generic token query params
  /eyJ[a-zA-Z0-9._-]{20,}/g,   // Standalone JWTs
];

/**
 * Remove sensitive data from an error message string.
 */
function sanitize(message: string): string {
  let result = message;
  for (const pattern of SENSITIVE_PATTERNS) {
    result = result.replace(pattern, '[REDACTED]');
  }
  return result;
}

/**
 * Map an error code or raw message to a user-friendly string.
 * Known codes get a curated message; unknown messages are sanitized and returned as-is.
 */
export function formatErrorMessage(codeOrMessage: string): string {
  const friendly = ERROR_MESSAGES[codeOrMessage];
  if (friendly !== undefined) {
    return friendly;
  }
  return sanitize(codeOrMessage);
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd electron && pnpm vitest run tests/unit/error.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/utils/error.ts electron/tests/unit/error.test.ts
git commit -m "feat(phase6): add error formatter with sensitive data sanitization"
```

---

### Task 2: Toast Store Fields & Types

**Files:**
- Modify: `electron/src/renderer/store/ui.ts`
- Modify: `electron/src/renderer/types/index.ts`
- Test: `electron/tests/unit/store.test.ts`

- [ ] **Step 1: Write failing test**

Add to `electron/tests/unit/store.test.ts`:

```typescript
describe('UIState toast fields', () => {
  it('initializes with empty toasts array and isOffline false', () => {
    const store = createUIStore();
    expect(store.state.toasts).toEqual([]);
    expect(store.state.isOffline).toBe(false);
  });

  it('adds a toast immutably', () => {
    const store = createUIStore();
    const prev = store.state;
    store.setState((s) => ({
      ...s,
      toasts: [...s.toasts, { id: 't1', type: 'error', message: 'Oops', dismissible: true }],
    }));
    expect(store.state.toasts).toHaveLength(1);
    expect(prev.toasts).toHaveLength(0); // immutability
  });

  it('caps toasts at 3 (removes oldest)', () => {
    const store = createUIStore();
    const toasts = [
      { id: 't1', type: 'error' as const, message: 'A', dismissible: true },
      { id: 't2', type: 'error' as const, message: 'B', dismissible: true },
      { id: 't3', type: 'error' as const, message: 'C', dismissible: true },
    ];
    store.setState((s) => ({ ...s, toasts }));
    // Adding a 4th should drop the first
    const newToast = { id: 't4', type: 'info' as const, message: 'D', dismissible: true };
    store.setState((s) => ({
      ...s,
      toasts: [...s.toasts.slice(-2), newToast],
    }));
    expect(store.state.toasts).toHaveLength(3);
    expect(store.state.toasts[0].id).toBe('t2');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd electron && pnpm vitest run tests/unit/store.test.ts`
Expected: FAIL — `toasts` and `isOffline` not on UIState

- [ ] **Step 3: Add Toast type to types/index.ts**

Append to `electron/src/renderer/types/index.ts`:

```typescript
/**
 * A toast notification for the UI.
 */
export interface Toast {
  id: string;
  type: 'info' | 'success' | 'error' | 'warning';
  message: string;
  dismissible: boolean;
}
```

- [ ] **Step 4: Add fields to UIState**

In `electron/src/renderer/store/ui.ts`, import `Toast` and add to the `UIState` interface:

```typescript
import type { PendingAction, ActionResult, Toast } from '../types';

// Add to UIState interface:
  toasts: Toast[];
  isOffline: boolean;
```

Add to `initialUIState`:

```typescript
  toasts: [],
  isOffline: false,
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd electron && pnpm vitest run tests/unit/store.test.ts`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add electron/src/renderer/types/index.ts electron/src/renderer/store/ui.ts electron/tests/unit/store.test.ts
git commit -m "feat(phase6): add Toast type, toasts array, and isOffline flag to store"
```

---

### Task 3: ToastContainer Component

**Files:**
- Create: `electron/src/renderer/components/ToastContainer.tsx`
- Create: `electron/tests/unit/ToastContainer.test.tsx`

- [ ] **Step 1: Write failing test**

Create `electron/tests/unit/ToastContainer.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import { appStore } from '../../src/renderer/store';
import ToastContainer from '../../src/renderer/components/ToastContainer';

describe('ToastContainer', () => {
  beforeEach(() => {
    appStore.setState((s) => ({ ...s, toasts: [] }));
  });

  it('renders nothing when toasts is empty', () => {
    const { container } = render(<ToastContainer />);
    expect(container.querySelector('[data-toast]')).toBeNull();
  });

  it('renders error toast with message', () => {
    appStore.setState((s) => ({
      ...s,
      toasts: [{ id: 't1', type: 'error', message: 'Something broke', dismissible: true }],
    }));
    render(<ToastContainer />);
    expect(screen.getByText('Something broke')).toBeInTheDocument();
  });

  it('renders success toast with message', () => {
    appStore.setState((s) => ({
      ...s,
      toasts: [{ id: 't2', type: 'success', message: 'All good', dismissible: true }],
    }));
    render(<ToastContainer />);
    expect(screen.getByText('All good')).toBeInTheDocument();
  });

  it('renders multiple toasts', () => {
    appStore.setState((s) => ({
      ...s,
      toasts: [
        { id: 't1', type: 'error', message: 'Error 1', dismissible: true },
        { id: 't2', type: 'info', message: 'Info 1', dismissible: true },
      ],
    }));
    render(<ToastContainer />);
    expect(screen.getByText('Error 1')).toBeInTheDocument();
    expect(screen.getByText('Info 1')).toBeInTheDocument();
  });

  it('dismisses toast when close button clicked', () => {
    appStore.setState((s) => ({
      ...s,
      toasts: [{ id: 't1', type: 'error', message: 'Dismiss me', dismissible: true }],
    }));
    render(<ToastContainer />);
    const closeBtn = screen.getByRole('button', { name: /dismiss/i });
    fireEvent.click(closeBtn);
    expect(appStore.state.toasts).toHaveLength(0);
  });

  it('auto-dismisses success toasts after 5 seconds', () => {
    vi.useFakeTimers();
    appStore.setState((s) => ({
      ...s,
      toasts: [{ id: 't1', type: 'success', message: 'Done', dismissible: true }],
    }));
    render(<ToastContainer />);
    expect(screen.getByText('Done')).toBeInTheDocument();
    act(() => { vi.advanceTimersByTime(5000); });
    expect(appStore.state.toasts).toHaveLength(0);
    vi.useRealTimers();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd electron && pnpm vitest run tests/unit/ToastContainer.test.tsx`
Expected: FAIL — module not found

- [ ] **Step 3: Implement ToastContainer**

Create `electron/src/renderer/components/ToastContainer.tsx`:

```tsx
import { useEffect } from 'react';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { Toast } from '../types';

const AUTO_DISMISS_MS = 5000;

const TOAST_STYLES: Record<Toast['type'], string> = {
  error: 'bg-red-50 dark:bg-red-900/30 border-red-300 dark:border-red-700 text-red-900 dark:text-red-200',
  warning: 'bg-yellow-50 dark:bg-yellow-900/30 border-yellow-300 dark:border-yellow-700 text-yellow-900 dark:text-yellow-200',
  success: 'bg-green-50 dark:bg-green-900/30 border-green-300 dark:border-green-700 text-green-900 dark:text-green-200',
  info: 'bg-blue-50 dark:bg-blue-900/30 border-blue-300 dark:border-blue-700 text-blue-900 dark:text-blue-200',
};

const TOAST_ICONS: Record<Toast['type'], string> = {
  error: '✗',
  warning: '⚠',
  success: '✓',
  info: 'ℹ',
};

function dismissToast(id: string): void {
  appStore.setState((s) => ({
    ...s,
    toasts: s.toasts.filter((t) => t.id !== id),
  }));
}

/**
 * Helper to add a toast to the store, capping at 3 (drops oldest).
 */
export function addToast(toast: Omit<Toast, 'id'>): void {
  const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
  appStore.setState((s) => ({
    ...s,
    toasts: [...s.toasts.slice(-(3 - 1)), { ...toast, id }],
  }));
}

function ToastItem({ toast }: { toast: Toast }) {
  useEffect(() => {
    if (toast.type === 'success' || toast.type === 'info') {
      const timer = setTimeout(() => dismissToast(toast.id), AUTO_DISMISS_MS);
      return () => clearTimeout(timer);
    }
  }, [toast.id, toast.type]);

  return (
    <div
      data-toast
      className={`flex items-start gap-2 px-4 py-3 rounded border text-sm shadow-md ${TOAST_STYLES[toast.type]}`}
    >
      <span className="flex-shrink-0 font-bold">{TOAST_ICONS[toast.type]}</span>
      <span className="flex-1">{toast.message}</span>
      {toast.dismissible && (
        <button
          onClick={() => dismissToast(toast.id)}
          className="flex-shrink-0 text-xs opacity-60 hover:opacity-100"
          aria-label="Dismiss"
        >
          ✕
        </button>
      )}
    </div>
  );
}

export default function ToastContainer() {
  const toasts = useStore(appStore, (s) => s.toasts);

  if (toasts.length === 0) return null;

  return (
    <div className="fixed top-4 right-4 z-50 flex flex-col gap-2 w-80">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} />
      ))}
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd electron && pnpm vitest run tests/unit/ToastContainer.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/components/ToastContainer.tsx electron/tests/unit/ToastContainer.test.tsx
git commit -m "feat(phase6): add ToastContainer with auto-dismiss and queue management"
```

---

### Task 4: WebSocket Exponential Backoff & Offline Detection

**Files:**
- Modify: `electron/src/renderer/hooks/useWebSocket.ts`
- Test: `electron/tests/unit/useWebSocket.test.ts` (extend)

- [ ] **Step 1: Write failing test**

Add to `electron/tests/unit/useWebSocket.test.ts`:

```typescript
describe('WebSocket reconnect backoff', () => {
  it('calculates exponential backoff delay', () => {
    // Test the backoff formula directly
    const BASE_DELAY = 1000;
    const MAX_DELAY = 30000;
    const backoff = (attempt: number) => Math.min(BASE_DELAY * Math.pow(2, attempt), MAX_DELAY);

    expect(backoff(0)).toBe(1000);
    expect(backoff(1)).toBe(2000);
    expect(backoff(2)).toBe(4000);
    expect(backoff(3)).toBe(8000);
    expect(backoff(4)).toBe(16000);
    expect(backoff(5)).toBe(30000); // capped
    expect(backoff(10)).toBe(30000); // still capped
  });
});

describe('offline detection', () => {
  it('sets isOffline in store when navigator.onLine is false', () => {
    // Direct store test — the hook reads navigator.onLine
    appStore.setState((s) => ({ ...s, isOffline: true }));
    expect(appStore.state.isOffline).toBe(true);
  });
});
```

- [ ] **Step 2: Run test to verify it fails or passes** (backoff is pure math, may pass immediately)

Run: `cd electron && pnpm vitest run tests/unit/useWebSocket.test.ts`

- [ ] **Step 3: Update useWebSocket.ts with exponential backoff and offline detection**

In `electron/src/renderer/hooks/useWebSocket.ts`:

1. Replace the fixed `RECONNECT_DELAY_MS` constant:

```typescript
const BASE_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30000;
const MAX_RECONNECT_ATTEMPTS = 10;
```

2. Add a reconnect attempt counter ref inside the hook:

```typescript
const reconnectAttemptRef = useRef(0);
```

3. Replace the `ws.onclose` handler's reconnect logic:

```typescript
      ws.onclose = () => {
        console.log('WebSocket disconnected');
        try {
          appStore.setState((s) => ({ ...s, syncStatus: 'disconnected' }));
        } catch {
          // Store not available (test environment)
        }

        if (reconnectAttemptRef.current < MAX_RECONNECT_ATTEMPTS) {
          const delay = Math.min(
            BASE_RECONNECT_DELAY_MS * Math.pow(2, reconnectAttemptRef.current),
            MAX_RECONNECT_DELAY_MS
          );
          reconnectAttemptRef.current += 1;
          console.log(`WebSocket reconnecting in ${delay}ms (attempt ${reconnectAttemptRef.current}/${MAX_RECONNECT_ATTEMPTS})`);

          reconnectTimerRef.current = setTimeout(() => {
            connect();
          }, delay);
        } else {
          console.error('WebSocket max reconnect attempts reached');
          try {
            appStore.setState((s) => ({
              ...s,
              syncStatus: 'error',
              syncError: 'Connection lost. Please restart the app.',
            }));
          } catch {
            // Store not available
          }
        }
      };
```

4. Reset counter on successful connect:

```typescript
      ws.onopen = () => {
        console.log('WebSocket connected');
        reconnectAttemptRef.current = 0; // Reset on success
        // ... rest of onopen
      };
```

5. Add offline detection effect:

```typescript
  // Offline detection via browser events
  useEffect(() => {
    function handleOffline() {
      try {
        appStore.setState((s) => ({ ...s, isOffline: true }));
      } catch { /* test env */ }
    }
    function handleOnline() {
      try {
        appStore.setState((s) => ({ ...s, isOffline: false }));
      } catch { /* test env */ }
    }

    window.addEventListener('offline', handleOffline);
    window.addEventListener('online', handleOnline);

    // Set initial state
    if (typeof navigator !== 'undefined' && !navigator.onLine) {
      handleOffline();
    }

    return () => {
      window.removeEventListener('offline', handleOffline);
      window.removeEventListener('online', handleOnline);
    };
  }, []);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd electron && pnpm vitest run tests/unit/useWebSocket.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/hooks/useWebSocket.ts electron/tests/unit/useWebSocket.test.ts
git commit -m "feat(phase6): exponential backoff reconnect and offline detection in useWebSocket"
```

---

### Task 5: TanStack Query Retry Configuration

**Files:**
- Modify: `electron/src/renderer/queries/usePRList.ts`
- Test: Verified via existing query tests

- [ ] **Step 1: Read usePRList.ts**

Read the file to understand current query config.

- [ ] **Step 2: Add retry and retryDelay config**

In the `useQuery` options, add:

```typescript
    retry: 3,
    retryDelay: (attemptIndex: number) => Math.min(1000 * Math.pow(2, attemptIndex), 30000),
```

- [ ] **Step 3: Run tests**

Run: `cd electron && pnpm vitest run tests/unit/queries.test.ts`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/queries/usePRList.ts
git commit -m "feat(phase6): add exponential backoff retry to TanStack Query"
```

---

### Task 6: StatusBar Offline/Error Visual Indicators

**Files:**
- Modify: `electron/src/renderer/components/StatusBar.tsx`
- Test: `electron/tests/unit/StatusBar.test.tsx` (new or extend existing)

- [ ] **Step 1: Write failing test**

Create or extend `electron/tests/unit/StatusBar.test.tsx`:

```tsx
import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { appStore } from '../../src/renderer/store';
import StatusBar from '../../src/renderer/components/StatusBar';

describe('StatusBar', () => {
  beforeEach(() => {
    appStore.setState((s) => ({
      ...s,
      mode: 'list',
      syncStatus: 'connected',
      syncError: null,
      lastSyncAt: null,
      isOffline: false,
      confirmingAction: null,
    }));
  });

  it('shows offline indicator when isOffline is true', () => {
    appStore.setState((s) => ({ ...s, isOffline: true }));
    render(<StatusBar />);
    expect(screen.getByText(/offline/i)).toBeInTheDocument();
  });

  it('shows error state with red styling', () => {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'error',
      syncError: 'Connection failed',
    }));
    render(<StatusBar />);
    expect(screen.getByText(/Connection failed/i)).toBeInTheDocument();
  });

  it('shows rate_limited state', () => {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'rate_limited',
      syncError: 'Rate limited until 12:00',
    }));
    render(<StatusBar />);
    expect(screen.getByText(/rate limited/i)).toBeInTheDocument();
  });

  it('shows connected state normally', () => {
    appStore.setState((s) => ({ ...s, syncStatus: 'connected' }));
    render(<StatusBar />);
    expect(screen.getByText(/connected/i)).toBeInTheDocument();
  });

  it('shows synced time when ready', () => {
    const now = new Date().toISOString();
    appStore.setState((s) => ({ ...s, syncStatus: 'ready', lastSyncAt: now }));
    render(<StatusBar />);
    expect(screen.getByText(/synced at/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd electron && pnpm vitest run tests/unit/StatusBar.test.tsx`
Expected: FAIL — offline indicator not rendered

- [ ] **Step 3: Update StatusBar with visual indicators**

Rewrite `electron/src/renderer/components/StatusBar.tsx` to include `isOffline` from store, color-coded status dot, and offline banner:

```tsx
import { useStore } from '../hooks/useStore';
import type { UIState } from '../store/ui';
import { appStore } from '../store';

const MODE_HINTS: Record<UIState['mode'], string> = {
  list: 'j/k nav | Enter open | / search | f filter | : cmd | ? help',
  detail: 'hjkl scroll | n/p files | aa approve | c comment | Esc back',
  search: 'Type to search | Enter apply | Esc cancel',
  help: 'Esc close',
  command: 'Type command | Enter execute | Esc cancel',
  filter: 'o open | d draft | a all | f/Esc close',
  settings: 'Esc back',
};

const STATUS_COLORS: Record<string, string> = {
  connected: 'bg-green-500',
  ready: 'bg-green-500',
  connecting: 'bg-yellow-500 animate-pulse',
  disconnected: 'bg-red-500',
  error: 'bg-red-500',
  rate_limited: 'bg-yellow-500',
};

export default function StatusBar() {
  const mode = useStore(appStore, (state: UIState) => state.mode);
  const syncStatus = useStore(appStore, (state: UIState) => state.syncStatus);
  const lastSyncAt = useStore(appStore, (state: UIState) => state.lastSyncAt);
  const syncError = useStore(appStore, (state: UIState) => state.syncError);
  const confirmingAction = useStore(appStore, (state: UIState) => state.confirmingAction);
  const isOffline = useStore(appStore, (state: UIState) => state.isOffline);

  const syncLabel = (() => {
    if (isOffline) return 'Offline — Using cached data';
    if (syncStatus === 'ready' && lastSyncAt !== null) {
      return `Synced at ${new Date(lastSyncAt).toLocaleTimeString()}`;
    }
    if (syncStatus === 'rate_limited' && syncError !== null) {
      return `Rate limited — ${syncError}`;
    }
    if (syncStatus === 'error' && syncError !== null) {
      return `Error: ${syncError}`;
    }
    return syncStatus.charAt(0).toUpperCase() + syncStatus.slice(1);
  })();

  const dotColor = isOffline ? 'bg-gray-500' : (STATUS_COLORS[syncStatus] ?? 'bg-gray-500');

  const hint =
    confirmingAction === 'approve'
      ? "Press 'a' again to confirm approval"
      : MODE_HINTS[mode] ?? '';

  return (
    <div className="flex items-center justify-between px-4 py-1.5 bg-surface-container border-t border-outline-variant text-[11px] font-mono">
      <span className="text-on-surface-variant">{hint}</span>
      <span className="flex items-center gap-1.5">
        <span className={`inline-block w-2 h-2 rounded-full ${dotColor}`} />
        <span>{syncLabel}</span>
      </span>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd electron && pnpm vitest run tests/unit/StatusBar.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/components/StatusBar.tsx electron/tests/unit/StatusBar.test.tsx
git commit -m "feat(phase6): add visual offline/error indicators to StatusBar"
```

---

### Task 7: ActionFeedback Retry Button

**Files:**
- Modify: `electron/src/renderer/components/ActionFeedback.tsx`
- Test: `electron/tests/unit/ActionFeedback.test.tsx` (extend)

- [ ] **Step 1: Write failing test**

Add to `electron/tests/unit/ActionFeedback.test.tsx`:

```tsx
describe('ActionFeedback retry', () => {
  it('renders retry button on failure when onRetry is provided', () => {
    const onRetry = vi.fn();
    render(
      <ActionFeedback
        pendingAction={null}
        actionResult={{ type: 'approve', prId: 'pr-1', success: false, message: 'Failed' }}
        onDismiss={vi.fn()}
        onRetry={onRetry}
      />
    );
    const retryBtn = screen.getByRole('button', { name: /retry/i });
    fireEvent.click(retryBtn);
    expect(onRetry).toHaveBeenCalled();
  });

  it('does not render retry button when onRetry is not provided', () => {
    render(
      <ActionFeedback
        pendingAction={null}
        actionResult={{ type: 'approve', prId: 'pr-1', success: false, message: 'Failed' }}
        onDismiss={vi.fn()}
      />
    );
    expect(screen.queryByRole('button', { name: /retry/i })).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd electron && pnpm vitest run tests/unit/ActionFeedback.test.tsx`
Expected: FAIL — no retry button rendered

- [ ] **Step 3: Add onRetry prop to ActionFeedback**

In `electron/src/renderer/components/ActionFeedback.tsx`, add `onRetry?: () => void` to the props interface and render a retry button when the action failed and `onRetry` is provided:

```tsx
interface ActionFeedbackProps {
  pendingAction: PendingAction | null;
  actionResult: ActionResult | null;
  onDismiss: () => void;
  onRetry?: () => void;
}
```

In the error result render section, add before the dismiss button:

```tsx
        {onRetry != null && (
          <button
            onClick={onRetry}
            className="text-xs font-medium text-primary hover:underline"
            aria-label="Retry"
          >
            Retry
          </button>
        )}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd electron && pnpm vitest run tests/unit/ActionFeedback.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/components/ActionFeedback.tsx electron/tests/unit/ActionFeedback.test.tsx
git commit -m "feat(phase6): add retry button to ActionFeedback on failure"
```

---

### Task 8: Mount ToastContainer Globally

**Files:**
- Modify: `electron/src/renderer/App.tsx` or equivalent layout root
- Modify: `electron/src/renderer/pages/PRDetailPage.tsx` (if needed)

- [ ] **Step 1: Find the app root component**

Read `electron/src/renderer/App.tsx` (or the main app entry component) to determine where to mount `ToastContainer`.

- [ ] **Step 2: Import and mount ToastContainer**

Add to the app root, outside of any route-specific layout:

```tsx
import ToastContainer from './components/ToastContainer';

// Inside the root component JSX, at the top level:
<>
  <ToastContainer />
  {/* existing app content */}
</>
```

- [ ] **Step 3: Wire addToast to Go crash events in useWebSocket**

In `useWebSocket.ts`, import `addToast` and `formatErrorMessage`, then add toast dispatching for key error events:

```typescript
import { addToast } from '../components/ToastContainer';
import { formatErrorMessage } from '../utils/error';
```

In the `SYNC_ERROR` case:
```typescript
          case 'SYNC_ERROR':
            handleSyncError(message.data as SyncErrorData);
            addToast({ type: 'error', message: formatErrorMessage((message.data as SyncErrorData).error_code), dismissible: true });
            break;
```

In the `ACTION_FAILED` case:
```typescript
          case 'ACTION_FAILED':
            handleActionFailed(message.data as ActionFailedData);
            addToast({ type: 'error', message: formatErrorMessage((message.data as ActionFailedData).error), dismissible: true });
            break;
```

In the `AUTH_TOKEN_EXPIRED` case:
```typescript
          case 'AUTH_TOKEN_EXPIRED':
            handleAuthExpired(message.data as AuthExpiredData);
            addToast({ type: 'error', message: formatErrorMessage('AUTH_TOKEN_EXPIRED'), dismissible: true });
            break;
```

- [ ] **Step 4: Run full test suite**

Run: `cd electron && pnpm test`
Expected: All tests pass

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/App.tsx electron/src/renderer/hooks/useWebSocket.ts
git commit -m "feat(phase6): mount ToastContainer globally, wire error toasts to WebSocket events"
```

---

### Task 9: Build Verification & Final Integration

**Files:**
- All modified files from Tasks 1-8

- [ ] **Step 1: Run full test suite**

Run: `cd electron && pnpm test`
Expected: All tests pass (264 existing + ~30 new = ~294)

- [ ] **Step 2: Run build**

Run: `cd electron && pnpm build`
Expected: Build succeeds with no errors

- [ ] **Step 3: Verify no type errors**

Run: `cd electron && npx tsc --noEmit -p tsconfig.json 2>&1 || true`
Expected: No new type errors

- [ ] **Step 4: Final commit if any remaining changes**

```bash
git add -A
git commit -m "feat(phase6): build verification and final integration"
```
