package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const localFetchRemoteURL = "https://github.com/acme/api.git"

func TestFetchIntoLocal_WritesOnlyBistroRefs(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	fixture.setUpstreamPRRef(t, 123, headSHA)

	client := NewClient(NewCache())

	// Act
	err := client.FetchIntoLocal(context.Background(), fixture.userPath, fixture.identity, fixture.baseSHA.String(), headSHA.String(), nil)

	// Assert
	if err != nil {
		t.Fatalf("FetchIntoLocal: %v", err)
	}
	userRepo := fixture.openUserRepo(t)
	if _, err := userRepo.CommitObject(headSHA); err != nil {
		t.Fatalf("expected fetched commit to resolve locally: %v", err)
	}

	refs := refsByHash(t, userRepo, headSHA)
	if len(refs) == 0 {
		t.Fatal("expected at least one ref for fetched commit")
	}
	for _, refName := range refs {
		if !strings.HasPrefix(refName, "refs/bistro/") {
			t.Fatalf("fetched commit ref %q is outside refs/bistro/: %v", refName, refs)
		}
	}
}

func TestFetchIntoLocal_NeverTouchesUserState(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	fixture.setUpstreamPRRef(t, 123, headSHA)
	before := captureUserState(t, fixture.userPath)

	client := NewClient(NewCache())

	// Act
	err := client.FetchIntoLocal(context.Background(), fixture.userPath, fixture.identity, fixture.baseSHA.String(), headSHA.String(), nil)

	// Assert
	if err != nil {
		t.Fatalf("FetchIntoLocal: %v", err)
	}
	after := captureUserState(t, fixture.userPath)
	if before.headTarget != after.headTarget {
		t.Fatalf("HEAD changed: before=%q after=%q", before.headTarget, after.headTarget)
	}
	assertStringSlicesEqual(t, "refs/heads", before.headRefs, after.headRefs)
	assertStringSlicesEqual(t, "refs/tags", before.tagRefs, after.tagRefs)
	assertStringSlicesEqual(t, "refs/remotes", before.remoteRefs, after.remoteRefs)
	if !bytes.Equal(before.indexBytes, after.indexBytes) {
		t.Fatal("index bytes changed")
	}
	if !equalStringMaps(before.worktreeHashes, after.worktreeHashes) {
		t.Fatalf("worktree hashes changed: before=%v after=%v", before.worktreeHashes, after.worktreeHashes)
	}
}

func TestFetchIntoLocal_RemoteMismatch(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	fixture.replaceOriginURLs(t, "file://"+fixture.upstreamPath, "https://github.com/other/repo.git")
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	client := NewClient(NewCache())

	// Act
	err := client.FetchIntoLocal(context.Background(), fixture.userPath, fixture.identity, fixture.baseSHA.String(), headSHA.String(), nil)

	// Assert
	if err == nil {
		t.Fatal("expected remote_mismatch error")
	}
	var fetchErr *LocalFetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected LocalFetchError, got %T: %v", err, err)
	}
	if fetchErr.Code != "remote_mismatch" {
		t.Fatalf("expected remote_mismatch, got %q", fetchErr.Code)
	}
}

func TestFetchIntoLocal_SkipsWhenPresent(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	client := NewClient(NewCache())
	beforeRefs := allRefNames(t, fixture.openUserRepo(t))

	// Act
	err := client.FetchIntoLocal(context.Background(), fixture.userPath, fixture.identity, fixture.baseSHA.String(), fixture.baseSHA.String(), nil)

	// Assert
	if err != nil {
		t.Fatalf("FetchIntoLocal: %v", err)
	}
	afterRefs := allRefNames(t, fixture.openUserRepo(t))
	assertStringSlicesEqual(t, "refs", beforeRefs, afterRefs)
}

