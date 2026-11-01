package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/repolocation"
)

type mockPRCheckoutInfoReader struct {
	info *store.PRCheckoutInfo
	err  error
	prID string
}

func (m *mockPRCheckoutInfoReader) GetPRCheckoutInfo(_ context.Context, prID string) (*store.PRCheckoutInfo, error) {
	m.prID = prID
	return m.info, m.err
}

type mockRepoLocationReader struct {
	location *repolocation.Location
	err      error
}

func (m *mockRepoLocationReader) Get(context.Context, string) (*repolocation.Location, error) {
	return m.location, m.err
}

type mockPRCheckoutInspector struct {
	inspection git.CheckoutInspection
	err        error
}

func (m *mockPRCheckoutInspector) InspectPRCheckout(string, string) (git.CheckoutInspection, error) {
	return m.inspection, m.err
}

type mockPRCheckoutChecker struct {
	err       error
	repoID    string
	localPath string
	headSHA   string
}

func (m *mockPRCheckoutChecker) CheckoutPRHead(_ context.Context, repoID, localPath, headSHA string) error {
	m.repoID, m.localPath, m.headSHA = repoID, localPath, headSHA
	return m.err
}

func newCheckoutService() *Editor {
	return &Editor{
		PRs:       &mockPRCheckoutInfoReader{info: &store.PRCheckoutInfo{RepoID: "github:owner/repo", HeadSHA: "sha1"}},
		Repos:     &mockRepoLocationReader{location: &repolocation.Location{LocalPath: "/tmp/repo", Source: "local", LocationStatus: "ok"}},
		Inspector: &mockPRCheckoutInspector{inspection: git.CheckoutInspection{HeadMatches: true}},
		Checker:   &mockPRCheckoutChecker{},
	}
}

func TestCheckoutStatusAndHeadUseResolvedTarget(t *testing.T) {
	s := newCheckoutService()
	got, err := s.CheckoutStatus(context.Background(), PRRef{PRID: "GitHub:Owner/Repo#1"})
	if err != nil || got.PRID != "github:owner/repo#1" || got.LocalPath != "/tmp/repo" || got.HeadSHA != "sha1" || !got.HeadMatches {
		t.Fatalf("status = %+v, error = %v", got, err)
	}
	result, err := s.CheckoutHead(context.Background(), CheckoutHeadRequest{PRID: "GitHub:Owner/Repo#1", ExpectedHeadSHA: "sha1"})
	checker := s.Checker.(*mockPRCheckoutChecker)
	if err != nil || result.PRID != got.PRID || result.LocalPath != got.LocalPath || checker.repoID != "github:owner/repo" || checker.localPath != "/tmp/repo" || checker.headSHA != "sha1" || s.PRs.(*mockPRCheckoutInfoReader).prID != got.PRID {
		t.Fatalf("checkout result = %+v, error = %v", result, err)
	}
}

func TestCheckoutRefusals(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Editor)
		reason string
		code   Code
	}{
		{"missing PR", func(s *Editor) { s.PRs.(*mockPRCheckoutInfoReader).info = nil }, "PR not found", CodeNotFound},
		{"missing head", func(s *Editor) { s.PRs.(*mockPRCheckoutInfoReader).info.HeadSHA = "" }, "PR head SHA missing", CodeInvalid},
		{"no path", func(s *Editor) { s.Repos.(*mockRepoLocationReader).location = nil }, "no local path", CodeInvalid},
		{"cache clone", func(s *Editor) { s.Repos.(*mockRepoLocationReader).location.Source = "cache" }, "no local checkout linked — link a local checkout for this repo first", CodeInvalid},
		{"broken link", func(s *Editor) { s.Repos.(*mockRepoLocationReader).location.LocationStatus = "error" }, "no local checkout linked — link a local checkout for this repo first", CodeInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newCheckoutService()
			test.change(s)
			_, statusErr := s.CheckoutStatus(context.Background(), PRRef{PRID: "pr-1"})
			_, checkoutErr := s.CheckoutHead(context.Background(), CheckoutHeadRequest{PRID: "pr-1"})
			for _, err := range []error{statusErr, checkoutErr} {
				var se *ServiceError
				if !errors.As(err, &se) || se.Code != test.code || se.Reason != test.reason {
					t.Fatalf("error = %v", err)
				}
			}
			if s.Checker.(*mockPRCheckoutChecker).localPath != "" {
				t.Fatal("refused checkout still mutated worktree")
			}
		})
	}
}

func TestCheckoutHeadRefusesMovedHead(t *testing.T) {
	s := newCheckoutService()
	_, err := s.CheckoutHead(context.Background(), CheckoutHeadRequest{PRID: "pr-1", ExpectedHeadSHA: "old-sha"})
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeInvalid || se.Reason != "PR head changed since it was inspected; try again" {
		t.Fatalf("error = %v", err)
	}
	if s.Checker.(*mockPRCheckoutChecker).localPath != "" {
		t.Fatal("moved head still checked out")
	}
}

func TestCheckoutDependencyErrors(t *testing.T) {
	for _, cause := range []error{errors.New("storage unavailable"), fmt.Errorf("wrapped: %w", context.Canceled), context.DeadlineExceeded} {
		for _, dependency := range []string{"PRs", "Repos", "Inspector", "Checker"} {
			s := newCheckoutService()
			want := CodeInternal
			switch dependency {
			case "PRs":
				s.PRs.(*mockPRCheckoutInfoReader).err = cause
			case "Repos":
				s.Repos.(*mockRepoLocationReader).err = cause
			case "Inspector":
				s.Inspector.(*mockPRCheckoutInspector).err = cause
				want = CodeInvalid
			case "Checker":
				s.Checker.(*mockPRCheckoutChecker).err = cause
				want = CodeInvalid
			}
			if errors.Is(cause, context.Canceled) {
				want = CodeCancelled
			}
			if errors.Is(cause, context.DeadlineExceeded) {
				want = CodeTimeout
			}
			var err error
			if dependency == "Checker" {
				_, err = s.CheckoutHead(context.Background(), CheckoutHeadRequest{PRID: "pr-1"})
			} else {
				_, err = s.CheckoutStatus(context.Background(), PRRef{PRID: "pr-1"})
			}
			var se *ServiceError
			if !errors.As(err, &se) || se.Code != want {
				t.Fatalf("%s error = %v, want %s", dependency, err, want)
			}
		}
	}
	s := newCheckoutService()
	s.Checker.(*mockPRCheckoutChecker).err = git.ErrWorktreeDirty
	_, err := s.CheckoutHead(context.Background(), CheckoutHeadRequest{PRID: "pr-1"})
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeInvalid || se.Reason != "worktree dirty" {
		t.Fatalf("dirty error = %v", err)
	}
}

func TestCheckoutRequiresPRID(t *testing.T) {
	s := newCheckoutService()
	_, statusErr := s.CheckoutStatus(context.Background(), PRRef{})
	_, headErr := s.CheckoutHead(context.Background(), CheckoutHeadRequest{})
	for _, err := range []error{statusErr, headErr} {
		var se *ServiceError
		if !errors.As(err, &se) || se.Code != CodeInvalid {
			t.Fatalf("error = %v", err)
		}
	}
}
