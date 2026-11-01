package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shurcooL/githubv4"
)

func TestGetReviewThreads_PaginatesGraphQL(t *testing.T) {
	const pageSize = 100
	const threadCursor = "threads-page-2"
	const commentCursor = "comments-page-2"
	for _, scenario := range []string{"threads", "comments"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Query     string
					Variables map[string]any
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode query: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				comments := []map[string]any{{"databaseId": pageSize + 1}}
				pageInfo := map[string]any{"hasNextPage": false, "endCursor": ""}
				if request.Variables["commentsCursor"] == commentCursor {
					if request.Variables["threadID"] != "thread-1" {
						t.Errorf("unexpected thread ID %v", request.Variables["threadID"])
					}
					response := map[string]any{"data": map[string]any{"node": map[string]any{"comments": map[string]any{"nodes": comments, "pageInfo": pageInfo}}}}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Errorf("encode comment page: %v", err)
					}
					return
				}
				var nodes []map[string]any
				if request.Variables["threadsCursor"] == threadCursor {
					nodes = append(nodes, map[string]any{"id": "thread-101", "isResolved": true, "isOutdated": true, "comments": map[string]any{"nodes": comments, "pageInfo": pageInfo}})
				} else if scenario == "threads" {
					for i := 1; i <= pageSize; i++ {
						nodes = append(nodes, map[string]any{"id": fmt.Sprintf("thread-%d", i), "isResolved": true, "isOutdated": true, "comments": map[string]any{"nodes": []map[string]any{{"databaseId": i}}, "pageInfo": pageInfo}})
					}
					pageInfo = map[string]any{"hasNextPage": true, "endCursor": threadCursor}
				} else {
					comments = nil
					for i := 1; i <= pageSize; i++ {
						comments = append(comments, map[string]any{"databaseId": i})
					}
					nodes = append(nodes, map[string]any{"id": "thread-1", "isResolved": true, "isOutdated": true, "comments": map[string]any{"nodes": comments, "pageInfo": map[string]any{"hasNextPage": true, "endCursor": commentCursor}}})
				}
				connection := map[string]any{"nodes": nodes, "pageInfo": pageInfo}
				if !strings.Contains(request.Query, "pageInfo") {
					delete(connection, "pageInfo")
					for _, node := range nodes {
						delete(node["comments"].(map[string]any), "pageInfo")
					}
				}
				response := map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{"reviewThreads": connection}}}}
				if err := json.NewEncoder(w).Encode(response); err != nil {
					t.Errorf("encode thread page: %v", err)
				}
			}))
			defer server.Close()

			client, closeREST := newListPRsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				// Only the comment beyond the first GraphQL page is available via REST.
				fmt.Fprintf(w, `[{"id":%d,"body":"later comment"}]`, pageSize+1)
			})
			defer closeREST()
			client.gqlClient = githubv4.NewEnterpriseClient(server.URL, server.Client())
			threads, err := client.GetReviewThreads(context.Background(), "github:owner/repo:1")
			if err != nil {
				t.Fatal(err)
			}
			if len(threads) != 1 {
				t.Fatalf("got %d threads, want 1", len(threads))
			}
			wantID := "thread-101"
			if scenario == "comments" {
				wantID = "thread-1"
			}
			thread := threads[0]
			if thread.ID != wantID || !thread.StateKnown || !thread.Resolved || !thread.Outdated || thread.Comments[0].ThreadID != wantID {
				t.Fatalf("later-page thread not enriched: %+v", thread)
			}
		})
	}
}
