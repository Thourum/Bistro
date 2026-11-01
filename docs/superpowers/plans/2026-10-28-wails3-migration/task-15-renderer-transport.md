# Task 15: Renderer transport

Read first: [README](README.md), [inventory-wails.md](inventory-wails.md), [inventory-renderer.md](inventory-renderer.md).

### Task 15: Renderer `call.ts`, fault mapping, engine event table

**Files:**
- Create: `frontend/src/services/engine/call.ts`, `frontend/src/services/engine/call.test.ts`, `frontend/src/services/events/index.ts`, `frontend/src/services/events/{sync,review,comments,actions,expertise,classification,auth,settings}.ts`, `frontend/src/test/cancellable.ts`
- Do not touch `services/ws/`. It stays until Task 18, so the old call sites keep compiling.

**Interfaces:**
- Consumes: the generated `@bindings/services` and the `Events.CustomEvents` augmentation.
- Produces:
  - `type ServiceCode = 'not_found' | 'unauthenticated' | 'rate_limited' | 'invalid' | 'cancelled' | 'timeout' | 'internal'`
  - `DEFAULT_TIMEOUT_MS = 30_000`
  - `interface CallOptions { timeoutMs?: number | null; onStart?: (promise: CancellablePromise<unknown>) => void }`
  - `function toServiceFault(error: unknown): Fault`, whose fault context is `{ code: ServiceCode, reason: string | null }`
  - `function call<T>(start: () => CancellablePromise<T>, options?: CallOptions): ResultAsync<T, Fault>`
  - `function callWrite<T>(start: () => CancellablePromise<T>, options?: CallOptions): ResultAsync<T, Fault>`
  - `function faultReason(fault: Fault): string | null`
  - `function subscribeEngineEvents(): () => void`
  - test helper `function cancellable<T>(result: Promise<T>): CancellablePromise<T> & { cancel: Mock }`

- [ ] **Step 1: `call.ts`.**
  ```ts
  import { errAsync, type Fault, ResultAsync, ServiceError } from '@thourum/fault';
  import { CancelError, type CancellablePromise } from '@wailsio/runtime';
  import { z } from 'zod';
  import { guardBenchWrite } from '../../utils/benchSafety';

  export type ServiceCode = 'not_found' | 'unauthenticated' | 'rate_limited' | 'invalid' | 'cancelled' | 'timeout' | 'internal';

  export const DEFAULT_TIMEOUT_MS = 30_000;

  const TAG_BY_CODE = {
    not_found: 'NOT_FOUND',
    unauthenticated: 'AUTHENTICATION_ERROR',
    rate_limited: 'RATE_LIMITED',
    invalid: 'VALIDATION_ERROR',
    cancelled: 'CANCELLED',
    timeout: 'TIMEOUT_ERROR',
    internal: 'INTERNAL_ERROR',
  } as const satisfies Record<ServiceCode, string>;

  // The Go ServiceError arrives as RuntimeError.cause; it is the one untyped edge.
  const serviceErrorSchema = z.object({
    code: z.enum(['not_found', 'unauthenticated', 'rate_limited', 'invalid', 'cancelled', 'timeout', 'internal']),
    message: z.string(),
    reason: z.string().optional(),
  });

  export interface CallOptions {
    /** Milliseconds before the call is cancelled; null disables the timeout. */
    timeoutMs?: number | null;
    onStart?: (promise: CancellablePromise<unknown>) => void;
  }

  function serviceFault(code: ServiceCode, message: string, reason: string | null = null): Fault {
    return ServiceError(TAG_BY_CODE[code], message).withContext({ code, reason });
  }

  export function toServiceFault(error: unknown): Fault {
    if (error instanceof CancelError) return serviceFault('cancelled', 'Request cancelled');
    const cause = error instanceof Error ? error.cause : undefined;
    const parsed = serviceErrorSchema.safeParse(cause);
    if (parsed.success) return serviceFault(parsed.data.code, parsed.data.message, parsed.data.reason ?? null);
    const message = error instanceof Error ? error.message : String(error);
    return serviceFault('internal', message).withCause(error);
  }

  export function faultReason(fault: Fault): string | null {
    const reason = fault.context?.reason;
    return typeof reason === 'string' ? reason : null;
  }

  export function call<T>(start: () => CancellablePromise<T>, options: CallOptions = {}): ResultAsync<T, Fault> {
    const timeoutMs = options.timeoutMs === undefined ? DEFAULT_TIMEOUT_MS : options.timeoutMs;
    const promise = start();
    options.onStart?.(promise);
    if (timeoutMs === null) return ResultAsync.fromPromise(promise, toServiceFault);
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      promise.cancel();
    }, timeoutMs);
    return ResultAsync.fromPromise(
      promise.finally(() => clearTimeout(timer)),
      (error) => (timedOut ? serviceFault('timeout', `No response after ${timeoutMs} ms`) : toServiceFault(error)),
    );
  }

  export function callWrite<T>(start: () => CancellablePromise<T>, options?: CallOptions): ResultAsync<T, Fault> {
    return guardBenchWrite().match(
      () => call(start, options),
      (fault) => errAsync<T, Fault>(fault),
    );
  }
  ```
  Adapt it to the real APIs:
  - **`guardBenchWrite`.** Read `utils/benchSafety.ts`. It may take a label or return a different shape. Keep the semantics (refuse with `BENCH_WRITE_BLOCKED` while the keys bench is active) and change `IS_KEYS_BENCH` to read only the URL `bench_keys=1`, with no `window.electron`.
  - **`Fault.context`.** If `Fault` has no public `context` accessor, read the installed `fault-*.d.ts` and use whatever accessor it exposes.
  - **Wails imports.** Confirm `CancelError` and `CancellablePromise` are exported from `@wailsio/runtime` 3.0.0 (`node_modules/@wailsio/runtime/types`).
