import { err, Fault, ok, ResultAsync } from '@thourum/fault';
import { z } from 'zod';
import { router } from '../router';
import { createLogger } from '../utils/logger';
import { collectDiffFrames } from './frames';

const DEFAULT_SCROLL_FRAMES = 1200;
const DEFAULT_SCROLL_STEP_PX = 48;
const PR_LIST_RENDERED_EVENT = 'bistro:pr-list-rendered';
const log = createLogger('bench');

const collectorSchema = z.url({ protocol: /^https?$/ });
const optionsSchema = z.object({
  collector: collectorSchema,
  quitAfterReady: z.boolean(),
  prId: z.string().min(1).optional(),
  frames: z.coerce.number().int().positive().default(DEFAULT_SCROLL_FRAMES),
  stepPx: z.coerce.number().positive().default(DEFAULT_SCROLL_STEP_PX),
});
type BenchOptions = z.infer<typeof optionsSchema>;
const listRenderedSchema = z.object({
  t: z.number().nonnegative(),
  timeOrigin: z.number().positive(),
  rows: z.number().int().positive(),
});
type ListRendered = z.infer<typeof listRenderedSchema>;

function benchFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause : String(cause))
    .withTag('BENCH_ERROR')
    .withCause(cause);
}

function postEvent<T extends Readonly<{ event: string }>>(
  collector: string,
  event: T,
): ResultAsync<void, Fault> {
  return ResultAsync.fromThrowable(
    () =>
      fetch(`${collector}/event`, {
        method: 'POST',
        body: JSON.stringify(event),
        headers: { 'Content-Type': 'text/plain' },
      }),
    benchFault,
  )().andThen((response) =>
    response.ok
      ? ok(undefined)
      : err(
          new Fault(`Collector returned HTTP ${response.status}`).withTag(
            'HTTP_ERROR',
          ),
        ),
  );
}

async function reportFailure(collector: string, fault: Fault): Promise<void> {
  await postEvent(collector, { event: 'error', message: fault.message }).match(
    () => undefined,
    (postFault) =>
      log.error(
        { err: postFault.message, harnessError: fault.message },
        'bench error delivery failed',
      ),
  );
}

async function onListRendered(
  options: BenchOptions,
  list: ListRendered,
): Promise<void> {
  const posted = await postEvent(options.collector, {
    event: 'pr_list_rendered',
    ...list,
  });
  if (posted.isErr()) await reportFailure(options.collector, posted.error);
  if (options.quitAfterReady) {
    if (window.electron?.quitApp) {
      const quit = await ResultAsync.fromThrowable(
        () => window.electron.quitApp(),
        benchFault,
      )();
      if (quit.isErr()) await reportFailure(options.collector, quit.error);
    }
    return;
  }
  if (posted.isErr() || options.prId === undefined) return;
  const prId = options.prId;
  const collected = await ResultAsync.fromThrowable(
    () => router.navigate({ to: '/prs/$prId/diff', params: { prId } }),
    benchFault,
  )()
    .andThen(() => collectDiffFrames(prId, options))
    .andThen((frames) => postEvent(options.collector, frames))
    .andThen(() => postEvent(options.collector, { event: 'diff_done' }));
  if (collected.isErr())
    await reportFailure(options.collector, collected.error);
}

/** Installs the benchmark listener before React renders; normal launches never import this module. */
export function startBench(): void {
  // Hash history appends outer location.search when parsing a fresh hash. Keep
  // the bench query outside the hash and set the route through its own history.
  if (!location.hash) router.history.replace('/prs/?filter=all');
  const search = new URLSearchParams(location.search);
  const collector = collectorSchema.safeParse(search.get('bench_collector'));
  if (!collector.success) {
    log.error({ err: collector.error.message }, 'invalid bench collector');
    return;
  }
  const options = optionsSchema.safeParse({
    collector: collector.data,
    quitAfterReady: search.get('bench_quit_after_ready') === '1',
    prId: search.get('bench_diff') ?? undefined,
    frames: search.get('bench_frames') ?? undefined,
    stepPx: search.get('bench_step_px') ?? undefined,
  });
  if (!options.success) {
    void reportFailure(
      collector.data,
      new Fault(options.error.message).withTag('VALIDATION_ERROR'),
    );
    return;
  }
  window.addEventListener(
    PR_LIST_RENDERED_EVENT,
    (event) => {
      const list = listRenderedSchema.safeParse(
        event instanceof CustomEvent ? event.detail : undefined,
      );
      if (!list.success) {
        void reportFailure(
          collector.data,
          new Fault(list.error.message).withTag('VALIDATION_ERROR'),
        );
        return;
      }
      void onListRendered(options.data, list.data);
    },
    { once: true },
  );
}
