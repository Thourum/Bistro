import { randomUUID } from 'node:crypto';
import { appendFile, mkdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Logger } from 'electron-log';

export const TELEMETRY_SCHEMA_VERSION = 1 as const;

const EVENT_NAMES = [
  'app_started',
  'settings_updated',
  'review_action_requested',
  'fs_patch_read',
] as const;

const PLATFORMS = ['darwin', 'linux', 'win32', 'unknown'] as const;
const SETTINGS_SECTIONS = [
  'appearance',
  'keyboard',
  'prList',
  'privacy',
  'sync',
  'advanced',
] as const;
const REVIEW_ACTIONS = [
  'approve',
  'request_changes',
  'post_comment',
  'unknown',
] as const;

type TelemetryEventName = (typeof EVENT_NAMES)[number];

export interface SanitizedTelemetryEvent {
  name: TelemetryEventName;
  properties: Record<string, boolean | number | string>;
}

export interface TelemetryPayload extends SanitizedTelemetryEvent {
  schemaVersion: typeof TELEMETRY_SCHEMA_VERSION;
  anonymousInstallId: string;
  timestamp: string;
}

type TelemetryTransport = (
  endpoint: string,
  payloads: readonly TelemetryPayload[],
) => Promise<void>;

function enumValue<T extends readonly string[]>(
  value: unknown,
  allowed: T,
  fallback: T[number],
): T[number] {
  return typeof value === 'string' &&
    (allowed as readonly string[]).includes(value)
    ? (value as T[number])
    : fallback;
}

function boolValue(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined;
}

function appVersionValue(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined;
  return /^[0-9A-Za-z.+-]{1,32}$/.test(value) ? value : undefined;
}

export function sanitizeTelemetryEvent(
  name: unknown,
  rawProperties: Record<string, unknown> = {},
): SanitizedTelemetryEvent | null {
  if (typeof name !== 'string' || !EVENT_NAMES.includes(name as never)) {
    return null;
  }
  const eventName = name as TelemetryEventName;

  if (eventName === 'app_started') {
    const appVersion = appVersionValue(rawProperties.appVersion);
    return {
      name: eventName,
      properties: {
        platform: enumValue(rawProperties.platform, PLATFORMS, 'unknown'),
        telemetrySchemaVersion: TELEMETRY_SCHEMA_VERSION,
        ...(appVersion === undefined ? {} : { appVersion }),
      },
    };
  }

  if (eventName === 'settings_updated') {
    const telemetryEnabled = boolValue(rawProperties.telemetryEnabled);
    return {
      name: eventName,
      properties: {
        section: enumValue(
          rawProperties.section,
          SETTINGS_SECTIONS,
          'advanced',
        ),
        ...(telemetryEnabled === undefined ? {} : { telemetryEnabled }),
      },
    };
  }

  if (eventName === 'review_action_requested') {
    return {
      name: eventName,
      properties: {
        action: enumValue(rawProperties.action, REVIEW_ACTIONS, 'unknown'),
        online: boolValue(rawProperties.online) ?? false,
      },
    };
  }

  return {
    name: eventName,
    properties: {
      allowed: boolValue(rawProperties.allowed) ?? false,
    },
  };
}

export function resolveTelemetryEndpoint(
  raw: string | undefined,
): string | null {
  const trimmed = raw?.trim();
  if (trimmed === undefined || trimmed === '' || !URL.canParse(trimmed)) {
    return null;
  }

  const parsed = new URL(trimmed);
  if (parsed.username !== '' || parsed.password !== '') return null;
  if (parsed.protocol === 'https:') return trimmed;

  const loopbackHosts = new Set(['127.0.0.1', 'localhost', '[::1]', '::1']);
  return parsed.protocol === 'http:' && loopbackHosts.has(parsed.hostname)
    ? trimmed
    : null;
}

async function defaultTransport(
  endpoint: string,
  payloads: readonly TelemetryPayload[],
): Promise<void> {
  const response = await fetch(endpoint, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ events: payloads }),
  });
  if (!response.ok) {
    throw new Error(`telemetry upload failed: ${response.status}`);
  }
}

export class TelemetryService {
  private enabled: boolean;
  private readonly eventsPath: string;
  private readonly installIdPath: string;

  constructor(options: {
    enabled: boolean;
    storageDir: string;
    endpoint: string | null;
    appVersion: string;
    platform: NodeJS.Platform;
    logger: Logger;
    transport?: TelemetryTransport;
  }) {
    this.enabled = options.enabled;
    this.endpoint = options.endpoint;
    this.appVersion = options.appVersion;
    this.platform = options.platform;
    this.logger = options.logger;
    this.transport = options.transport ?? defaultTransport;
    this.eventsPath = join(options.storageDir, 'events.jsonl');
    this.installIdPath = join(options.storageDir, 'anonymous-install-id');
  }

  private readonly endpoint: string | null;
  private readonly appVersion: string;
  private readonly platform: NodeJS.Platform;
  private readonly logger: Logger;
  private readonly transport: TelemetryTransport;

  setEnabled(enabled: boolean): void {
    this.enabled = enabled;
  }

  async track(
    name: unknown,
    properties: Record<string, unknown> = {},
  ): Promise<void> {
    const event = sanitizeTelemetryEvent(name, properties);
    if (!this.enabled || event === null) return;

    const payload = await this.payloadFor(event).catch((error: unknown) => {
      this.logger.warn(`telemetry payload skipped: ${String(error)}`);
      return null;
    });
    if (payload === null) return;

    await this.persist(payload).catch((error: unknown) => {
      this.logger.warn(`telemetry persist failed: ${String(error)}`);
    });
    if (this.endpoint !== null) {
      await this.transport(this.endpoint, [payload]).catch((error: unknown) => {
        this.logger.warn(`telemetry transmit failed: ${String(error)}`);
      });
    }
  }

  trackAppStarted(): Promise<void> {
    return this.track('app_started', {
      appVersion: this.appVersion,
      platform: this.platform,
    });
  }

  private async payloadFor(
    event: SanitizedTelemetryEvent,
  ): Promise<TelemetryPayload> {
    const anonymousInstallId = await this.getOrCreateInstallId();
    return {
      schemaVersion: TELEMETRY_SCHEMA_VERSION,
      anonymousInstallId,
      timestamp: new Date().toISOString(),
      ...event,
    };
  }

  private async getOrCreateInstallId(): Promise<string> {
    return readFile(this.installIdPath, 'utf-8')
      .then((value) => value.trim())
      .then((value) => (value.length > 0 ? value : this.createInstallId()))
      .catch(() => this.createInstallId());
  }

  private async createInstallId(): Promise<string> {
    const value = randomUUID();
    await mkdir(join(this.installIdPath, '..'), { recursive: true });
    await writeFile(this.installIdPath, `${value}\n`, 'utf-8');
    return value;
  }

  private async persist(payload: TelemetryPayload): Promise<void> {
    await mkdir(join(this.eventsPath, '..'), { recursive: true });
    await appendFile(this.eventsPath, `${JSON.stringify(payload)}\n`, 'utf-8');
  }
}
