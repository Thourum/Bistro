import { err, Fault, ok, type Result } from '@thourum/fault';
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
const scanFailureSchema = z.object({
  ok: z.literal(false),
  message: z.string(),
});
const scanRepositoriesSchema = z.object({ repositories: z.array(z.unknown()) });
const scanCountSchema = z.object({
  ok: z.literal(true),
  discovered: z.number().int().nonnegative(),
  partial: z.boolean(),
});

export interface ValidateDevFolderResult {
  canonicalPath: string;
}

export interface ScanDevFolderResult {
  discovered: number;
  partial: boolean;
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
): Result<ValidateDevFolderResult, Fault> {
  if (validationFailureSchema.safeParse(response).success) {
    return err(
      new Fault(
        sendResultErrorMessage(response, 'Unable to validate that folder.'),
      ).withTag('VALIDATION_ERROR'),
    );
  }
  const parsed = validationSuccessSchema.safeParse(response);
  if (parsed.success) return ok({ canonicalPath: parsed.data.canonical_path });
  log.warn('invalid dev folder validation response');
  return err(
    new Fault('Dev folder validation returned an invalid response.').withTag(
      'VALIDATION_ERROR',
    ),
  );
}

/** Accepts both legacy repository lists and the current discovery count. */
export function parseScanDevFolderResult(
  response: unknown,
): Result<ScanDevFolderResult, Fault> {
  const repositories = scanRepositoriesSchema.safeParse(response);
  if (repositories.success) {
    return ok({
      discovered: repositories.data.repositories.length,
      partial: false,
    });
  }
  const failure = scanFailureSchema.safeParse(response);
  if (failure.success) {
    return err(new Fault(failure.data.message).withTag('VALIDATION_ERROR'));
  }
  const parsed = scanCountSchema.safeParse(response);
  if (parsed.success)
    return ok({
      discovered: parsed.data.discovered,
      partial: parsed.data.partial,
    });
  log.warn('invalid dev folder scan response');
  return err(
    new Fault('Dev folder scan returned an invalid response.').withTag(
      'VALIDATION_ERROR',
    ),
  );
}
