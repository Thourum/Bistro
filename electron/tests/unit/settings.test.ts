import { describe, expect, it } from 'vitest';
import { DEFAULT_SETTINGS } from '../../src/types/settings';

describe('settings defaults', () => {
  it('defaults the repositories dev folder path to null', () => {
    expect(DEFAULT_SETTINGS.repositories.devFolderPath).toBeNull();
  });

  it('keeps repositories settings available when settings round-trip', () => {
    const settings = {
      ...DEFAULT_SETTINGS,
      repositories: {
        devFolderPath: '/Users/alice/dev',
      },
    };

    const roundTripped = JSON.parse(JSON.stringify(settings));

    expect(roundTripped.repositories).toEqual({
      devFolderPath: '/Users/alice/dev',
    });
  });
});
