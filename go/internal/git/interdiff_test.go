package git

import (
	"errors"
	"strings"
	"testing"
)

func TestInterdiffAfterForcePush(t *testing.T) {
	fixture := newForcePushFixture(t)
	client := NewClient(NewCache())
	syncHead(t, client, fixture, fixture.head1)
	head2 := forcePush(t, fixture)
	syncHead(t, client, fixture, head2)
	request := InterdiffRequest{
		RepoURL: forcePushRepoURL, RepoPath: fixture.cacheDir, Path: "foo.go",
		FromHead: fixture.head1, ToHead: head2,
		FromMergeBase: fixture.baseSHA, ToMergeBase: fixture.baseSHA,
	}

	result, err := client.Interdiff(request)
	if err != nil {
		t.Fatalf("Interdiff: %v", err)
	}
	if !strings.Contains(result.Patch, "-var x = 1") || !strings.Contains(result.Patch, "+var x = 2") {
		t.Fatalf("patch = %q, want head1 → head2 change", result.Patch)
	}
	if result.Rebased {
		t.Error("Rebased = true with identical merge bases")
	}

	// head1 changed foo.go relative to base, so treating them as the two merge
	// bases means upstream touched the file.
	request.ToMergeBase = fixture.head1
	if result, err = client.Interdiff(request); err != nil || !result.Rebased {
		t.Errorf("upstream-changed file: rebased=%v err=%v, want rebased", result.Rebased, err)
	}

	for _, tt := range []struct {
		name          string
		path          string
		fromHead      string
		toHead        string
		fromMergeBase string
		toMergeBase   string
		wantPatch     string
		wantRebased   bool
	}{
		{name: "identical heads", path: "foo.go", fromHead: head2, toHead: head2, fromMergeBase: fixture.baseSHA, toMergeBase: fixture.baseSHA},
		{name: "absent file", path: "missing.go", fromHead: fixture.head1, toHead: head2, fromMergeBase: fixture.baseSHA, toMergeBase: fixture.head1},
		{name: "absent directory", path: "missing/file.go", fromHead: fixture.head1, toHead: head2, fromMergeBase: fixture.baseSHA, toMergeBase: fixture.head1},
		{name: "unknown merge base", path: "foo.go", fromHead: head2, toHead: head2, toMergeBase: fixture.head1},
		{name: "unreadable merge base", path: "foo.go", fromHead: head2, toHead: head2, fromMergeBase: strings.Repeat("ab", 20), toMergeBase: fixture.baseSHA, wantRebased: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := client.Interdiff(InterdiffRequest{
				RepoURL: request.RepoURL, RepoPath: request.RepoPath, Path: tt.path,
				FromHead: tt.fromHead, ToHead: tt.toHead,
				FromMergeBase: tt.fromMergeBase, ToMergeBase: tt.toMergeBase,
			})
			if err != nil || got.Patch != tt.wantPatch || got.Rebased != tt.wantRebased {
				t.Errorf("Interdiff = %+v, err=%v; want patch=%q rebased=%v", got, err, tt.wantPatch, tt.wantRebased)
			}
		})
	}

	request.FromHead = strings.Repeat("ab", 20)
	if _, err := client.Interdiff(request); !errors.Is(err, ErrHeadUnavailable) {
		t.Errorf("missing head: err = %v, want ErrHeadUnavailable", err)
	}
	request.FromHead, request.ToHead = fixture.head1, request.FromHead
	if _, err := client.Interdiff(request); !errors.Is(err, ErrHeadUnavailable) {
		t.Errorf("missing target head: err = %v, want ErrHeadUnavailable", err)
	}
}

func TestInterdiffAddedAndDeletedFile(t *testing.T) {
	fixture := newForcePushFixture(t)
	client := NewClient(NewCache())
	const filePath = "added.go"
	addedHead := commitFile(t, fixture.remote, fixture.remoteDir, filePath, "package added\n", "add file").String()
	for _, tt := range []struct {
		name     string
		fromHead string
		toHead   string
		wantLine string
	}{
		{name: "addition", fromHead: fixture.baseSHA, toHead: addedHead, wantLine: "+package added"},
		{name: "deletion", fromHead: addedHead, toHead: fixture.baseSHA, wantLine: "-package added"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.Interdiff(InterdiffRequest{
				RepoURL: forcePushRepoURL, RepoPath: fixture.remoteDir, Path: filePath,
				FromHead: tt.fromHead, ToHead: tt.toHead,
				FromMergeBase: tt.fromHead, ToMergeBase: tt.toHead,
			})
			if err != nil || !strings.Contains(result.Patch, tt.wantLine) || !result.Rebased {
				t.Fatalf("Interdiff = %+v, err=%v; want %q and rebased", result, err, tt.wantLine)
			}
			if strings.Contains(result.Patch, "foo.go") {
				t.Errorf("patch includes an unrelated changed file: %q", result.Patch)
			}
		})
	}
}
