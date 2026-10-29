package services

import (
	"context"
	"errors"
	"testing"

	"github.com/itterno/bistro/internal/services/servicestest"
	"github.com/itterno/bistro/internal/store/reviewdraft"
	"github.com/itterno/bistro/internal/store/storetest"
)

type draftUser string

func (f draftUser) Get(context.Context, string) (string, bool, error) { return string(f), f != "", nil }

func TestDraftSaveRejectsInvalidPositions(t *testing.T) {
	s := &Drafts{Drafts: reviewdraft.New(storetest.Open(t).DB), CurrentUser: draftUser("me"), Events: &servicestest.Recorder{}}
	valid := DraftSaveRequest{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 4, Body: "x", CommitSHA: "head"}
	for name, change := range map[string]func(*DraftSaveRequest){
		"bad side":       func(r *DraftSaveRequest) { r.Side = "MIDDLE" },
		"start >= line":  func(r *DraftSaveRequest) { r.StartLine = 4 },
		"blank body":     func(r *DraftSaveRequest) { r.Body = " " },
		"missing commit": func(r *DraftSaveRequest) { r.CommitSHA = "" },
		"zero line":      func(r *DraftSaveRequest) { r.Line = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			req := valid
			change(&req)
			_, err := s.Save(context.Background(), req)
			var se *ServiceError
			if !errors.As(err, &se) || se.Code != CodeInvalid {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
func TestDraftSaveListDeleteEmitsNormalizedChanges(t *testing.T) {
	ctx := context.Background()
	events := &servicestest.Recorder{}
	s := &Drafts{Drafts: reviewdraft.New(storetest.Open(t).DB), CurrentUser: draftUser("me"), Events: events}
	saved, err := s.Save(ctx, DraftSaveRequest{PRID: "github:OWNER/REPO:1", Path: "a.go", Side: "LEFT", StartLine: 2, Line: 4, Body: " review ", CommitSHA: "head"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Draft.PRID != "github:owner/repo:1" || saved.Draft.StartLine != 2 || saved.Draft.Body != "review" {
		t.Fatalf("saved = %+v", saved)
	}
	emitted := events.Named(EventDraftsChanged)
	if len(emitted) != 1 || emitted[0] != (DraftsChanged{PRID: "github:owner/repo:1"}) {
		t.Fatalf("events = %+v", events.Events())
	}
	updated, err := s.Save(ctx, DraftSaveRequest{PRID: saved.Draft.PRID, ID: saved.Draft.ID, Body: "edited"})
	if err != nil || updated.Draft.Body != "edited" || updated.Draft.Line != 4 {
		t.Fatalf("updated = %+v, %v", updated, err)
	}
	listed, err := s.List(ctx, PRRef{PRID: saved.Draft.PRID})
	if err != nil || len(listed.Drafts) != 1 {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	s.CurrentUser = draftUser("other")
	listed, err = s.List(ctx, PRRef{PRID: saved.Draft.PRID})
	if err != nil || len(listed.Drafts) != 0 {
		t.Fatalf("other user's list = %+v, %v", listed, err)
	}
	s.CurrentUser = draftUser("me")
	deleted, err := s.Delete(ctx, DraftDeleteRequest{PRID: saved.Draft.PRID, DraftIDs: []string{saved.Draft.ID}})
	if err != nil || deleted.Deleted != 1 {
		t.Fatalf("delete = %+v, %v", deleted, err)
	}
	listed, err = s.List(ctx, PRRef{PRID: saved.Draft.PRID})
	if err != nil || len(listed.Drafts) != 0 {
		t.Fatalf("list after delete = %+v, %v", listed, err)
	}
	// The old handler counts requested IDs and broadcasts even when no row exists.
	deleted, err = s.Delete(ctx, DraftDeleteRequest{PRID: saved.Draft.PRID, DraftIDs: []string{"unknown"}})
	if err != nil || deleted.Deleted != 1 || len(events.Named(EventDraftsChanged)) != 4 {
		t.Fatalf("unknown delete = %+v, %v; events = %+v", deleted, err, events.Events())
	}
}
func TestDraftsRequireAccount(t *testing.T) {
	s := &Drafts{CurrentUser: draftUser("")}
	for name, call := range map[string]func() error{
		"list": func() error { _, err := s.List(context.Background(), PRRef{PRID: "pr-1"}); return err },
		"save": func() error {
			_, err := s.Save(context.Background(), DraftSaveRequest{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 1, Body: "x", CommitSHA: "head"})
			return err
		},
		"delete": func() error {
			_, err := s.Delete(context.Background(), DraftDeleteRequest{PRID: "pr-1", DraftIDs: []string{"id"}})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			var se *ServiceError
			if !errors.As(err, &se) || se.Code != CodeUnauthenticated {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

type draftIdentityFailure struct{ err error }

func (f draftIdentityFailure) Get(context.Context, string) (string, bool, error) {
	return "", false, f.err
}
func TestDraftIdentityFailures(t *testing.T) {
	for _, tt := range []struct {
		err  error
		code Code
	}{{errors.New("identity store unavailable"), CodeInternal}, {context.Canceled, CodeCancelled}, {context.DeadlineExceeded, CodeTimeout}} {
		service := &Drafts{CurrentUser: draftIdentityFailure{err: tt.err}}
		_, err := service.List(context.Background(), PRRef{PRID: "pr-1"})
		var se *ServiceError
		if !errors.As(err, &se) || se.Code != tt.code {
			t.Fatalf("error = %v, want %s", err, tt.code)
		}
	}
}
