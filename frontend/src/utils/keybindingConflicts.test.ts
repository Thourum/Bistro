// @vitest-environment jsdom
import { afterEach, assert, describe, expect, it, vi } from 'vitest';
import {
  findKeybindingConflicts,
  resetKeybinding,
} from './keybindingConflicts';
import { getEffectiveBindings, type KeyBinding } from './keybindings';
import { KEYMAPS } from './keymaps';

const DEFAULT_KEYBINDINGS = getEffectiveBindings('vim', {});

function remap(actionId: string, keys: string | readonly string[]): KeyBinding {
  const binding = DEFAULT_KEYBINDINGS.find(
    (entry) => entry.actionId === actionId,
  );
  assert.isDefined(binding, `missing ${actionId}`);
  return { ...binding, keys };
}

const conflictIds = (candidate: KeyBinding): string[] =>
  findKeybindingConflicts(DEFAULT_KEYBINDINGS, candidate).map(
    (binding) => binding.actionId,
  );

function scopedBinding(activation: KeyBinding['activation']): KeyBinding {
  return {
    actionId: 'candidate',
    keys: 'n',
    description: 'Candidate',
    activation,
  };
}

describe('findKeybindingConflicts', () => {
  it.each(Object.keys(KEYMAPS) as (keyof typeof KEYMAPS)[])(
    'finds no conflicts among the %s defaults',
    (style) => {
      const bindings = getEffectiveBindings(style, {});
      const conflicts = bindings.flatMap((binding) =>
        findKeybindingConflicts(bindings, binding).map(
          (other) => `${binding.actionId} ↔ ${other.actionId}`,
        ),
      );
      expect(conflicts).toEqual([]);
    },
  );

  it('detects Continue colliding with practice-list navigation before completion', () => {
    const candidate = remap('onboarding.continue', ['g', 'g']);
    expect(conflictIds(candidate)).toEqual(['list.top']);
  });

  it('rejects restoring a default reused after the action was remapped', () => {
    const overrides = {
      'detail.approve': ['q', 'q'],
      'detail.dismissNits': ['a', 'a'],
    };
    const bindings = getEffectiveBindings('standard', overrides);
    const reused = bindings.find(
      (binding) => binding.actionId === 'detail.dismissNits',
    );
    assert.isDefined(reused);
    expect(findKeybindingConflicts(bindings, reused)).toEqual([]);
    const reset = resetKeybinding({
      style: 'standard',
      overrides,
      actionId: 'detail.approve',
    });
    expect(reset.isErr()).toBe(true);
    expect(reset._unsafeUnwrapErr().message).toContain('Dismiss nits');
    expect(overrides['detail.approve']).toEqual(['q', 'q']);
    expect(
      resetKeybinding({
        style: 'standard',
        overrides,
        actionId: 'detail.dismissNits',
      })._unsafeUnwrap(),
    ).toEqual({ 'detail.approve': ['q', 'q'] });
  });

  it('preserves only the literal filter Escape alias, not arbitrary remaps', () => {
    expect(conflictIds(remap('filter.close', 'Control+G'))).toEqual([]);
    const bindings = getEffectiveBindings('emacs', {});
    const closeFilter = bindings.find(
      (binding) => binding.actionId === 'filter.close',
    );
    assert.isDefined(closeFilter);
    expect(
      findKeybindingConflicts(bindings, {
        ...closeFilter,
        keys: 'Control+G',
      }).map((binding) => binding.actionId),
    ).toEqual(['global.cancel']);
  });

  it('separates auth screens while checking engine-connected login actions', () => {
    expect(conflictIds(remap('login.retry', 'r'))).toEqual([]);
    expect(conflictIds(remap('login.retry', 'g'))).toEqual([
      'login.provider.github',
    ]);
  });

  it('detects an onboarding scan action colliding with Continue in sync', () => {
    expect(conflictIds(remap('onboarding.continue', 'Escape'))).toEqual([
      'global.escape',
      'onboarding.stopScan',
    ]);
  });

  it('allows resetting an action with no default in the selected style', () => {
    const overrides = { 'global.cancel': ['Control+Q'] };
    expect(
      resetKeybinding({
        style: 'vim',
        overrides,
        actionId: 'global.cancel',
      })._unsafeUnwrap(),
    ).toEqual({});
  });

  it('flags a key already bound on the same route', () => {
    expect(conflictIds(remap('diff.markViewed', 'n'))).toEqual([
      'diff.nextComment',
    ]);
  });

  it('ignores the same key on a route where the bindings never meet', () => {
    expect(conflictIds(remap('list.mute', 'x'))).toEqual([]);
  });

  it('keeps overlay scopes apart from route scopes', () => {
    expect(conflictIds(remap('threads.next', 'n'))).toEqual([]);
  });

  it('treats a single key that prefixes a sequence as a conflict', () => {
    expect(conflictIds(remap('diff.markViewed', ['d']))).toEqual([
      'diff.deleteDraft',
    ]);
  });

  it('allows sequences that only share a first step', () => {
    expect(conflictIds(remap('list.top', ['g', '7']))).toEqual([]);
  });

  it('does not report the binding being remapped', () => {
    expect(conflictIds(remap('diff.nextComment', 'n'))).toEqual([]);
  });

  it('sees multi-route bindings on each of their routes', () => {
    expect(conflictIds(remap('diff.toggleStyle', 'Shift+/'))).toEqual([
      'global.help',
    ]);
  });

  it('treats an existing single key as a prefix of the candidate sequence', () => {
    expect(conflictIds(remap('diff.deleteDraft', ['n', 'n']))).toEqual([
      'diff.nextComment',
    ]);
  });

  it('detects equal sequences', () => {
    expect(conflictIds(remap('diff.markViewed', ['d', 'd']))).toEqual([
      'diff.deleteDraft',
    ]);
  });

  it('does not confuse line-comment j with diff-route bindings', () => {
    const lineCommentBinding = remap('lineComment.down', 'j');
    const diffBindings = DEFAULT_KEYBINDINGS.filter((binding) =>
      binding.activation.routes?.includes('diff'),
    );
    expect(findKeybindingConflicts(diffBindings, lineCommentBinding)).toEqual(
      [],
    );
  });

  it('normalizes the case of every sequence step', () => {
    expect(conflictIds(remap('diff.markViewed', ['D', 'D']))).toEqual([
      'diff.deleteDraft',
    ]);
  });

  it('normalizes case when a single key prefixes a sequence', () => {
    expect(conflictIds(remap('diff.deleteDraft', ['N', 'n']))).toEqual([
      'diff.nextComment',
    ]);
  });

  it('normalizes modifier aliases and order', () => {
    const bindings = [
      {
        ...scopedBinding({ routes: ['diff'] }),
        actionId: 'existing',
        keys: 'Control+Shift+N',
      },
    ];
    const candidate = {
      ...scopedBinding({ routes: ['diff'] }),
      keys: 'shift+ctrl+n',
    };
    expect(findKeybindingConflicts(bindings, candidate)).toEqual(bindings);
  });

  describe('platform-adaptive Mod', () => {
    afterEach(() => vi.unstubAllGlobals());

    it.each([
      { platform: 'MacIntel', modifier: 'Meta', otherModifier: 'Control' },
      { platform: 'Win32', modifier: 'Control', otherModifier: 'Meta' },
      { platform: 'Linux x86_64', modifier: 'Control', otherModifier: 'Meta' },
    ])(
      'matches registration on $platform',
      ({ platform, modifier, otherModifier }) => {
        vi.stubGlobal('navigator', { platform, userAgent: '' });
        const bindings = [
          {
            ...scopedBinding({ routes: ['diff'] }),
            actionId: 'primary',
            keys: `${modifier}+n`,
          },
          {
            ...scopedBinding({ routes: ['diff'] }),
            actionId: 'other',
            keys: `${otherModifier}+n`,
          },
        ];
        const candidate = {
          ...scopedBinding({ routes: ['diff'] }),
          keys: 'Mod+N',
        };
        expect(
          findKeybindingConflicts(bindings, candidate).map(
            (binding) => binding.actionId,
          ),
        ).toEqual(['primary']);
      },
    );
  });

  it.each<{
    activation: KeyBinding['activation'];
    conflicts: string[];
    scope: string;
  }>([
    { scope: 'implicit all routes', activation: {}, conflicts: ['route'] },
    { scope: 'no routes', activation: { routes: [] }, conflicts: [] },
    {
      scope: 'overlays only',
      activation: { overlays: ['threads'] },
      conflicts: ['overlay'],
    },
    {
      scope: 'empty overlays only',
      activation: { overlays: [] },
      conflicts: [],
    },
    {
      scope: 'empty routes with an overlay',
      activation: { routes: [], overlays: ['threads'] },
      conflicts: ['overlay'],
    },
    {
      scope: 'a route and an overlay',
      activation: { routes: ['diff'], overlays: ['threads'] },
      conflicts: ['route', 'overlay'],
    },
    {
      scope: 'disjoint overlays',
      activation: { overlays: ['lineComment'] },
      conflicts: [],
    },
  ])('uses activation semantics for $scope', ({ activation, conflicts }) => {
    const bindings: readonly KeyBinding[] = [
      { ...scopedBinding({ routes: ['diff'] }), actionId: 'route' },
      { ...scopedBinding({ overlays: ['threads'] }), actionId: 'overlay' },
    ];
    const candidate = scopedBinding(activation);
    expect(
      findKeybindingConflicts(bindings, candidate).map(
        (binding) => binding.actionId,
      ),
    ).toEqual(conflicts);
  });

  it('detects implicit all-route activation on an existing binding', () => {
    const bindings = [{ ...scopedBinding({}), actionId: 'existing' }];
    expect(
      findKeybindingConflicts(bindings, scopedBinding({ routes: ['login'] })),
    ).toEqual(bindings);
  });

  it('catches a shifted character that fires an existing binding', () => {
    const candidate = remap('diff.markViewed', 'Shift+?');
    const events = [
      new KeyboardEvent('keydown', { key: '?', code: 'Slash', shiftKey: true }),
    ];
    expect(
      findKeybindingConflicts(DEFAULT_KEYBINDINGS, candidate, events).map(
        (binding) => binding.actionId,
      ),
    ).toEqual(['global.help']);
  });
});
