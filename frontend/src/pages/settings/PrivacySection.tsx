import { useStore } from '../../hooks/useStore';
import { settingsStore } from '../../store/settings';
import { RowToggle, Section, saveSettings } from './SettingsComponents';

export function PrivacySection() {
  const privacy = useStore(settingsStore, (s) => s.privacy);

  return (
    <Section id="privacy" title="Privacy">
      <div className="divide-y divide-outline-variant/10 border-y border-outline-variant/10">
        <RowToggle
          title="Usage Telemetry"
          description="Opt in to anonymous product events. No repo names, usernames, file paths, PR text, code, diffs, comments, or tokens are sent."
          on={privacy.telemetryEnabled}
          onChange={(on) =>
            saveSettings({
              privacy: { telemetryEnabled: on },
            })
          }
          inline
        />
      </div>
    </Section>
  );
}
