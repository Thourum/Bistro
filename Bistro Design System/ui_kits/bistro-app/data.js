// Bistro UI kit — shared mock data. Assigned to window for cross-script use.
window.BistroData = {
  repos: [
    { key: 'core-engine / production', accent: true, prs: [
      { number: 1042, title: 'feat: implement zero-radius architecture for canvas nodes', author: 'alice', time: '2m ago', ci: 'success' },
      { number: 1041, title: 'fix: memory leak in viewport observer cleanup', author: 'bob_dev', time: '15m ago', ci: 'failure' },
      { number: 1039, title: 'refactor: migrate tailwind config to typescript', author: 'charlie', time: '1h ago', ci: 'running' },
    ]},
    { key: 'internal-tools / registry', accent: false, prs: [
      { number: 982, title: 'chore: update dependency graph for v2.4.0', author: 'system', time: '4h ago', ci: 'success' },
    ]},
    { key: 'legacy / monolith-v1', accent: false, collapsed: true, prs: [
      { number: 871, title: 'fix: patch CVE-2023-9812 context leakage', author: 'sarah_dev', time: '1d ago', ci: 'success' },
      { number: 864, title: 'docs: archive deprecated BufferManager API', author: 'alex_mercer', time: '2d ago', ci: 'unknown' },
    ]},
  ],
  // The PR opened in overview / diff
  pr: {
    number: 2048,
    title: 'refactor: core/engine logic for multi-tenant isolation',
    author: 'alex_mercer',
    date: '2023-10-24 14:02:11',
    source: 'feat/isolation-v2',
    target: 'main',
    files: 12,
    added: 842,
    removed: 156,
    description: "This PR implements the structural changes required to support hard isolation between tenants at the engine layer. It removes the shared memory buffer and introduces a keyed proxy pattern for all state access.",
    impact: [
      'Decreases cold-boot latency by 14%',
      'Fixes CVE-2023-9812 context leakage',
      'Deprecates old `BufferManager` instance',
    ],
    comments: [
      { author: 'sarah_dev', time: '2h ago', body: "Should we consider adding a fallback for legacy tenants that haven't migrated to the new key structure? I'm worried about breaking the SDK.", actions: true },
      { author: 'system_ci', time: '5h ago', body: 'Unit tests passed. Integration tests pending for pkg/driver/v2.', actions: false },
    ],
    checks: 'running',
  },
  diff: {
    file: 'src/core/monolith_engine.rs',
    lang: 'RUST',
    added: 24,
    removed: 12,
    files: [
      { path: 'src/core/monolith_engine.rs', active: true },
      { path: 'src/ui/layout_grid.tsx' },
      { path: 'config/system_params.yaml' },
      { path: 'tests/unit/protocol_tests.rs' },
      { path: 'scripts/deploy_monolith.sh' },
    ],
    lines: [
      { o: 41, n: 41, t: 'context', c: 'pub fn initialize_kernel(&mut self) -> Result<(), EngineError> {' },
      { o: 42, n: 42, t: 'context', c: '    log::info!("Starting Monolith Kernel...");' },
      { n: 43, t: 'remove', c: '    self.status = EngineStatus::Initializing;' },
      { o: 43, t: 'add', c: '    self.set_state(EngineStatus::Initializing)?;' },
      { o: 44, t: 'add', focused: true, c: '    self.verify_integrity_hash()?;' },
      { o: 45, n: 45, t: 'context', c: '    self.runtime.spawn(async move {' },
      { o: 46, n: 46, t: 'context', c: '        match self.execute_bootstrap().await {' },
      { o: 47, t: 'add', c: '            Ok(_) => log::debug!("Bootstrap sequence verified."),' },
      { o: 48, t: 'add', c: '            Err(e) => {' },
      { o: 49, t: 'add', c: '                log::error!("Critical failure during bootstrap: {:?}", e);' },
      { o: 50, t: 'add', c: '                std::process::exit(1);' },
      { o: 51, t: 'add', c: '            }' },
      { o: 52, n: 52, t: 'context', c: '        }' },
      { o: 53, n: 53, t: 'context', c: '    });' },
    ],
  },
  commands: [
    { id: 'help', icon: 'keyboard', title: 'Show keyboard shortcuts', shortcut: ['?'] },
    { id: 'settings', icon: 'settings', title: 'Open project settings', shortcut: ['⌘', ','] },
    { id: 'sync', icon: 'sync', title: 'Sync now', subtitle: 'Force sync all watched repos' },
    { id: 'clear', icon: 'filter_alt_off', title: 'Clear all filters and search' },
    { id: 'repos', icon: 'source_environment', title: 'Open repository picker', subtitle: 'Discover and watch repositories' },
    { id: 'focus-list', icon: 'list_alt', title: 'Focus PR list' },
    { id: 'about', icon: 'info', title: 'About BISTRO' },
  ],
  shortcuts: {
    Navigation: [['J / K', 'Move down / up'], ['G G', 'Jump to top'], ['Shift+G', 'Jump to bottom'], ['N / P', 'Next / prev file'], ['H J K L', 'Scroll diff']],
    Actions: [['Enter', 'Open PR'], ['A A', 'Approve (double-tap)'], ['R', 'Request changes'], ['C', 'Comment on line'], ['Esc', 'Back / close']],
    'Search & Commands': [[':', 'Command palette'], ['/', 'Search'], ['?', 'Keyboard shortcuts'], ['⌘ ,', 'Settings'], ['Space', 'Toggle fold / view diff']],
  },
};
