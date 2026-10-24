import {
  err,
  errAsync,
  Fault,
  fromThrowable,
  ok,
  ResultAsync,
  ServiceError,
} from '@thourum/fault';
import { z } from 'zod';
import { engineLogTailSchema, engineStatusSchema } from '../../types/engine';
import { addToast } from '../components/ToastContainer';
import { appStore } from '../store';
import { parseResponse } from '../utils/error';

function engineIpcFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause.message : String(cause))
    .withTag('ENGINE_IPC_ERROR')
    .withCause(cause);
}

function engineRequest<T>(
  request: () => Promise<unknown>,
  schema: z.ZodType<T>,
): ResultAsync<T, Fault> {
  return fromThrowable(request, engineIpcFault)()
    .asyncAndThen((promise) => ResultAsync.fromPromise(promise, engineIpcFault))
    .andThen((response) => {
      const envelope = z
        .union([z.object({ ok: schema }), z.object({ err: z.string() })])
        .safeParse(response);
      if (!envelope.success)
        return err(
          ServiceError('ENGINE_IPC_ERROR', 'Invalid engine response.'),
        );
      return 'err' in envelope.data
        ? err(ServiceError('ENGINE_IPC_ERROR', envelope.data.err))
        : ok(envelope.data.ok);
    });
}

function showEngineError(fault: Fault): void {
  addToast({ type: 'error', message: fault.message, dismissible: true });
}

function unavailableFault(): Fault {
  return ServiceError('ENGINE_IPC_ERROR', 'Engine controls are unavailable.');
}

export function getEngineLogTail(
  lines: number,
): ResultAsync<readonly string[], Fault> {
  const api = window.electron;
  if (api?.getEngineLogTail === undefined) return errAsync(unavailableFault());
  return engineRequest(() => api.getEngineLogTail(lines), engineLogTailSchema);
}

export function restartEngine(): void {
  const api = window.electron;
  if (api?.restartEngine === undefined) {
    showEngineError(unavailableFault());
    return;
  }
  void engineRequest(() => api.restartEngine(), z.boolean()).match(
    () => undefined,
    showEngineError,
  );
}

export function openEngineLogFolder(): void {
  const api = window.electron;
  if (api?.openLogFolder === undefined) {
    showEngineError(unavailableFault());
    return;
  }
  void engineRequest(() => api.openLogFolder(), z.boolean()).match(
    () => undefined,
    showEngineError,
  );
}

export function reportEngineCrash(): void {
  const api = window.electron;
  if (api?.reportCrash === undefined) {
    showEngineError(unavailableFault());
    return;
  }
  void engineRequest(() => api.reportCrash(), z.boolean()).match(
    () => undefined,
    showEngineError,
  );
}

export function initEngineStatus(): () => void {
  const api = window.electron;
  if (
    api?.on === undefined ||
    api.getEngineStatus === undefined ||
    api.getAppVersion === undefined
  ) {
    showEngineError(unavailableFault());
    return () => undefined;
  }
  let receivedPush = false;
  let isActive = true;
  const unsubscribe = api.on('engine:status', (payload) => {
    receivedPush = true;
    parseResponse(engineStatusSchema, payload).match(
      (engineStatus) =>
        appStore.setState((state) => ({ ...state, engineStatus })),
      showEngineError,
    );
  });
  void engineRequest(() => api.getEngineStatus(), engineStatusSchema).match(
    (engineStatus) => {
      if (isActive && !receivedPush)
        appStore.setState((state) => ({ ...state, engineStatus }));
    },
    (fault) => {
      if (isActive) showEngineError(fault);
    },
  );
  void engineRequest(() => api.getAppVersion(), z.string()).match(
    (appVersion) => {
      if (isActive) appStore.setState((state) => ({ ...state, appVersion }));
    },
    (fault) => {
      if (isActive) showEngineError(fault);
    },
  );
  return () => {
    isActive = false;
    unsubscribe();
  };
}
