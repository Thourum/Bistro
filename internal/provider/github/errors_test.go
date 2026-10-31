package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/provider"
)

func originalAPIError(t *testing.T, err error) error {
	t.Helper()
	var rate *gh.RateLimitError
	if errors.As(err, &rate) {
		return rate
	}
	var abuse *gh.AbuseRateLimitError
	if errors.As(err, &abuse) {
		return abuse
	}
	var response *gh.ErrorResponse
	if errors.As(err, &response) {
		return response
	}
	t.Fatal("original GitHub API error lost")
	return nil
}

func TestListPRsProviderErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		status    int
		remaining string
		body      string
		wantType  provider.ErrorType
	}{
		{"primary rate limit", http.StatusForbidden, "0", `{"message":"API rate limit exceeded"}`, provider.ErrorTypeRateLimit},
		{"secondary rate limit", http.StatusForbidden, "1", `{"message":"secondary rate limit exceeded","documentation_url":"https://docs.github.com/rest/using-the-rest-api/rate-limits-for-the-rest-api#about-secondary-rate-limits"}`, provider.ErrorTypeRateLimit},
		{"429", http.StatusTooManyRequests, "1", `{"message":"too many requests"}`, provider.ErrorTypeRateLimit},
		{"unauthorized", http.StatusUnauthorized, "1", `{"message":"unauthorized"}`, provider.ErrorTypeUnauthorized},
		{"not found", http.StatusNotFound, "1", `{"message":"not found"}`, provider.ErrorTypeNotFound},
		{"forbidden", http.StatusForbidden, "1", `{"message":"forbidden"}`, provider.ErrorTypeUnknown},
		{"server error", http.StatusInternalServerError, "1", `{"message":"server error"}`, provider.ErrorTypeUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", tt.remaining)
				w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Hour).Unix()))
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			client := &Client{gh: gh.NewClient(server.Client())}
			client.gh.BaseURL, _ = url.Parse(server.URL + "/")

			_, err := client.ListPRs(context.Background(), provider.PRFilters{Owner: "owner", Repo: "repo"})
			if err == nil {
				t.Fatal("expected API error")
			}
			var pe *provider.ProviderError
			typed := errors.As(err, &pe)
			if tt.wantType == provider.ErrorTypeUnknown {
				if typed {
					t.Fatalf("unrelated API error classified as %v", pe.Type)
				}
				return
			}
			if !typed || pe.Type != tt.wantType || pe.Status != tt.status {
				t.Fatalf("expected provider type %v and status %d, got %v", tt.wantType, tt.status, err)
			}
			original := originalAPIError(t, err)
			if !errors.Is(err, original) {
				t.Fatal("original error no longer reachable with errors.Is")
			}
			wantMessage := fmt.Sprintf("failed to list PRs for owner/repo: %s", original.Error())
			if err.Error() != wantMessage {
				t.Fatalf("message changed: got %q, want %q", err.Error(), wantMessage)
			}
			before := actions.ActionResult{Error: wantMessage}
			after := actions.ActionResult{Error: err.Error()}
			if after.IsRateLimit() != before.IsRateLimit() || after.IsRecoverable() != before.IsRecoverable() {
				t.Fatal("action error keyword classification changed")
			}
		})
	}
}
