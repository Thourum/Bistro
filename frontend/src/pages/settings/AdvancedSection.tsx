import { Fault, ResultAsync } from '@thourum/fault';
import { Button } from '../../components/ui/button';
import { openSettingsFile, resetSettings } from '../../store/settings';
import { Section, showSettingsError } from './SettingsComponents';

export function AdvancedSection() {
  return (
    <Section id="advanced" title="Advanced">
      <div className="flex flex-wrap gap-3">
        <Button
          variant="secondary"
          onClick={() => {
            void ResultAsync.fromPromise(openSettingsFile(), (cause) =>
              new Fault('Unable to open settings.json.')
                .withTag('SETTINGS_ERROR')
                .withCause(cause),
            ).match(() => undefined, showSettingsError);
          }}
          className="px-5 py-3"
        >
          Open settings.json
        </Button>
        <Button
          variant="destructive"
          onClick={() => {
            if (window.confirm('Reset all settings to defaults?') === true) {
              void ResultAsync.fromPromise(resetSettings(), (cause) =>
                new Fault('Unable to reset settings.')
                  .withTag('SETTINGS_ERROR')
                  .withCause(cause),
              ).match(() => undefined, showSettingsError);
            }
          }}
          className="px-5 py-3"
        >
          Reset to Defaults
        </Button>
      </div>
    </Section>
  );
}
