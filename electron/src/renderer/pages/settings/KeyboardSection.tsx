import { formatForDisplay } from '@tanstack/react-hotkeys';
import { Fault, ResultAsync } from '@thourum/fault';
import { useMemo } from 'react';
import { NavigationStylePicker } from '../../components/NavigationStylePicker';
import { Button } from '../../components/ui/button';
import { Kbd } from '../../components/ui/kbd';
import { useStore } from '../../hooks/useStore';
import { setNavigationStyle } from '../../services/navigationStyle';
import { openSettingsFile, settingsStore } from '../../store/settings';
import {
  getEffectiveBindings,
  isSequenceBinding,
} from '../../utils/keybindings';
import { Section, showSettingsError } from './SettingsComponents';

export function KeyboardSection() {
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const groups = useMemo(() => {
    const bindings = getEffectiveBindings(style, overrides);
    const map = new Map<string, typeof bindings>();
    for (const b of bindings) {
      const modeKey =
        b.activation.routes?.[0] ?? b.activation.overlays?.[0] ?? 'global';
      const bucket = map.get(modeKey) ?? [];
      bucket.push(b);
      map.set(modeKey, bucket);
    }
    return Array.from(map.entries());
  }, [style, overrides]);

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
        Rebinding UI is coming in the next release. Until then, edit{' '}
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
          settings.json
        </Button>{' '}
        directly under the <code>keybindings</code> key.
      </p>
      <div className="space-y-8">
        {groups.map(([mode, bindings]) => (
          <div key={mode}>
            <h3 className="font-mono text-[10px] font-bold text-outline tracking-widest uppercase mb-3 pb-2 border-b border-outline-variant/20">
              {mode}
            </h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-x-12 gap-y-2">
              {bindings.map((b) => (
                <div
                  key={b.actionId}
                  className="flex items-center justify-between py-2 gap-4 border-b border-outline-variant/10 last:border-b-0"
                >
                  <span className="text-sm text-on-surface truncate">
                    {b.description}
                  </span>
                  <div className="flex items-center gap-1 shrink-0">
                    {(isSequenceBinding(b) ? b.keys : [b.keys as string]).map(
                      (key, index) => (
                        <Kbd
                          // biome-ignore lint/suspicious/noArrayIndexKey: Sequence chords can repeat, and these labels have no state.
                          key={index}
                          className="px-2 text-[10px] font-bold text-on-surface"
                        >
                          {formatForDisplay(key)}
                        </Kbd>
                      ),
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}
