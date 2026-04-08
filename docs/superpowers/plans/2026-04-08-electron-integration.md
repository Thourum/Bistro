# Electron Integration — WebSocket & IPC Bridge to Go Sync Engine

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Integrate the Bistro Go Sync Engine with the Electron UI via WebSocket for real-time PR data and action feedback.

**Architecture:** Go sync engine runs as a subprocess, exposes WebSocket server on `ws://localhost:8080`. Electron connects via WebSocket, sends actions, receives real-time events.

**Tech Stack:** Go 1.21+ (WebSocket server), TypeScript/Electron (WebSocket client), IPC bridge, neverthrow Result types.

---

## File Structure

```
go-sync-engine/
├── go/
│   ├── cmd/bistro-sync/
│   │   └── main.go (MODIFY) — Add --ws-port, --db-path flags
│   └── internal/
│       └── server/app.go (MODIFY) — Start WebSocket server on init

electron/
├── src/
│   ├── main/
│   │   ├── go-spawn.ts (MODIFY) — Spawn sync engine with flags
│   │   ├── ipc.ts (MODIFY) — Add go:sync handlers for WebSocket commands
│   │   └── websocket-client.ts (NEW) — WebSocket connection & event handling
│   ├── types/
│   │   └── ipc.ts (MODIFY) — Add sync:subscribe, sync:action types
│   └── renderer/
│       ├── hooks/
│       │   └── useSyncEvents.ts (NEW) — Subscribe to sync events
│       └── pages/
│           └── PRListPage.tsx (MODIFY) — Use useSyncEvents hook
```

---

## Task 1: Go Sync Engine WebSocket Server

**Files:**
- Modify: `go/cmd/bistro-sync/main.go` — Add CLI flags
- Modify: `go/internal/server/app.go` — Start WebSocket server

**Steps:**

- [ ] **Step 1: Add CLI flags to main.go**

```go
import "flag"

func main() {
	wsPort := flag.Int("ws-port", 8080, "WebSocket server port")
	dbPath := flag.String("db-path", "./data/sync.db", "SQLite database path")
	flag.Parse()

	// Pass to server initialization
	app := server.NewApp(*wsPort, *dbPath)
	if err := app.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Update NewApp signature in app.go**

```go
func NewApp(wsPort int, dbPath string) *App {
	return &App{
		wsPort: wsPort,
		dbPath: dbPath,
		// ... rest of initialization
	}
}
```

- [ ] **Step 3: Start WebSocket server in Run method**

```go
func (a *App) Run(ctx context.Context) error {
	// Start WebSocket server
	wsServer := ws.NewServer(a.log, a.eventEmitter)
	go func() {
		if err := wsServer.ListenAndServe(fmt.Sprintf(":%d", a.wsPort)); err != nil && err != http.ErrServerClosed {
			a.log.Error("WebSocket server error", "error", err)
		}
	}()

	// ... rest of app startup
}
```

- [ ] **Step 4: Verify build succeeds**

```bash
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
go build ./cmd/bistro-sync
```

Expected: Binary builds successfully

- [ ] **Step 5: Commit**

```bash
git add go/cmd/bistro-sync/main.go go/internal/server/app.go
git commit -m "feat: add WebSocket server integration with CLI flags"
```

---

## Task 2: Electron WebSocket Client

**Files:**
- Create: `electron/src/main/websocket-client.ts` — WebSocket connection management

**Steps:**

- [ ] **Step 1: Write WebSocket client**

```typescript
// electron/src/main/websocket-client.ts
import { EventEmitter } from 'events';
import { Logger } from 'electron-log';

export interface SyncEvent {
  type: 'sync_started' | 'progress' | 'sync_complete' | 'action_complete' | 'action_failed';
  data?: Record<string, unknown>;
  timestamp: number;
}

export class WebSocketClient extends EventEmitter {
  private ws: WebSocket | null = null;
  private url: string;
  private logger: Logger;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private reconnectDelay = 1000;

