package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type mockPRCheckoutInfoReader struct {
	info *store.PRCheckoutInfo
	err  error
}

func (m *mockPRCheckoutInfoReader) GetPRCheckoutInfo(_ context.Context, _ string) (*store.PRCheckoutInfo, error) {
	return m.info, m.err
}

type mockRepoLocationReader struct {
	location *store.RepoLocation
	err      error
}

func (m *mockRepoLocationReader) Get(_ context.Context, _ string) (*store.RepoLocation, error) {
	return m.location, m.err
}

type mockPRCheckoutInspector struct {
	inspection git.CheckoutInspection
	err        error
}

func (m *mockPRCheckoutInspector) InspectPRCheckout(_, _ string) (git.CheckoutInspection, error) {
	return m.inspection, m.err
}

type mockPRCheckoutChecker struct {
	err error
}

func (m *mockPRCheckoutChecker) CheckoutPRHead(_ context.Context, _, _ string) error {
	return m.err
}

func checkoutMsg(t *testing.T, msgType, prID string) ws.InboundMessage {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"pr_id": prID})
	return ws.InboundMessage{ID: "req-1", Type: msgType, Data: data}
}

func TestCheckPRCheckoutHandler_HeadMatches(t *testing.T) {
	prs := &mockPRCheckoutInfoReader{info: &store.PRCheckoutInfo{RepoID: "github:o/r", HeadSHA: "sha1"}}
	repos := &mockRepoLocationReader{location: &store.RepoLocation{LocalPath: "/tmp/repo"}}
	inspector := &mockPRCheckoutInspector{inspection: git.CheckoutInspection{HeadMatches: true, SHAPresent: true}}

	h := NewCheckPRCheckoutHandler(prs, repos, inspector, testRepoLogger())
	resp, err := h(context.Background(), checkoutMsg(t, "CHECK_PR_CHECKOUT", "pr-1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := extractData(t, resp)
	if data["head_matches"] != true || data["sha_present"] != true {
		t.Errorf("unexpected status: %+v", data)
	}
}

func TestCheckPRCheckoutHandler_NoLocalPath(t *testing.T) {
	prs := &mockPRCheckoutInfoReader{info: &store.PRCheckoutInfo{RepoID: "github:o/r", HeadSHA: "sha1"}}
	repos := &mockRepoLocationReader{location: nil}
	inspector := &mockPRCheckoutInspector{}

	h := NewCheckPRCheckoutHandler(prs, repos, inspector, testRepoLogger())
	resp, err := h(context.Background(), checkoutMsg(t, "CHECK_PR_CHECKOUT", "pr-1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := extractData(t, resp)
	if data["error"] != "no local path" {
		t.Errorf("expected 'no local path' error, got %+v", data)
	}
}

func TestCheckPRCheckoutHandler_MissingPRID(t *testing.T) {
	h := NewCheckPRCheckoutHandler(&mockPRCheckoutInfoReader{}, &mockRepoLocationReader{}, &mockPRCheckoutInspector{}, testRepoLogger())
	data, _ := json.Marshal(map[string]string{})
	if _, err := h(context.Background(), ws.InboundMessage{ID: "x", Data: data}); err == nil {
		t.Error("expected error for missing pr_id")
	}
}

func TestCheckoutPRHeadHandler_Success(t *testing.T) {
	prs := &mockPRCheckoutInfoReader{info: &store.PRCheckoutInfo{RepoID: "github:o/r", HeadSHA: "sha1"}}
	repos := &mockRepoLocationReader{location: &store.RepoLocation{LocalPath: "/tmp/repo"}}
	checker := &mockPRCheckoutChecker{}

	h := NewCheckoutPRHeadHandler(prs, repos, checker, testRepoLogger())
	resp, err := h(context.Background(), checkoutMsg(t, "CHECKOUT_PR_HEAD", "pr-1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := extractData(t, resp)
	if data["ok"] != true {
		t.Errorf("expected ok=true, got %+v", data)
	}
}

func TestCheckoutPRHeadHandler_DirtyWorktree(t *testing.T) {
	prs := &mockPRCheckoutInfoReader{info: &store.PRCheckoutInfo{RepoID: "github:o/r", HeadSHA: "sha1"}}
	repos := &mockRepoLocationReader{location: &store.RepoLocation{LocalPath: "/tmp/repo"}}
	checker := &mockPRCheckoutChecker{err: git.ErrWorktreeDirty}

	h := NewCheckoutPRHeadHandler(prs, repos, checker, testRepoLogger())
	resp, err := h(context.Background(), checkoutMsg(t, "CHECKOUT_PR_HEAD", "pr-1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := extractData(t, resp)
	if data["ok"] != false || data["reason"] != "worktree dirty" {
		t.Errorf("expected ok=false reason='worktree dirty', got %+v", data)
	}
}

func TestCheckoutPRHeadHandler_PRNotFound(t *testing.T) {
	prs := &mockPRCheckoutInfoReader{info: nil}
	repos := &mockRepoLocationReader{}
	checker := &mockPRCheckoutChecker{}

	h := NewCheckoutPRHeadHandler(prs, repos, checker, testRepoLogger())
	resp, err := h(context.Background(), checkoutMsg(t, "CHECKOUT_PR_HEAD", "pr-missing"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := extractData(t, resp)
	if data["reason"] != "PR not found" {
		t.Errorf("expected 'PR not found', got %+v", data)
	}
}
