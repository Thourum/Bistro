package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type fixedUser string

func (f fixedUser) GetCurrentUser(context.Context, string) (string, bool, error) {
	return string(f), f != "", nil
}

func draftMessage(t *testing.T, msgType string, data map[string]any) ws.InboundMessage {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return ws.InboundMessage{ID: "req-1", Type: msgType, Data: raw}
}

func draftResponseData(t *testing.T, resp any) map[string]any {
	t.Helper()
	envelope, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response = %#v", resp)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data = %#v", envelope["data"])
	}
	return data
}

func TestDraftSave_RejectsInvalidPositions(t *testing.T) {
	deps := DraftDeps{Drafts: store.NewReviewDrafts(newReadStateTestDB(t).DB), CurrentUser: fixedUser("me"), Broadcaster: &spyBroadcaster{}}
	save := NewDraftSaveHandler(deps)
	valid := map[string]any{"pr_id": "pr-1", "path": "a.go", "side": "RIGHT", "line": 4, "body": "x", "commit_sha": "head"}
	cases := map[string]func(map[string]any){
		"bad side":       func(m map[string]any) { m["side"] = "MIDDLE" },
		"start >= line":  func(m map[string]any) { m["start_line"] = 4 },
		"blank body":     func(m map[string]any) { m["body"] = "   " },
		"missing commit": func(m map[string]any) { m["commit_sha"] = "" },
		"zero line":      func(m map[string]any) { m["line"] = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			data := map[string]any{}
			for k, v := range valid {
				data[k] = v
			}
			mutate(data)
			resp, err := save(context.Background(), draftMessage(t, "DRAFT_SAVE", data))
			if err != nil {
				t.Fatalf("handler err: %v", err)
			}
			if draftResponseData(t, resp)["error"] == nil {
				t.Fatalf("expected a validation error, got %#v", resp)
			}
		})
	}
}

func TestDraftSaveListDelete_BroadcastsDraftsChanged(t *testing.T) {
	spy := &spyBroadcaster{}
	deps := DraftDeps{Drafts: store.NewReviewDrafts(newReadStateTestDB(t).DB), CurrentUser: fixedUser("me"), Broadcaster: spy}
	ctx := context.Background()

	resp, err := NewDraftSaveHandler(deps)(ctx, draftMessage(t, "DRAFT_SAVE", map[string]any{
		"pr_id": "pr-1", "path": "a.go", "side": "LEFT", "start_line": 2, "line": 4, "body": "range", "commit_sha": "head",
	}))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	saved, ok := draftResponseData(t, resp)["draft"].(store.ReviewDraft)
	if !ok || saved.StartLine != 2 {
		t.Fatalf("saved = %#v", draftResponseData(t, resp)["draft"])
	}

	listResp, _ := NewDraftsListHandler(deps)(ctx, draftMessage(t, "DRAFTS_LIST", map[string]any{"pr_id": "pr-1"}))
	if drafts, _ := draftResponseData(t, listResp)["drafts"].([]store.ReviewDraft); len(drafts) != 1 {
		t.Fatalf("list = %#v", draftResponseData(t, listResp))
	}

	if _, err := NewDraftDeleteHandler(deps)(ctx, draftMessage(t, "DRAFT_DELETE", map[string]any{"pr_id": "pr-1", "draft_ids": []string{saved.ID}})); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(spy.messages) != 2 {
		t.Fatalf("broadcasts = %d, want DRAFTS_CHANGED after save and delete", len(spy.messages))
	}
}

func TestDraftSave_RequiresSignedInUser(t *testing.T) {
	deps := DraftDeps{Drafts: store.NewReviewDrafts(newReadStateTestDB(t).DB), CurrentUser: fixedUser(""), Broadcaster: &spyBroadcaster{}}
	resp, _ := NewDraftSaveHandler(deps)(context.Background(), draftMessage(t, "DRAFT_SAVE", map[string]any{
		"pr_id": "pr-1", "path": "a.go", "side": "RIGHT", "line": 1, "body": "x", "commit_sha": "head",
	}))
	if draftResponseData(t, resp)["error"] == nil {
		t.Fatal("expected an error without a signed-in user")
	}
}
