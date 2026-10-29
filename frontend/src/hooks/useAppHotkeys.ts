import {
  getSequenceManager,
  type HotkeySequence,
  type RegisterableHotkey,
} from '@tanstack/hotkeys';
import {
  type UseHotkeyDefinition,
  type UseHotkeySequenceDefinition,
  useHotkeySequences,
  useHotkeys,
} from '@tanstack/react-hotkeys';
import { useEffect, useMemo, useRef } from 'react';
import {
  closeOnboardingSandboxComposer,
  dispatchSandboxAction,
} from '../services/onboardingSandbox';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { emitAppEvent } from '../utils/appEvents';
import { isKeysBench, permitsBenchAction } from '../utils/benchSafety';
import {
  getBindingAction,
  type KeyActionContext,
} from '../utils/keybindingActions';
import {
  blurOnboardingRepoFilter,
  continuesSequence,
  hotkeyOptions,
  isEscape,
  isSandboxComposerTarget,
  isStyleCycle,
  isStylePickerTarget,
  yieldsToFocusedControl,
} from '../utils/keybindingActivation';
import {
  getEffectiveBindings,
  isBindingEnabled,
  isSequenceBinding,
  type KeyBinding,
} from '../utils/keybindings';
import { SEQUENCE_TIMEOUT_MS } from '../utils/keymaps';
import { SANDBOX_ACTION_IDS } from '../utils/onboardingSandbox';
import {
  useBindingActivationContext,
  useKeyActionContext,
} from './useKeyActionContext';
import { useStore } from './useStore';

/** Sandbox actions on the onboarding route drive the fake PR, not the app. */
function runBinding(actionId: string, context: KeyActionContext): void {
  if (
    context.routeBase === 'onboarding' &&
    context.activeOverlay === null &&
    appStore.state.onboardingStep === 'practice' &&
    appStore.state.onboardingSandbox !== null &&
    SANDBOX_ACTION_IDS.has(actionId)
  ) {
    if (permitsBenchAction(actionId)) dispatchSandboxAction(actionId);
    return;
  }
  getBindingAction(actionId)?.(context);
}

/** Allows footer hints to use exactly the same runtime gate as registration. */
export function useBindingEnabled(binding: KeyBinding | undefined): boolean {
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const bindings = useMemo(
    () => getEffectiveBindings(style, overrides),
    [style, overrides],
  );
  const continueKeys = bindings.find(
    (candidate) => candidate.actionId === 'onboarding.continue',
  )?.keys;
  const { activation } = useBindingActivationContext(continueKeys);
  return binding !== undefined && isBindingEnabled(binding, activation);
}

