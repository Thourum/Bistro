import { err, ok, type Result } from 'neverthrow';
import { z } from 'zod';
import { createLogger } from '../utils/logger';

const log = createLogger('OnboardingPage');
const resultMessageSchema = z.object({
  error: z.unknown().optional(),
  message: z.unknown().optional(),
});
const validationFailureSchema = z.object({ ok: z.literal(false) });
const validationSuccessSchema = z.object({
  ok: z.literal(true),
  canonical_path: z.string(),
});
const scanRepositoriesSchema = z.object({ repositories: z.array(z.unknown()) });
const scanCountSchema = z.object({
  ok: z.literal(true),
  // The previous guard accepted every number, including non-finite values.
  discovered: z.custom<number>((value) => typeof value === 'number'),
});

export interface ValidateDevFolderResult {
  canonicalPath: string;
}

interface ScanDevFolderResult {
  discovered: number;
}

function sendResultErrorMessage(response: unknown, fallback: string): string {
  const parsed = resultMessageSchema.safeParse(response);
  if (!parsed.success) return fallback;
  if (typeof parsed.data.error === 'string') return parsed.data.error;
  return typeof parsed.data.message === 'string'
    ? parsed.data.message
    : fallback;
}

/** Interprets the engine's development-folder validation response. */
export function parseValidateDevFolderResult(
  response: unknown,
): Result<ValidateDevFolderResult, string> {
  if (validationFailureSchema.safeParse(response).success) {
    return err(
      sendResultErrorMessage(response, 'Unable to validate that folder.'),
    );
  }
  const parsed = validationSuccessSchema.safeParse(response);
  if (parsed.success) return ok({ canonicalPath: parsed.data.canonical_path });
  log.warn('invalid dev folder validation response');
  return err('Dev folder validation returned an invalid response.');
}

/** Accepts both legacy repository lists and the current discovery count. */
export function parseScanDevFolderResult(
  response: unknown,
): Result<ScanDevFolderResult, string> {
  const repositories = scanRepositoriesSchema.safeParse(response);
  if (repositories.success) {
    return ok({ discovered: repositories.data.repositories.length });
  }
  const parsed = scanCountSchema.safeParse(response);
  if (parsed.success) return ok({ discovered: parsed.data.discovered });
  log.warn('invalid dev folder scan response');
  return err('Dev folder scan returned an invalid response.');
}
