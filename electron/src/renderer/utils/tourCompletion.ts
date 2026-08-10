import type { AppearanceSettings, AppSettings } from '../../types/settings';

/** Build the settings patch persisted whenever an open tour is dismissed. */
export function createTourCompletionPatch(
  appearance: AppearanceSettings,
): Partial<AppSettings> {
  return {
    appearance: { ...appearance, tourComplete: true },
  };
}