  constructor(url: string, logger: Logger) {
    super();
    this.url = url;
    this.logger = logger;
  }

  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      try {
        this.ws = new WebSocket(this.url);

        this.ws.onopen = () => {
          this.logger.info('WebSocket connected');
          this.reconnectAttempts = 0;
          this.emit('connected');
          resolve();
        };

        this.ws.onmessage = (event) => {
          try {
            const syncEvent: SyncEvent = JSON.parse(event.data);
            this.emit('event', syncEvent);
          } catch (error) {
            this.logger.error('Failed to parse sync event', error);
          }
        };

        this.ws.onerror = (error) => {
          this.logger.error('WebSocket error', error);
          this.emit('error', error);
          reject(error);
        };

        this.ws.onclose = () => {
          this.logger.info('WebSocket disconnected');
          this.emit('disconnected');
          this.attemptReconnect();
        };
      } catch (error) {
        reject(error);
      }
    });
  }

  private attemptReconnect(): void {
    if (this.reconnectAttempts < this.maxReconnectAttempts) {
      this.reconnectAttempts++;
      const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
      this.logger.info(`Reconnecting in ${delay}ms...`);

      setTimeout(() => {
        this.connect().catch((error) => {
          this.logger.error('Reconnection failed', error);
        });
      }, delay);
    }
  }

  send(action: { type: string; pr_id: string; comment?: string; line?: number }): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(action));
    } else {
      this.logger.error('WebSocket not connected');
      throw new Error('WebSocket not connected');
    }
  }

  close(): void {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }
}
```

- [ ] **Step 2: Update go-spawn.ts to pass WebSocket port**

```typescript
// electron/src/main/go-spawn.ts (MODIFY)
import { ChildProcess, spawn } from 'child_process';
import { Logger } from 'electron-log';

export function spawnGoSyncEngine(logger: Logger, wsPort: number = 8080): ChildProcess {
  const dbPath = '/path/to/sync.db'; // Use app data directory

  const child = spawn('bistro-sync', [
    '--ws-port', wsPort.toString(),
    '--db-path', dbPath
  ]);

  child.stdout?.on('data', (data) => {
    logger.info(`[Go] ${data.toString()}`);
  });

  child.stderr?.on('data', (data) => {
    logger.error(`[Go] ${data.toString()}`);
  });

  return child;
}
```

- [ ] **Step 3: Update main process to initialize WebSocket client**

```typescript
// electron/src/main/index.ts (MODIFY)
import { WebSocketClient } from './websocket-client';

let wsClient: WebSocketClient | null = null;

function initializeApp() {
  // ... existing window setup ...

  // Initialize WebSocket connection
  wsClient = new WebSocketClient('ws://localhost:8080', logger);

  wsClient.on('event', (event) => {
    // Broadcast to renderer
    mainWindow?.webContents.send('sync:event', event);
  });

  wsClient.connect().catch((error) => {
    logger.error('Failed to connect to sync engine', error);
  });
}
```

- [ ] **Step 4: Commit**

```bash
git add electron/src/main/websocket-client.ts electron/src/main/go-spawn.ts electron/src/main/index.ts
git commit -m "feat: add WebSocket client for sync engine integration"
```

---

## Task 3: IPC Handlers for Sync Commands

**Files:**
- Modify: `electron/src/types/ipc.ts` — Add sync types
- Modify: `electron/src/main/ipc.ts` — Add sync handlers

**Steps:**

- [ ] **Step 1: Update IPC types**

```typescript
// electron/src/types/ipc.ts (ADD)
export interface SyncSubscribeRequest {
  // No params - just subscribe to all events
}

export interface SyncActionRequest {
  type: 'approve_pr' | 'request_changes' | 'post_comment';
  pr_id: string;
  comment?: string;
  line?: number;
}

export interface SyncActionResponse {
  ok: { success: boolean; message?: string };
}
```

- [ ] **Step 2: Add sync handlers to IPC**

```typescript
// electron/src/main/ipc.ts (ADD handlers)
import { WebSocketClient } from './websocket-client';

let wsClient: WebSocketClient | null = null;

export function setupIPC(dbPath: string, logger: Logger, webSocket: WebSocketClient): void {
  wsClient = webSocket;

  // ... existing handlers ...

  // ---- sync:subscribe handler ----
  ipcMain.on('sync:subscribe', (event) => {
    // Just mark that this renderer is interested in sync events
    // Main process broadcasts all sync:event messages to all renderers
    logger.info('Renderer subscribed to sync events');
  });

  // ---- sync:action handler ----
  ipcMain.handle('sync:action', async (event, request: SyncActionRequest) => {
    try {
      if (!wsClient || !wsClient.isConnected()) {
        return { err: 'WebSocket not connected' };
      }

      wsClient.send({
        type: request.type,
        pr_id: request.pr_id,
        comment: request.comment,
        line: request.line
      });

      return { ok: { success: true } };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`sync:action error: ${message}`);
      return { err: message };
    }
  });

  logger.info('Sync IPC handlers registered');
}
```

- [ ] **Step 3: Update main index.ts to pass WebSocket to setupIPC**

```typescript
// electron/src/main/index.ts (MODIFY)
setupIPC(dbPath, logger, wsClient);
```

- [ ] **Step 4: Commit**

```bash
git add electron/src/types/ipc.ts electron/src/main/ipc.ts
git commit -m "feat: add sync action IPC handlers"
```

---

## Task 4: Renderer Hook for Sync Events

**Files:**
- Create: `electron/src/renderer/hooks/useSyncEvents.ts` — Subscribe to sync events

**Steps:**

- [ ] **Step 1: Create hook**

```typescript
// electron/src/renderer/hooks/useSyncEvents.ts
import { useEffect, useCallback } from 'react';
import { useStore } from '@tanstack/react-store';
import { prStore } from '../store';
import type { SyncEvent } from '../../main/websocket-client';

