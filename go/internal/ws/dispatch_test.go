package ws

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDispatchLowercasesGitHubIdentities(t *testing.T) {
	dispatcher := NewDispatcher(nil)
	var received json.RawMessage
	dispatcher.Register("PROBE", func(_ context.Context, msg InboundMessage) (any, error) {
		received = msg.Data
		return nil, nil
	})

	raw := `{"id":"1","type":"PROBE","data":{` +
		`"repo_id":"github:Thourum/Testing","pr_ids":["github:Acme/API:1","gitea:Org/Repo:2"],` +
		`"items":[{"pr_id":"github:Acme/API:3"}],"body":"Keep Me","comment_id":9007199254740993}}`
	if _, err := dispatcher.Dispatch(context.Background(), []byte(raw)); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	var got struct {
		RepoID string   `json:"repo_id"`
		PRIDs  []string `json:"pr_ids"`
		Items  []struct {
			PRID string `json:"pr_id"`
		} `json:"items"`
		Body      string      `json:"body"`
		CommentID json.Number `json:"comment_id"`
	}
	if err := json.Unmarshal(received, &got); err != nil {
		t.Fatalf("decode received data: %v", err)
	}
	if got.RepoID != "github:thourum/testing" || got.PRIDs[0] != "github:acme/api:1" ||
		got.PRIDs[1] != "gitea:Org/Repo:2" || got.Items[0].PRID != "github:acme/api:3" ||
		got.Body != "Keep Me" || got.CommentID != "9007199254740993" {
		t.Fatalf("received %s", received)
	}
}
