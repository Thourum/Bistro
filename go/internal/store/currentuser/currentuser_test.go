package currentuser

import (
	"context"
	"github.com/itterno/bistro/internal/store/storetest"
	"testing"
)

func TestCurrentUserStore_SetAndGet(t *testing.T) {
	db := storetest.Open(t)
	s := New(db)
	ctx := context.Background()

	// Get on empty store returns "", false
	u, ok, err := s.Get(ctx, "github")
	if err != nil {
		t.Fatalf("empty store: unexpected error: %v", err)
	}
	if ok || u != "" {
		t.Errorf("empty store: got (%q, %v) want ('', false)", u, ok)
	}

	// Set upserts
	if err := s.Set(ctx, "github", "alice"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	u, ok, err = s.Get(ctx, "github")
	if err != nil {
		t.Fatalf("after set: unexpected error: %v", err)
	}
	if !ok || u != "alice" {
		t.Errorf("after set: got (%q, %v) want ('alice', true)", u, ok)
	}

	// Upsert replaces
	if err := s.Set(ctx, "github", "bob"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	u, ok, err = s.Get(ctx, "github")
	if err != nil {
		t.Fatalf("after overwrite: unexpected error: %v", err)
	}
	if !ok || u != "bob" {
		t.Errorf("after overwrite: got (%q, %v) want ('bob', true)", u, ok)
	}

	// Different provider is isolated
	if err := s.Set(ctx, "gitlab", "carol"); err != nil {
		t.Fatalf("Set gitlab: %v", err)
	}
	u, _, _ = s.Get(ctx, "github")
	if u != "bob" {
		t.Errorf("github should still be 'bob', got %q", u)
	}

	if err := s.Delete(ctx, "github"); err != nil {
		t.Fatalf("Delete github: %v", err)
	}
	u, ok, err = s.Get(ctx, "github")
	if err != nil {
		t.Fatalf("after delete: unexpected error: %v", err)
	}
	if ok || u != "" {
		t.Errorf("after delete: got (%q, %v) want ('', false)", u, ok)
	}
	u, ok, err = s.Get(ctx, "gitlab")
	if err != nil {
		t.Fatalf("gitlab after github delete: unexpected error: %v", err)
	}
	if !ok || u != "carol" {
		t.Errorf("gitlab should remain carol after github delete, got (%q, %v)", u, ok)
	}
}
