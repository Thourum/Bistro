import { afterEach, assert, describe, expect, it, vi } from 'vitest';
import {
  closeSandbox,
  dispatchSandboxAction,
  openSandbox,
} from '../services/onboardingSandbox';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import { useAppHotkeys } from './useAppHotkeys';

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
}));

vi.mock('react', () => ({
  useMemo: (factory: () => unknown) => factory(),
  useCallback: (callback: unknown) => callback,
  useRef: (value: unknown) => ({ current: value }),
  // The capture-phase sequence tracker needs a window; these tests drive
  // the TanStack callbacks directly.
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
    matches: [],
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

afterEach(() => {
  appStore.setState(() => initialUIState);
  settingsStore.setState(() => initialSettings);
  captured.overlay = null;
  captured.pathname = '/onboarding';
  vi.unstubAllGlobals();
});

function findHotkey(keys: string): CapturedHotkey {
  const hotkey = captured.hotkeys.find(
    (definition) => definition.hotkey === keys,
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

describe('dispatch safety', () => {
  it('requires a fresh approval gesture, not a key repeat', () => {
    captured.pathname = '/prs/1';
    settingsStore.setState((settings) => ({
      ...settings,
      navigation: { ...settings.navigation, style: 'standard' },
    }));
    useAppHotkeys();
    const approve = findHotkey('a');
    expect(approve.options.requireReset).toBe(true);
    approve.callback({ ...unconsumedEvent, repeat: false } as KeyboardEvent);
    expect(appStore.state.confirmingAction).toBe('approve');
    approve.callback({ ...unconsumedEvent, repeat: true } as KeyboardEvent);
    expect(appStore.state.confirmingAction).toBe('approve');
    approve.callback({ ...unconsumedEvent, repeat: false } as KeyboardEvent);
    expect(appStore.state.confirmingAction).toBe('approve_confirmed');
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
