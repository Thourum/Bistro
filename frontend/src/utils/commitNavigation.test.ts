import { describe, expect, it } from 'vitest';
import { commitPosition, shaFromCommitPath } from './commitNavigation';
import { routeBaseFromPath } from './keybindingActivation';

describe('commit navigation', () => {
  const shas = ['first', 'middle', 'last'];
  it('resolves positions and clamps neighbours at both ends', () => {
    expect(commitPosition(shas, 'middle')).toEqual({
      index: 1,
      total: 3,
      prevSha: 'first',
      nextSha: 'last',
    });
    expect(commitPosition(shas, 'first')?.prevSha).toBeNull();
    expect(commitPosition(shas, 'last')?.nextSha).toBeNull();
    expect(commitPosition(shas, 'unknown')).toBeNull();
    expect(commitPosition([], 'first')).toBeNull();
  });
  it('recognizes only commit list and single-commit routes', () => {
    expect(shaFromCommitPath('/prs/p/commits/abc')).toBe('abc');
    expect(shaFromCommitPath('/prs/p/commits')).toBeNull();
    expect(shaFromCommitPath('/prs/p/commits/abc/extra')).toBeNull();
    expect(routeBaseFromPath('/prs/p/commits')).toBe('commits');
    expect(routeBaseFromPath('/prs/p/commits/abc')).toBe('commit');
  });
});
