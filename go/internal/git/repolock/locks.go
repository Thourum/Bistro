package repolock

import (
	"os"
	"sync"

	"github.com/itterno/bistro/internal/git/remote"
)

// Locks provides a mutex per repo identity plus one global
// cache-maintenance lock that excludes ALL per-repo operations.
// ponytail: RWMutex-over-map; fine for a desktop app's repo counts.
type Locks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	maint sync.RWMutex // per-repo ops take RLock; clear-all takes Lock
}

// New creates a lock manager for repo operations and cache maintenance.
func New() *Locks {
	return &Locks{
		locks: make(map[string]*sync.Mutex),
	}
}

// WithRepo runs fn holding the repo's mutex AND maint.RLock.
func (l *Locks) WithRepo(repoID string, fn func() error) error {
	l.maint.RLock()
	defer l.maint.RUnlock()

	repoLock := l.lockForRepo(repoID)
	repoLock.Lock()
	defer repoLock.Unlock()

	return fn()
}

// WithMaintenance runs fn holding the exclusive maintenance lock.
func (l *Locks) WithMaintenance(fn func() error) error {
	l.maint.Lock()
	defer l.maint.Unlock()

	return fn()
}

func (l *Locks) lockForRepo(repoID string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()

	repoLock, ok := l.locks[repoID]
	if ok {
		return repoLock
	}

	repoLock = &sync.Mutex{}
	l.locks[repoID] = repoLock
	return repoLock
}

// Key returns the normalized repository identity, or the raw URL for overrides and invalid remotes.
func Key(repoURL string) string {
	if os.Getenv("BISTRO_REPO_DIR") != "" {
		return repoURL
	}
	identity, err := remote.ParseURL(repoURL)
	if err != nil {
		return repoURL
	}
	return identity.ID()
}
