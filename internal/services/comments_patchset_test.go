package services

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
)

func TestRequestCommentsCurrentPatchsetIgnoresBaseTip(t *testing.T) {
	db := storetest.Open(t).DB
	mustExecDB(t, db, `INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES ('pr-head-only','github','o','r',1,'T','a','open','2026-01-01','2026-01-01','main','f','tip-3','h1')`)
	mustExecDB(t, db, `INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-head-only', 1, 'h1', 'tip-1', '2026-01-01'),
		       ('pr-head-only', 2, 'h1', 'tip-2', '2026-01-01'),
		       ('pr-head-only', 3, 'h2', 'tip-3', '2026-01-01')`)
	mustExecDB(t, db, `INSERT INTO comments
		(id, pr_id, author, body, created_at, updated_at,
		 kind, patchset_number, anchored_line, anchor_head_sha, outdated,
		 anchor_patchset_number, anchor_current_patchset_number)
		VALUES ('comment-head-only','pr-head-only','bob','good','2026-01-01','2026-01-01',
		        'review', 2, 11, 'h1', 0, 2, 2)`)

	handler := (&Comments{DB: db, Log: testRepoLogger()})
	response, err := handler.List(context.Background(), PRRef{PRID: "pr-head-only"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	comments := response.Comments
	if len(comments) != 1 {
		t.Fatalf("comments = %+v, want one row (no duplicate patchset joins)", comments)
	}
	if !comments[0].Anchored || comments[0].AnchoredLine == nil || *comments[0].AnchoredLine != 11 {
		t.Fatalf("comment = %+v, want fresh P2 anchor despite a different base tip", comments[0])
	}
}
