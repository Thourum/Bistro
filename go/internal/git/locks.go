package git

import "sync"

// RepoLocks provides a mutex per repo identity plus one global
// cache-maintenance lock that excludes ALL per-repo operations.
// ponytail: RWMutex-over-map; fine for a desktop app's repo counts.
type RepoLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	maint sync.RWMutex // per-repo ops take RLock; clear-all takes Lock
}

func NewRepoLocks() *RepoLocks {
	return &RepoLocks{
		locks: make(map[string]*sync.Mutex),
	}
}

// WithRepo runs fn holding the repo's mutex AND maint.RLock.
func (l *RepoLocks) WithRepo(repoID string, fn func() error) error {
	l.maint.RLock()
	defer l.maint.RUnlock()

	repoLock := l.lockForRepo(repoID)
	repoLock.Lock()
	defer repoLock.Unlock()

	return fn()
}

// WithMaintenance runs fn holding the exclusive maintenance lock.
func (l *RepoLocks) WithMaintenance(fn func() error) error {
	l.maint.Lock()
	defer l.maint.Unlock()

	return fn()
}

func (l *RepoLocks) lockForRepo(repoID string) *sync.Mutex {
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
