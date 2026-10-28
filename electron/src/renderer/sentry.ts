import type { Breadcrumb, ErrorEvent } from '@sentry/electron/renderer';
import * as Sentry from '@sentry/electron/renderer';

const isBenchRequested =
  (typeof window !== 'undefined' && window.electron?.isKeysBench === true) ||
  (typeof location !== 'undefined' &&
    new URLSearchParams(location.search).has('bench_keys'));
const enabled =
  !isBenchRequested && import.meta.env.VITE_SENTRY_ENABLED === 'true';

function stripLocalPath(filename: string): string {
  return filename.replace(/^.*[/\\]/, '');
}

function scrubEvent(event: ErrorEvent): ErrorEvent | null {
  delete event.user;
  delete event.request;
  event.exception?.values?.forEach((exception) => {
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
    category === 'xhr' ||
    category === 'ui.click'
  ) {
    return null;
  }
  return breadcrumb;
}

if (enabled) {
  Sentry.init({
    sendDefaultPii: false,
    tracesSampleRate: 0,
    denyUrls: [/^file:\/\//i],
    beforeSend: scrubEvent,
    beforeBreadcrumb: filterBreadcrumb,
  });
}

export { Sentry };
