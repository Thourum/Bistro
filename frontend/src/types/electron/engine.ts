import { z } from 'zod';

export const SLOW_ENGINE_MS = 10_000;
export const engineStatusSchema = z.object({
  state: z.enum(['starting', 'ready', 'crashed']),
  startedAt: z.number().int().nonnegative(),
  isSpawned: z.boolean(),
  code: z.number().int().nullable(),
  signal: z.string().nullable(),
  error: z.string().nullable(),
});
export type EngineStatus = z.infer<typeof engineStatusSchema>;
export type EngineReply<T> = { ok: T } | { err: string };
export const engineLogTailSchema = z.array(z.string());
export const engineLogRequestSchema = z.object({
  lines: z.number().int().min(1).max(60),
});
