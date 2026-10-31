package provider

import (
	"testing"
	"time"
)

func review(author, state string, minuteOffset int) *Review {
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	return &Review{
		Author:      author,
		State:       state,
		SubmittedAt: base.Add(time.Duration(minuteOffset) * time.Minute),
	}
}

func TestDeriveReviewStatus_NoReviews(t *testing.T) {
	status, approvedBy, changesBy := DeriveReviewStatus(nil)
	if status != "pending" || approvedBy != "" || changesBy != "" {
		t.Errorf("got (%q, %q, %q), want (pending, , )", status, approvedBy, changesBy)
	}
}

func TestDeriveReviewStatus_Approved(t *testing.T) {
	status, approvedBy, _ := DeriveReviewStatus([]*Review{
		review("alice", "approved", 0),
	})
	if status != "approved" || approvedBy != "alice" {
		t.Errorf("got (%q, %q), want (approved, alice)", status, approvedBy)
	}
}

func TestDeriveReviewStatus_ChangesRequestedWinsOverApproval(t *testing.T) {
	status, _, changesBy := DeriveReviewStatus([]*Review{
		review("alice", "approved", 0),
		review("bob", "changes_requested", 1),
	})
	if status != "changes_requested" || changesBy != "bob" {
		t.Errorf("got (%q, %q), want (changes_requested, bob)", status, changesBy)
	}
}

func TestDeriveReviewStatus_LatestPerReviewerWins(t *testing.T) {
	// Bob requested changes, then approved — his latest vote counts.
	status, approvedBy, changesBy := DeriveReviewStatus([]*Review{
		review("bob", "changes_requested", 0),
		review("bob", "approved", 5),
	})
	if status != "approved" || approvedBy != "bob" || changesBy != "" {
		t.Errorf("got (%q, %q, %q), want (approved, bob, )", status, approvedBy, changesBy)
	}
}

func TestDeriveReviewStatus_CommentsDoNotChangeVote(t *testing.T) {
	// A trailing comment must not clear an earlier approval.
	status, approvedBy, _ := DeriveReviewStatus([]*Review{
		review("alice", "approved", 0),
		review("alice", "commented", 5),
	})
	if status != "approved" || approvedBy != "alice" {
		t.Errorf("got (%q, %q), want (approved, alice)", status, approvedBy)
	}
}

func TestDeriveReviewStatus_DismissedClearsVote(t *testing.T) {
	status, _, changesBy := DeriveReviewStatus([]*Review{
		review("bob", "changes_requested", 0),
		review("bob", "dismissed", 5),
	})
	if status != "pending" || changesBy != "" {
		t.Errorf("got (%q, %q), want (pending, )", status, changesBy)
	}
}

func TestDeriveReviewStatus_OutOfOrderInput(t *testing.T) {
	// Input is not guaranteed to be sorted by submission time.
	status, approvedBy, _ := DeriveReviewStatus([]*Review{
		review("bob", "approved", 5),
		review("bob", "changes_requested", 0),
	})
	if status != "approved" || approvedBy != "bob" {
		t.Errorf("got (%q, %q), want (approved, bob)", status, approvedBy)
	}
}

func TestDeriveReviewStatus_PendingAndUnknownStatesIgnored(t *testing.T) {
	status, _, _ := DeriveReviewStatus([]*Review{
		review("alice", "pending", 0),
		review("bob", "commented", 1),
		review("", "approved", 2), // missing author is ignored
	})
	if status != "pending" {
		t.Errorf("got %q, want pending", status)
	}
}
