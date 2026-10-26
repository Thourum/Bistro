package sync

import (
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestWritePRRowListOnlyRevisionSemantics(t *testing.T) {
	tests := []struct {
		name       string
		headSHA    string
		baseSHA    string
		isRevision bool
	}{
		{name: "unchanged revision", headSHA: "head-1", baseSHA: "base-1"},
		{name: "changed head SHA", headSHA: "head-2", baseSHA: "base-1", isRevision: true},
		{name: "changed base SHA", headSHA: "head-1", baseSHA: "base-2", isRevision: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := storetest.Open(t)
			writer := NewWriter(nil, db)
			now := time.Now().UTC()
			initialPR := &provider.PR{
				ID: "github:org/repo:9", Provider: "github", Owner: "org", Repo: "repo",
				Number: 9, Title: "initial", Author: "a", State: "open",
				BaseBranch: "main", HeadBranch: "f", BaseSHA: "base-1", HeadSHA: "head-1",
				CreatedAt: now, UpdatedAt: now, CommentCount: 4, CIStatus: "success",
				ReviewStatus: "approved", ApprovedBy: "reviewer", ChangesRequestedBy: "prior-requester",
				RequestedReviewers: []string{"next-reviewer"},
			}
			if err := writer.WriteSyncResults([]*PRFetchResult{{PR: initialPR}}); err != nil {
				t.Fatalf("initial full write: %v", err)
			}

			listPR := &provider.PR{
				ID: initialPR.ID, Provider: initialPR.Provider, Owner: initialPR.Owner, Repo: initialPR.Repo,
				Number: initialPR.Number, Title: "from list", Author: initialPR.Author, State: initialPR.State,
				BaseBranch: initialPR.BaseBranch, HeadBranch: initialPR.HeadBranch, BaseSHA: tt.baseSHA, HeadSHA: tt.headSHA,
				CreatedAt: initialPR.CreatedAt, UpdatedAt: initialPR.UpdatedAt,
			}
			if err := writer.WriteSyncResults([]*PRFetchResult{{PR: listPR, ListOnly: true}}); err != nil {
				t.Fatalf("list-only write: %v", err)
			}

			var hydrated, commentCount int
			var headSHA, baseSHA string
			var ciStatus, reviewStatus, approvedBy, changesRequestedBy *string
			if err := db.QueryRow(`
				SELECT hydrated, head_sha, base_sha, comment_count, ci_status,
					review_status, approved_by, changes_requested_by
				FROM prs WHERE id = ?`, initialPR.ID,
			).Scan(
				&hydrated, &headSHA, &baseSHA, &commentCount, &ciStatus,
				&reviewStatus, &approvedBy, &changesRequestedBy,
			); err != nil {
				t.Fatalf("select list-only row: %v", err)
			}

			if headSHA != tt.headSHA || baseSHA != tt.baseSHA {
				t.Fatalf("stored SHA pair = (%q, %q), want (%q, %q)", headSHA, baseSHA, tt.headSHA, tt.baseSHA)
			}
			if tt.isRevision {
				if hydrated != 0 {
					t.Errorf("hydrated after revised list-only write = %d, want 0", hydrated)
				}
				if commentCount != 0 || ciStatus != nil || reviewStatus != nil || approvedBy != nil || changesRequestedBy != nil {
					t.Errorf("detail fields after revised list-only write = count:%d ci:%v review:%v approved:%v changes:%v, want cleared", commentCount, ciStatus, reviewStatus, approvedBy, changesRequestedBy)
				}
			} else {
				if hydrated != 1 {
					t.Errorf("hydrated after unchanged list-only write = %d, want 1", hydrated)
				}
				if commentCount != 4 || stringValue(ciStatus) != "success" || stringValue(reviewStatus) != "approved" || stringValue(approvedBy) != "reviewer" || stringValue(changesRequestedBy) != "prior-requester" {
					t.Errorf("detail fields after unchanged list-only write = count:%d ci:%v review:%v approved:%v changes:%v, want preserved", commentCount, ciStatus, reviewStatus, approvedBy, changesRequestedBy)
				}
			}

			if !tt.isRevision {
				return
			}

			fullPR := &provider.PR{
				ID: listPR.ID, Provider: listPR.Provider, Owner: listPR.Owner, Repo: listPR.Repo,
				Number: listPR.Number, Title: listPR.Title, Author: listPR.Author, State: listPR.State,
				BaseBranch: listPR.BaseBranch, HeadBranch: listPR.HeadBranch, BaseSHA: listPR.BaseSHA, HeadSHA: listPR.HeadSHA,
				CreatedAt: listPR.CreatedAt, UpdatedAt: listPR.UpdatedAt, CommentCount: 2, CIStatus: "pending",
				ReviewStatus: "changes_requested", ChangesRequestedBy: "new-reviewer",
			}
			if err := writer.WriteSyncResults([]*PRFetchResult{{PR: fullPR}}); err != nil {
				t.Fatalf("rehydrating full write: %v", err)
			}
			if err := db.QueryRow(`SELECT hydrated, comment_count, ci_status, review_status, changes_requested_by FROM prs WHERE id = ?`, initialPR.ID).Scan(
				&hydrated, &commentCount, &ciStatus, &reviewStatus, &changesRequestedBy,
			); err != nil {
				t.Fatalf("select rehydrated row: %v", err)
			}
			if hydrated != 1 || commentCount != 2 || stringValue(ciStatus) != "pending" || stringValue(reviewStatus) != "changes_requested" || stringValue(changesRequestedBy) != "new-reviewer" {
				t.Errorf("rehydrated fields = hydrated:%d count:%d ci:%v review:%v changes:%v, want current full details", hydrated, commentCount, ciStatus, reviewStatus, changesRequestedBy)
			}
		})
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