- [ ] **Step 2: `src/test/cancellable.ts`.** It returns `Object.assign(result, { cancel: vi.fn() })`, typed as `CancellablePromise<T> & { cancel: Mock }`. It needs no `as` cast if you build it with `Object.assign` on a real `Promise`; if the type checker still needs one, keep a single commented cast in this test-only helper.
- [ ] **Step 3: `call.test.ts`.**
  - A rejection whose `cause` is `{code:'not_found', message:'gone', reason:'pr_missing'}` → `fault.tag === 'NOT_FOUND'` and `faultReason(fault) === 'pr_missing'`.
  - A rejection with a non-conforming cause → `INTERNAL_ERROR`.
  - A `CancelError` → `CANCELLED`.
  - Timeout, with `vi.useFakeTimers()` and a never-settling promise rejected on `cancel`: advance 30 000 ms, `cancel` was called once, and the result is `TIMEOUT_ERROR`.
  - `timeoutMs: null`: after advancing 10 minutes `cancel` was never called.
  - `callWrite` with the keys bench active (stub `location.search` to `?bench_keys=1` before importing): `start` is never called and the result is `BENCH_WRITE_BLOCKED`.
- [ ] **Step 4: Event table.** In `services/events/index.ts`, map **every** generated event name to a handler, and use `satisfies` for exhaustiveness:
  ```ts
  import { Events } from '@wailsio/runtime';

  type EngineEvent = keyof Events.CustomEvents;
  type EngineEventHandlers = { [K in EngineEvent]: (data: Events.CustomEvents[K]) => void };

  export const eventHandlers = {
    'auth:redirect': handleAuthRedirect,
    'sync:complete': handleSyncComplete,
    // … one entry per row of the events table
  } satisfies EngineEventHandlers;

  function subscribe<K extends EngineEvent>(name: K): () => void {
    const handler: EngineEventHandlers[K] = eventHandlers[name];
    return Events.On(name, (event) => handler(event.data));
  }

  export function subscribeEngineEvents(): () => void {
    const names = Object.keys(eventHandlers) as EngineEvent[]; // keys of a literal checked by `satisfies` above
    const offs = names.map((name) => subscribe(name));
    return () => {
      for (const off of offs) off();
    };
  }
  ```
  If `Events.CustomEvents` also includes Wails' own updater events, use `Exclude<…, \`wails:${string}\`>`.
- [ ] **Step 5: Push handlers.** Put them in `services/events/<domain>.ts`, ported from the old `services/ws/events/<domain>.ts` push handlers ([inventory-renderer.md](inventory-renderer.md), old event table). Each takes the generated payload type and writes `appStore` exactly as before.
  - `auth:redirect` opens `data.url` with the existing `openUrl` util; there is no request-id matching.
  - `action:complete` branches on `data.success`.
  - `thread:state-changed` bumps `threadsRevision`.
  - `comments:classified` bumps `commentsRevision`.
  - `settings:changed` calls `settingsStore.setState(() => data)` through the existing settings store setter.

  Leave the old files untouched.
- [ ] **Step 6: Event tests.** Port two cases from `ws/events/events.test.ts` into `services/events/events.test.ts`, calling the handlers directly:
  - `comments:classified` bumps `commentsRevision`;
  - `expertise:error` maps `message || code` into the toast and statuses.
- [ ] **Step 7: Verify.** `cd frontend && pnpm type-check && pnpm lint && pnpm test`
- [ ] **Step 8: Commit.** `git add -A && git commit -m "feat: add Wails call helper and typed engine event table to the renderer"`