export function useSyncEvents() {
  const store = useStore(prStore);

  useEffect(() => {
    // Subscribe to sync events from main process
    const unsubscribe = window.ipcBridge.on('sync:event', (event: SyncEvent) => {
      handleSyncEvent(event, store);
    });

    // Request subscription
    window.ipcBridge.send('sync:subscribe', {});

    return () => {
      unsubscribe();
    };
  }, [store]);

  const handleSyncEvent = useCallback((event: SyncEvent, store: any) => {
    switch (event.type) {
      case 'sync_started':
        store.setState((state) => ({
          ...state,
          isSyncing: true,
          syncProgress: 0,
        }));
        break;

      case 'progress':
        const progress = (event.data?.current as number) / (event.data?.total as number);
        store.setState((state) => ({
          ...state,
          syncProgress: progress,
        }));
        break;

      case 'sync_complete':
        store.setState((state) => ({
          ...state,
          isSyncing: false,
          syncProgress: 100,
          lastSyncedAt: new Date(event.timestamp),
        }));
        // Refresh PR list
        break;

      case 'action_complete':
        // Show success toast
        console.log('Action completed:', event.data);
        break;

      case 'action_failed':
        // Show error toast
        console.error('Action failed:', event.data);
        break;
    }
  }, []);
}
```

- [ ] **Step 2: Update preload.ts to expose sync event listener**

```typescript
// electron/src/main/preload.ts (MODIFY)
const ipcBridge = {
  // ... existing methods ...

  on: (channel: string, listener: (data: any) => void) => {
    const validChannels = ['sync:event'];
    if (validChannels.includes(channel)) {
      ipcRenderer.on(channel, (event, data) => listener(data));
      return () => ipcRenderer.removeListener(channel, listener);
    }
  }
};
```

- [ ] **Step 3: Use hook in PRListPage**

```typescript
// electron/src/renderer/pages/PRListPage.tsx (MODIFY)
import { useSyncEvents } from '../hooks/useSyncEvents';

export function PRListPage() {
  useSyncEvents(); // Hook subscribes to events and updates store

  // ... rest of component
}
```

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/hooks/useSyncEvents.ts electron/src/main/preload.ts electron/src/renderer/pages/PRListPage.tsx
git commit -m "feat: add useSyncEvents hook for real-time PR updates"
```

---

## Task 5: Testing & Integration

**Files:**
- Test: Full end-to-end sync flow
- Verify: UI updates with real PR data

**Steps:**

- [ ] **Step 1: Manual testing**

```bash
# Terminal 1: Start sync engine
cd /Users/samuelasvanyi/dev/bistro/.worktrees/go-sync-engine/go
./bistro-sync --ws-port 8080 --db-path ./sync.db

# Terminal 2: Build and run Electron
cd /Users/samuelasvanyi/dev/bistro/electron
pnpm dev
```

- [ ] **Step 2: Verify connections**

Check logs:
- Sync engine: "WebSocket server listening on :8080"
- Electron main: "WebSocket connected"
- UI: PR list updates in real-time

- [ ] **Step 3: Test sync flow**

1. Click sync button in UI
2. Observe:
   - Sync engine fetches PRs
   - Progress events update UI
   - PR list populates
   - WebSocket events flow through

- [ ] **Step 4: Commit build artifacts**

```bash
git add electron/dist pnpm-lock.yaml
git commit -m "build: Electron integration ready for testing"
```

---

## Success Criteria

- ✅ Go sync engine accepts `--ws-port` and `--db-path` flags
- ✅ WebSocket server starts on configured port
- ✅ Electron connects to WebSocket successfully
- ✅ Sync events flow to renderer process
- ✅ PR list updates in real-time
- ✅ Action commands (approve, request changes, post comment) work
- ✅ Clean builds (no warnings)
- ✅ 4 clean commits

---

**Plan complete. Ready for execution via subagent-driven-development.**
