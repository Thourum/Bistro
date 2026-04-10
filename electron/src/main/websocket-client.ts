import { EventEmitter } from 'node:events';
import type { Logger } from 'electron-log';
import WebSocket from 'ws';

export interface SyncEvent {
  type:
    | 'sync_started'
    | 'progress'
    | 'sync_complete'
    | 'action_complete'
    | 'action_failed';
  data?: Record<string, unknown>;
  timestamp: number;
}

const MAX_RECONNECT_ATTEMPTS = 5;
const BASE_RECONNECT_DELAY_MS = 1000;

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
    if (this.stopped) return;

    try {
      this.ws = new WebSocket(this.url);

      this.ws.on('open', () => {
        this.logger.info('WebSocket connected to Go sync engine');
        this.reconnectAttempts = 0;
        this.emit('connected');
      });

      this.ws.on('message', (data: WebSocket.Data) => {
        try {
          const syncEvent: SyncEvent = JSON.parse(data.toString());
          this.emit('event', syncEvent);
        } catch (error) {
          this.logger.error('Failed to parse sync event', error);
        }
      });

      this.ws.on('error', (error: Error) => {
        this.logger.error('WebSocket error:', error.message);
        this.emit('error', error);
      });

      this.ws.on('close', () => {
        this.logger.info('WebSocket disconnected');
        this.ws = null;
        this.emit('disconnected');
        this.scheduleReconnect();
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      this.logger.error('Failed to create WebSocket:', message);
      this.scheduleReconnect();
    }
  }

  send(message: Record<string, unknown>): void {
    if (!this.isConnected()) {
      throw new Error('WebSocket not connected');
    }
    this.ws?.send(JSON.stringify(message));
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
    if (this.stopped) return;
    if (this.reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      this.logger.error(
        `WebSocket max reconnect attempts (${MAX_RECONNECT_ATTEMPTS}) reached`,
      );
      this.emit('max_reconnect_exceeded');
      return;
    }

    const delay = BASE_RECONNECT_DELAY_MS * 2 ** this.reconnectAttempts;
    this.reconnectAttempts += 1;
    this.logger.info(
      `WebSocket reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${MAX_RECONNECT_ATTEMPTS})`,
    );

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }
}
