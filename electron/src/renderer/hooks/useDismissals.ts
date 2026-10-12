import { useEffect } from 'react';
import { z } from 'zod';
import { dismissComments, undoDismiss } from '../services/dismissals';
import type { Comment } from '../types';
import { selectBotIds, selectNitIds } from '../utils/dismissals';

/** DOM events dispatched by the d/u keybindings (emitThreadsEvent pattern). */
const DISMISS_NITS_EVENT = 'bistro:comments:dismiss-nits';
const DISMISS_BOTS_EVENT = 'bistro:comments:dismiss-bots';
const UNDO_DISMISS_EVENT = 'bistro:comments:undo-dismiss';

const dismissBotsDetailSchema = z.object({ vendor: z.string().optional() });

/** Wires the PR detail view's d n / d b / u events to dismissal services. */
export function useDismissals(
  prId: string | undefined,
  comments: readonly Comment[],
): void {
  useEffect(() => {
    if (prId === undefined) return;

    const onDismissNits = (): void => {
      dismissComments(prId, selectNitIds(comments));
    };

    const onDismissBots = (event: Event): void => {
      const parsed = dismissBotsDetailSchema.safeParse(
        event instanceof CustomEvent ? event.detail : undefined,
      );
      const vendor = parsed.success ? parsed.data.vendor : undefined;
      dismissComments(prId, selectBotIds(comments, vendor));
    };

    document.addEventListener(DISMISS_NITS_EVENT, onDismissNits);
    document.addEventListener(DISMISS_BOTS_EVENT, onDismissBots);
    document.addEventListener(UNDO_DISMISS_EVENT, undoDismiss);
    return () => {
      document.removeEventListener(DISMISS_NITS_EVENT, onDismissNits);
      document.removeEventListener(DISMISS_BOTS_EVENT, onDismissBots);
      document.removeEventListener(UNDO_DISMISS_EVENT, undoDismiss);
    };
  }, [prId, comments]);
}
