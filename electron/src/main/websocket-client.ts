import { EventEmitter } from 'node:events';
import {
  err,
  Fault,
  fromThrowable,
  ok,
  type Result,
  ServiceError,
} from '@thourum/fault';
import type { Logger } from 'electron-log';
import WebSocket from 'ws';
import { z } from 'zod';

const syncEventSchema = z.object({
  type: z.string(),
  data: z.record(z.string(), z.unknown()).optional(),
  id: z.string().optional(),
  timestamp: z.string().optional(),
});
export type SyncEvent = z.infer<typeof syncEventSchema>;

const MAX_RECONNECT_DELAY_MS = 30_000;
const BASE_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_BACKOFF_STEPS = 5;

export class WebSocketClient extends EventEmitter {
  private ws: WebSocket | null = null;
  private readonly url: string;
  private readonly logger: Logger;
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private stopped = false;

  constructor(url: string, logger: Logger) {
    super();
    this.url = url;
    this.logger = logger;
  }

  connect(): void {
    if (this.stopped || this.ws !== null) return;
    const socket = fromThrowable(
      () => new WebSocket(this.url),
      (cause) =>
        new Fault(cause instanceof Error ? cause : String(cause))
          .withTag('CONNECTION_ERROR')
          .withCause(cause),
    )();
    if (socket.isErr()) {
      this.logger.error('Failed to create WebSocket:', socket.error.message);
      this.scheduleReconnect();
      return;
    }
    this.ws = socket.value;
    socket.value.on('open', () => {
      if (this.ws !== socket.value || this.stopped) return;
      this.logger.info('WebSocket connected to Go sync engine');
      this.reconnectAttempts = 0;
      this.emit('connected');
    });
    socket.value.on('message', (data: WebSocket.Data) => {
      if (this.ws !== socket.value || this.stopped) return;
      fromThrowable(
        () => JSON.parse(data.toString()) as unknown,
        (cause) =>
          ServiceError('PARSE_ERROR', 'Failed to parse sync event').withCause(
            cause,
          ),
      )()
        .andThen((value) => {
          const parsed = syncEventSchema.safeParse(value);
          return parsed.success
            ? ok(parsed.data)
            : err(
                ServiceError(
                  'VALIDATION_ERROR',
                  'Invalid sync event',
                ).withCause(parsed.error),
              );
        })
        .match(
          (event) => {
            this.emit('event', event);
          },
          (error) => {
            this.logger.warn('Invalid sync event', error.message);
          },
        );
    });
    socket.value.on('error', (error: Error) => {
      // EventEmitter's reserved 'error' event throws without a listener.
      this.logger.error('WebSocket error:', error.message);
    });
    socket.value.on('close', () => {
      if (this.ws !== socket.value) return;
      this.logger.info('WebSocket disconnected');
      this.ws = null;
      this.emit('disconnected');
      this.scheduleReconnect();
    });
  }

  send(message: Record<string, unknown>): Result<void, Fault> {
    if (!this.ws || !this.isConnected())
      return err(ServiceError('CONNECTION_ERROR', 'WebSocket not connected'));
    return fromThrowable(
      () => this.ws?.send(JSON.stringify(message)),
      (cause) =>
        new Fault(cause instanceof Error ? cause : String(cause))
          .withTag('CONNECTION_ERROR')
          .withCause(cause),
    )().map(() => undefined);
  }

  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  stop(): void {
    this.stopped = true;
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  private scheduleReconnect(): void {
    if (this.stopped || this.reconnectTimer !== null) return;

    const delay = Math.min(
      BASE_RECONNECT_DELAY_MS * 2 ** this.reconnectAttempts,
      MAX_RECONNECT_DELAY_MS,
    );
    this.reconnectAttempts = Math.min(
      this.reconnectAttempts + 1,
      MAX_RECONNECT_BACKOFF_STEPS,
    );
    this.logger.info(`WebSocket reconnecting in ${delay}ms`);

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }
}
