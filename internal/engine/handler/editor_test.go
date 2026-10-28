package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/repolocation"
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
	location *repolocation.Location
	err      error
}

func (m *mockRepoLocationReader) Get(_ context.Context, _ string) (*repolocation.Location, error) {
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
	// gotRepoID records the repoID the handler passed through, so tests can
	// assert the per-repo lock key reaches the git client.
	gotRepoID   string
	gotLocalPth string
}

func (m *mockPRCheckoutChecker) CheckoutPRHead(_ context.Context, repoID, localPath, _ string) error {
	m.gotRepoID = repoID
	m.gotLocalPth = localPath
	return m.err
}

func checkoutMsg(t *testing.T, msgType, prID string) ws.InboundMessage {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"pr_id": prID})
	return ws.InboundMessage{ID: "req-1", Type: msgType, Data: data}
}

func TestCheckPRCheckoutHandler_HeadMatches(t *testing.T) {
	prs := &mockPRCheckoutInfoReader{info: &store.PRCheckoutInfo{RepoID: "github:o/r", HeadSHA: "sha1"}}
	repos := &mockRepoLocationReader{location: &repolocation.Location{LocalPath: "/tmp/repo", Source: "local", LocationStatus: "ok"}}
	inspector := &mockPRCheckoutInspector{inspection: git.CheckoutInspection{HeadMatches: true}}

	h := NewCheckPRCheckoutHandler(prs, repos, inspector, testRepoLogger())
	resp, err := h(context.Background(), checkoutMsg(t, "CHECK_PR_CHECKOUT", "pr-1"))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	data := extractData(t, resp)
	if data["head_matches"] != true || data["head_sha"] != "sha1" {
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
	repos := &mockRepoLocationReader{location: &repolocation.Location{LocalPath: "/tmp/repo", Source: "local", LocationStatus: "ok"}}
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
	repos := &mockRepoLocationReader{location: &repolocation.Location{LocalPath: "/tmp/repo", Source: "local", LocationStatus: "ok"}}
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

// TestCheckoutPRHeadHandler_PassesDBResolvedTarget pins the trust boundary and
// the lock key: both the repo_id used for per-repo locking and the checkout
// path come from the engine's own DB state (prs.repo_id → repos.local_path),
// never from the WS request, which carries only pr_id. Only a linked local
// checkout (source "local", status "ok") qualifies — bare cache clones are
// refused with a "link a local checkout" reason.
func TestCheckoutPRHeadHandler_PassesDBResolvedTarget(t *testing.T) {
	tests := []struct {
		name          string
		location      *repolocation.Location
		wantLocalPath string
		wantReason    string
	}{
		{
			name:          "local checkout",
			location:      &repolocation.Location{LocalPath: "/home/dev/src/repo", Source: "local", LocationStatus: "ok"},
			wantLocalPath: "/home/dev/src/repo",
		},
		{
			name:       "cache clone refused",
			location:   &repolocation.Location{LocalPath: "/home/dev/.cache/bistro/abc123", Source: "cache", LocationStatus: "ok"},
			wantReason: "no local checkout linked — link a local checkout for this repo first",
		},
		{
			name:       "broken local link refused",
			location:   &repolocation.Location{LocalPath: "/home/dev/src/repo", Source: "local", LocationStatus: "error"},
			wantReason: "no local checkout linked — link a local checkout for this repo first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prs := &mockPRCheckoutInfoReader{info: &store.PRCheckoutInfo{RepoID: "github:owner/repo", HeadSHA: "sha1"}}
			repos := &mockRepoLocationReader{location: tt.location}
			checker := &mockPRCheckoutChecker{}

			h := NewCheckoutPRHeadHandler(prs, repos, checker, testRepoLogger())
			resp, err := h(context.Background(), checkoutMsg(t, "CHECKOUT_PR_HEAD", "pr-1"))
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if checker.gotLocalPth != tt.wantLocalPath {
				t.Errorf("localPath passed to CheckoutPRHead = %q, want %q", checker.gotLocalPth, tt.wantLocalPath)
			}
			if tt.wantReason != "" {
				if data := extractData(t, resp); data["reason"] != tt.wantReason {
					t.Errorf("reason = %v, want %q", data["reason"], tt.wantReason)
				}
			} else if checker.gotRepoID != "github:owner/repo" {
				t.Errorf("repoID passed to CheckoutPRHead = %q", checker.gotRepoID)
			}
		})
	}
}

// A force-push between CHECK_PR_CHECKOUT and CHECKOUT_PR_HEAD must not land
// the user on a revision they never inspected.
func TestCheckoutPRHeadHandler_RefusesWhenHeadMoved(t *testing.T) {
	prs := &mockPRCheckoutInfoReader{info: &store.PRCheckoutInfo{RepoID: "github:o/r", HeadSHA: "sha2"}}
	repos := &mockRepoLocationReader{location: &repolocation.Location{LocalPath: "/tmp/repo", Source: "local", LocationStatus: "ok"}}
	checker := &mockPRCheckoutChecker{}

	h := NewCheckoutPRHeadHandler(prs, repos, checker, testRepoLogger())
	data, _ := json.Marshal(map[string]string{"pr_id": "pr-1", "expected_head_sha": "sha1"})
	resp, err := h(context.Background(), ws.InboundMessage{ID: "req-1", Type: "CHECKOUT_PR_HEAD", Data: data})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	out := extractData(t, resp)
	if out["ok"] != false || out["reason"] != "PR head changed since it was inspected; try again" {
		t.Errorf("expected refusal, got %+v", out)
	}
	if checker.gotLocalPth != "" {
		t.Error("CheckoutPRHead must not run when head moved")
	}
}
