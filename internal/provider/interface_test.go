package provider

import "testing"

func TestPRValidation(t *testing.T) {
	pr := &PR{
		ID:       "github:org/repo:123",
		Provider: "github",
		Owner:    "org",
		Repo:     "repo",
		Number:   123,
		Title:    "Fix bug",
		Author:   "alice",
	}

	if pr.ID == "" {
		t.Error("PR ID should not be empty")
	}
	if pr.Provider != "github" {
		t.Error("expected provider github")
	}
}

func TestCommentValidation(t *testing.T) {
	comment := &Comment{
		ID:     "comment-1",
		PRID:   "github:org/repo:123",
		Author: "bob",
		Body:   "Great fix!",
	}

	if comment.ID == "" {
		t.Error("Comment ID should not be empty")
	}
}

func TestCIStatusValidation(t *testing.T) {
	status := &CIStatus{
		Status: "success",
		Checks: []*Check{
			{Name: "Tests", Status: "success"},
		},
	}

	if status.Status == "" {
		t.Error("CIStatus should have status")
	}
	if len(status.Checks) == 0 {
		t.Error("CIStatus should have checks")
	}
}
