package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeLocalResolver struct {
	path             string
	hasPath          bool
	localPathCalls   int
	degradedCalls    []fakeDegradedCall
	usedCalls        []string
	cacheClonedCalls []fakeCacheClonedCall
}

type fakeDegradedCall struct {
	repoURL string
	status  string
	code    string
	msg     string
}

type fakeCacheClonedCall struct {
	repoURL string
	path    string
}

func (r *fakeLocalResolver) LocalPath(repoURL string) (string, bool) {
	r.localPathCalls++
	return r.path, r.hasPath
}

func (r *fakeLocalResolver) OnLocalDegraded(repoURL, status, code, msg string) {
	r.degradedCalls = append(r.degradedCalls, fakeDegradedCall{
		repoURL: repoURL,
		status:  status,
		code:    code,
		msg:     msg,
	})
}

func (r *fakeLocalResolver) OnUsed(repoURL string) {
	r.usedCalls = append(r.usedCalls, repoURL)
}

func (r *fakeLocalResolver) OnCacheCloned(repoURL, path string) {
	r.cacheClonedCalls = append(r.cacheClonedCalls, fakeCacheClonedCall{repoURL: repoURL, path: path})
}

func TestCloneLocalPathExistsUsesRegisteredRepo(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	resolver := &fakeLocalResolver{path: fixture.userPath, hasPath: true}
	client := NewClient(NewCache())
	client.SetLocalResolver(resolver)

	// Act
	result, err := client.Clone(context.Background(), localFetchRemoteURL, fixture.baseSHA.String(), headSHA.String())

	// Assert
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if result.LocalPath != fixture.userPath {
		t.Fatalf("expected local path %q, got %q", fixture.userPath, result.LocalPath)
	}
	if len(resolver.usedCalls) != 1 || resolver.usedCalls[0] != localFetchRemoteURL {
		t.Fatalf("expected OnUsed once for repo URL, got %#v", resolver.usedCalls)
	}
	if len(resolver.cacheClonedCalls) != 0 {
		t.Fatalf("expected no cache clone callback, got %#v", resolver.cacheClonedCalls)
	}
	assertCacheRootUntouched(t, cacheRoot)
}

func TestCloneLocalPathMissingReturnsLocationError(t *testing.T) {
	// Arrange
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	missingPath := filepath.Join(t.TempDir(), "missing")
	resolver := &fakeLocalResolver{path: missingPath, hasPath: true}
	client := NewClient(NewCache())
	client.SetLocalResolver(resolver)

	// Act
	result, err := client.Clone(context.Background(), localFetchRemoteURL, "base", "head")

	// Assert
	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	var locationErr *LocationError
	if !errors.As(err, &locationErr) {
		t.Fatalf("expected LocationError, got %T: %v", err, err)
	}
	if locationErr.Code != "path_missing" {
		t.Fatalf("expected path_missing, got %q", locationErr.Code)
	}
	if len(resolver.degradedCalls) != 1 {
		t.Fatalf("expected one degraded callback, got %#v", resolver.degradedCalls)
	}
	degraded := resolver.degradedCalls[0]
	if degraded.repoURL != localFetchRemoteURL || degraded.status != "missing" || degraded.code != "path_missing" {
		t.Fatalf("unexpected degraded callback: %#v", degraded)
	}
	assertCacheRootUntouched(t, cacheRoot)
}

func TestCloneLocalRegistrationWinsOverWarmCache(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	cache := NewCache()
	cachedDir := t.TempDir()
	cache.Set(localFetchRemoteURL, fixture.baseSHA.String(), headSHA.String(), &RepoEntry{
		URL:       localFetchRemoteURL,
		LocalPath: cachedDir,
		BaseSHA:   fixture.baseSHA.String(),
		HeadSHA:   headSHA.String(),
	})
	resolver := &fakeLocalResolver{path: fixture.userPath, hasPath: true}
	client := NewClient(cache)
	client.SetLocalResolver(resolver)

	// Act
	result, err := client.Clone(context.Background(), localFetchRemoteURL, fixture.baseSHA.String(), headSHA.String())

	// Assert
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if result.LocalPath != fixture.userPath {
		t.Fatalf("expected local path %q, got %q", fixture.userPath, result.LocalPath)
	}
	if len(resolver.usedCalls) != 1 {
		t.Fatalf("expected OnUsed once, got %#v", resolver.usedCalls)
	}
}

