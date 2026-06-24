// Bistro UI kit — PR Inbox. Mirrors PR-Inbox (Minimal Light).
const { Eyebrow: InboxEyebrow, Badge: InboxBadge, PRRow, SegmentedControl } = window.BistroDesignSystem_8e22c3;

function RepoGroup({ group, baseIndex, selected, onSelect, onOpen }) {
  const [open, setOpen] = React.useState(!group.collapsed);
  return (
    <div style={{ marginBottom: open ? 4 : 0 }}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%',
          padding: '14px 12px', background: 'var(--color-surface-container-low)', cursor: 'pointer',
          border: 'none', borderLeft: '2px solid transparent',
        }}
      >
        <span style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
          <InboxBadge tone={group.accent ? 'primary' : 'neutral'}>Group</InboxBadge>
          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 14, fontWeight: 700, letterSpacing: '-0.01em', color: group.collapsed && !open ? 'var(--color-outline)' : 'var(--color-on-surface)' }}>{group.key}</span>
        </span>
        <InboxEyebrow style={{ fontSize: 10 }}>{open ? 'Collapse' : 'Expand'} [Space]</InboxEyebrow>
      </button>
      {open && (
        <div style={{ marginTop: 2 }}>
          {group.prs.map((pr, i) => (
            <PRRow key={pr.number} {...pr} selected={selected === baseIndex + i}
              onClick={() => { onSelect(baseIndex + i); onOpen(pr); }} />
          ))}
        </div>
      )}
    </div>
  );
}

function InboxScreen({ onOpenPR }) {
  const data = window.BistroData;
  const [filter, setFilter] = React.useState('open');
  const [selected, setSelected] = React.useState(0);
  let cursor = 0;
  return (
    <div style={{ height: '100%', overflowY: 'auto', background: 'var(--color-surface)' }}>
      <div style={{ maxWidth: 1600, margin: '0 auto', padding: '40px 48px 64px' }}>
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 4 }}>
          <div style={{ fontFamily: 'var(--font-sans)', fontSize: 18 }}>
            <span style={{ color: 'var(--color-outline)' }}>bistro-ui</span>
            <span style={{ color: 'var(--color-outline-variant)', margin: '0 8px' }}>/</span>
            <span style={{ color: 'var(--color-on-surface)', fontWeight: 700 }}>web-client</span>
          </div>
          <div style={{ border: '1px solid var(--color-outline-variant)', padding: '10px 16px', background: 'var(--color-surface-container-lowest)' }}>
            <div style={{ display: 'flex', gap: 8 }}>
              <InboxEyebrow style={{ fontSize: 11 }}>Filter:</InboxEyebrow>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, fontWeight: 700, color: 'var(--color-on-surface)', letterSpacing: '0.06em' }}>IS:OPEN AUTHOR:ME</span>
            </div>
            <div style={{ display: 'flex', gap: 8, marginTop: 6 }}>
              <InboxEyebrow style={{ fontSize: 11 }}>Sort:</InboxEyebrow>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--color-on-surface-variant)', letterSpacing: '0.06em' }}>NEWEST_FIRST</span>
            </div>
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderBottom: '1px solid var(--color-outline-variant)', paddingBottom: 14, marginBottom: 8 }}>
          <h1 style={{ margin: 0, fontFamily: 'var(--font-mono)', fontSize: 22, letterSpacing: '0.18em', textTransform: 'uppercase', color: 'var(--color-outline)', fontWeight: 400 }}>Open Pull Requests</h1>
          <span style={{ background: 'var(--color-primary)', color: 'var(--color-on-primary)', fontFamily: 'var(--font-mono)', fontSize: 12, padding: '8px 14px', letterSpacing: '0.08em' }}>CMD + K</span>
        </div>
        <div style={{ marginBottom: 20 }}>
          <SegmentedControl value={filter} onChange={setFilter} options={[
            { value: 'needs', label: 'Needs Action', count: 2 },
            { value: 'open', label: 'Open', count: 6 },
            { value: 'draft', label: 'Draft', count: 1 },
            { value: 'all', label: 'All' },
          ]} />
        </div>
        {data.repos.map((g) => {
          const base = cursor; cursor += g.prs.length;
          return <RepoGroup key={g.key} group={g} baseIndex={base} selected={selected} onSelect={setSelected} onOpen={onOpenPR} />;
        })}
      </div>
    </div>
  );
}

window.InboxScreen = InboxScreen;
