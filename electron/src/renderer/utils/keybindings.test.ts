import { Store } from '@tanstack/store';
import {
  afterEach,
  assert,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest';

vi.mock('../router', () => ({
  router: { navigate: vi.fn() },
}));

import type { OverlayContextValue } from '../overlays/OverlayContext';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import { getBindingAction, type KeyActionContext } from './keybindingActions';
import {
  hasListEscapeWork,
  hotkeyOptions,
  isBindingEnabled,
  isEscape,
  sameBindingKeys,
} from './keybindingActivation';
import {
  ACTION_CATALOG,
  getEffectiveBindings,
  isActive,
  type KeyBinding,
} from './keybindings';
import type { SandboxPane } from './onboardingSandbox';

function binding(actionId: string): KeyBinding {
  const found = getEffectiveBindings('vim', {}).find(
    (candidate) => candidate.actionId === actionId,
  );
  assert(found !== undefined, `Expected binding ${actionId}`);
  return found;
}

describe('getEffectiveBindings', () => {
  it('lets a user override beat the style table and an empty override unbind', () => {
    const bindings = getEffectiveBindings('vim', {
      'list.down': ['x'],
      'list.mute': [],
      'list.top': ['g', 'h'],
    });
    const keysOf = (id: string) =>
      bindings.find((b) => b.actionId === id)?.keys;
    expect(keysOf('list.down')).toBe('x');
    expect(keysOf('list.top')).toEqual(['g', 'h']);
    expect(keysOf('list.mute')).toBeUndefined();
    expect(keysOf('list.up')).toBe('k');
  });
});

describe('hotkeyOptions', () => {
  it('suppresses cancel while typing and preserves Escape-only event handling', () => {
    const cancel = { actionId: 'global.cancel', keys: 'Control+G' };
    expect(hotkeyOptions(cancel)).toEqual({ ignoreInputs: true });
    expect(isEscape(cancel)).toBe(false);
    expect(hotkeyOptions(binding('global.escape'))).toEqual({
      ignoreInputs: false,
      preventDefault: false,
    });
    expect(hotkeyOptions(binding('list.down'))).toEqual({ ignoreInputs: true });
    expect(hotkeyOptions(binding('onboarding.continue'))).toEqual({
      ignoreInputs: true,
      preventDefault: false,
      requireReset: true,
    });
  });
});

describe('isBindingEnabled on the onboarding route', () => {
  const onboarding = (sandboxPane: SandboxPane | null) => ({
    routeBase: 'onboarding' as const,
    activeOverlay: null,
    sandboxPane,
    onboardingStep:
      sandboxPane === null ? ('sync' as const) : ('practice' as const),
  });

  it('activates sandbox actions by the sandbox pane, not the real route', () => {
    expect(isBindingEnabled(binding('list.down'), onboarding('list'))).toBe(
      true,
    );
    expect(isBindingEnabled(binding('list.down'), onboarding('detail'))).toBe(
      false,
    );
    expect(isBindingEnabled(binding('diff.nextFile'), onboarding('diff'))).toBe(
      true,
    );
    expect(isBindingEnabled(binding('list.mute'), onboarding('list'))).toBe(
      false,
    );
    expect(isBindingEnabled(binding('list.down'), onboarding(null))).toBe(
      false,
    );
  });

  it('only lets Space continue from the sandbox list pane', () => {
    expect(
      isBindingEnabled(binding('onboarding.continue'), onboarding('list')),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('onboarding.continue'), onboarding('detail')),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('onboarding.continue'), onboarding('diff')),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('onboarding.continue'), onboarding(null)),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('detail.viewDiff'), onboarding('detail')),
    ).toBe(true);
  });

  it('keeps overlay precedence and other routes unchanged', () => {
    expect(
      isBindingEnabled(binding('list.down'), {
        ...onboarding('list'),
        activeOverlay: 'command',
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('global.escape'), {
        ...onboarding('detail'),
        activeOverlay: 'command',
      }),
    ).toBe(true);
    for (const routeBase of [
      'list',
      'detail',
      'diff',
      'settings',
      'login',
      null,
    ] as const) {
      const context = { routeBase, activeOverlay: null };
      // First-run actions are additionally gated on the derived screen.
      const routeBindings = getEffectiveBindings('vim', {}).filter(
        ({ actionId }) =>
          !actionId.startsWith('login.') &&
          !actionId.startsWith('connecting.') &&
          !actionId.startsWith('firstRun.'),
      );
      for (const configuredBinding of routeBindings) {
        expect(
          isBindingEnabled(configuredBinding, {
            ...context,
            sandboxPane: 'diff',
          }),
        ).toBe(isActive(configuredBinding, context));
      }
    }
  });

  it('enables style cycling only while the sandbox step is mounted', () => {
    for (const actionId of ['onboarding.nextStyle', 'onboarding.prevStyle']) {
      const styleBinding = binding(actionId);
      expect(isBindingEnabled(styleBinding, onboarding(null))).toBe(false);
      expect(isBindingEnabled(styleBinding, onboarding('list'))).toBe(true);
      expect(isBindingEnabled(styleBinding, onboarding('detail'))).toBe(true);
    }
  });
});

