package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/plumbing/transport/file"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/diff"
)

// raceFixturePR is one PR on a linear local remote: its base and head commits.
type raceFixturePR struct {
	number  int
	baseSHA string
	headSHA string
}

// fixtureRemote is a local repository the tests commit into and publish PR refs from.
type fixtureRemote struct {
	t    *testing.T
	dir  string
	repo *gogit.Repository
	wt   *gogit.Worktree
}

func newFixtureRemote(t *testing.T, dir string) *fixtureRemote {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	return &fixtureRemote{t: t, dir: dir, repo: repo, wt: wt}
}

func (f *fixtureRemote) commit(content string) plumbing.Hash {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, "foo.go"), []byte(content), 0644); err != nil {
		f.t.Fatalf("WriteFile: %v", err)
	}
	if _, err := f.wt.Add("foo.go"); err != nil {
		f.t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}
	h, err := f.wt.Commit(content, &gogit.CommitOptions{Author: sig})
	if err != nil {
		f.t.Fatalf("Commit: %v", err)
	}
	return h
}

func (f *fixtureRemote) checkout(branch plumbing.ReferenceName, create bool) {
	f.t.Helper()
	if err := f.wt.Checkout(&gogit.CheckoutOptions{Branch: branch, Create: create}); err != nil {
		f.t.Fatalf("Checkout %s: %v", branch, err)
	}
}

func (f *fixtureRemote) publishPR(number int, head plumbing.Hash) {
	f.t.Helper()
	ref := plumbing.NewHashReference(plumbing.ReferenceName(fmt.Sprintf("refs/pull/%d/head", number)), head)
	if err := f.repo.Storer.SetReference(ref); err != nil {
		f.t.Fatalf("SetReference: %v", err)
	}
}

// buildLinearRemoteWithPRs commits c1..c4 on the default branch and publishes
// c2 and c4 as refs/pull/1/head and refs/pull/2/head. PR1 is c1..c2 and PR2 is c3..c4.
func buildLinearRemoteWithPRs(t *testing.T, dir string) []raceFixturePR {
	t.Helper()
	remote := newFixtureRemote(t, dir)
	c1, c2 := remote.commit("v1\n"), remote.commit("v2\n")
	c3, c4 := remote.commit("v3\n"), remote.commit("v4\n")
	remote.publishPR(1, c2)
	remote.publishPR(2, c4)
	return []raceFixturePR{
		{number: 1, baseSHA: c1.String(), headSHA: c2.String()},
		{number: 2, baseSHA: c3.String(), headSHA: c4.String()},
	}
}

// buildDivergentRemoteWithPRs commits c1 on the default branch and gives each
// PR a head commit on its own side branch. A clone for one PR then lacks the
// other PR's head, so alternating PRs replaces the shared cache directory.
func buildDivergentRemoteWithPRs(t *testing.T, dir string) []raceFixturePR {
	t.Helper()
	remote := newFixtureRemote(t, dir)
	c1 := remote.commit("v1\n")
	head, err := remote.repo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	prs := make([]raceFixturePR, 0, 2)
	for number := 1; number <= 2; number++ {
		remote.checkout(plumbing.NewBranchReferenceName(fmt.Sprintf("pr-%d", number)), true)
		prHead := remote.commit(fmt.Sprintf("pr%d\n", number))
		remote.publishPR(number, prHead)
		prs = append(prs, raceFixturePR{number: number, baseSHA: c1.String(), headSHA: prHead.String()})
		remote.checkout(head.Name(), false)
	}
	return prs
}

// localHTTPSTransport serves an https remote from a local repository so the
// production clone path (which only accepts github.com URLs) runs offline.
// It counts upload-pack sessions, one per clone or fetch the client performs.
type localHTTPSTransport struct {
	endpoint *transport.Endpoint
	sessions *atomic.Int32
}

func (l localHTTPSTransport) NewUploadPackSession(_ *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	l.sessions.Add(1)
	return file.DefaultClient.NewUploadPackSession(l.endpoint, auth)
}

func (l localHTTPSTransport) NewReceivePackSession(_ *transport.Endpoint, auth transport.AuthMethod) (transport.ReceivePackSession, error) {
	return file.DefaultClient.NewReceivePackSession(l.endpoint, auth)
}

// runConcurrentPRDiffs diffs every PR from its own goroutine, several rounds
// each, through one client sharing a repo cache. It returns the number of
// fetches the remote served.
func runConcurrentPRDiffs(t *testing.T, buildRemote func(*testing.T, string) []raceFixturePR) int32 {
	t.Helper()
	remoteDir := t.TempDir()
	prs := buildRemote(t, remoteDir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	endpoint, err := transport.NewEndpoint(remoteDir)
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}
	var sessions atomic.Int32
	client.InstallProtocol("https", localHTTPSTransport{endpoint: endpoint, sessions: &sessions})
	t.Cleanup(func() { client.InstallProtocol("https", githttp.DefaultClient) })

	const repoURL = "https://github.com/bistro-test/race-repo.git"
	gitClient := NewClient(clonecache.New())
	const rounds = 5
	errs := make(chan error, len(prs)*rounds)
	var wg sync.WaitGroup
	for _, pr := range prs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range rounds {
				err := gitClient.WithPRClone(context.Background(), repoURL, pr.baseSHA, pr.headSHA,
					&CloneOptions{PRNumber: pr.number, BaseBranch: "master", Depth: 50},
					func(clone *CloneResult) error {
						_, diffErr := diff.Generate(clone.LocalPath, pr.baseSHA, pr.headSHA, diff.DefaultOptions())
						return diffErr
					})
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent PR diff failed: %v", err)
	}
	return sessions.Load()
}

// TestWithPRCloneReusesOnDiskCloneAcrossPRs: PRs of one repo share a single
// cache directory, so a second PR whose commits are already on disk must reuse
// that clone instead of re-fetching.
func TestWithPRCloneReusesOnDiskCloneAcrossPRs(t *testing.T) {
	if got := runConcurrentPRDiffs(t, buildLinearRemoteWithPRs); got != 1 {
		t.Errorf("expected one fetch of the shared repo cache, got %d", got)
	}
}

// TestWithPRCloneDiffSurvivesCloneReplacementBySiblingPR reproduces the
// production race: a clone for one PR swaps the shared cache directory while
// another PR's diff is reading it ("packfile not found").
func TestWithPRCloneDiffSurvivesCloneReplacementBySiblingPR(t *testing.T) {
	// Fewer fetches than PRs would mean the directory was never replaced and
	// the test did not exercise the race.
	if got := runConcurrentPRDiffs(t, buildDivergentRemoteWithPRs); got < 2 {
		t.Fatalf("expected the sibling PR to replace the clone, got %d fetches", got)
	}
}