/** Registers all effective bindings; runtime gates give overlapping keys one owner. */
export function useAppHotkeys(): void {
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const bindings = useMemo(
    () => getEffectiveBindings(style, overrides),
    [style, overrides],
  );
  const continueKeys = bindings.find(
    (binding) => binding.actionId === 'onboarding.continue',
  )?.keys;
  const { activation, makeCtx } = useKeyActionContext(continueKeys);
  const activationRef = useRef(activation);
  activationRef.current = activation;
  // Weak membership identifies this dispatch without retaining DOM targets.
  const midSequenceEventsRef = useRef(new WeakSet<KeyboardEvent>());

  const hotkeyDefs = useMemo<UseHotkeyDefinition[]>(
    () =>
      bindings
        .filter((b) => !isSequenceBinding(b))
        .map((binding) => ({
          hotkey: binding.keys as RegisterableHotkey,
          callback: (event: KeyboardEvent) => {
            // Escape only: bail when a handler closer to the event already
            // consumed it.
            //
            // Radix's dismissable layer listens in the document CAPTURE phase
            // and calls preventDefault() when it dismisses. Escape is a
            // discrete event, so React flushes that state update *between* the
            // capture and bubble phases — and `useHotkeys` re-syncs
            // `handle.callback` on every render. So by the time this
            // bubble-phase callback runs it is a fresh closure that already
            // sees `activeOverlay: null`, and `global.escape` fell through to
            // its route branch: one keypress both closed the overlay and
            // navigated out of the route.
            //
            // Capturing state at registration time cannot fix that (the
            // callback is replaced), so we read the event instead. This
            // requires `preventDefault: false` below — the manager otherwise
            // calls preventDefault() itself before every callback, which would
            // make this flag always true.
            if (isEscape(binding) && event.defaultPrevented) return;
            if (midSequenceEventsRef.current.has(event)) return;
            const currentActivation = activationRef.current;
            if (!isBindingEnabled(binding, currentActivation)) return;
            if (
              isEscape(binding) &&
              currentActivation.routeBase === 'onboarding' &&
              currentActivation.activeOverlay === null &&
              currentActivation.onboardingStep === 'sync' &&
              blurOnboardingRepoFilter(event.target)
            ) {
              event.preventDefault();
              return;
            }
            // Escape in a fake composer cancels it, not the sandbox's Back.
            if (
              isEscape(binding) &&
              currentActivation.routeBase === 'onboarding' &&
              currentActivation.onboardingStep === 'practice' &&
              isSandboxComposerTarget(event.target)
            ) {
              closeOnboardingSandboxComposer();
              event.preventDefault();
              return;
            }
            if (binding.actionId === 'detail.approve' && event.repeat) return;
            if (isStyleCycle(binding)) {
              if (!isStylePickerTarget(event.target)) return;
              // Tab selects a style but still moves focus to the next control.
            } else {
              if (yieldsToFocusedControl(binding, event.target)) return;
              // These opt out of the manager's preventDefault (see
              // `hotkeyOptions`), so stop the native scroll/activation here.
              if (
                binding.actionId === 'onboarding.continue' ||
                binding.keys === 'Enter' ||
                binding.keys === 'Space' ||
                binding.keys === 'Tab' ||
                binding.keys === 'Shift+Tab'
              )
                event.preventDefault();
            }
            runBinding(binding.actionId, makeCtx());
            // A store update can re-enable another Escape owner during this event.
            if (isEscape(binding)) event.preventDefault();
          },
          options: {
            enabled: isBindingEnabled(binding, activation),
            ...hotkeyOptions(binding, activation.activeOverlay),
            ...(isStyleCycle(binding) ? { stopPropagation: false } : {}),
          },
        })),
    [bindings, activation, makeCtx],
  );

  const sequenceDefs = useMemo<UseHotkeySequenceDefinition[]>(
    () =>
      bindings.filter(isSequenceBinding).map((binding) => ({
        sequence: [...binding.keys] as HotkeySequence,
        callback: () => {
          if (isBindingEnabled(binding, activationRef.current))
            runBinding(binding.actionId, makeCtx());
        },
        options: {
          enabled: isBindingEnabled(binding, activation),
          ignoreInputs: true,
        },
      })),
    [bindings, activation, makeCtx],
  );

  useEffect(() => {
    if (!isKeysBench()) return;
    emitAppEvent('bistro:bench:bindings', {
      actionIds: bindings
        .filter((binding) => isBindingEnabled(binding, activation))
        .map((binding) => binding.actionId),
    });
  }, [bindings, activation]);

  const timeoutMs = SEQUENCE_TIMEOUT_MS[style];

  // Read TanStack's public progress before its document listeners dispatch.
  // Unlike a parallel history, this never counts keys the manager ignored.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent): void => {
      if (
        continuesSequence(event, {
          sequences: [...getSequenceManager().registrations.state.values()],
          timeoutMs,
          now: Date.now(),
        })
      )
        midSequenceEventsRef.current.add(event);
    };
    window.addEventListener('keydown', onKeyDown, { capture: true });
    return () => {
      window.removeEventListener('keydown', onKeyDown, { capture: true });
      midSequenceEventsRef.current = new WeakSet();
    };
  }, [timeoutMs]);

  useHotkeys(hotkeyDefs, { conflictBehavior: 'allow' });
  useHotkeySequences(sequenceDefs, {
    conflictBehavior: 'allow',
    timeout: SEQUENCE_TIMEOUT_MS[style],
  });
}
