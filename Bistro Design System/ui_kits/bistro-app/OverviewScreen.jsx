// Bistro UI kit — PR Overview. Mirrors PR-Overview (Minimal Refined).
const { Eyebrow: OvEyebrow, CommentCard, Button: OvButton, StatusDot: OvDot } = window.BistroDesignSystem_8e22c3;

function MetaCol({ label, value, mono = true, color }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <OvEyebrow style={{ fontSize: 11 }}>{label}</OvEyebrow>
      <span style={{ fontFamily: mono ? 'var(--font-mono)' : 'var(--font-sans)', fontSize: 14, color: color || 'var(--color-on-surface)' }}>{value}</span>
    </div>
  );
}

function OverviewScreen({ onViewDiff }) {
  const pr = window.BistroData.pr;
  return (
    <div style={{ height: '100%', overflowY: 'auto', background: 'var(--color-surface)' }}>
      <div style={{ display: 'flex', maxWidth: 1700, margin: '0 auto' }}>
        <div style={{ flex: 1, minWidth: 0, padding: '40px 56px 64px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 28 }}>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 700, letterSpacing: '0.1em', color: 'var(--color-primary)' }}>PR-{pr.number}</span>
            <span style={{ flex: 1, height: 1, background: 'var(--color-outline-variant)' }} />
          </div>
          <h1 style={{ margin: '0 0 32px', fontFamily: 'var(--font-sans)', fontSize: 52, fontWeight: 800, lineHeight: 1.05, letterSpacing: '-0.03em', color: 'var(--color-on-surface)', maxWidth: 1100 }}>{pr.title}</h1>

          <div style={{ display: 'flex', gap: 48, flexWrap: 'wrap', marginBottom: 36 }}>
            <MetaCol label="Author" value={'@' + pr.author} />
            <MetaCol label="Date" value={pr.date} />
            <MetaCol label="Source" value={<span style={{ background: 'var(--color-surface-container)', padding: '3px 8px' }}>{pr.source}</span>} />
            <MetaCol label="Target" value={<span style={{ background: 'var(--color-surface-container)', padding: '3px 8px' }}>{pr.target}</span>} />
            <MetaCol label="Files" value={pr.files} />
            <MetaCol label="Added" value={'+' + pr.added} color="var(--color-diff-add-text)" />
            <MetaCol label="Removed" value={'-' + pr.removed} color="var(--color-diff-remove-text)" />
          </div>

          <div style={{ border: '1px solid var(--color-primary)', padding: '28px 32px', marginBottom: 24 }}>
            <p style={{ margin: 0, fontFamily: 'var(--font-sans)', fontSize: 18, lineHeight: 1.6, color: 'var(--color-on-surface)' }}>{pr.description}</p>
          </div>

          <div style={{ background: 'var(--color-surface-container-lowest)', border: '1px solid var(--color-outline-variant)', padding: '28px 32px', marginBottom: 44 }}>
            <OvEyebrow style={{ fontSize: 12 }}>Technical Impact</OvEyebrow>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 14, marginTop: 18 }}>
              {pr.impact.map((t, i) => (
                <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <span className="material-symbols-outlined" style={{ fontSize: 18, color: 'var(--color-primary)' }}>check</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 14, color: 'var(--color-on-surface-variant)' }} dangerouslySetInnerHTML={{ __html: t.replace(/`([^`]+)`/g, '<span style="background:var(--color-surface-container);padding:1px 5px">$1</span>') }} />
                </div>
              ))}
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
            <h2 style={{ margin: 0, fontFamily: 'var(--font-mono)', fontSize: 15, letterSpacing: '0.18em', textTransform: 'uppercase', color: 'var(--color-on-surface)', fontWeight: 700 }}>Recent Comments</h2>
            <OvEyebrow style={{ fontSize: 11 }}>03 Unresolved</OvEyebrow>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16, marginBottom: 36 }}>
            {pr.comments.map((c, i) => (
              <CommentCard key={i} author={c.author} time={c.time}
                actions={c.actions ? <><OvButton variant="ghost" size="sm">Reply</OvButton><OvButton variant="ghost" size="sm">Resolve</OvButton></> : undefined}>
                {c.body}
              </CommentCard>
            ))}
          </div>

          <button type="button" onClick={onViewDiff}
            style={{ display: 'inline-flex', alignItems: 'center', gap: 12, padding: '14px 20px', background: 'var(--color-surface-container)', border: 'none', cursor: 'pointer', fontFamily: 'var(--font-mono)', fontSize: 13, letterSpacing: '0.1em', textTransform: 'uppercase', color: 'var(--color-on-surface-variant)' }}>
            <span className="material-symbols-outlined" style={{ fontSize: 18 }}>keyboard_command_key</span>
            Press [Space] on any file to view diff
          </button>
        </div>

        {/* Right status rail */}
        <div style={{ width: 180, flexShrink: 0, padding: '40px 24px', display: 'flex', flexDirection: 'column', gap: 36, alignItems: 'center' }}>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
            <span style={{ width: 44, height: 44, border: '1px solid var(--color-primary)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <span className="material-symbols-outlined" style={{ fontSize: 22, color: 'var(--color-primary)' }}>autorenew</span>
            </span>
            <OvEyebrow tone="primary" style={{ fontSize: 10, textAlign: 'center' }}>Checks Running</OvEyebrow>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
            <span style={{ width: 44, height: 44, border: '1px solid var(--color-outline-variant)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <span className="material-symbols-outlined" style={{ fontSize: 22, color: 'var(--color-outline)' }}>person</span>
            </span>
            <OvEyebrow style={{ fontSize: 10, textAlign: 'center' }}>Awaiting Review</OvEyebrow>
          </div>
        </div>
      </div>
    </div>
  );
}

window.OverviewScreen = OverviewScreen;
