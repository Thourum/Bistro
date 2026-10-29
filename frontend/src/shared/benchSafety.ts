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
