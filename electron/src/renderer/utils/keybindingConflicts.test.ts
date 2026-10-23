import { assert, describe, expect, it } from 'vitest';
import { findKeybindingConflicts } from './keybindingConflicts';
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
      // Escape layers on purpose; runtime gates pick its one owner.
      const conflicts = bindings
        .filter((binding) => binding.keys !== 'Escape')
        .flatMap((binding) =>
          findKeybindingConflicts(bindings, binding).map(
            (other) => `${binding.actionId} ↔ ${other.actionId}`,
          ),
        );
      expect(conflicts).toEqual([]);
    },
  );

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

  it.each(['j', 'k', 'Shift+J', 'Shift+K', 'h', 'l', 'Enter'])(
    'does not confuse line-comment %s with diff-route bindings',
    (keys) => {
      const lineCommentBinding = DEFAULT_KEYBINDINGS.find(
        (binding) =>
          binding.keys === keys &&
          binding.activation.overlays?.includes('lineComment'),
      );
      assert.isDefined(lineCommentBinding);
      const diffBindings = DEFAULT_KEYBINDINGS.filter((binding) =>
        binding.activation.routes?.includes('diff'),
      );
      expect(findKeybindingConflicts(diffBindings, lineCommentBinding)).toEqual(
        [],
      );
    },
  );

  it('allows the detail and diff comment actions to share c', () => {
    expect(conflictIds(remap('diff.lineComment', 'c'))).toEqual([]);
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
});
