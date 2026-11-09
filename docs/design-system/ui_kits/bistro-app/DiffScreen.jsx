// Bistro UI kit — Diff View. Mirrors Diff-View (Minimal Light).
const { Eyebrow: DiffEyebrow, Badge: DiffBadge, DiffLine, CommentCard: DiffComment, Button: DiffButton, LineFocusChip } = window.BistroDesignSystem_8e22c3;

function DiffScreen({ onApprove }) {
  const d = window.BistroData.diff;
  const [composing, setComposing] = React.useState(true);
  return (
    <div style={{ height: '100%', display: 'flex', background: 'var(--color-surface)', position: 'relative' }}>
      {/* File navigator */}
      <aside style={{ width: 268, flexShrink: 0, borderRight: '1px solid var(--color-outline-variant)', padding: '24px 16px', background: 'var(--color-surface)' }}>
        <DiffEyebrow style={{ fontSize: 11, display: 'block', marginBottom: 16 }}>Files_changed ({window.BistroData.pr.files})</DiffEyebrow>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          {d.files.map((f) => (
            <div key={f.path} style={{
              display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', cursor: 'crosshair',
              background: f.active ? 'var(--color-surface-container-lowest)' : 'transparent',
              border: f.active ? '1px solid var(--color-primary)' : '1px solid transparent',
            }}>
              <span className="material-symbols-outlined" style={{ fontSize: 16, color: f.active ? 'var(--color-primary)' : 'var(--color-outline)' }}>description</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: f.active ? 'var(--color-primary)' : 'var(--color-on-surface-variant)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{f.path}</span>
            </div>
          ))}
        </div>
      </aside>

      {/* Diff body */}
      <div style={{ flex: 1, minWidth: 0, overflowY: 'auto' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '20px 28px', borderBottom: '1px solid var(--color-outline-variant)' }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 15, color: 'var(--color-on-surface)' }}>{d.file}</span>
          <DiffBadge>{d.lang}</DiffBadge>
          <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: 15, color: 'var(--color-diff-add-text)' }}>+{d.added}</span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 15, color: 'var(--color-diff-remove-text)' }}>-{d.removed}</span>
        </div>

        <div>
          {d.lines.slice(0, 5).map((ln, i) => (
            <DiffLine key={i} oldNo={ln.o} newNo={ln.n} type={ln.t} focused={ln.focused}>{ln.c}</DiffLine>
          ))}
          {composing && (
            <div style={{ padding: '24px 56px 24px 104px', background: 'var(--color-surface-container-low)' }}>
              <div style={{ maxWidth: 1100 }}>
                <DiffComment author="J_Doe System Arch" tag="Active_draft"
                  actions={<><DiffButton variant="ghost" size="sm" onClick={() => setComposing(false)}>Cancel</DiffButton><DiffButton variant="primary" size="sm" onClick={() => setComposing(false)}>Comment</DiffButton></>} />
              </div>
            </div>
          )}
          {d.lines.slice(5).map((ln, i) => (
            <DiffLine key={i} oldNo={ln.o} newNo={ln.n} type={ln.t} focused={ln.focused}>{ln.c}</DiffLine>
          ))}
        </div>
      </div>

      {/* Floating line-focus chip */}
      <div style={{ position: 'absolute', right: 24, bottom: 24 }}>
        <LineFocusChip label="Line_focus: 44" sub={d.file} />
      </div>
    </div>
  );
}

window.DiffScreen = DiffScreen;
