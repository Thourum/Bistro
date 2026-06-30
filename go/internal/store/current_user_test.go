package store

import (
	"context"
	"testing"
)

func TestCurrentUserStore_SetAndGet(t *testing.T) {
	db := newTestDB(t)
	s := NewCurrentUserStore(db)
	ctx := context.Background()

	// GetCurrentUser on empty store returns "", false
	u, ok, err := s.GetCurrentUser(ctx, "github")
	if err != nil {
		t.Fatalf("empty store: unexpected error: %v", err)
	}
	if ok || u != "" {
		t.Errorf("empty store: got (%q, %v) want ('', false)", u, ok)
	}

	// SetCurrentUser upserts
	if err := s.SetCurrentUser(ctx, "github", "alice"); err != nil {
		t.Fatalf("SetCurrentUser: %v", err)
	}
	u, ok, err = s.GetCurrentUser(ctx, "github")
	if err != nil {
		t.Fatalf("after set: unexpected error: %v", err)
	}
	if !ok || u != "alice" {
		t.Errorf("after set: got (%q, %v) want ('alice', true)", u, ok)
	}

	// Upsert replaces
	if err := s.SetCurrentUser(ctx, "github", "bob"); err != nil {
		t.Fatalf("SetCurrentUser overwrite: %v", err)
	}
	u, ok, err = s.GetCurrentUser(ctx, "github")
	if err != nil {
		t.Fatalf("after overwrite: unexpected error: %v", err)
	}
	if !ok || u != "bob" {
		t.Errorf("after overwrite: got (%q, %v) want ('bob', true)", u, ok)
	}

	// Different provider is isolated
	if err := s.SetCurrentUser(ctx, "gitlab", "carol"); err != nil {
		t.Fatalf("SetCurrentUser gitlab: %v", err)
	}
	u, _, _ = s.GetCurrentUser(ctx, "github")
	if u != "bob" {
		t.Errorf("github should still be 'bob', got %q", u)
	}

	if err := s.DeleteCurrentUser(ctx, "github"); err != nil {
		t.Fatalf("DeleteCurrentUser github: %v", err)
	}
	u, ok, err = s.GetCurrentUser(ctx, "github")
	if err != nil {
		t.Fatalf("after delete: unexpected error: %v", err)
	}
	if ok || u != "" {
		t.Errorf("after delete: got (%q, %v) want ('', false)", u, ok)
	}
	u, ok, err = s.GetCurrentUser(ctx, "gitlab")
	if err != nil {
		t.Fatalf("gitlab after github delete: unexpected error: %v", err)
	}
	if !ok || u != "carol" {
		t.Errorf("gitlab should remain carol after github delete, got (%q, %v)", u, ok)
	}
}
