package sync

import (
	"context"
	"testing"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
)

func TestGenerateDiffUsesMergeBaseForDiffAndAnchors(t *testing.T) {
	dir, rootSHA, headSHA := makeExpertiseHookRepo(t)
	repo, err := gitlib.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	const baseBranch = "base-tip"
	const prNumber = 123
	if err := worktree.Checkout(&gitlib.CheckoutOptions{
		Hash: plumbing.NewHash(rootSHA), Branch: plumbing.NewBranchReferenceName(baseBranch), Create: true,
	}); err != nil {
		t.Fatalf("Checkout base-tip: %v", err)
	}
	writeExpertiseHookFile(t, dir, "src/app.go", "// upstream\npackage main\n\nfunc main() {}\n")
	writeExpertiseHookFile(t, dir, "upstream.txt", "upstream\n")
	baseSHA := commitExpertiseHookRepo(t, worktree, "upstream change")
	if err := repo.Storer.SetReference(plumbing.NewHashReference("refs/pull/123/head", plumbing.NewHash(headSHA))); err != nil {
		t.Fatalf("SetReference PR: %v", err)
	}

	for _, name := range []string{"full clone", "shallow clone"} {
		t.Run(name, func(t *testing.T) {
			localPath := dir
			if name == "shallow clone" {
				localPath = t.TempDir()
				if _, err := gitlib.PlainClone(localPath, true, &gitlib.CloneOptions{
					URL: "file://" + dir, Depth: 1, ReferenceName: plumbing.NewBranchReferenceName(baseBranch),
				}); err != nil {
					t.Fatalf("PlainClone: %v", err)
				}
				shallowRepo, err := gitlib.PlainOpen(localPath)
				if err != nil {
					t.Fatalf("PlainOpen shallow: %v", err)
				}
				if _, err := shallowRepo.CommitObject(plumbing.NewHash(headSHA)); err != nil {
					t.Fatalf("shallow clone missing PR head: %v", err)
				}
			}

			position := 4 // Three-dot diff: two context lines, deletion, then addition.
			result := &PRFetchResult{
				PR: &provider.PR{BaseSHA: baseSHA, HeadSHA: headSHA, BaseBranch: baseBranch, Number: prNumber},
				Comments: []*provider.Comment{{
					ID: "review-comment", Kind: "review", CommitSHA: headSHA,
					FilePath: "src/app.go", Side: "RIGHT", Position: &position,
				}},
			}
			orchestrator := &Orchestrator{git: git.NewClient(nil), anchorResolver: git.NewAnchorResolver()}
			if err := orchestrator.generateDiff(context.Background(), result, &git.CloneResult{LocalPath: localPath}, silentLog()); err != nil {
				t.Fatalf("generateDiff: %v", err)
			}
			if result.MergeBaseSHA != rootSHA {
				t.Errorf("MergeBaseSHA=%q, want %s", result.MergeBaseSHA, rootSHA)
			}
			if !result.DiffComputed || len(result.DiffFiles) != 2 {
				t.Errorf("DiffComputed=%v, files=%v; want only the two PR files", result.DiffComputed, result.DiffFiles)
			}
			for _, file := range result.DiffFiles {
				if file.PatchID == "" || file.PatchID != git.PatchID(file.Patch) {
					t.Errorf("PatchID for %s = %q, want full-patch fingerprint", file.FilePath, file.PatchID)
				}
				if file.FilePath != "src/app.go" && file.FilePath != "docs/readme.md" {
					t.Errorf("unexpected PR diff file %q", file.FilePath)
				}
			}
			if len(result.AnchorUpdates) != 1 || !result.AnchorUpdates[0].Anchored || result.AnchorUpdates[0].AnchoredLine != 3 {
				t.Errorf("AnchorUpdates=%+v; want review-comment anchored at line 3", result.AnchorUpdates)
			}
		})
	}
}

func TestGenerateDiffFallsBackToBaseTipForUnrelatedHistory(t *testing.T) {
	dir, baseSHA, _ := makeExpertiseHookRepo(t)
	repo, err := gitlib.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD,
		plumbing.NewBranchReferenceName("unrelated"))); err != nil {
		t.Fatalf("SetReference unrelated HEAD: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	writeExpertiseHookFile(t, dir, "unrelated.txt", "unrelated\n")
	headSHA := commitExpertiseHookRepo(t, worktree, "unrelated root")
	result := &PRFetchResult{PR: &provider.PR{BaseSHA: baseSHA, HeadSHA: headSHA}}
	orchestrator := &Orchestrator{git: git.NewClient(nil)}
	if err := orchestrator.generateDiff(context.Background(), result, &git.CloneResult{LocalPath: dir}, silentLog()); err != nil {
		t.Fatalf("generateDiff fallback: %v", err)
	}
	if result.MergeBaseSHA != "" {
		t.Errorf("MergeBaseSHA=%q, want unknown for unrelated histories", result.MergeBaseSHA)
	}
	if !result.DiffComputed || len(result.DiffFiles) != 3 {
		t.Errorf("DiffComputed=%v, files=%v; want base-tip diff with three files", result.DiffComputed, result.DiffFiles)
	}
}
