package server

import (
	"context"
	"log/slog"

	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/store/repolocation"
)

type repoLocationResolver struct {
	locations *repolocation.Store
	log       *slog.Logger
}

func newRepoLocationResolver(locations *repolocation.Store, log *slog.Logger) *repoLocationResolver {
	return &repoLocationResolver{locations: locations, log: log}
}

// LocalPath returns a local checkout only for an exact, valid remote identity.
func (r *repoLocationResolver) LocalPath(repoURL string) (string, bool) {
	if r == nil || r.locations == nil {
		return "", false
	}
	identity, err := remote.ParseURL(repoURL)
	if err != nil {
		r.logDebug("repo location resolver ignored invalid url", "repo_url", repoURL, "error", err)
		return "", false
	}
	location, err := r.locations.Get(context.Background(), identity.ID())
	if err != nil {
		r.logWarn("repo location resolver get local failed", "repo_id", identity.ID(), "error", err)
		return "", false
	}
	if location == nil || location.Source != "local" || location.LocalPath == "" {
		return "", false
	}
	return location.LocalPath, true
}

// OnLocalDegraded records a local checkout's degraded status when its remote is valid.
func (r *repoLocationResolver) OnLocalDegraded(repoURL, status, code, msg string) {
	if r == nil || r.locations == nil {
		return
	}
	identity, err := remote.ParseURL(repoURL)
	if err != nil {
		r.logWarn("repo location resolver ignored degraded invalid url", "repo_url", repoURL, "error", err)
		return
	}
	if err := r.locations.MarkDegraded(context.Background(), identity.ID(), status, code, msg); err != nil {
		r.logWarn("repo location resolver mark degraded failed", "repo_id", identity.ID(), "error", err)
	}
}

// OnUsed records when the remote's checkout was most recently used.
func (r *repoLocationResolver) OnUsed(repoURL string) {
	if r == nil || r.locations == nil {
		return
	}
	identity, err := remote.ParseURL(repoURL)
	if err != nil {
		r.logDebug("repo location resolver ignored used invalid url", "repo_url", repoURL, "error", err)
		return
	}
	if err := r.locations.Touch(context.Background(), identity.ID()); err != nil {
		r.logWarn("repo location resolver touch failed", "repo_id", identity.ID(), "error", err)
	}
}

// OnCacheCloned stores the newly cloned cache path for a valid remote.
func (r *repoLocationResolver) OnCacheCloned(repoURL, path string) {
	if r == nil || r.locations == nil {
		return
	}
	identity, err := remote.ParseURL(repoURL)
	if err != nil {
		r.logDebug("repo location resolver ignored cache clone invalid url", "repo_url", repoURL, "error", err)
		return
	}
	if err := r.locations.SetCache(context.Background(), identity.ID(), path); err != nil {
		r.logWarn("repo location resolver set cache failed", "repo_id", identity.ID(), "error", err)
	}
}

func (r *repoLocationResolver) logDebug(msg string, args ...any) {
	if r != nil && r.log != nil {
		r.log.Debug(msg, args...)
	}
}

func (r *repoLocationResolver) logWarn(msg string, args ...any) {
	if r != nil && r.log != nil {
		r.log.Warn(msg, args...)
	}
}