func TestFetchIntoLocal_TargetedRefspecsOnly(t *testing.T) {
	// Arrange
	fixture := newLocalFetchFixture(t)
	const prNumber = 123
	headSHA := fixture.commitUpstream(t, "feature.txt", "feature\n", "feature")
	fixture.setUpstreamPRRef(t, prNumber, headSHA)
	unrelatedSHA := fixture.commitUpstreamBranch(t, "unrelated", "unrelated.txt", "unrelated\n", "unrelated")

	client := NewClient(NewCache())

	// Act
	err := client.FetchIntoLocal(context.Background(), fixture.userPath, fixture.identity, fixture.baseSHA.String(), headSHA.String(), &CloneOptions{
		PRNumber:   prNumber,
		BaseBranch: fixture.baseBranch,
		Depth:      50,
	})

	// Assert
	if err != nil {
		t.Fatalf("FetchIntoLocal: %v", err)
	}
	userRepo := fixture.openUserRepo(t)
	safeID := SafeRepoID(fixture.identity)
	expectedRefs := []string{
		fmt.Sprintf("refs/bistro/%s/base/%s", safeID, fixture.baseBranch),
		fmt.Sprintf("refs/bistro/%s/pull/%d/head", safeID, prNumber),
	}
	for _, refName := range expectedRefs {
		if _, err := userRepo.Reference(plumbing.ReferenceName(refName), true); err != nil {
			t.Fatalf("expected targeted ref %s: %v", refName, err)
		}
	}
	for _, refName := range refsByHash(t, userRepo, unrelatedSHA) {
		if strings.HasPrefix(refName, "refs/bistro/"+safeID+"/") {
			t.Fatalf("unrelated branch landed under bistro refs: %s", refName)
		}
	}
}

type localFetchFixture struct {
	upstreamPath string
	userPath     string
	identity     Identity
	baseBranch   string
	baseSHA      plumbing.Hash
}

func newLocalFetchFixture(t *testing.T) localFetchFixture {
	t.Helper()
	root := t.TempDir()
	upstreamPath := filepath.Join(root, "upstream")
	userPath := filepath.Join(root, "user")

	upstream, err := gitlib.PlainInit(upstreamPath, false)
	if err != nil {
		t.Fatalf("PlainInit upstream: %v", err)
	}
	baseSHA := commitFile(t, upstream, upstreamPath, "README.md", "base\n", "base")
	head, err := upstream.Head()
	if err != nil {
		t.Fatalf("upstream Head: %v", err)
	}

	_, err = gitlib.PlainClone(userPath, false, &gitlib.CloneOptions{URL: "file://" + upstreamPath})
	if err != nil {
		t.Fatalf("PlainClone user: %v", err)
	}

	identity, err := ParseRemoteURL(localFetchRemoteURL)
	if err != nil {
		t.Fatalf("ParseRemoteURL: %v", err)
	}
	fixture := localFetchFixture{
		upstreamPath: upstreamPath,
		userPath:     userPath,
		identity:     identity,
		baseBranch:   head.Name().Short(),
		baseSHA:      baseSHA,
	}
	fixture.replaceOriginURLs(t, "file://"+upstreamPath, localFetchRemoteURL)
	return fixture
}

func (f localFetchFixture) openUpstreamRepo(t *testing.T) *gitlib.Repository {
	t.Helper()
	repo, err := gitlib.PlainOpen(f.upstreamPath)
	if err != nil {
		t.Fatalf("PlainOpen upstream: %v", err)
	}
	return repo
}

func (f localFetchFixture) openUserRepo(t *testing.T) *gitlib.Repository {
	t.Helper()
	repo, err := gitlib.PlainOpen(f.userPath)
	if err != nil {
		t.Fatalf("PlainOpen user: %v", err)
	}
	return repo
}

func (f localFetchFixture) replaceOriginURLs(t *testing.T, urls ...string) {
	t.Helper()
	repo := f.openUserRepo(t)
	if err := repo.DeleteRemote("origin"); err != nil && err != gitlib.ErrRemoteNotFound {
		t.Fatalf("DeleteRemote origin: %v", err)
	}
	_, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: urls})
	if err != nil {
		t.Fatalf("CreateRemote origin: %v", err)
	}
}

func (f localFetchFixture) commitUpstream(t *testing.T, fileName, content, message string) plumbing.Hash {
	t.Helper()
	return commitFile(t, f.openUpstreamRepo(t), f.upstreamPath, fileName, content, message)
}

func (f localFetchFixture) commitUpstreamBranch(t *testing.T, branchName, fileName, content, message string) plumbing.Hash {
	t.Helper()
	repo := f.openUpstreamRepo(t)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), f.baseSHA)); err != nil {
		t.Fatalf("SetReference branch: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree upstream: %v", err)
	}
	if err := worktree.Checkout(&gitlib.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(branchName)}); err != nil {
		t.Fatalf("Checkout unrelated branch: %v", err)
	}
	return commitFile(t, repo, f.upstreamPath, fileName, content, message)
}

