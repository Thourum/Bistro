package expertise

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
)

type panickingIndexerGit struct {
	*fakeIndexerGit
	panicAt string
}

func (git panickingIndexerGit) HeadSHA(ctx context.Context, path string) (string, error) {
	if git.panicAt == "index" {
		panic("index panic")
	}
	return git.fakeIndexerGit.HeadSHA(ctx, path)
}

func (git panickingIndexerGit) BlameFile(ctx context.Context, path, file string) (map[AuthorKey]int, int, error) {
	if git.panicAt == "blame" {
		panic("blame panic")
	}
	return git.fakeIndexerGit.BlameFile(ctx, path, file)
}

func TestExpertiseJobsRecoverAndReportPanics(t *testing.T) {
	for _, phase := range []string{"blame", "index"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			db := storetest.Open(t)
			store := NewStore(db.DB)
			git := panickingIndexerGit{fakeIndexerGit: newFakeIndexerGit(1), panicAt: phase}
			indexer := NewIndexer(newSilentExpertiseLogger(), store, git, nil)
			var reports atomic.Int32
			indexer.OnPanic = func(recovered any, stack []byte) {
				reports.Add(1)
				if recovered != phase+" panic" || len(stack) == 0 {
					t.Errorf("panic report = %v", recovered)
				}
			}
			engine := NewEngine(newSilentExpertiseLogger(), store, git, indexer, nil)
			engine.startIndex(fixtureRepoID, t.TempDir())
			engine.mu.Lock()
			running, ok := engine.running[fixtureRepoID]
			engine.mu.Unlock()
			if ok {
				<-running.done
			}
			engine.Shutdown()
			state, err := store.RepoState(ctx, fixtureRepoID)
			if err != nil || state.Status != ExpertiseStatusError || !strings.Contains(state.Error.String, "panicked") || reports.Load() != 1 {
				t.Fatalf("state=%+v err=%v reports=%d", state, err, reports.Load())
			}
			// The same indexer remains usable after either job panic.
			indexer.git = git.fakeIndexerGit
			if err := indexer.Index(ctx, fixtureRepoID, t.TempDir()); err != nil {
				t.Fatal(err)
			}
			state, err = store.RepoState(ctx, fixtureRepoID)
			if err != nil || state.Status != ExpertiseStatusReady {
				t.Fatalf("retry state=%+v err=%v", state, err)
			}
		})
	}
}
