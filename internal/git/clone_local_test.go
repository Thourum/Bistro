package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/logging"
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
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	resolver := &fakeLocalResolver{path: fixture.userPath, hasPath: true}
	client := NewClient(clonecache.New())
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
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	missingPath := filepath.Join(t.TempDir(), "missing")
	resolver := &fakeLocalResolver{path: missingPath, hasPath: true}
	client := NewClient(clonecache.New())
	client.SetLocalResolver(resolver)

	// Act
	result, err := client.Clone(context.Background(), localFetchRemoteURL, "base", "head")

	// Assert
	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	var locationErr *remote.LocationError
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
	cache := clonecache.New()
	cachedDir := t.TempDir()
	cache.Set(localFetchRemoteURL, fixture.baseSHA.String(), headSHA.String(), &clonecache.RepoEntry{
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
	cache := clonecache.New()
	cachedDir := t.TempDir()
	cache.Set(localFetchRemoteURL, "base", "head", &clonecache.RepoEntry{
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
	var locationErr *remote.LocationError
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

func TestCloneLocalRemoteMismatchRecordsRemoteMismatchStatus(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	fixture.replaceOriginURLs(t, "file://"+fixture.upstreamPath, "https://github.com/other/repo.git")
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	resolver := &fakeLocalResolver{path: fixture.userPath, hasPath: true}
	client := NewClient(clonecache.New())
	client.SetLocalResolver(resolver)

	// Act
	result, err := client.Clone(context.Background(), localFetchRemoteURL, fixture.baseSHA.String(), headSHA.String())

	// Assert
	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	var locationErr *remote.LocationError
	if !errors.As(err, &locationErr) {
		t.Fatalf("expected LocationError, got %T: %v", err, err)
	}
	if locationErr.Code != "remote_mismatch" {
		t.Fatalf("expected remote_mismatch code, got %q", locationErr.Code)
	}
	if len(resolver.degradedCalls) != 1 {
		t.Fatalf("expected one degraded callback, got %#v", resolver.degradedCalls)
	}
	degraded := resolver.degradedCalls[0]
	if degraded.repoURL != localFetchRemoteURL || degraded.status != "remote_mismatch" || degraded.code != "remote_mismatch" {
		t.Fatalf("unexpected degraded callback: %#v", degraded)
	}
	assertCacheRootUntouched(t, cacheRoot)
}

func TestCloneUsesNormalizedIdentityForRepoLockKey(t *testing.T) {
	// Arrange
	t.Setenv("BISTRO_REPO_DIR", "")
	const httpsURL = "https://github.com/Acme/API.git"
	const sshURL = "git@github.com:acme/api.git"

	// Act
	httpsKey := repolock.Key(httpsURL)
	sshKey := repolock.Key(sshURL)

	// Assert
	if httpsKey != sshKey {
		t.Fatalf("expected equivalent remotes to share lock key, got %q and %q", httpsKey, sshKey)
	}
	if httpsKey != "github:acme/api" {
		t.Fatalf("expected normalized identity lock key, got %q", httpsKey)
	}
}

func TestCloneUsesRawRepoURLForLockKeyWithBistroRepoDir(t *testing.T) {
	// Arrange
	overrideDir := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", overrideDir)
	const repoURL = "git@github.com:acme/api.git"

	// Act
	lockKey := repolock.Key(repoURL)

	// Assert
	if lockKey != repoURL {
		t.Fatalf("expected env override to use raw repo URL lock key, got %q", lockKey)
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
	client := NewClient(clonecache.New())
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
	if len(resolver.cacheClonedCalls) != 0 {
		t.Fatalf("expected env override to skip cache clone callbacks, got %#v", resolver.cacheClonedCalls)
	}
}

func TestCloneLocalCacheCloneRecordsCachePathAndHit(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	fixture.setUpstreamPRRef(t, 7, headSHA)
	cacheRoot := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	resolver := &fakeLocalResolver{}
	client := NewClient(clonecache.New())
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

func TestCloneLocalTargetedFailureDoesNotRemoveExistingCacheClone(t *testing.T) {
	// Arrange
	const url = "https://github.com/acme/api.git"
	markerPath := writeExistingCacheMarker(t, url)
	client := NewClient(clonecache.New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	_, ok, err := client.targetedClone(ctx, logging.FromContext(context.Background()), url, "base", "head", &CloneOptions{PRNumber: 17})

	// Assert
	if err != nil {
		t.Fatalf("targeted clone fallback: %v", err)
	}
	if ok {
		t.Fatal("expected targeted clone failure")
	}
	assertFileExists(t, markerPath, "targeted failure should preserve existing cache marker")
}

func TestCloneLocalBroadFailureDoesNotRemoveExistingCacheClone(t *testing.T) {
	// Arrange
	const url = "https://github.com/acme/api.git"
	markerPath := writeExistingCacheMarker(t, url)
	client := NewClient(clonecache.New())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	result, err := client.Clone(ctx, url, "base", "head")

	// Assert
	if result != nil {
		t.Fatalf("expected nil result, got %#v", result)
	}
	if err == nil {
		t.Fatal("expected broad clone failure")
	}
	assertFileExists(t, markerPath, "broad clone failure should preserve existing cache marker")
}

func writeExistingCacheMarker(t *testing.T, repoURL string) string {
	t.Helper()
	t.Setenv("BISTRO_REPO_DIR", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cacheDir, err := clonecache.RepoDir(repoURL)
	if err != nil {
		t.Fatalf("repoCacheDir: %v", err)
	}
	markerPath := filepath.Join(cacheDir, "keep.txt")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("MkdirAll cacheDir: %v", err)
	}
	if err := os.WriteFile(markerPath, []byte("existing cache"), 0644); err != nil {
		t.Fatalf("WriteFile marker: %v", err)
	}
	return markerPath
}

func assertFileExists(t *testing.T, path, context string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s: %v", context, err)
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
