import { z } from 'zod';

export const NATIVE_KEYS_BENCH_ARG = '--bistro-bench-keys';
export const benchCollectorSchema = z
  .string()
  .regex(/^http:\/\/(?:127\.0\.0\.1|localhost):\d+\/?$/)
  .refine((value) => {
    const port = Number(value.match(/:(\d+)\/?$/)?.[1]);
    return port > 0 && port <= 65535;
  }, 'Collector must use a loopback HTTP port');

// Audited engine view endpoints. Cache refreshes are allowed; remote writes are not.
const benchReadMessageSchema = z.enum([
  'AUTH_CHECK',
  'REQUEST_PR_LIST',
  'REQUEST_DIFF',
  'REQUEST_COMMENTS',
  'REQUEST_REPO_LIST',
  'REQUEST_REVIEW_QUEUE',
  'GET_PATCHSETS',
  'REQUEST_TRIAGE',
  'REQUEST_INTERDIFF',
  'GET_REVIEW_THREADS',
  'DRAFTS_LIST',
  'CHECK_PR_CHECKOUT',
  'GET_EXPERTISE',
  'GET_EXPERTISE_SETTINGS',
]);
const benchRulesSettingsSchema = z
  .object({
    mode: z.literal('rules'),
    dismissal_scope: z.literal('local'),
    provider: z.string().optional(),
    model: z.string().optional(),
    endpoint: z.string().optional(),
  })
  .strict();

/** Permits audited reads or the rules-only, local classification safety reset. */
export function isBenchMessageAllowed(type: unknown, data: unknown): boolean {
  return (
    benchReadMessageSchema.safeParse(type).success ||
    (type === 'SET_CLASSIFICATION_SETTINGS' &&
      benchRulesSettingsSchema.safeParse(data).success)
  );
}
