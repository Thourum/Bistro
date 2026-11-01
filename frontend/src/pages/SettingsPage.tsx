import { useState } from 'react';
import { AdvancedSection } from './settings/AdvancedSection';
import { AppearanceSection } from './settings/AppearanceSection';
import { ClassificationSection } from './settings/ClassificationSection';
import { ConnectionsSection } from './settings/ConnectionsSection';
import { ExpertiseSection } from './settings/ExpertiseSection';
import { KeyboardSection } from './settings/KeyboardSection';
import { PrivacySection } from './settings/PrivacySection';
import { RepositoriesSection } from './settings/RepositoriesSection';
import { SyncSection } from './settings/SyncSection';

export { expertiseSettingsPayloadFromInputs } from './settings/ExpertiseSection';

type SettingsSection =
  | 'appearance'
  | 'keyboard'
  | 'connections'
  | 'repositories'
  | 'sync'
  | 'classification'
  | 'expertise'
  | 'privacy'
  | 'advanced';

const SECTIONS: ReadonlyArray<{ id: SettingsSection; label: string }> = [
  { id: 'appearance', label: 'Appearance' },
  { id: 'keyboard', label: 'Keyboard' },
  { id: 'connections', label: 'Connections' },
  { id: 'repositories', label: 'Repositories' },
  { id: 'sync', label: 'Sync' },
  { id: 'classification', label: 'Comment Classification' },
  { id: 'expertise', label: 'Expertise' },
  { id: 'privacy', label: 'Privacy' },
  { id: 'advanced', label: 'Advanced' },
];

/**
 * SettingsPage — matches `settings_minimal/code.html` with a left rail
 * sidebar and stacked, scroll-spy-free content column.
 *
 * All mutations are routed through `updateSettings`, which writes a delta
 * through IPC to `settings.json`. The main process echoes the full settings
 * object back and our `useSettingsEffect` hook re-applies theme / font /
 * density CSS to the DOM.
 */
export function SettingsPage() {
  const [active, setActive] = useState<SettingsSection>('appearance');

  return (
    <main className="flex-1 overflow-y-auto bg-surface-container-lowest">
      <div className="max-w-6xl mx-auto px-8 md:px-12 lg:px-20 py-16 pb-40">
        <Header />
        <div className="grid grid-cols-1 md:grid-cols-12 gap-16">
          <Sidebar active={active} onSelect={setActive} />

          <div className="md:col-span-9 space-y-24">
            <AppearanceSection />
            <KeyboardSection />
            <ConnectionsSection />
            <RepositoriesSection />
            <SyncSection />
            <ClassificationSection />
            {active === 'expertise' && <ExpertiseSection />}
            <PrivacySection />
            <AdvancedSection />
          </div>
        </div>
      </div>
    </main>
  );
}

function Header() {
  return (
    <header className="mb-16">
      <div className="flex items-center gap-2 mb-4">
        <span className="font-mono text-[11px] tracking-widest text-primary uppercase">
          Workspace / Global
        </span>
      </div>
      <h1 className="text-5xl md:text-6xl font-black tracking-tighter mb-4 text-on-surface">
        Settings
      </h1>
      <p className="text-on-surface-variant font-mono text-[11px] tracking-widest uppercase border-l-2 border-primary pl-4">
        Technical Atelier / Preferences
      </p>
    </header>
  );
}

interface SidebarProps {
  active: SettingsSection;
  onSelect: (section: SettingsSection) => void;
}

function Sidebar({ active, onSelect }: SidebarProps) {
  return (
    <nav className="md:col-span-3 md:sticky md:top-8 md:self-start flex flex-col gap-1">
      <div className="mb-4 font-mono text-[10px] text-outline uppercase tracking-widest">
        Configuration
      </div>
      {SECTIONS.map((section) => {
        const isActive = section.id === active;
        return (
          <button
            key={section.id}
            type="button"
            onClick={() => {
              onSelect(section.id);
              const el = document.getElementById(
                `settings-section-${section.id}`,
              );
              if (el !== null && typeof el.scrollIntoView === 'function') {
                el.scrollIntoView({ behavior: 'smooth', block: 'start' });
              }
            }}
            className={`text-left px-4 py-2 text-sm transition-colors border-l-2 ${
              isActive
                ? 'text-primary font-bold border-primary bg-surface-container-low'
                : 'text-on-surface-variant border-transparent hover:bg-surface-container-low'
            }`}
          >
            {section.label}
          </button>
        );
      })}
    </nav>
  );
}
