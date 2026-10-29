import { getSequenceManager } from '@tanstack/hotkeys';
import {
  err,
  errAsync,
  Fault,
  ok,
  type Result,
  ResultAsync,
} from '@thourum/fault';
import { z } from 'zod';
import { router } from '../router';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { permitsBenchAction } from '../utils/benchSafety';
import { getBindingAction } from '../utils/keybindingActions';
import { isTextareaSubmit } from '../utils/keybindings';

const COMMAND_POLL_MS = 100;
const PREPARE_TIMEOUT_MS = 30_000;
const COMMENT_SELECTOR = 'textarea[aria-label="PR comment"]';
const SYNTHETIC_PR_IDS = [
  'github:bench/large-diff:1',
  'github:bench/large-diff:2',
] as const;
const commandSchema = z.object({
  cellId: z.string().min(1),
  actionId: z.string().min(1),
  style: z.enum(['vim', 'emacs', 'standard']),
  focus: z.enum(['outside', 'inside']),
  phase: z.enum(['prepare', 'snapshot']),
  route: z.enum(['list', 'detail', 'diff']).optional(),
});
type KeysCommand = z.infer<typeof commandSchema>;
type KeysEvent = Readonly<{ event: string } & Record<string, unknown>>;
interface KeysReporter {
  post(event: KeysEvent): ResultAsync<void, Fault>;
  fail(fault: Fault): Promise<void>;
}

function keysFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause : String(cause))
    .withTag('BENCH_KEYS_ERROR')
    .withCause(cause);
}

function parseCommand(value: unknown): Result<KeysCommand, Fault> {
  const parsed = commandSchema.safeParse(value);
  return parsed.success
    ? ok(parsed.data)
    : err(keysFault(parsed.error.message));
}

function readCommand(
  collector: string,
): ResultAsync<KeysCommand | null, Fault> {
  return ResultAsync.fromThrowable(
    () =>
      fetch(`${collector}/keys-command`, {
        cache: 'no-store',
        redirect: 'error',
      }),
    keysFault,
  )().andThen((response) => {
    if (response.status === 204) return ok(null);
    if (!response.ok)
      return err(keysFault(`Keys collector HTTP ${response.status}`));
    return ResultAsync.fromThrowable(
      () => response.json(),
      keysFault,
    )().andThen(parseCommand);
  });
}

function targetState(
  target: EventTarget | null,
): Readonly<{ targetKind: string; text: string }> {
  if (target instanceof HTMLTextAreaElement)
    return { targetKind: 'textarea', text: target.value };
  if (target instanceof HTMLInputElement)
    return { targetKind: 'input', text: target.value };
  if (target instanceof HTMLElement && target.isContentEditable)
    return { targetKind: 'contenteditable', text: target.textContent ?? '' };
  return {
    targetKind:
      target instanceof Element ? target.tagName.toLowerCase() : 'none',
    text: '',
  };
}

function snapshot(
  command: KeysCommand,
  actionIds: readonly string[],
): KeysEvent {
  const textarea =
    document.querySelector<HTMLTextAreaElement>(COMMENT_SELECTOR);
  const isLocalTextareaAction =
    command.focus === 'inside' &&
    ['textarea.submit', 'textarea.escape'].includes(command.actionId);
  return {
    event: command.phase === 'prepare' ? 'keys_prepared' : 'keys_snapshot',
    ...command,
    pathname: router.state.location.pathname,
    ...targetState(document.activeElement),
    text: textarea?.value ?? '',
    selectedPRIndex: appStore.state.selectedPRIndex,
    selectedFileIndex: appStore.state.selectedFileIndex,
    confirmingAction: appStore.state.confirmingAction,
    dryDispatch: true,
    intendedBindingActive:
      isLocalTextareaAction || actionIds.includes(command.actionId),
    expectedIgnored: command.focus === 'inside' && !isLocalTextareaAction,
    supported:
      isLocalTextareaAction ||
      (getBindingAction(command.actionId) !== undefined &&
        /^(list|inbox|detail|diff|global)\./.test(command.actionId)),
    t: performance.now(),
  };
}

