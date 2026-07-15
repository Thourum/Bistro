import { randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Logger } from 'electron-log';
import type { EventMessage, PostHogOptions } from 'posthog-node';
import { PostHog } from 'posthog-node';

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

type TelemetryClient = Pick<
  PostHog,
  'capture' | 'disable' | 'enable' | 'flush'
>;

type TelemetryClientFactory = (options: {
  apiKey: string;
  host: string;
  enabled: boolean;
}) => TelemetryClient;

const DEFAULT_POSTHOG_HOST = 'https://us.i.posthog.com';

const DEFAULT_POSTHOG_API_KEY_ENV_KEYS = [
  'BISTRO_POSTHOG_API_KEY',
  'POSTHOG_API_KEY',
] as const;

const DEFAULT_POSTHOG_HOST_ENV_KEYS = [
  'BISTRO_POSTHOG_HOST',
  'POSTHOG_HOST',
] as const;

const posthogClientFactory: TelemetryClientFactory = ({
  apiKey,
  host,
  enabled,
}) =>
  new PostHog(apiKey, {
    host,
    defaultOptIn: enabled,
    disabled: false,
    disableGeoip: true,
    disableRemoteConfig: true,
    disableRemoteFeatureFlags: true,
    featureFlagsRequestMaxRetries: 0,
    fetchRetryCount: 3,
    flushAt: 20,
    flushInterval: 10_000,
  } satisfies PostHogOptions);

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

export class TelemetryService {
  private enabled: boolean;
  private readonly installIdPath: string;

  constructor(options: {
    enabled: boolean;
    storageDir: string;
    endpoint: string | null;
    appVersion: string;
    platform: NodeJS.Platform;
    logger: Logger;
    apiKey?: string | null;
    clientFactory?: TelemetryClientFactory;
  }) {
    this.enabled = options.enabled;
    this.appVersion = options.appVersion;
    this.platform = options.platform;
    this.logger = options.logger;
    this.installIdPath = join(options.storageDir, 'anonymous-install-id');
    this.client = this.createClient(options);
  }

  private readonly appVersion: string;
  private readonly platform: NodeJS.Platform;
  private readonly logger: Logger;
  private readonly client: TelemetryClient | null;

  setEnabled(enabled: boolean): void {
    this.enabled = enabled;
    const updateConsent = enabled
      ? this.client?.enable()
      : this.client?.disable();
    void updateConsent?.catch((error: unknown) => {
      this.logger.warn(`telemetry consent update failed: ${String(error)}`);
    });
  }

  async track(
    name: unknown,
    properties: Record<string, unknown> = {},
  ): Promise<void> {
    const event = sanitizeTelemetryEvent(name, properties);
    if (!this.enabled || event === null) return;

    if (this.client === null) return;

    const payload = await this.payloadFor(event).catch((error: unknown) => {
      this.logger.warn(`telemetry payload skipped: ${String(error)}`);
      return null;
    });
    if (payload === null) return;

    this.capture(payload);
  }

  trackAppStarted(): Promise<void> {
    return this.track('app_started', {
      appVersion: this.appVersion,
      platform: this.platform,
    });
  }

  flush(): Promise<void> {
    return (
      this.client?.flush().catch((error: unknown) => {
        this.logger.warn(`telemetry flush failed: ${String(error)}`);
      }) ?? Promise.resolve()
    );
  }

  private createClient(options: {
    endpoint: string | null;
    apiKey?: string | null;
    clientFactory?: TelemetryClientFactory;
  }): TelemetryClient | null {
    const apiKey = options.apiKey ?? this.resolveApiKey();
    if (apiKey === null) return null;

    return (options.clientFactory ?? posthogClientFactory)({
      apiKey,
      host: options.endpoint ?? this.resolvePostHogHost(),
      enabled: this.enabled,
    });
  }

  private resolvePostHogHost(): string {
    const value = DEFAULT_POSTHOG_HOST_ENV_KEYS.map((key) =>
      resolveTelemetryEndpoint(process.env[key]),
    ).find((candidate) => candidate !== null);
    return value ?? DEFAULT_POSTHOG_HOST;
  }

  private resolveApiKey(): string | null {
    const value = DEFAULT_POSTHOG_API_KEY_ENV_KEYS.map((key) =>
      process.env[key]?.trim(),
    ).find((candidate) => candidate !== undefined && candidate.length > 0);
    return value ?? null;
  }

  private capture(payload: TelemetryPayload): void {
    const message = {
      distinctId: payload.anonymousInstallId,
      event: payload.name,
      timestamp: new Date(payload.timestamp),
      properties: {
        ...payload.properties,
        telemetrySchemaVersion: payload.schemaVersion,
      },
    } satisfies EventMessage;
    this.client?.capture(message);
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
}
