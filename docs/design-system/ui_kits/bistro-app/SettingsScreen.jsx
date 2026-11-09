// Bistro UI kit — Settings. Mirrors Settings (Minimal).
const { Eyebrow: SetEyebrow, Switch: SetSwitch, SegmentedControl: SetSeg, Input: SetInput, Button: SetButton, Kbd: SetKbd } = window.BistroDesignSystem_8e22c3;

function SettingRow({ title, desc, control }) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 32, padding: '20px 0', borderBottom: '1px solid var(--color-outline-variant)' }}>
      <div style={{ maxWidth: 520 }}>
        <div style={{ fontFamily: 'var(--font-sans)', fontSize: 15, fontWeight: 600, color: 'var(--color-on-surface)' }}>{title}</div>
        {desc && <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--color-on-surface-variant)', marginTop: 4, lineHeight: 1.5 }}>{desc}</div>}
      </div>
      <div style={{ flexShrink: 0 }}>{control}</div>
    </div>
  );
}

function SettingsSection({ eyebrow, title, children }) {
  return (
    <section style={{ marginBottom: 56 }}>
      <SetEyebrow tone="primary" style={{ display: 'block', marginBottom: 8 }}>{eyebrow}</SetEyebrow>
      <h2 style={{ margin: '0 0 8px', fontFamily: 'var(--font-sans)', fontSize: 30, fontWeight: 400, letterSpacing: '-0.03em', color: 'var(--color-on-surface)' }}>{title}</h2>
      <div>{children}</div>
    </section>
  );
}

function SettingsScreen() {
  const [theme, setTheme] = React.useState('light');
  const [view, setView] = React.useState('grouped');
  const [autoSync, setAutoSync] = React.useState(true);
  const [offline, setOffline] = React.useState(true);
  const [uiFont, setUiFont] = React.useState('Geist');
  return (
    <div style={{ height: '100%', overflowY: 'auto', background: 'var(--color-surface-container-lowest)' }}>
      <div style={{ maxWidth: 1152, margin: '0 auto', padding: '80px 80px 120px' }}>
        <header style={{ marginBottom: 64 }}>
          <SetEyebrow tone="primary" style={{ display: 'block', marginBottom: 16 }}>Workspace / Global</SetEyebrow>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-sans)', fontSize: 60, fontWeight: 400, letterSpacing: '-0.05em', lineHeight: 1, color: 'var(--color-on-surface)' }}>Settings</h1>
        </header>

        <SettingsSection eyebrow="01 / Appearance" title="Appearance">
          <SettingRow title="Theme" desc="Dark mode is the developer default; light is the designed primary." control={
            <SetSeg value={theme} onChange={setTheme} options={[
              { value: 'light', label: 'Light' }, { value: 'dark', label: 'Dark' }, { value: 'system', label: 'System' },
            ]} />
          } />
          <SettingRow title="UI font" desc="Resolves through --app-ui-font. Changes apply live." control={
            <div style={{ width: 220 }}><SetInput value={uiFont} onChange={(e) => setUiFont(e.target.value)} icon="text_fields" /></div>
          } />
          <SettingRow title="Inbox view" desc="Flat list, or grouped by repository with foldable sections." control={
            <SetSeg value={view} onChange={setView} options={[{ value: 'flat', label: 'Flat' }, { value: 'grouped', label: 'Grouped' }]} />
          } />
        </SettingsSection>

        <SettingsSection eyebrow="02 / Sync" title="Sync behavior">
          <SettingRow title="Auto-sync" desc="Keep the local SQLite cache warm by syncing watched repos every 60 seconds." control={<SetSwitch checked={autoSync} onChange={(e) => setAutoSync(e.target.checked)} />} />
          <SettingRow title="Offline queue" desc="Queue approvals and comments locally when disconnected; replay on reconnect." control={<SetSwitch checked={offline} onChange={(e) => setOffline(e.target.checked)} />} />
        </SettingsSection>

        <SettingsSection eyebrow="03 / Keyboard" title="Key bindings">
          <SettingRow title="Approve" desc="Double-tap to confirm. Remap any binding; the footer legend updates everywhere." control={<span style={{ display: 'flex', gap: 6 }}><SetKbd>A</SetKbd><SetKbd>A</SetKbd></span>} />
          <SettingRow title="Command palette" control={<SetKbd tone="primary">:</SetKbd>} />
          <SettingRow title="Comment on line" control={<SetKbd>C</SetKbd>} />
          <div style={{ marginTop: 24 }}>
            <SetButton variant="outline" icon="restart_alt">Reset to defaults</SetButton>
          </div>
        </SettingsSection>
      </div>
    </div>
  );
}

window.SettingsScreen = SettingsScreen;