function waitForSurface(route: string): ResultAsync<void, Fault> {
  return new ResultAsync(
    new Promise<Result<void, Fault>>((resolve) => {
      const deadline = performance.now() + PREPARE_TIMEOUT_MS;
      const poll = (): void => {
        const selector =
          route === 'detail'
            ? '[data-testid="pr-detail-page"]'
            : route === 'diff'
              ? '[data-diff-viewport]'
              : '[data-testid="pr-list-page"]';
        if (document.querySelector(selector) !== null) {
          requestAnimationFrame(() =>
            requestAnimationFrame(() => resolve(ok(undefined))),
          );
          return;
        }
        if (performance.now() >= deadline) {
          resolve(
            err(
              keysFault(
                `Keys ${route} fixture did not render within ${PREPARE_TIMEOUT_MS}ms`,
              ),
            ),
          );
          return;
        }
        setTimeout(poll, COMMAND_POLL_MS);
      };
      poll();
    }),
  );
}

function prepare(command: KeysCommand, prId: string): ResultAsync<void, Fault> {
  const route =
    command.route ??
    (command.focus === 'inside'
      ? 'detail'
      : command.actionId.startsWith('diff.')
        ? 'diff'
        : command.actionId.startsWith('detail.') ||
            command.actionId.startsWith('threads.')
          ? 'detail'
          : 'list');
  if (command.focus === 'inside' && route !== 'detail')
    return errAsync(keysFault('Inside focus requires detail route'));
  getSequenceManager().resetAll();
  settingsStore.setState((state) => ({
    ...state,
    navigation: { ...state.navigation, style: command.style },
    keybindings: {},
  }));
  appStore.setState((state) => ({
    ...state,
    selectedPRIndex: 0,
    selectedFileIndex: 0,
    confirmingAction: null,
    lineComment: null,
    replyingInlineCommentThreadId: null,
    selectedPatchsetPair: null,
    sinceLastReviewActive: false,
    interdiffActive: false,
    hideWhitespaceHunks: false,
    diffCollapseOverrides: [],
    commentMinSeverity: null,
    commentHumanOnly: false,
  }));
  // Remount the real composer so each cell starts with empty React-owned text.
  return ResultAsync.fromThrowable(
    () => router.navigate({ to: '/prs', search: { filter: 'all' } }),
    keysFault,
  )()
    .andThen(() => waitForSurface('list'))
    .andThen(() =>
      ResultAsync.fromThrowable(
        () =>
          route === 'list'
            ? Promise.resolve()
            : router.navigate({
                to: route === 'diff' ? '/prs/$prId/diff' : '/prs/$prId',
                params: { prId },
              }),
        keysFault,
      )(),
    )
    .andThen(() => waitForSurface(route))
    .andThen(() => {
      if (route === 'detail' && command.focus === 'inside')
        appStore.setState((state) => ({
          ...state,
          confirmingAction: 'comment',
        }));
      return new ResultAsync(
        new Promise<Result<void, Fault>>((resolve) => {
          requestAnimationFrame(() =>
            requestAnimationFrame(() => {
              const textarea =
                document.querySelector<HTMLTextAreaElement>(COMMENT_SELECTOR);
              if (command.focus === 'inside') {
                if (textarea === null) {
                  resolve(err(keysFault('PR comment textarea missing')));
                  return;
                }
                textarea.focus();
              } else {
                if (document.activeElement instanceof HTMLElement)
                  document.activeElement.blur();
                // Body receives native keys after blur, without a tabindex ancestor
                // that would make the real Tab ownership gate yield to a control.
              }
              resolve(ok(undefined));
            }),
          );
        }),
      );
    });
}