func (f localFetchFixture) setUpstreamPRRef(t *testing.T, prNumber int, hash plumbing.Hash) {
	t.Helper()
	repo := f.openUpstreamRepo(t)
	refName := plumbing.ReferenceName(fmt.Sprintf("refs/pull/%d/head", prNumber))
	if err := repo.Storer.SetReference(plumbing.NewHashReference(refName, hash)); err != nil {
		t.Fatalf("SetReference PR ref: %v", err)
	}
}

func commitFile(t *testing.T, repo *gitlib.Repository, repoPath, fileName, content, message string) plumbing.Hash {
	t.Helper()
	filePath := filepath.Join(repoPath, fileName)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := worktree.Add(fileName); err != nil {
		t.Fatalf("Add: %v", err)
	}
	hash, err := worktree.Commit(message, &gitlib.CommitOptions{Author: &object.Signature{
		Name:  "Bistro Test",
		Email: "bistro@example.com",
		When:  time.Now(),
	}})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return hash
}

type capturedUserState struct {
	headTarget     string
	headRefs       []string
	tagRefs        []string
	remoteRefs     []string
	indexBytes     []byte
	worktreeHashes map[string]string
}

func captureUserState(t *testing.T, repoPath string) capturedUserState {
	t.Helper()
	repo, err := gitlib.PlainOpen(repoPath)
	if err != nil {
		t.Fatalf("PlainOpen capture: %v", err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head capture: %v", err)
	}
	indexBytes, err := os.ReadFile(filepath.Join(repoPath, ".git", "index"))
	if err != nil {
		t.Fatalf("ReadFile index: %v", err)
	}
	return capturedUserState{
		headTarget:     head.Target().String(),
		headRefs:       refsWithPrefix(t, repo, "refs/heads/"),
		tagRefs:        refsWithPrefix(t, repo, "refs/tags/"),
		remoteRefs:     refsWithPrefix(t, repo, "refs/remotes/"),
		indexBytes:     append([]byte(nil), indexBytes...),
		worktreeHashes: hashWorktreeFiles(t, repoPath),
	}
}

func refsByHash(t *testing.T, repo *gitlib.Repository, hash plumbing.Hash) []string {
	t.Helper()
	var names []string
	iter, err := repo.References()
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	defer iter.Close()
	if err := iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Hash() == hash {
			names = append(names, ref.Name().String())
		}
		return nil
	}); err != nil {
		t.Fatalf("iterate refs: %v", err)
	}
	sort.Strings(names)
	return names
}

func refsWithPrefix(t *testing.T, repo *gitlib.Repository, prefix string) []string {
	t.Helper()
	refs := allRefNames(t, repo)
	prefixed := make([]string, 0, len(refs))
	for _, ref := range refs {
		if strings.HasPrefix(ref, prefix) {
			prefixed = append(prefixed, ref)
		}
	}
	return prefixed
}

func allRefNames(t *testing.T, repo *gitlib.Repository) []string {
	t.Helper()
	var names []string
	iter, err := repo.References()
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	defer iter.Close()
	if err := iter.ForEach(func(ref *plumbing.Reference) error {
		names = append(names, ref.Name().String())
		return nil
	}); err != nil {
		t.Fatalf("iterate refs: %v", err)
	}
	sort.Strings(names)
	return names
}

func hashWorktreeFiles(t *testing.T, repoPath string) map[string]string {
	t.Helper()
	hashes := map[string]string{}
	if err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repoPath, path)
		if err != nil {
			return err
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) || d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		hashes[rel] = fmt.Sprintf("%x", sum[:])
		return nil
	}); err != nil {
		t.Fatalf("WalkDir worktree: %v", err)
	}
	return hashes
}

func assertStringSlicesEqual(t *testing.T, label string, want, got []string) {
	t.Helper()
	if strings.Join(want, "\x00") != strings.Join(got, "\x00") {
		t.Fatalf("%s mismatch:\nwant=%v\ngot =%v", label, want, got)
	}
}

func equalStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}
