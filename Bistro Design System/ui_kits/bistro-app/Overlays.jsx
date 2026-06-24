// Bistro UI kit — Command Palette + Help overlays.
const { CommandRow, Kbd: OvlKbd, Eyebrow: OvlEyebrow } = window.BistroDesignSystem_8e22c3;

function CommandPalette({ onClose, onCommand }) {
  const data = window.BistroData;
  const [query, setQuery] = React.useState('');
  const [sel, setSel] = React.useState(0);
  const inputRef = React.useRef(null);
  React.useEffect(() => { inputRef.current?.focus(); }, []);

  const recents = data.repos[0].prs.slice(0, 3);
  const filteredCmds = data.commands.filter((c) => c.title.toLowerCase().includes(query.toLowerCase()));
  const items = [...recents.map((p) => ({ kind: 'pr', p })), ...filteredCmds.map((c) => ({ kind: 'cmd', c }))];

  const onKey = (e) => {
    if (e.key === 'Escape') { onClose(); }
    else if (e.key === 'ArrowDown') { e.preventDefault(); setSel((s) => Math.min(s + 1, items.length - 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)); }
    else if (e.key === 'Enter') { const it = items[sel]; if (it?.kind === 'cmd') onCommand(it.c.id); else onClose(); }
  };

  return (
    <div onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }} onKeyDown={onKey}
      style={{ position: 'absolute', inset: 0, zIndex: 40, backdropFilter: 'blur(20px)', WebkitBackdropFilter: 'blur(20px)', background: 'rgba(248,249,250,0.4)', display: 'flex', justifyContent: 'center', paddingTop: 120 }}>
      <div style={{ width: 680, maxHeight: 560, background: 'var(--color-surface-container-lowest)', border: '1px solid var(--color-primary)', boxShadow: 'var(--shadow-ambient)', display: 'flex', flexDirection: 'column' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '0 16px', height: 56, borderBottom: '1px solid var(--color-outline-variant)' }}>
          <span className="material-symbols-outlined" style={{ color: 'var(--color-primary)' }}>search</span>
          <input ref={inputRef} value={query} onChange={(e) => { setQuery(e.target.value); setSel(0); }}
            placeholder="Type a command or search…"
            style={{ flex: 1, border: 'none', outline: 'none', background: 'transparent', fontFamily: 'var(--font-sans)', fontSize: 18, color: 'var(--color-on-surface)' }} />
          <OvlKbd tone="muted">ESC</OvlKbd>
          <OvlEyebrow style={{ fontSize: 10 }}>to close</OvlEyebrow>
        </div>
        <div style={{ overflowY: 'auto', padding: '8px 8px' }}>
          {recents.length > 0 && <OvlEyebrow style={{ fontSize: 10, display: 'block', padding: '8px 10px' }}>Recently opened PRs</OvlEyebrow>}
          {recents.map((p, i) => (
            <CommandRow key={'pr' + p.number} icon="rebase" title={p.title}
              subtitle={`web-client#${p.number} • ${p.time} by @${p.author}`} selected={sel === i} onClick={onClose} />
          ))}
          <OvlEyebrow style={{ fontSize: 10, display: 'block', padding: '16px 10px 8px' }}>Available commands</OvlEyebrow>
          {filteredCmds.map((c, i) => (
            <CommandRow key={c.id} icon={c.icon} title={c.title} subtitle={c.subtitle} shortcut={c.shortcut}
              selected={sel === recents.length + i} onClick={() => onCommand(c.id)} />
          ))}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', height: 32, padding: '0 16px', background: 'var(--color-surface-container-low)', borderTop: '1px solid var(--color-outline-variant)' }}>
          <div style={{ display: 'flex', gap: 16 }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <span className="material-symbols-outlined" style={{ fontSize: 14, color: 'var(--color-outline)' }}>keyboard_arrow_up</span>
              <span className="material-symbols-outlined" style={{ fontSize: 14, color: 'var(--color-outline)' }}>keyboard_arrow_down</span>
              <OvlEyebrow style={{ fontSize: 9 }}>Navigate</OvlEyebrow>
            </span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}><OvlKbd>↩</OvlKbd><OvlEyebrow style={{ fontSize: 9 }}>Select</OvlEyebrow></span>
          </div>
          <OvlEyebrow style={{ fontSize: 9 }}>Bistro // V.01</OvlEyebrow>
        </div>
      </div>
    </div>
  );
}

function HelpOverlay({ onClose }) {
  const sections = window.BistroData.shortcuts;
  return (
    <div onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}
      style={{ position: 'absolute', inset: 0, zIndex: 40, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <div style={{ width: 680, maxHeight: '80%', overflowY: 'auto', background: 'var(--color-surface-container)', boxShadow: 'var(--shadow-ambient)', padding: 32 }}>
        <h2 style={{ margin: '0 0 24px', fontFamily: 'var(--font-mono)', fontSize: 18, fontWeight: 700, color: 'var(--color-on-surface)' }}>Keyboard Shortcuts</h2>
        {Object.entries(sections).map(([title, rows]) => (
          <section key={title} style={{ marginBottom: 28 }}>
            <h3 style={{ margin: '0 0 14px', fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 700, letterSpacing: '0.15em', textTransform: 'uppercase', color: 'var(--color-on-surface)' }}>{title}</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
              {rows.map(([key, desc]) => (
                <div key={key} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <kbd style={{ fontFamily: 'var(--font-mono)', fontSize: 12, padding: '3px 10px', background: 'var(--color-surface-container-highest)', border: '1px solid var(--color-outline-variant)', color: 'var(--color-on-surface-variant)' }}>{key}</kbd>
                  <span style={{ fontFamily: 'var(--font-sans)', fontSize: 14, color: 'var(--color-on-surface)' }}>{desc}</span>
                </div>
              ))}
            </div>
          </section>
        ))}
        <div style={{ textAlign: 'center', marginTop: 8, paddingTop: 20, borderTop: '1px solid var(--color-outline-variant)', fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--color-on-surface-variant)' }}>
          Press <kbd style={{ fontFamily: 'var(--font-mono)', background: 'var(--color-surface-container)', padding: '1px 6px' }}>?</kbd> or <kbd style={{ fontFamily: 'var(--font-mono)', background: 'var(--color-surface-container)', padding: '1px 6px' }}>Esc</kbd> to close
        </div>
      </div>
    </div>
  );
}

window.CommandPalette = CommandPalette;
window.HelpOverlay = HelpOverlay;
