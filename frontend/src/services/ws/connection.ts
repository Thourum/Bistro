import {
  err,
  Fault,
  fromThrowable,
  ok,
  type Result,
  ResultAsync,
  ServiceError,
} from '@thourum/fault';
import { appStore } from '../../store';
import { settingsStore } from '../../store/settings';
import { extractResponseError, parseWsMessage } from '../../types/wsSchemas';
import { guardBenchWsMessage } from '../../utils/benchSafety';
import { createLogger } from '../../utils/logger';
import { dispatchWsEvent } from './events';

const log = createLogger('WebSocket');
const ENGINE_PORT_POLL_INTERVAL_MS = 1000;
const MAX_WS_PORT = 65535;
const REQUEST_TIMEOUT_MS = 5000;
const PR_SYNC_RESPONSE_MARGIN_MS = 15000;
const PR_SYNC_REQUEST_TIMEOUT_MS = 2 * 60 * 1000 + PR_SYNC_RESPONSE_MARGIN_MS;
// Checkout fetches and mutates HEAD; the generic budget expires mid-checkout.
const CHECKOUT_REQUEST_TIMEOUT_MS = 60 * 1000;
// The engine caps a scan at 30 s, then persists for up to 5 s.
const SCAN_REQUEST_TIMEOUT_MS = 45 * 1000;
// Queue and repo discovery have 30 s engine deadlines plus a reply margin.
const REPO_LIST_REQUEST_TIMEOUT_MS = 35_000;
const REVIEW_QUEUE_REQUEST_TIMEOUT_MS = 35_000;
const REQUEST_TIMEOUTS: Readonly<Record<string, number>> = Object.freeze({
  REQUEST_PR_SYNC: PR_SYNC_REQUEST_TIMEOUT_MS,
  REQUEST_REPO_LIST: REPO_LIST_REQUEST_TIMEOUT_MS,
  REQUEST_REVIEW_QUEUE: REVIEW_QUEUE_REQUEST_TIMEOUT_MS,
  SCAN_DEV_FOLDER: SCAN_REQUEST_TIMEOUT_MS,
  CHECKOUT_PR_HEAD: CHECKOUT_REQUEST_TIMEOUT_MS,
});
const CONNECT_WAIT_INTERVAL_MS = 250;
const BASE_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30000;
const MAX_PORT_POLL_ATTEMPTS = 10;
const MAX_RECONNECT_BACKOFF_STEPS = 5;
const BROWSER_WS_URL = 'ws://127.0.0.1:8080/ws';

interface RequestOptions {
  readonly onSent?: (id: string) => void;
}

interface WsClient {
  connect(): void;
  disconnect(): void;
  send(type: string, data: unknown): string | undefined;
  request(
    type: string,
    data: unknown,
    options?: RequestOptions,
  ): ResultAsync<unknown, Fault>;
}

function isValidWsPort(port: unknown): port is number {
  return (
    typeof port === 'number' &&
    Number.isInteger(port) &&
    port > 0 &&
    port <= MAX_WS_PORT
  );
}

function externalFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause : 'Unknown error')
    .withTag('EXTERNAL_ERROR')
    .withCause(cause);
}

