# go:command IPC Wiring — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the `go:command` IPC handler to forward approve/request-changes/post-comment actions from Electron renderer through the main process WebSocket client to the Go sync engine, with correct field mapping.

**Architecture:** The renderer sends actions via `useActionDispatch` → WebSocket `send()` directly to Go. However, the `go:command` IPC handler (an alternative path via Electron main process) is currently a stub. This plan wires it properly so both paths work: (1) direct WebSocket from renderer, and (2) IPC through main process for offline queueing. The key challenge is mapping Electron field names (`type: 'approve_pr'`, `comment`) to Go field names (`Action: 'approve'`, `Body`).

**Tech Stack:** Electron IPC, TypeScript, WebSocket, Go actions

---

## File Structure

| File | Responsibility |
|---|---|
| **Modify:** `electron/src/main/ipc.ts` | Wire `go:command` handler to forward via wsClient |
| **Modify:** `electron/src/types/ipc.ts` | Add `file` field to GoCommandRequest |
| **Create:** `electron/tests/unit/ipc-gocommand.test.ts` | Test action mapping and forwarding |

---

### Task 1: Wire go:command IPC Handler

**Files:**
- Modify: `electron/src/main/ipc.ts:71-79`
- Modify: `electron/src/types/ipc.ts`

- [ ] **Step 1: Read current files**

Read `electron/src/main/ipc.ts` and `electron/src/types/ipc.ts` to confirm current state.

- [ ] **Step 2: Update GoCommandRequest type**

In `electron/src/types/ipc.ts`, ensure `GoCommandRequest` has all needed fields:

```typescript
export interface GoCommandRequest {
  type: 'approve_pr' | 'request_changes' | 'post_comment';
  pr_id: string;
  comment?: string;
  line?: number;
  file?: string;
}
```

- [ ] **Step 3: Implement the go:command handler**

Replace the stub in `electron/src/main/ipc.ts`:

```typescript
  // ---- go:command handler ----
  ipcMain.handle('go:command', async (_event, request: GoCommandRequest) => {
    try {
      if (!wsClient.isConnected()) {
        return { err: 'WebSocket not connected — action queued for retry' };
      }

      // Map Electron action types to Go ActionCommand values
      const ACTION_MAP: Record<GoCommandRequest['type'], string> = {
        approve_pr: 'approve',
        request_changes: 'request_changes',
        post_comment: 'post_comment',
      };

      const action = ACTION_MAP[request.type];
      if (action === undefined) {
        return { err: `Unknown action type: ${request.type}` };
      }

      logger.info(`Forwarding go:command: ${action} for PR ${request.pr_id}`);

      wsClient.send({
        type: 'ACTION_REQUEST',
        action,
        pr_id: request.pr_id,
        body: request.comment ?? '',
        line: request.line ?? 0,
        file: request.file ?? '',
      });

      return { ok: true };
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unknown error';
      logger.error(`go:command error: ${message}`);
      return { err: message };
    }
  });
```

- [ ] **Step 4: Run build to verify compilation**

Run: `cd electron && pnpm build`
Expected: Build succeeds

- [ ] **Step 5: Run full test suite**

Run: `cd electron && pnpm test`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add electron/src/main/ipc.ts electron/src/types/ipc.ts
git commit -m "feat: wire go:command IPC handler to forward actions to Go via WebSocket"
```

---

### Task 2: Sync the sync:action and go:command Handlers

**Files:**
- Modify: `electron/src/main/ipc.ts`

The `sync:action` handler (line ~88) already forwards to Go but uses a different format. Consolidate both handlers to use the same mapping logic.

- [ ] **Step 1: Read the sync:action handler**

Confirm its current shape and ensure it also maps fields correctly.

- [ ] **Step 2: Update sync:action to use the same ACTION_MAP**

Extract the `ACTION_MAP` to a module-level constant and use it in both handlers:

```typescript
const ACTION_TYPE_MAP: Record<string, string> = {
  approve_pr: 'approve',
  request_changes: 'request_changes',
  post_comment: 'post_comment',
  // Also accept Go-native names as passthrough
  approve: 'approve',
};
```

- [ ] **Step 3: Run build and tests**

Run: `cd electron && pnpm build && pnpm test`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add electron/src/main/ipc.ts
git commit -m "refactor: consolidate action type mapping for go:command and sync:action IPC handlers"
```

---

### Task 3: Build Verification

- [ ] **Step 1: Run full test suite**

Run: `cd electron && pnpm test`
Expected: All tests pass

- [ ] **Step 2: Run build**

Run: `cd electron && pnpm build`
Expected: Clean build