describe('scoped onboarding and first-run actions', () => {
  it('clears pending intent before list filters, preserves sort, and never hides the rail', () => {
    const store = new Store({
      ...initialUIState,
      confirmingAction: 'approve' as const,
      searchQuery: 'review',
      repoFilter: 'acme/api',
      filterState: 'all' as const,
    });
    const context: KeyActionContext = {
      store,
      routeBase: 'list',
      activeOverlay: null,
      overlays: {} as OverlayContextValue,
      location: { pathname: '/prs' },
    };
    expect(hasListEscapeWork(store.state)).toBe(true);
    getBindingAction('global.escape')?.(context);
    expect(store.state.confirmingAction).toBeNull();
    expect(store.state.searchQuery).toBe('review');
    getBindingAction('global.escape')?.(context);
    expect(hasListEscapeWork(store.state)).toBe(false);
    expect(store.state.searchQuery).toBe('');
    expect(store.state.repoFilter).toBeNull();
    expect(store.state.filterState).toBe('open');
    expect(store.state.sortBy).toBe(initialUIState.sortBy);
    expect(store.state.firstRunDismissPending).toBe(false);
  });

  it('compares normalized keys without erasing sequence boundaries', () => {
    expect(sameBindingKeys('ctrl+n', 'Control+N')).toBe(true);
    expect(sameBindingKeys(['ctrl+n', 'a'], ['Control+N', 'A'])).toBe(true);
    expect(sameBindingKeys('Space', ['Space'])).toBe(true);
    expect(sameBindingKeys(['g', 'g'], 'g')).toBe(false);
    const diff = binding('detail.viewDiff');
    expect(
      isBindingEnabled(diff, {
        routeBase: 'onboarding',
        activeOverlay: null,
        sandboxPane: 'detail',
        onboardingStep: 'practice',
        isPracticeComplete: true,
        continueKeys: 'Enter',
      }),
    ).toBe(true);
  });
  it('separates sync controls, practice, and scan cancellation', () => {
    const base = {
      routeBase: 'onboarding' as const,
      activeOverlay: null,
      sandboxPane: null,
    };
    expect(
      isBindingEnabled(binding('onboarding.repoToggle'), {
        ...base,
        onboardingStep: 'sync',
      }),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('onboarding.nextStyle'), {
        ...base,
        onboardingStep: 'sync',
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('onboarding.stopScan'), {
        ...base,
        onboardingStep: 'sync',
        isScanning: false,
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('onboarding.stopScan'), {
        ...base,
        onboardingStep: 'sync',
        isScanning: true,
      }),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('global.escape'), {
        ...base,
        onboardingStep: 'sync',
        isScanning: true,
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(
        {
          ...binding('global.escape'),
          actionId: 'global.cancel',
          keys: 'Control+G',
        },
        {
          ...base,
          onboardingStep: 'sync',
          isScanning: true,
        },
      ),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('onboarding.chooseFolder'), {
        ...base,
        onboardingStep: 'sync',
        isChoosingFolder: true,
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('onboarding.repoToggle'), {
        ...base,
        onboardingStep: 'practice',
        sandboxPane: 'list',
      }),
    ).toBe(false);
  });

  it('gives list Escape cleanup priority over rail dismissal and overlays priority over both', () => {
    const base = {
      routeBase: 'list' as const,
      activeOverlay: null,
      sandboxPane: null,
      isFirstRunRailVisible: true,
    };
    expect(
      isBindingEnabled(binding('firstRun.hide'), {
        ...base,
        hasListEscapeWork: false,
      }),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('global.escape'), {
        ...base,
        hasListEscapeWork: false,
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('firstRun.hide'), {
        ...base,
        hasListEscapeWork: true,
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('global.escape'), {
        ...base,
        hasListEscapeWork: true,
      }),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('firstRun.hide'), {
        ...base,
        activeOverlay: 'search',
        hasListEscapeWork: false,
      }),
    ).toBe(false);
  });

  it('keeps Space on fake PR diff opening until practice complete, then allows Finish', () => {
    const base = {
      routeBase: 'onboarding' as const,
      activeOverlay: null,
      sandboxPane: 'detail' as const,
      onboardingStep: 'practice' as const,
      continueKeys: 'Space',
    };
    expect(
      isBindingEnabled(binding('onboarding.continue'), {
        ...base,
        isPracticeComplete: false,
      }),
    ).toBe(false);
    expect(
      isBindingEnabled(binding('detail.viewDiff'), {
        ...base,
        isPracticeComplete: false,
      }),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('onboarding.continue'), {
        ...base,
        isPracticeComplete: true,
      }),
    ).toBe(true);
    expect(
      isBindingEnabled(binding('detail.viewDiff'), {
        ...base,
        isPracticeComplete: true,
      }),
    ).toBe(false);
  });
});

describe('first-run milestones from real keybinding handlers', () => {
  const previousAppearance = settingsStore.state.appearance;
  const context: KeyActionContext = {
    store: appStore,
    routeBase: 'list',
    activeOverlay: null,
    overlays: {} as OverlayContextValue,
    location: { pathname: '/prs' },
  };

  beforeEach(() => {
    appStore.setState(() => ({ ...initialUIState }));
    settingsStore.setState((state) => ({
      ...state,
      appearance: {
        ...previousAppearance,
        onboardingComplete: true,
        tourComplete: false,
      },
    }));
  });

  afterEach(() => {
    appStore.setState(() => ({ ...initialUIState }));
    settingsStore.setState((state) => ({
      ...state,
      appearance: previousAppearance,
    }));
  });

  it('records a real PR open but not a header or an empty list', () => {
    const open = getBindingAction('list.open');
    assert(open !== undefined);
    open(context);
    expect(appStore.state.firstRunRail.completed).toEqual([]);
    appStore.setState((state) => ({
      ...state,
      visiblePRIds: [''],
      visiblePRRepoKeys: ['acme/api'],
    }));
    open(context);
    expect(appStore.state.firstRunRail.completed).toEqual([]);
    appStore.setState((state) => ({ ...state, visiblePRIds: ['pr-1'] }));
    open(context);
    expect(appStore.state.firstRunRail.completed).toEqual(['open']);
  });

  it('records viewDiff only when it has a PR to open', () => {
    const viewDiff = getBindingAction('detail.viewDiff');
    assert(viewDiff !== undefined);
    const detailContext: KeyActionContext = { ...context, routeBase: 'detail' };
    viewDiff(detailContext);
    expect(appStore.state.firstRunRail.completed).toEqual([]);
    viewDiff({ ...detailContext, prId: 'pr-1' });
    expect(appStore.state.firstRunRail.completed).toEqual(['range']);
  });

  it('records sinceLastReview only when turning the range on', () => {
    const toggleRange = getBindingAction('diff.sinceLastReview');
    assert(toggleRange !== undefined);
    const diffContext: KeyActionContext = {
      ...context,
      routeBase: 'diff',
      prId: 'pr-1',
    };
    appStore.setState((state) => ({ ...state, sinceLastReviewActive: true }));
    toggleRange(diffContext);
    expect(appStore.state.sinceLastReviewActive).toBe(false);
    expect(appStore.state.firstRunRail.completed).toEqual([]);
    toggleRange(diffContext);
    expect(appStore.state.sinceLastReviewActive).toBe(true);
    expect(appStore.state.firstRunRail.completed).toEqual(['range']);
  });
});

describe('inbox keybindings', () => {
  it('resolves an action for every configured binding', () => {
    for (const configuredBinding of ACTION_CATALOG) {
      expect(
        getBindingAction(configuredBinding.actionId),
        configuredBinding.actionId,
      ).toBeTypeOf('function');
    }
  });

  it('defines lane jumps and hides repetitive help entries', () => {
    for (let lane = 1; lane <= 6; lane += 1) {
      const laneBinding = binding(`inbox.lane${lane}`);
      expect(laneBinding.keys).toEqual(['g', String(lane)]);
      expect(laneBinding.hidden).toBe(lane !== 1);
    }
  });

  it('keeps list cursor navigation clamped to visible PRs', () => {
    const store = new Store({
      ...initialUIState,
      selectedPRIndex: 2,
      visiblePRIds: ['1', '2', '3'],
    });
    const context: KeyActionContext = {
      store,
      routeBase: 'list',
      activeOverlay: null,
      overlays: {} as OverlayContextValue,
      location: { pathname: '/prs' },
    };

    getBindingAction('list.down')?.(context);
    expect(store.state.selectedPRIndex).toBe(2);
    getBindingAction('list.up')?.(context);
    expect(store.state.selectedPRIndex).toBe(1);
    getBindingAction('list.pageDown')?.(context);
    expect(store.state.selectedPRIndex).toBe(2);
  });

  it('keeps Shift-R route-scoped between inbox and detail actions', () => {
    const inboxMarkRead = binding('inbox.markLaneRead');
    const detailRequestChanges = binding('detail.requestChanges');
    const withoutOverlay = { activeOverlay: null } as const;

    expect(
      isActive(inboxMarkRead, { ...withoutOverlay, routeBase: 'list' }),
    ).toBe(true);
    expect(
      isActive(inboxMarkRead, { ...withoutOverlay, routeBase: 'detail' }),
    ).toBe(false);
    expect(
      isActive(detailRequestChanges, {
        ...withoutOverlay,
        routeBase: 'detail',
      }),
    ).toBe(true);
    expect(
      isActive(detailRequestChanges, { ...withoutOverlay, routeBase: 'list' }),
    ).toBe(false);
  });
});
