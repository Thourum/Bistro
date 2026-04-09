package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	ghlib "github.com/google/go-github/v63/github"
)

func TestClient_GetUsername(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"login": "octocat", "id": 1}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient("test-token")
	ghClient, _ := ghlib.NewClient(nil).WithEnterpriseURLs(server.URL+"/", server.URL+"/")
	client.gh = ghClient

	username, err := client.GetUsername(context.Background())
	if err != nil {
		t.Fatalf("GetUsername failed: %v", err)
	}
	if username != "octocat" {
		t.Fatalf("expected octocat, got %s", username)
	}
}
