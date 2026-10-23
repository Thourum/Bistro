import {
  formatForDisplay,
  useHotkeySequenceRecorder,
} from '@tanstack/react-hotkeys';
import { Fault, ResultAsync } from '@thourum/fault';
import { useEffect, useMemo } from 'react';
import { NavigationStylePicker } from '../../components/NavigationStylePicker';
import { addToast } from '../../components/ToastContainer';
import { Button } from '../../components/ui/button';
import { Kbd } from '../../components/ui/kbd';
import { useStore } from '../../hooks/useStore';
import { setNavigationStyle } from '../../services/navigationStyle';
import { appStore } from '../../store';
import {
  openSettingsFile,
  settingsStore,
  updateSettings,
} from '../../store/settings';
import { findKeybindingConflicts } from '../../utils/keybindingConflicts';
import { getEffectiveBindings, type KeyBinding } from '../../utils/keybindings';
import { Section, showSettingsError } from './SettingsComponents';

function setRecordingKeybinding(isRecordingKeybinding: boolean): void {
  appStore.setState((s) => ({ ...s, isRecordingKeybinding }));
}

interface KeybindingRowProps {
  binding: KeyBinding;
  isOverridden: boolean;
  onRecord: (keys: readonly string[]) => void;
  /** Returns the rejection message when the keys collide with another binding. */
  findConflict: (
    keys: readonly string[],
    events: readonly KeyboardEvent[],
  ) => string | undefined;
  onReset: () => void;
}

function KeybindingRow({
  binding,
  isOverridden,
  onRecord,
  onReset,
  findConflict,
}: KeybindingRowProps) {
  // 'key' mode matches DEFAULT_KEYBINDINGS notation ("J", "Shift+J"); 'code' records "[KeyJ]".
  const recorder = useHotkeySequenceRecorder({
    recordBy: 'key',
    onRecord: (sequence) => onRecord([...sequence]),
    // Rejecting here keeps the recording open so the user can try another key.
    validate: (sequence, { events }) => findConflict(sequence, events) ?? true,
    onReject: (rejection) =>
      addToast({
        type: 'error',
        message: rejection.message,
        dismissible: true,
      }),
  });

  useEffect(() => {
    setRecordingKeybinding(recorder.isRecording);
  }, [recorder.isRecording]);

  useEffect(() => () => setRecordingKeybinding(false), []);

  const keys = recorder.isRecording
    ? recorder.steps
    : typeof binding.keys === 'string'
      ? [binding.keys]
      : binding.keys;

  return (
    <button
      type="button"
      onClick={() => {
        if (!recorder.isRecording) recorder.startRecording();
      }}
      // One recording at a time: moving focus to another row abandons this one.
      onBlur={() => recorder.cancelRecording()}
      onKeyDown={(event) => {
        if (
          !recorder.isRecording &&
          event.key === 'Backspace' &&
          isOverridden
        ) {
          event.preventDefault();
          onReset();
        }
      }}
      aria-label={`${binding.description}: press Enter to rebind${isOverridden ? ', Backspace to reset' : ''}`}
      className={`flex w-full items-center justify-between py-2 pl-2 gap-4 border-b border-l-2 border-b-outline-variant/10 last:border-b-0 text-left focus-visible:outline focus-visible:outline-1 focus-visible:outline-primary ${recorder.isRecording ? 'border-l-primary' : 'border-l-transparent'}`}
    >
      <span className="text-sm text-on-surface truncate">
        {binding.description}
        {isOverridden && (
          <span className="ml-2 font-mono text-[9px] uppercase text-primary">
            custom
          </span>
        )}
      </span>
      <span aria-live="polite" className="flex items-center gap-1 shrink-0">
        {keys.map((key, index) => (
          <Kbd
            // biome-ignore lint/suspicious/noArrayIndexKey: Sequence chords can repeat, and these labels have no state.
            key={index}
            className="px-2 text-[10px] font-bold text-on-surface"
          >
            {formatForDisplay(key)}
          </Kbd>
        ))}
        {recorder.isRecording && (
          <span className="ml-1 font-mono text-[10px] uppercase tracking-widest text-primary">
            {keys.length === 0 ? 'Press keys · ' : ''}Enter save · Esc cancel
          </span>
        )}
      </span>
    </button>
  );
}

export function KeyboardSection() {
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const effective = useMemo(
    () => getEffectiveBindings(style, overrides),
    [style, overrides],
  );
  const groups = useMemo(() => {
    const map = new Map<string, KeyBinding[]>();
    for (const b of effective) {
      const modeKey =
        b.activation.routes?.[0] ?? b.activation.overlays?.[0] ?? 'global';
      const bucket = map.get(modeKey) ?? [];
      bucket.push(b);
      map.set(modeKey, bucket);
    }
    return Array.from(map.entries());
  }, [effective]);

  const conflictMessage = (
    binding: KeyBinding,
    keys: readonly string[],
    events: readonly KeyboardEvent[],
  ): string | undefined => {
    const [first] = keys;
    if (first === undefined) return undefined;
    const candidate: KeyBinding = {
      ...binding,
      keys: keys.length === 1 ? first : keys,
    };
    const [conflict] = findKeybindingConflicts(effective, candidate, events);
    return conflict === undefined
      ? undefined
      : `${keys.map((key) => formatForDisplay(key)).join(' ')} already runs "${conflict.description}"`;
  };

  const saveBinding = (actionId: string, keys: readonly string[]): void => {
    void updateSettings({
      keybindings: { ...overrides, [actionId]: [...keys] },
    }).match(() => undefined, showSettingsError);
  };

  const resetBinding = (actionId: string): void => {
    const { [actionId]: _removed, ...rest } = overrides;
    void updateSettings({ keybindings: rest }).match(
      () => undefined,
      showSettingsError,
    );
  };

  return (
    <Section id="keyboard" title="Keyboard">
      <NavigationStylePicker
        selected={style}
        onSelect={(next) =>
          void setNavigationStyle(next).match(
            () => undefined,
            showSettingsError,
          )
        }
      />
      <p className="font-mono text-[11px] text-on-surface-variant tracking-wide">
        Press Enter on an action to record a new key or chord (Enter saves, Esc
        cancels). Backspace restores the default. Conflicts with keys active in
        the same place are rejected.{' '}
        <Button
          variant="link"
          size="sm"
          onClick={() => {
            void ResultAsync.fromPromise(openSettingsFile(), (cause) =>
              new Fault('Unable to open settings.json.')
                .withTag('SETTINGS_ERROR')
                .withCause(cause),
            ).match(() => undefined, showSettingsError);
          }}
          className="h-auto px-0 py-0 align-baseline normal-case tracking-normal"
        >
          Advanced: edit settings.json
        </Button>
      </p>
      <div className="space-y-8">
        {groups.map(([mode, bindings]) => (
          <div key={mode}>
            <h3 className="font-mono text-[10px] font-bold text-outline tracking-widest uppercase mb-3 pb-2 border-b border-outline-variant/20">
              {mode}
            </h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-x-12 gap-y-2">
              {bindings.map((b) => (
                <KeybindingRow
                  key={b.actionId}
                  binding={b}
                  isOverridden={overrides[b.actionId] !== undefined}
                  onRecord={(keys) => saveBinding(b.actionId, keys)}
                  findConflict={(keys, events) =>
                    conflictMessage(b, keys, events)
                  }
                  onReset={() => resetBinding(b.actionId)}
                />
              ))}
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}
