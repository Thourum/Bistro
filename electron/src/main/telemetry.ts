import { randomUUID } from 'node:crypto';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { Fault, fromPromise, fromThrowable, ResultAsync } from '@thourum/fault';
import type { Logger } from 'electron-log';
import type { EventMessage, PostHogOptions } from 'posthog-node';
import { PostHog } from 'posthog-node';

const TELEMETRY_SCHEMA_VERSION = 1 as const;

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
    this.client = fromThrowable(
      () => this.createClient(options),
      (cause) =>
        new Fault(String(cause)).withTag('TELEMETRY_ERROR').withCause(cause),
    )().match(
      (client) => client,
      (error) => {
        options.logger.warn(
          `telemetry initialization failed: ${error.message}`,
        );
        return null;
      },
    );
  }

  private readonly appVersion: string;
  private readonly platform: NodeJS.Platform;
  private readonly logger: Logger;
  private readonly client: TelemetryClient | null;
  private installIdPromise: Promise<string | null> | null = null;

  setEnabled(enabled: boolean): void {
    this.enabled = enabled;
    const updateConsent = fromThrowable(
      () => (enabled ? this.client?.enable() : this.client?.disable()),
      (cause) =>
        new Fault(String(cause)).withTag('TELEMETRY_ERROR').withCause(cause),
    )();
    updateConsent.match(
      (promise) => {
        if (promise)
          void fromPromise(promise, (cause) =>
            new Fault(String(cause))
              .withTag('TELEMETRY_ERROR')
              .withCause(cause),
          ).orInspect((error) =>
            this.logger.warn(
              `telemetry consent update failed: ${error.message}`,
            ),
          );
      },
      (error) =>
        this.logger.warn(`telemetry consent update failed: ${error.message}`),
    );
  }

  async track(
    name: unknown,
    properties: Record<string, unknown> = {},
  ): Promise<void> {
    const event = sanitizeTelemetryEvent(name, properties);
    if (!this.enabled || event === null || this.client === null) return;

    const payload = await this.payloadFor(event);
    if (payload === null) return;
    fromThrowable(
      () => this.capture(payload),
      (cause) =>
        new Fault(String(cause)).withTag('TELEMETRY_ERROR').withCause(cause),
    )().orInspect((error) =>
      this.logger.warn(`telemetry capture failed: ${error.message}`),
    );
  }

  trackAppStarted(): Promise<void> {
    return this.track('app_started', {
      appVersion: this.appVersion,
      platform: this.platform,
    });
  }

  flush(): Promise<void> {
    if (this.client === null) return Promise.resolve();
    return fromThrowable(
      () => this.client?.flush(),
      (cause) =>
        new Fault(String(cause)).withTag('TELEMETRY_ERROR').withCause(cause),
    )()
      .asyncAndThen((result) =>
        result
          ? fromPromise(result, (cause) =>
              new Fault(String(cause))
                .withTag('TELEMETRY_ERROR')
                .withCause(cause),
            )
          : ResultAsync.fromSafePromise(Promise.resolve()),
      )
      .match(
        () => undefined,
        (error) => {
          this.logger.warn(`telemetry flush failed: ${error.message}`);
        },
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
  ): Promise<TelemetryPayload | null> {
    this.installIdPromise ??= this.getOrCreateInstallId().match(
      (value) => value,
      (error) => {
        this.logger.warn(`telemetry install id unavailable: ${error.message}`);
        this.installIdPromise = null;
        return null;
      },
    );
    const anonymousInstallId = await this.installIdPromise;
    if (anonymousInstallId === null) return null;
    return {
      schemaVersion: TELEMETRY_SCHEMA_VERSION,
      anonymousInstallId,
      timestamp: new Date().toISOString(),
      ...event,
    };
  }

  private getOrCreateInstallId(): ResultAsync<string, Fault> {
    return fromPromise(readFile(this.installIdPath, 'utf-8'), (cause) =>
      new Fault(String(cause)).withTag('FILE_SYSTEM_ERROR').withCause(cause),
    )
      .andThen((content) =>
        content.trim() === ''
          ? this.createInstallId()
          : ResultAsync.fromSafePromise(Promise.resolve(content.trim())),
      )
      .orElse(() => this.createInstallId());
  }

  private createInstallId(): ResultAsync<string, Fault> {
    const value = randomUUID();
    return fromPromise(
      mkdir(dirname(this.installIdPath), { recursive: true }),
      (cause) =>
        new Fault(String(cause)).withTag('FILE_SYSTEM_ERROR').withCause(cause),
    )
      .andThen(() =>
        fromPromise(
          writeFile(this.installIdPath, `${value}\n`, 'utf-8'),
          (cause) =>
            new Fault(String(cause))
              .withTag('FILE_SYSTEM_ERROR')
              .withCause(cause),
        ),
      )
      .map(() => value);
  }
}
