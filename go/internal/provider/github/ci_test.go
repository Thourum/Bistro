package github

import (
	"testing"

	ghlib "github.com/google/go-github/v84/github"
)

func TestNormalizeCheckRun(t *testing.T) {
	ghCheck := &ghlib.CheckRun{
		Name:       ghlib.Ptr("Tests"),
		Status:     ghlib.Ptr("completed"),
		Conclusion: ghlib.Ptr("success"),
	}

	check := normalizeCheckRun(ghCheck)
	if check.Name != "Tests" {
		t.Errorf("expected name Tests, got %s", check.Name)
	}
	if check.Status != "success" {
		t.Errorf("expected status success, got %s", check.Status)
	}
}

func TestNormalizeCheckRunPending(t *testing.T) {
	ghCheck := &ghlib.CheckRun{
		Name:   ghlib.Ptr("Lint"),
		Status: ghlib.Ptr("in_progress"),
	}

	check := normalizeCheckRun(ghCheck)
	if check.Status != "pending" {
		t.Errorf("expected status pending, got %s", check.Status)
	}
}

func TestGetCIStatus(t *testing.T) {
	client := NewClient("test-token")
	if client == nil {
		t.Error("expected non-nil client")
	}
}
