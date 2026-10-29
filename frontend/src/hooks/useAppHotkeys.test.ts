import {
  afterEach,
  assert,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest';
import {
  closeSandbox,
  dispatchSandboxAction,
  openSandbox,
  selectOnboardingSandboxPane,
} from '../services/onboardingSandbox';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import { getEffectiveBindings } from '../utils/keybindings';
import {
  createOnboardingSelection,
  mergeOnboardingRepos,
} from '../utils/onboardingRepos';
import { SANDBOX_TASKS } from '../utils/onboardingSandbox';
import { useAppHotkeys, useBindingEnabled } from './useAppHotkeys';
import { useKeyActionContext } from './useKeyActionContext';

interface CapturedHotkey {
  hotkey: string;
  callback(event: KeyboardEvent): void;
  options: {
    enabled: boolean;
    ignoreInputs?: boolean;
    preventDefault?: boolean;
    requireReset?: boolean;
  };
}
interface CapturedSequence {
  sequence: readonly string[];
  callback(): void;
  options: {
    enabled: boolean;
    ignoreInputs?: boolean;
    preventDefault?: boolean;
    requireReset?: boolean;
  };
}

const captured = vi.hoisted(() => ({
  hotkeys: [] as CapturedHotkey[],
  sequences: [] as CapturedSequence[],
  overlay: null as 'command' | null,
  pathname: '/onboarding',
  prId: null as string | null,
}));
const approvePR = vi.hoisted(() => vi.fn());
vi.mock('../services/prActions', () => ({ approvePR }));

vi.mock('react', () => ({
  useMemo: (factory: () => unknown) => factory(),
  useCallback: (callback: unknown) => callback,
  useRef: (value: unknown) => ({ current: value }),
  useState: (initial: () => unknown) => [initial(), () => undefined],
  useEffect: () => undefined,
}));
vi.mock('@tanstack/react-hotkeys', () => ({
  useHotkeys: (definitions: CapturedHotkey[]) => {
    captured.hotkeys = definitions;
  },
  useHotkeySequences: (definitions: CapturedSequence[]) => {
    captured.sequences = definitions;
  },
}));
vi.mock('@tanstack/react-router', () => ({
  useRouterState: () => ({
    location: { pathname: captured.pathname },
    matches:
      captured.prId === null ? [] : [{ params: { prId: captured.prId } }],
  }),
}));
vi.mock('../router', () => ({ router: { navigate: vi.fn() } }));
vi.mock('../overlays/OverlayContext', () => ({
  useOverlays: () => ({
    activeOverlay: captured.overlay,
    closeOverlay: () => {
      captured.overlay = null;
    },
  }),
}));
vi.mock('./useStore', () => ({
  useStore: <State>(
    store: { state: State },
    selector: (state: State) => unknown,
  ) => selector(store.state),
}));

const initialSettings = settingsStore.state;

// Past startup: route-derived bindings apply only once auth was checked.
beforeEach(() => {
  appStore.setState((state) => ({
    ...state,
    authChecked: true,
    onboardingStep: 'practice',
  }));
});

afterEach(() => {
  appStore.setState(() => initialUIState);
  settingsStore.setState(() => initialSettings);
  captured.overlay = null;
  captured.pathname = '/onboarding';
  captured.prId = null;
  approvePR.mockClear();
  vi.unstubAllGlobals();
});

function findHotkey(keys: string): CapturedHotkey {
  const hotkey = captured.hotkeys.find(
    // Keys are shared across contexts; only the enabled binding would fire.
    (definition) => definition.hotkey === keys && definition.options.enabled,
  );
  assert(hotkey !== undefined, `Expected hotkey ${keys}`);
  return hotkey;
}

const unconsumedEvent = {
  defaultPrevented: false,
  preventDefault: vi.fn(),
} as unknown as KeyboardEvent;

describe('onboarding hotkey routing', () => {
  it('closes an overlay with Escape or C-g without navigating the fake PR', () => {
    settingsStore.setState((settings) => ({
      ...settings,
      navigation: { ...settings.navigation, style: 'emacs' },
    }));
    openSandbox();
    dispatchSandboxAction('list.open');
    const inDetail = appStore.state.onboardingSandbox;
    captured.overlay = 'command';
    useAppHotkeys();
    for (const keys of ['Escape', 'Control+G']) {
      captured.overlay = 'command';
      const hotkey = findHotkey(keys);
      expect(hotkey.options.enabled).toBe(true);
      hotkey.callback(unconsumedEvent);
      expect(captured.overlay).toBeNull();
      expect(appStore.state.onboardingSandbox).toBe(inDetail);
    }
  });

  it('routes single keys and sequences to the sandbox and resets or closes it', () => {
    openSandbox();
    useAppHotkeys();
    findHotkey('j').callback(unconsumedEvent);
    expect(appStore.state.onboardingSandbox?.cursor).toBe(1);
    expect(appStore.state.selectedPRIndex).toBe(0);
    const top = captured.sequences.find(
      (definition) => definition.sequence.join(' ') === 'g g',
    );
    assert(top !== undefined, 'Expected top sequence');
    expect(top.options.enabled).toBe(true);
    top.callback();
    expect(appStore.state.onboardingSandbox?.cursor).toBe(0);
    findHotkey('Enter').callback(unconsumedEvent);
    expect(appStore.state.onboardingSandbox?.pane).toBe('detail');
    openSandbox();
    expect(appStore.state.onboardingSandbox?.pane).toBe('list');
    closeSandbox();
    dispatchSandboxAction('list.open');
    expect(appStore.state.onboardingSandbox).toBeNull();
  });

  it('keeps consumed Escape out of the sandbox', () => {
    openSandbox();
    dispatchSandboxAction('list.open');
    useAppHotkeys();
    findHotkey('Escape').callback({ defaultPrevented: true } as KeyboardEvent);
    expect(appStore.state.onboardingSandbox?.pane).toBe('detail');
  });
});

describe('scoped onboarding actions', () => {
  it('consumes cleanup Escape so a newly-enabled rail hide cannot run on that press', () => {
    captured.pathname = '/prs';
    settingsStore.setState((s) => ({
      ...s,
      appearance: {
        ...s.appearance,
        onboardingComplete: true,
        tourComplete: false,
      },
    }));
    appStore.setState((s) => ({ ...s, searchQuery: 'review' }));
    useAppHotkeys();
    const cleanup = findHotkey('Escape');
    const event = {
      defaultPrevented: false,
      target: null,
      preventDefault() {
        this.defaultPrevented = true;
      },
    };
    cleanup.callback(event as unknown as KeyboardEvent);
    expect(event.defaultPrevented).toBe(true);
    expect(appStore.state.searchQuery).toBe('');
    useAppHotkeys();
    findHotkey('Escape').callback(event as unknown as KeyboardEvent);
    expect(appStore.state.firstRunDismissPending).toBe(false);
    expect(settingsStore.state.appearance.tourComplete).toBe(false);
  });

  afterEach(() => vi.unstubAllGlobals());

  it.each([false, true])(
    'blurs the repo filter before any Escape intent while scanning=%s',
    (isScanning) => {
      class FakeFilter {
        blur = vi.fn();
        closest(): FakeFilter {
          return this;
        }
      }
      vi.stubGlobal('HTMLElement', FakeFilter);
      const filter = new FakeFilter();
      appStore.setState((s) => ({
        ...s,
        onboardingStep: 'sync',
        onboardingSync: {
          selection: createOnboardingSelection(),
          initialReposPending: false,
          repoError: null,
          queuePending: false,
          queueError: null,
          folder: isScanning
            ? { status: 'scanning', path: '/dev' }
            : { status: 'none' },
        },
      }));
      useAppHotkeys();
      const escapeHotkey = findHotkey('Escape');
      const preventDefault = vi.fn();
      escapeHotkey.callback({
        target: filter,
        defaultPrevented: false,
        preventDefault,
      } as never);
      expect(filter.blur).toHaveBeenCalledOnce();
      expect(preventDefault).toHaveBeenCalledOnce();
      expect(appStore.state.onboardingIntent).toBeNull();
      escapeHotkey.callback({
        target: null,
        defaultPrevented: false,
        preventDefault,
      } as never);
      expect(appStore.state.onboardingIntent).toBe(
        isScanning ? 'stopScan' : null,
      );
    },
  );

  it('gives completed-practice Space to Continue and shares its gate with footer hints', () => {
    settingsStore.setState((s) => ({
      ...s,
      navigation: { ...s.navigation, style: 'vim' },
    }));
    openSandbox();
    selectOnboardingSandboxPane('detail');
    const continueBinding = getEffectiveBindings('vim', {}).find(
      (binding) => binding.actionId === 'onboarding.continue',
    );
    assert(continueBinding !== undefined);
    expect(useBindingEnabled(continueBinding)).toBe(false);
    appStore.setState((s) => ({
      ...s,
      onboardingSandbox:
        s.onboardingSandbox === null
          ? null
          : {
              ...s.onboardingSandbox,
              completed: SANDBOX_TASKS.map((task) => task.id),
            },
    }));
    expect(useBindingEnabled(continueBinding)).toBe(true);
    useAppHotkeys();
    const enabledSpaces = captured.hotkeys.filter(
      (binding) => binding.hotkey === 'Space' && binding.options.enabled,
    );
    expect(enabledSpaces).toHaveLength(1);
    enabledSpaces[0]?.callback({
      target: null,
      defaultPrevented: false,
      preventDefault: vi.fn(),
    } as never);
    expect(appStore.state.confirmingAction).toBe('onboarding_continue');
    expect(appStore.state.onboardingSandbox?.pane).toBe('detail');
  });
  it('keeps fake approval and range out of real intents and rail progress', () => {
    settingsStore.setState((s) => ({
      ...s,
      navigation: { ...s.navigation, style: 'standard' },
      appearance: {
        ...s.appearance,
        onboardingComplete: false,
        tourComplete: false,
      },
    }));
    openSandbox();
    selectOnboardingSandboxPane('detail');
    useAppHotkeys();
    const approve = captured.sequences.find(
      (definition) => definition.sequence.join(' ') === 'a a',
    );
    assert(approve !== undefined, 'Expected approve sequence');
    approve.callback();
    expect(appStore.state.onboardingSandbox?.approved).toBe(true);
    expect(approvePR).not.toHaveBeenCalled();
    selectOnboardingSandboxPane('diff');
    useAppHotkeys();
    findHotkey('s').callback(unconsumedEvent);
    expect(appStore.state.onboardingSandbox?.sinceLastReview).toBe(true);
    expect(appStore.state.firstRunRail.completed).toEqual([]);
    expect(appStore.state.confirmingAction).toBeNull();
  });

  it('moves Step-1 cursor without opening or moving practice', () => {
    const selection = mergeOnboardingRepos(createOnboardingSelection(), [
      {
        id: 'github:acme/api',
        provider: 'github',
        owner: 'acme',
        repo: 'api',
        watched: false,
      },
      {
        id: 'github:acme/web',
        provider: 'github',
        owner: 'acme',
        repo: 'web',
        watched: false,
      },
    ]);
    settingsStore.setState((s) => ({
      ...s,
      navigation: { ...s.navigation, style: 'vim' },
    }));
    appStore.setState((s) => ({
      ...s,
      onboardingStep: 'sync',
      onboardingSandbox: null,
      onboardingSync: {
        selection,
        initialReposPending: false,
        repoError: null,
        queuePending: false,
        queueError: null,
        folder: { status: 'none' },
      },
    }));
    useAppHotkeys();
    findHotkey('j').callback(unconsumedEvent);
    expect(appStore.state.onboardingSync?.selection.cursor).toBe(1);
    expect(appStore.state.onboardingSandbox).toBeNull();
    expect(appStore.state.selectedPRIndex).toBe(0);
  });
});

describe('shared action context', () => {
  it.each(['unchecked', 'ready'] as const)(
    'uses the effective %s screen for palette context and hotkey activation',
    (screen) => {
      captured.pathname = '/prs/1/diff';
      captured.prId = '1';
      settingsStore.setState((settings) => ({
        ...settings,
        navigation: { ...settings.navigation, style: 'vim' },
      }));
      appStore.setState((state) => ({
        ...state,
        authChecked: screen !== 'unchecked',
        engineStatus: null,
      }));
      const { activation, makeCtx } = useKeyActionContext(undefined);
      const routeBase = screen === 'ready' ? 'diff' : 'connecting';
      expect(activation.routeBase).toBe(routeBase);
      expect(makeCtx()).toMatchObject({
        routeBase,
        prId: '1',
        location: { pathname: '/prs/1/diff' },
      });
      useAppHotkeys();
      const enabledNextComment = captured.hotkeys.some(
        (binding) => binding.hotkey === 'n' && binding.options.enabled,
      );
      expect(enabledNextComment).toBe(screen === 'ready');
    },
  );
});

describe('dispatch safety', () => {
  it('ignores a key repeat when approve is remapped to a single key', () => {
    captured.pathname = '/prs/1';
    captured.prId = '1';
    settingsStore.setState((settings) => ({
      ...settings,
      navigation: { ...settings.navigation, style: 'standard' },
      keybindings: { 'detail.approve': ['a'] },
    }));
    useAppHotkeys();
    const approve = findHotkey('a');
    expect(approve.options.requireReset).toBe(true);
    approve.callback({ ...unconsumedEvent, repeat: true } as KeyboardEvent);
    expect(approvePR).not.toHaveBeenCalled();
    approve.callback({ ...unconsumedEvent, repeat: false } as KeyboardEvent);
    expect(approvePR).toHaveBeenCalledExactlyOnceWith('1');
  });

  it('allows C-g in an overlay input but not in a review composer', () => {
    settingsStore.setState((settings) => ({
      ...settings,
      navigation: { ...settings.navigation, style: 'emacs' },
    }));
    captured.pathname = '/prs/1';
    captured.overlay = 'command';
    useAppHotkeys();
    const cancel = findHotkey('Control+G');
    expect(cancel.options.ignoreInputs).toBe(false);
    cancel.callback(unconsumedEvent);
    expect(captured.overlay).toBeNull();
    useAppHotkeys();
    expect(findHotkey('Control+G').options.ignoreInputs).toBe(true);
  });

  it.each(['standard', 'emacs'] as const)(
    'leaves %s Enter activation to focused buttons before opening a diff',
    (style) => {
      class FakeElement {
        closest(): FakeElement {
          return this;
        }
      }
      vi.stubGlobal('Element', FakeElement);
      settingsStore.setState((settings) => ({
        ...settings,
        navigation: { ...settings.navigation, style },
      }));
      openSandbox();
      dispatchSandboxAction('list.open');
      useAppHotkeys();
      const enter = captured.hotkeys.find(
        (definition) =>
          definition.hotkey === 'Enter' && definition.options.enabled,
      );
      assert(enter !== undefined);
      expect(enter.options.preventDefault).toBe(false);
      const preventDefault = vi.fn();
      enter.callback({ target: new FakeElement(), preventDefault } as never);
      expect(appStore.state.onboardingSandbox?.pane).toBe('detail');
      expect(preventDefault).not.toHaveBeenCalled();
      enter.callback({ target: null, preventDefault } as never);
      expect(appStore.state.onboardingSandbox?.pane).toBe('diff');
      expect(preventDefault).toHaveBeenCalledOnce();
    },
  );

  it('keeps Tab traversal native while the sandbox is open', () => {
    openSandbox();
    useAppHotkeys();
    expect(findHotkey('Tab').options.preventDefault).toBe(false);
    const preventDefault = vi.fn();
    findHotkey('Tab').callback({ target: null, preventDefault } as never);
    expect(settingsStore.state.navigation.style).toBe('vim');
    expect(preventDefault).not.toHaveBeenCalled();
  });
});

describe('onboarding continue', () => {
  it('lets a focused button handle Space natively instead of continuing', () => {
    class FakeElement {
      closest(selector: string): FakeElement | null {
        return selector.includes('button') ? this : null;
      }
    }
    vi.stubGlobal('Element', FakeElement);
    appStore.setState((state) => ({ ...state, onboardingStep: 'sync' }));
    useAppHotkeys();
    const space = findHotkey('Space');
    const preventDefault = vi.fn();

    space.callback({ target: new FakeElement(), preventDefault } as never);
    expect(appStore.state.confirmingAction).toBeNull();
    expect(preventDefault).not.toHaveBeenCalled();

    space.callback({ target: null, preventDefault } as never);
    expect(appStore.state.confirmingAction).toBe('onboarding_continue');
    expect(preventDefault).toHaveBeenCalledOnce();
  });
});