func TestCloneLocalBrokenRegistrationDoesNotFallBackToWarmCache(t *testing.T) {
	// Arrange
	cache := NewCache()
	cachedDir := t.TempDir()
	cache.Set(localFetchRemoteURL, "base", "head", &RepoEntry{
		URL:       localFetchRemoteURL,
		LocalPath: cachedDir,
		BaseSHA:   "base",
		HeadSHA:   "head",
	})
	missingPath := filepath.Join(t.TempDir(), "missing")
	resolver := &fakeLocalResolver{path: missingPath, hasPath: true}
	client := NewClient(cache)
	client.SetLocalResolver(resolver)

	// Act
	result, err := client.Clone(context.Background(), localFetchRemoteURL, "base", "head")

	// Assert
	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	var locationErr *LocationError
	if !errors.As(err, &locationErr) {
		t.Fatalf("expected LocationError, got %T: %v", err, err)
	}
	if locationErr.Code != "path_missing" {
		t.Fatalf("expected path_missing, got %q", locationErr.Code)
	}
	if len(resolver.usedCalls) != 0 {
		t.Fatalf("expected no OnUsed calls, got %#v", resolver.usedCalls)
	}
}

func TestCloneLocalBistroRepoDirSkipsResolver(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	fixture.setUpstreamPRRef(t, 7, headSHA)
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)
	resolver := &fakeLocalResolver{path: fixture.userPath, hasPath: true}
	client := NewClient(NewCache())
	client.SetLocalResolver(resolver)

	// Act
	result, err := client.CloneForPR(context.Background(), "file://"+fixture.upstreamPath, fixture.baseSHA.String(), headSHA.String(), &CloneOptions{PRNumber: 7, BaseBranch: fixture.baseBranch})

	// Assert
	if err != nil {
		t.Fatalf("CloneForPR: %v", err)
	}
	if result.LocalPath == fixture.userPath {
		t.Fatalf("expected env override clone path, got local registered path %q", result.LocalPath)
	}
	if resolver.localPathCalls != 0 {
		t.Fatalf("expected resolver not consulted, got %d calls", resolver.localPathCalls)
	}
}

func TestCloneLocalCacheCloneRecordsCachePathAndHit(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	fixture.setUpstreamPRRef(t, 7, headSHA)
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	resolver := &fakeLocalResolver{}
	client := NewClient(NewCache())
	client.SetLocalResolver(resolver)

	// Act
	result, err := client.CloneForPR(context.Background(), "file://"+fixture.upstreamPath, fixture.baseSHA.String(), headSHA.String(), &CloneOptions{PRNumber: 7, BaseBranch: fixture.baseBranch})
	if err != nil {
		t.Fatalf("first CloneForPR: %v", err)
	}
	secondResult, err := client.CloneForPR(context.Background(), "file://"+fixture.upstreamPath, fixture.baseSHA.String(), headSHA.String(), &CloneOptions{PRNumber: 7, BaseBranch: fixture.baseBranch})
	if err != nil {
		t.Fatalf("second CloneForPR: %v", err)
	}

	// Assert
	if result.LocalPath != secondResult.LocalPath {
		t.Fatalf("expected cache hit path %q, got %q", result.LocalPath, secondResult.LocalPath)
	}
	if len(resolver.cacheClonedCalls) != 2 {
		t.Fatalf("expected cache cloned callback for clone and hit, got %#v", resolver.cacheClonedCalls)
	}
	for _, call := range resolver.cacheClonedCalls {
		if call.repoURL != "file://"+fixture.upstreamPath {
			t.Fatalf("unexpected repo URL in cache callback: %#v", call)
		}
		if call.path != result.LocalPath {
			t.Fatalf("expected cache callback path %q, got %q", result.LocalPath, call.path)
		}
	}
}

func assertCacheRootUntouched(t *testing.T, cacheRoot string) {
	t.Helper()
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		t.Fatalf("ReadDir cache root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected cache root untouched, found %d entries", len(entries))
	}
}
