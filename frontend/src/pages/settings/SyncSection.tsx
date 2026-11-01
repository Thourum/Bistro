import { useStore } from '../../hooks/useStore';
import { settingsStore } from '../../store/settings';
import {
  RowStepper,
  RowToggle,
  Section,
  saveSettings,
} from './SettingsComponents';

export function SyncSection() {
  const sync = useStore(settingsStore, (s) => s.sync);

  return (
    <Section id="sync" title="Sync & Notifications">
      <div className="divide-y divide-outline-variant/10 border-y border-outline-variant/10">
        <RowToggle
          title="Auto Sync"
          description="Automatically refresh PRs in the background."
          on={sync.autoSync}
          onChange={(on) => saveSettings({ sync: { autoSync: on } })}
          inline
        />
        <RowStepper
          title="Sync Interval"
          description="Seconds between auto-sync ticks."
          value={sync.syncInterval}
          min={30}
          max={3600}
          step={30}
          onChange={(v) => saveSettings({ sync: { syncInterval: v } })}
        />
        <RowToggle
          title="Notifications"
          description="Show toasts for new PRs and review requests."
          on={sync.notifications}
          onChange={(on) => saveSettings({ sync: { notifications: on } })}
          inline
        />
      </div>
    </Section>
  );
}
