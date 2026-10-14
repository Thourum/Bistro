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
)

// raceFixturePR is one PR on a linear local remote: its base and head commits.
type raceFixturePR struct {
	number  int
	baseSHA string
	headSHA string
}

// buildLinearRemoteWithPRs commits c1..c4 on the default branch and publishes
// c2 and c4 as refs/pull/1/head and refs/pull/2/head. PR1 is c1..c2 and PR2 is c3..c4.
func buildLinearRemoteWithPRs(t *testing.T, dir string) []raceFixturePR {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}
	commit := func(content string) plumbing.Hash {
		if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := wt.Add("foo.go"); err != nil {
			t.Fatalf("Add: %v", err)
		}
		h, err := wt.Commit(content, &gogit.CommitOptions{Author: sig})
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		return h
	}
	c1, c2 := commit("v1\n"), commit("v2\n")
	c3, c4 := commit("v3\n"), commit("v4\n")
	for number, head := range map[int]plumbing.Hash{1: c2, 2: c4} {
		ref := plumbing.NewHashReference(plumbing.ReferenceName(fmt.Sprintf("refs/pull/%d/head", number)), head)
		if err := repo.Storer.SetReference(ref); err != nil {
			t.Fatalf("SetReference: %v", err)
		}
	}
	return []raceFixturePR{
		{number: 1, baseSHA: c1.String(), headSHA: c2.String()},
		{number: 2, baseSHA: c3.String(), headSHA: c4.String()},
	}
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

// TestWithPRCloneDiffSurvivesConcurrentPRClonesOfSameRepo reproduces the
// production race: PRs of one repo share a single cache directory, so a clone
// for one PR swaps that directory while another PR's diff is reading it. A
// second PR must also reuse the clone already on disk instead of re-fetching.
func TestWithPRCloneDiffSurvivesConcurrentPRClonesOfSameRepo(t *testing.T) {
	remoteDir := t.TempDir()
	prs := buildLinearRemoteWithPRs(t, remoteDir)
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
	gitClient := NewClient(NewCache())
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
						_, diffErr := gitClient.GenerateDiff(clone.LocalPath, pr.baseSHA, pr.headSHA, DefaultDiffOptions())
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
	if got := sessions.Load(); got != 1 {
		t.Errorf("expected one fetch of the shared repo cache, got %d", got)
	}
}