// Only this service owns mutable transport state; consumers get a frozen API.
function createWsClient(): Readonly<WsClient> {
  const state = {
    socket: null as WebSocket | null,
    reconnectTimer: null as ReturnType<typeof setTimeout> | null,
    reconnectAttempt: 0,
    portPollAttempt: 0,
    resolvedPort: null as number | null,
    isActive: false,
    hasSentStartupScan: false,
  };
  const pendingRequests = new Map<string, (data: unknown) => void>();

  function resolveResponse(id: string | undefined, data: unknown): boolean {
    const handler = id === undefined ? undefined : pendingRequests.get(id);
    if (!handler || id === undefined) return false;
    handler(data);
    pendingRequests.delete(id);
    return true;
  }

  function onMessage(event: MessageEvent<string>): void {
    const parsed = parseWsMessage(event.data);
    if (parsed.isErr()) {
      log.warn({ err: parsed.error.message }, 'invalid WebSocket message');
      return;
    }
    const message = parsed.value;
    log.debug({ type: message.type, id: message.id }, 'received event');
    dispatchWsEvent(message, { resolveResponse });
  }

  function getWsUrl(): string | null {
    const electron =
      typeof window !== 'undefined' ? window.electron : undefined;
    if (electron === undefined) return BROWSER_WS_URL;
    const port = isValidWsPort(state.resolvedPort)
      ? state.resolvedPort
      : electron.wsPort;
    return isValidWsPort(port) ? `ws://127.0.0.1:${port}/ws` : null;
  }

  function scheduleConnection(delay: number): void {
    state.reconnectTimer = setTimeout(connectSocket, delay);
  }

  function waitForEnginePort(): void {
    if (state.portPollAttempt >= MAX_PORT_POLL_ATTEMPTS) {
      log.error('sync engine WebSocket port was not published');
      appStore.setState((previous) => ({
        ...previous,
        syncStatus: 'error',
        syncError: 'Sync engine did not start. Please restart the app.',
        authChecked: true,
        isAuthenticated: false,
      }));
      return;
    }
    state.portPollAttempt += 1;
    const getWsPort = window.electron?.getWsPort;
    if (getWsPort !== undefined) {
      void fromThrowable(getWsPort, externalFault)()
        .asyncAndThen((promise) =>
          ResultAsync.fromPromise(promise, externalFault),
        )
        .match(
          (port) => {
            if (isValidWsPort(port) && state.isActive)
              state.resolvedPort = port;
          },
          (fault) =>
            log.warn(
              { err: fault.message },
              'failed to query sync engine WebSocket port',
            ),
        );
    }
    log.info(
      'Go sync engine is still starting — waiting for its WebSocket port',
    );
    scheduleConnection(ENGINE_PORT_POLL_INTERVAL_MS);
  }

  function send(type: string, data: unknown): string | undefined {
    const permitted = guardBenchWsMessage(type, data);
    if (permitted.isErr()) {
      log.warn(
        { type, err: permitted.error.message },
        'benchmark refused message',
      );
      return undefined;
    }
    const socket = state.socket;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      log.error({ type }, 'send failed — not connected');
      log.warn({ type }, 'dropped message');
      return undefined;
    }
    const id = `req-${Date.now()}-${Math.random()}`;
    const message = { id, type, data, timestamp: new Date().toISOString() };
    log.info({ type, id }, 'sending message');
    const sent = fromThrowable(
      () => socket.send(JSON.stringify(message)),
      (cause) =>
        new Fault(cause instanceof Error ? cause : 'Unknown error')
          .withTag('CONNECTION_ERROR')
          .withCause(cause),
    )();
    if (sent.isErr()) {
      log.error(
        { type, id, err: sent.error.message },
        'failed to send WebSocket message',
      );
      return undefined;
    }
    return id;
  }

  function sendStartupDevFolderScan(): void {
    const path = settingsStore.state.repositories.devFolderPath;
    if (state.hasSentStartupScan || !path) return;
    state.hasSentStartupScan = true;
    send('SCAN_DEV_FOLDER', { path, reason: 'startup' });
  }

  function onOpen(): void {
    log.info('connected');
    state.reconnectAttempt = 0;
    appStore.setState((previous) => ({
      ...previous,
      syncStatus: 'connected',
      isWsOpen: true,
      syncError: null,
      lastSyncAt: new Date().toISOString(),
      isOffline: false,
      authChecked: false,
    }));
    if (appStore.state.authInterrupted) send('AUTH_CANCEL', {});
    send('AUTH_CHECK', {});
    sendStartupDevFolderScan();
  }

  function onError(): void {
    log.error('connection error');
    appStore.setState((previous) => ({
      ...previous,
      syncStatus: 'error',
      syncError: 'WebSocket connection failed',
    }));
  }

  function onClose(socket: WebSocket): void {
    if (state.socket !== socket) return;
    log.info('disconnected');
    state.socket = null;
    // Dropped sockets reconnect directly, so each open must rescan the folder.
    state.hasSentStartupScan = false;
    const isOnline = typeof navigator !== 'undefined' ? navigator.onLine : true;
    appStore.setState((previous) => ({
      ...previous,
      syncStatus: 'disconnected',
      isWsOpen: false,
      isOffline: !isOnline,
      authInterrupted:
        previous.authInterrupted ||
        previous.authLoading ||
        previous.authUrl !== null,
      authLoading: false,
      authStartRequestId: null,
      authUrl: null,
      authStartedAt: null,
      authTimedOut: false,
      authExchange: null,
    }));
    if (!state.isActive) return;
    const delay = Math.min(
      BASE_RECONNECT_DELAY_MS * 2 ** state.reconnectAttempt,
      MAX_RECONNECT_DELAY_MS,
    );
    state.reconnectAttempt = Math.min(
      state.reconnectAttempt + 1,
      MAX_RECONNECT_BACKOFF_STEPS,
    );
    log.info('scheduling reconnect');
    scheduleConnection(delay);
  }

  function connectSocket(): void {
    state.reconnectTimer = null;
    if (
      !state.isActive ||
      (state.socket && state.socket.readyState <= WebSocket.OPEN)
    ) {
      return;
    }
    const url = getWsUrl();
    if (url === null) {
      waitForEnginePort();
      return;
    }
    state.portPollAttempt = 0;
    log.info('attempting connection');
    appStore.setState((previous) => ({
      ...previous,
      syncStatus: 'connecting',
    }));
    const socket = fromThrowable(
      () => new WebSocket(url),
      (cause) =>
        new Fault(cause instanceof Error ? cause : 'Unknown error')
          .withTag('CONNECTION_ERROR')
          .withCause(cause),
    )();
    if (socket.isErr()) {
      log.error({ err: socket.error.message }, 'failed to create WebSocket');
      appStore.setState((previous) => ({
        ...previous,
        syncStatus: 'error',
        syncError: 'WebSocket connection failed',
      }));
      scheduleConnection(MAX_RECONNECT_DELAY_MS);
      return;
    }
    const ws = socket.value;
    state.socket = ws;
    ws.onopen = onOpen;
    ws.onmessage = onMessage;
    ws.onerror = onError;
    ws.onclose = () => onClose(ws);
  }

  function connect(): void {
    if (state.isActive) return;
    state.isActive = true;
    connectSocket();
  }

  function disconnect(): void {
    state.isActive = false;
    appStore.setState((previous) => ({ ...previous, isWsOpen: false }));
    if (state.reconnectTimer !== null) clearTimeout(state.reconnectTimer);
    state.reconnectTimer = null;
    const socket = state.socket;
    state.socket = null;
    if (socket) {
      socket.onopen = null;
      socket.onmessage = null;
      socket.onerror = null;
      socket.onclose = null;
      fromThrowable(() => socket.close(), externalFault)().orInspect((fault) =>
        log.warn({ err: fault.message }, 'failed to close WebSocket'),
      );
    }
    state.reconnectAttempt = 0;
    state.portPollAttempt = 0;
    state.resolvedPort = null;
    state.hasSentStartupScan = false;
  }

  function registerRequest(
    type: string,
    id: string,
    resolve: (response: Result<unknown, Fault>) => void,
  ): void {
    const timeout = setTimeout(
      () => {
        pendingRequests.delete(id);
        const readyState = state.socket?.readyState ?? 'no-socket';
        const fault = ServiceError(
          'TIMEOUT_ERROR',
          `Timeout waiting for ${type} response (socket state: ${readyState} — see bistro-sync.log for the backend side)`,
        );
        log.error({ type, id, err: fault.message }, 'request timed out');
        resolve(err(fault));
      },
      Object.hasOwn(REQUEST_TIMEOUTS, type)
        ? REQUEST_TIMEOUTS[type]
        : REQUEST_TIMEOUT_MS,
    );
    pendingRequests.set(id, (responseData) => {
      clearTimeout(timeout);
      const responseError = extractResponseError(responseData);
      resolve(
        responseError === null
          ? ok(responseData)
          : err(ServiceError('EXTERNAL_ERROR', responseError)),
      );
    });
  }

  function request(
    type: string,
    data: unknown,
    options?: RequestOptions,
  ): ResultAsync<unknown, Fault> {
    const permitted = guardBenchWsMessage(type, data);
    if (permitted.isErr()) return new ResultAsync(Promise.resolve(permitted));
    return new ResultAsync(
      new Promise<Result<unknown, Fault>>((resolve) => {
        const connectDeadline = Date.now() + REQUEST_TIMEOUT_MS;
        // Mount requests wait for OPEN instead of being dropped in the same tick.
        const trySend = (): void => {
          if (
            state.socket === null ||
            state.socket.readyState !== WebSocket.OPEN
          ) {
            if (Date.now() >= connectDeadline) {
              resolve(
                err(
                  ServiceError(
                    'CONNECTION_ERROR',
                    `WebSocket not connected (sending ${type})`,
                  ),
                ),
              );
              return;
            }
            setTimeout(trySend, CONNECT_WAIT_INTERVAL_MS);
            return;
          }
          const id = send(type, data);
          if (id === undefined) {
            resolve(
              err(ServiceError('CONNECTION_ERROR', 'WebSocket not connected')),
            );
            return;
          }
          registerRequest(type, id, resolve);
          options?.onSent?.(id);
        };
        trySend();
      }),
    );
  }

  return Object.freeze({ connect, disconnect, send, request });
}

export const wsClient: Readonly<WsClient> = createWsClient();
