import type { Breadcrumb, ErrorEvent } from '@sentry/electron/main';
import * as Sentry from '@sentry/electron/main';
import { app } from 'electron';
import { version } from '../../package.json';

const dsn = process.env.SENTRY_DSN ?? process.env.VITE_SENTRY_DSN ?? '';
let consentGranted = false;
let initialized = false;

function stripLocalPath(filename: string): string {
  return filename.replace(/^.*[/\\]/, '');
}

function scrubEvent(event: ErrorEvent): ErrorEvent | null {
  if (!consentGranted) return null;

  delete event.user;
  delete event.request;
  delete event.breadcrumbs;
  delete event.contexts;
  delete event.extra;
  if (event.message !== undefined) {
    event.message = '[redacted]';
  }
  event.exception?.values?.forEach((exception) => {
    if (exception.type !== undefined) {
      exception.type = 'Error';
    }
    if (exception.value !== undefined) {
      exception.value = '[redacted]';
    }
    exception.stacktrace?.frames?.forEach((frame) => {
      if (frame.filename) {
        frame.filename = stripLocalPath(frame.filename);
      }
    });
  });
  return event;
}

function filterBreadcrumb(breadcrumb: Breadcrumb): Breadcrumb | null {
  const category = breadcrumb.category ?? '';
  if (
    category === 'console' ||
    category === 'http' ||
    category === 'fetch' ||
    category === 'xhr'
  ) {
    return null;
  }
  return breadcrumb;
}

export function configureSentryConsent(enabled: boolean): boolean {
  consentGranted = enabled;
  if (!consentGranted || initialized || dsn.length === 0) {
    return false;
  }
  if (initialized) return isSentryEnabled();

  Sentry.init({
    dsn,
    release: `bistro-electron@${version}`,
    environment:
      process.env.SENTRY_ENVIRONMENT ??
      process.env.VITE_SENTRY_ENVIRONMENT ??
      (app.isPackaged ? 'production' : 'development'),
    attachScreenshot: false,
    tracesSampleRate: 0,
    denyUrls: [/^file:\/\//i],
    dataCollection: {
      userInfo: false,
      httpBodies: [],
      httpHeaders: {
        request: { deny: ['forwarded', '-ip', 'remote-', 'via', '-user'] },
        response: { deny: ['forwarded', '-ip', 'remote-', 'via', '-user'] },
      },
    },
    beforeSend: scrubEvent,
    beforeBreadcrumb: filterBreadcrumb,
  });

  initialized = true;
  return isSentryEnabled();
}

export function isSentryEnabled(): boolean {
  return consentGranted && initialized;
}

export function captureSentryException(
  error: unknown,
  context?: Parameters<typeof Sentry.captureException>[1],
): void {
  if (!isSentryEnabled()) return;
  Sentry.captureException(error, context);
}
