package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

func TestGetCIStatus_PaginatesCheckRuns(t *testing.T) {
	const pageSize = 100
	client, closeServer := newListPRsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/commits/head/check-runs" {
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("per_page") != fmt.Sprint(pageSize) {
			t.Errorf("expected per_page=%d, got %s", pageSize, r.URL.RawQuery)
		}
		checks := make([]*ghlib.CheckRun, 0, pageSize)
		if r.URL.Query().Get("page") == "2" {
			checks = append(checks, &ghlib.CheckRun{Name: ghlib.Ptr("failed-check"), Status: ghlib.Ptr("completed"), Conclusion: ghlib.Ptr("failure")})
		} else {
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?page=2&per_page=%d>; rel="next"`, r.Host, r.URL.Path, pageSize))
			for i := range pageSize {
				checks = append(checks, &ghlib.CheckRun{Name: ghlib.Ptr(fmt.Sprintf("check-%d", i)), Status: ghlib.Ptr("completed"), Conclusion: ghlib.Ptr("success")})
			}
		}
		if err := json.NewEncoder(w).Encode(ghlib.ListCheckRunsResults{Total: ghlib.Ptr(pageSize + 1), CheckRuns: checks}); err != nil {
			t.Errorf("encode checks: %v", err)
		}
	})
	defer closeServer()

	status, err := client.getCIStatus(context.Background(), "owner", "repo", "head")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "failure" || len(status.Checks) != pageSize+1 {
		t.Fatalf("got status=%s checks=%d, want failure and %d checks", status.Status, len(status.Checks), pageSize+1)
	}
	if status.Checks[pageSize].Name != "failed-check" {
		t.Errorf("last check = %q, want failed-check", status.Checks[pageSize].Name)
	}
}

func TestGetCIStatus(t *testing.T) {
	client := NewClient("test-token")
	if client == nil {
		t.Error("expected non-nil client")
	}
}

// TestGetCIStatus_InvalidID verifies GetCIStatus returns an error for a malformed ID.
func TestGetCIStatus_InvalidID(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.GetCIStatus(ctx, "invalid-id")
	if err == nil {
		t.Error("expected error for invalid PR ID, got nil")
	}
}
