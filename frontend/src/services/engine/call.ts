import {
  err,
  errAsync,
  type Fault,
  fromThrowable,
  type Result,
  ResultAsync,
  ServiceError,
} from '@thourum/fault';
import { CancelError, type CancellablePromise } from '@wailsio/runtime';
import { z } from 'zod';
import { guardBenchWrite } from '../../utils/benchSafety';

export type ServiceCode =
  | 'not_found'
  | 'unauthenticated'
  | 'rate_limited'
  | 'invalid'
  | 'cancelled'
  | 'timeout'
  | 'internal';

export const DEFAULT_TIMEOUT_MS = 30_000;

const TAG_BY_CODE = {
  not_found: 'NOT_FOUND',
  unauthenticated: 'AUTHENTICATION_ERROR',
  rate_limited: 'RATE_LIMITED',
  invalid: 'VALIDATION_ERROR',
  cancelled: 'CANCELLED',
  timeout: 'TIMEOUT_ERROR',
  internal: 'INTERNAL_ERROR',
} satisfies Record<ServiceCode, string>;

// The Go ServiceError arrives as RuntimeError.cause; it is one untyped edge.
const serviceErrorSchema = z.object({
  code: z.enum([
    'not_found',
    'unauthenticated',
    'rate_limited',
    'invalid',
    'cancelled',
    'timeout',
    'internal',
  ]),
  message: z.string(),
  reason: z.string().optional(),
});

export interface CallOptions {
  /** Milliseconds before call is cancelled; null disables timeout. */
  timeoutMs?: number | null;
  onStart?: (promise: CancellablePromise<unknown>) => void;
}

function serviceFault(
  code: ServiceCode,
  message: string,
  reason: string | null = null,
): Fault {
  return ServiceError(TAG_BY_CODE[code], message).withContext({ code, reason });
}

export function toServiceFault(error: unknown): Fault {
  if (error instanceof CancelError) {
    return serviceFault('cancelled', 'Request cancelled');
  }
  const cause = error instanceof Error ? error.cause : undefined;
  const parsed = serviceErrorSchema.safeParse(cause);
  if (parsed.success) {
    return serviceFault(
      parsed.data.code,
      parsed.data.message,
      parsed.data.reason ?? null,
    );
  }
  return serviceFault(
    'internal',
    error instanceof Error ? error.message : String(error),
  ).withCause(error);
}

export function faultReason(fault: Fault): string | null {
  const reason = fault.metadata.reason;
  return typeof reason === 'string' ? reason : null;
}

function awaitCall<T>(
  promise: CancellablePromise<T>,
  options: Readonly<CallOptions>,
): ResultAsync<T, Fault> {
  const response = ResultAsync.fromPromise(promise, toServiceFault);
  const timeoutMs =
    options.timeoutMs === undefined ? DEFAULT_TIMEOUT_MS : options.timeoutMs;
  if (timeoutMs === null) return response;

  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<Result<T, Fault>>((resolve) => {
    timer = setTimeout(() => {
      // Settle the timeout first so cancel's rejection cannot win the race.
      resolve(
        err(serviceFault('timeout', `No response after ${timeoutMs} ms`)),
      );
      void promise.cancel();
    }, timeoutMs);
  });
  return new ResultAsync(
    Promise.race([response, timeout]).finally(() => clearTimeout(timer)),
  );
}

/** Starts a generated Wails binding with cancellation and service fault mapping. */
export function call<T>(
  start: () => CancellablePromise<T>,
  options: Readonly<CallOptions> = {},
): ResultAsync<T, Fault> {
  return fromThrowable(start, toServiceFault)().match(
    (promise) => {
      const response = awaitCall(promise, options);
      return fromThrowable(
        () => options.onStart?.(promise),
        toServiceFault,
      )().match(
        () => response,
        (fault) => {
          void promise.cancel();
          return errAsync<T, Fault>(fault);
        },
      );
    },
    (fault) => errAsync<T, Fault>(fault),
  );
}

/** Refuses writes before starting a binding when the keys benchmark is active. */
export function callWrite<T>(
  start: () => CancellablePromise<T>,
  options?: Readonly<CallOptions>,
): ResultAsync<T, Fault> {
  return guardBenchWrite('engine-call').match(
    () => call(start, options),
    (fault) => errAsync<T, Fault>(fault),
  );
}
