package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	gh "github.com/google/go-github/v63/github"
)

func TestListUserRepos_EmptyToken(t *testing.T) {
	// Mock server that returns 401 Unauthorized
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer srv.Close()

	client := NewClient("")
	// Point the go-github client at our mock server
	ghClient := gh.NewClient(nil)
	ghClient.BaseURL, _ = ghClient.BaseURL.Parse(srv.URL + "/")
	client.gh = ghClient

	_, err := client.ListUserRepos(context.Background())
	if err == nil {
		t.Error("expected error when calling ListUserRepos with empty token, got nil")
	}
}