/** Records native DOM outcomes and dry action dispatch without adding hotkeys. */
export function startKeysBench(
  collector: string,
  reporter: KeysReporter,
): boolean {
  const fixture = z
    .enum(SYNTHETIC_PR_IDS)
    .safeParse(new URLSearchParams(location.search).get('bench_keys_pr'));
  if (!fixture.success) {
    void reporter.fail(
      keysFault('bench_keys_pr must name a seeded synthetic PR'),
    );
    return false;
  }
  const session: { command: KeysCommand | null; actionIds: readonly string[] } =
    { command: null, actionIds: [] };
  const report = (event: KeysEvent): void => {
    void reporter.post(event).match(() => undefined, reporter.fail);
  };
  document.addEventListener('bistro:bench:bindings', (event) => {
    const bindings = z
      .object({ actionIds: z.array(z.string()) })
      .safeParse(event instanceof CustomEvent ? event.detail : undefined);
    if (!bindings.success) {
      void reporter.fail(keysFault('Invalid runtime bindings detail'));
      return;
    }
    session.actionIds = bindings.data.actionIds;
  });
  document.addEventListener('bistro:bench:action', (event) => {
    const action = z
      .object({ actionId: z.string().min(1) })
      .safeParse(event instanceof CustomEvent ? event.detail : undefined);
    if (!action.success) {
      void reporter.fail(keysFault('Invalid action dispatch detail'));
      return;
    }
    report({
      event: 'keys_action',
      cellId: session.command?.cellId ?? null,
      actionId: action.data.actionId,
      dryDispatch: true,
      t: performance.now(),
    });
  });
  document.addEventListener('bistro:bench:blocked-write', (event) => {
    const blocked = z
      .object({ type: z.string().min(1) })
      .safeParse(event instanceof CustomEvent ? event.detail : undefined);
    if (!blocked.success) {
      void reporter.fail(keysFault('Invalid blocked-write detail'));
      return;
    }
    report({
      event: 'keys_blocked_write',
      cellId: session.command?.cellId ?? null,
      type: blocked.data.type,
      t: performance.now(),
    });
  });
  window.addEventListener(
    'keydown',
    (event) => {
      report({
        event: 'keys_keydown',
        cellId: session.command?.cellId ?? null,
        key: event.key,
        code: event.code,
        altKey: event.altKey,
        ctrlKey: event.ctrlKey,
        metaKey: event.metaKey,
        shiftKey: event.shiftKey,
        isComposing: event.isComposing,
        repeat: event.repeat,
        ...targetState(event.target),
        t: performance.now(),
      });
      if (
        event.target instanceof HTMLTextAreaElement &&
        (isTextareaSubmit(event) || event.key === 'Escape')
      ) {
        event.preventDefault();
        event.stopImmediatePropagation();
        if (event.key === 'Escape' || event.target.value.trim().length > 0)
          permitsBenchAction(
            event.key === 'Escape' ? 'textarea.escape' : 'textarea.submit',
          );
      }
    },
    { capture: true },
  );
  document.addEventListener('input', (event) => {
    report({
      event: 'keys_input',
      cellId: session.command?.cellId ?? null,
      inputType: event instanceof InputEvent ? event.inputType : '',
      isComposing: event instanceof InputEvent && event.isComposing,
      ...targetState(event.target),
      t: performance.now(),
    });
  });
  for (const phase of [
    'compositionstart',
    'compositionupdate',
    'compositionend',
  ])
    document.addEventListener(phase, (event) => {
      report({
        event: 'keys_composition',
        cellId: session.command?.cellId ?? null,
        phase,
        ...targetState(event.target),
        t: performance.now(),
      });
    });
  const poll = (): void => {
    void readCommand(collector)
      .andThen((command) => {
        if (command === null) return ok(undefined);
        if (
          command.phase === 'snapshot' &&
          session.command?.cellId !== command.cellId
        )
          return err(keysFault('Snapshot cell does not match prepared cell'));
        session.command = command;
        return (
          command.phase === 'prepare'
            ? prepare(command, fixture.data)
            : ResultAsync.fromSafePromise(Promise.resolve())
        ).andThen(() => reporter.post(snapshot(command, session.actionIds)));
      })
      .match(() => {
        setTimeout(poll, COMMAND_POLL_MS);
      }, reporter.fail);
  };
  window.addEventListener(
    'bistro:pr-list-rendered',
    () => {
      void wsClient
        .request('SET_CLASSIFICATION_SETTINGS', {
          mode: 'rules',
          dismissal_scope: 'local',
          provider: 'anthropic',
          model: '',
          endpoint: '',
        })
        .mapErr(keysFault)
        .andThen((response) => {
          const settings = z
            .object({
              mode: z.literal('rules'),
              dismissal_scope: z.literal('local'),
            })
            .safeParse(response);
          return settings.success
            ? ok(settings.data)
            : err(
                keysFault('Engine did not confirm rules-only classification'),
              );
        })
        .andThen((settings) =>
          reporter.post({
            event: 'keys_ready',
            prId: fixture.data,
            dryDispatch: true,
            classificationMode: settings.mode,
            externalWritesBlocked: true,
            userAgent: navigator.userAgent,
          }),
        )
        .match(poll, reporter.fail);
    },
    { once: true },
  );
  return true;
}
