package classify

import (
	"log/slog"
	"sync"
)

// EngineProvider hands out a current *Engine, reloading the rules directory
// on demand. Current() re-runs LoadDir(dir, mode()) — a handful of small YAML
// files, microseconds — on each call and reuses the cached engine when the
// fingerprint is unchanged. This is the spec's lazy model ("cold until the
// user looks"): no fsnotify, no watcher, no timer. Because the classification
// mode participates in the fingerprint, a mode change invalidates like a rule
// edit. Safe for concurrent use.
type EngineProvider struct {
	dir  string
	mode func() string
	log  *slog.Logger

	mu     sync.Mutex
	cached *Engine
}

// NewEngineProvider builds a provider over a rules directory. mode returns
// the live classification mode string. An empty dir skips directory loading
// entirely and always serves the embedded built-ins.
func NewEngineProvider(dir string, mode func() string, log *slog.Logger) *EngineProvider {
	if log == nil {
		log = slog.Default()
	}
	if mode == nil {
		mode = func() string { return "rules" }
	}
	return &EngineProvider{dir: dir, mode: mode, log: log}
}

// Current returns an engine for the directory's present contents, reusing
// the cached engine when nothing changed. When the directory yields no rule
// files (unreadable, empty, or dir == ""), it falls back to the embedded
// built-ins with fingerprint "builtin:"+mode.
func (p *EngineProvider) Current() *Engine {
	mode := p.mode()

	var loaded *Loaded
	if p.dir != "" {
		l, errs := LoadDir(p.dir, mode)
		for _, err := range errs {
			p.log.Warn("rule file skipped", "error", err)
		}
		if len(l.Files) > 0 {
			loaded = l
		}
	}
	if loaded == nil {
		files, err := Builtins()
		if err != nil {
			// Embedded files are compile-time trusted; this is a programmer
			// bug surfaced by tests. Serve an empty engine rather than nil.
			p.log.Error("embedded built-ins failed to parse", "error", err)
			files = nil
		}
		loaded = &Loaded{Files: files, Fingerprint: "builtin:" + mode}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached != nil && p.cached.Fingerprint() == loaded.Fingerprint {
		return p.cached
	}
	p.cached = NewEngine(loaded)
	return p.cached
}
