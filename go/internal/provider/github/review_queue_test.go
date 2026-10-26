package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
)

func TestReviewQueueTransportDoesNotLogUsername(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := `{"items":[]}`
		if r.URL.Path == "/user" {
			body = `{"login":"private-login"}`
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewClient("")
	httpClient := server.Client()
	httpClient.Transport = newLoggingTransport(httpClient.Transport)
	client.gh = gh.NewClient(httpClient)
	var err error
	client.gh.BaseURL, err = client.gh.BaseURL.Parse(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	ctx := logging.WithLogger(context.Background(), slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if _, err := client.GetReviewQueue(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "private-login") {
		t.Fatalf("search username logged: %s", output.String())
	}
}

func TestReviewQueueMalformedSearchItemsReturnErrors(t *testing.T) {
	for _, item := range []*gh.Issue{nil, {}, {RepositoryURL: gh.Ptr("https://api.github.com/repos/acme/bad%20repo")}} {
		for _, requested := range []bool{true, false} {
			var queue *provider.ReviewQueue
			var err error
			if requested {
				queue, err = collectReviewQueue([]*gh.Issue{item}, nil)
			} else {
				queue, err = collectReviewQueue(nil, []*gh.Issue{item})
			}
			if err == nil || queue != nil {
				t.Fatalf("malformed item accepted: queue=%+v err=%v", queue, err)
			}
		}
	}
}

func TestReviewQueuePreservesGitHubCasingAndMergesCaseVariants(t *testing.T) {
	queue, err := collectReviewQueue([]*gh.Issue{
		{RepositoryURL: gh.Ptr("https://api.github.com/repos/Itterno/Bistro")},
		{HTMLURL: gh.Ptr("https://github.com/itterno/bistro/pull/2")},
	}, []*gh.Issue{
		{RepositoryURL: gh.Ptr("https://api.github.com/repos/ITTERNO/BISTRO")},
		{RepositoryURL: gh.Ptr("https://api.github.com/repos/itterno/bistro")},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRequested := []provider.RequestedRepo{{RepoID: "github:itterno/bistro", Count: 2}}
	wantRepositories := []*provider.Repo{{Owner: "Itterno", Name: "Bistro", FullName: "Itterno/Bistro"}}
	if !reflect.DeepEqual(queue.Requested, wantRequested) || !reflect.DeepEqual(queue.Repositories, wantRepositories) || !reflect.DeepEqual(queue.RecentlyReviewed, []string{"github:itterno/bistro"}) {
		t.Fatalf("queue lost GitHub casing or duplicated case variants: %+v repositories=%+v", queue, queue.Repositories)
	}
}

func TestReviewQueueRejectsEmptyLoginAndRecentSearchFailure(t *testing.T) {
	for _, mode := range []string{"empty login", "recent failure"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/user" {
					login := "octocat"
					if mode == "empty login" {
						login = ""
					}
					if err := json.NewEncoder(w).Encode(map[string]string{"login": login}); err != nil {
						t.Error(err)
					}
					return
				}
				if mode == "empty login" {
					t.Error("searched with empty login")
				}
				if strings.Contains(r.URL.Query().Get("q"), "reviewed-by:") {
					w.WriteHeader(http.StatusForbidden)
					if _, err := w.Write([]byte(`{"message":"rate limited"}`)); err != nil {
						t.Error(err)
					}
					return
				}
				if _, err := w.Write([]byte(`{"items":[]}`)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewClient("")
			client.gh = gh.NewClient(server.Client())
			var err error
			client.gh.BaseURL, err = client.gh.BaseURL.Parse(server.URL + "/")
			if err != nil {
				t.Fatal(err)
			}
			queue, err := client.GetReviewQueue(context.Background(), time.Now())
			if err == nil || queue != nil {
				t.Fatalf("expected failure: %+v %v", queue, err)
			}
			if mode == "recent failure" {
				var responseError *gh.ErrorResponse
				if !errors.As(err, &responseError) {
					t.Fatalf("lost GitHub cause: %v", err)
				}
			}
		})
	}
}

func TestReviewQueueGroupsSortsAndCaps(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, mode := range []string{"populated", "empty", "failed"} {
		t.Run(mode, func(t *testing.T) {
			var queries []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/user" {
					if err := json.NewEncoder(w).Encode(map[string]string{"login": "octocat"}); err != nil {
						t.Error(err)
					}
					return
				}
				if r.URL.Path != "/search/issues" {
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				query := r.URL.Query().Get("q")
				queries = append(queries, query)
				if r.URL.Query().Get("per_page") != "100" || r.URL.Query().Get("page") != "" {
					t.Errorf("search must use one default first page of 100: %s", r.URL.RawQuery)
				}
				if mode == "failed" {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message":"rate limited"}`))
					return
				}
				names := []string{}
				if mode == "populated" && strings.Contains(query, "review-requested:") {
					names = []string{"Zed/one", "Acme/two", "Acme/two", "Acme/one"}
				}
				if strings.Contains(query, "reviewed-by:") {
					if r.URL.Query().Get("sort") != "updated" || r.URL.Query().Get("order") != "desc" {
						t.Error("recent reviews not newest first")
					}
					if mode == "populated" {
						names = []string{"Acme/two", "Acme/two"}
						for i := 0; i < 12; i++ {
							names = append(names, fmt.Sprintf("Acme/recent-%02d", i))
						}
					}
				}
				issues := make([]map[string]any, 0, len(names))
				for i, name := range names {
					issues = append(issues, map[string]any{"number": i + 1,
						"repository_url": "https://api.github.com/repos/" + name,
						"html_url":       fmt.Sprintf("https://github.com/%s/pull/%d", name, i+1),
						"pull_request":   map[string]any{"url": "https://api.github.com/repos/" + name + "/pulls/1"}})
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"total_count": len(issues), "items": issues}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewClient("")
			api := gh.NewClient(server.Client())
			var err error
			api.BaseURL, err = api.BaseURL.Parse(server.URL + "/")
			if err != nil {
				t.Fatal(err)
			}
			client.gh = api
			queue, err := client.GetReviewQueue(context.Background(), now)
			if mode == "failed" {
				if err == nil {
					t.Fatal("expected search failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantQueries := []string{"is:pr is:open archived:false review-requested:octocat", "is:pr reviewed-by:octocat updated:>=2026-09-24"}
			if !reflect.DeepEqual(queries, wantQueries) {
				t.Fatalf("queries = %#v", queries)
			}
			if mode == "empty" {
				if queue.Requested == nil || queue.RecentlyReviewed == nil || len(queue.Requested)+len(queue.RecentlyReviewed) != 0 {
					t.Fatalf("empty queue = %+v", queue)
				}
				return
			}
			wantRequested := []provider.RequestedRepo{{RepoID: "github:acme/two", Count: 2}, {RepoID: "github:acme/one", Count: 1}, {RepoID: "github:zed/one", Count: 1}}
			if !reflect.DeepEqual(queue.Requested, wantRequested) {
				t.Fatalf("requested = %+v", queue.Requested)
			}
			if len(queue.RecentlyReviewed) != 10 || queue.RecentlyReviewed[0] != "github:acme/two" || queue.RecentlyReviewed[9] != "github:acme/recent-08" {
				t.Fatalf("recent = %#v", queue.RecentlyReviewed)
			}
			if len(queue.Repositories) != 15 {
				t.Fatalf("must upsert every distinct repo from both pages, including beyond display cap: %d", len(queue.Repositories))
			}
		})
	}
}
