package store

import (
	"context"
	"errors"
	"testing"
)

func TestSyncCursorSuccessAndErrorDoNotAdvanceOnFailure(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	scope := "github:inbox:alice:search:involves"

	if err := db.SaveSyncCursorSuccess(ctx, scope, "2026-06-29T22:00:00Z", "etag-1"); err != nil {
		t.Fatalf("SaveSyncCursorSuccess: %v", err)
	}
	if err := db.MarkSyncCursorError(ctx, scope, errors.New("network interrupted")); err != nil {
		t.Fatalf("MarkSyncCursorError: %v", err)
	}

	cur, err := db.GetSyncCursor(ctx, scope)
	if err != nil {
		t.Fatalf("GetSyncCursor: %v", err)
	}
	if cur == nil {
		t.Fatal("expected cursor")
	}
	if cur.CursorValue != "2026-06-29T22:00:00Z" {
		t.Fatalf("cursor advanced on error: %q", cur.CursorValue)
	}
	if cur.ETag != "etag-1" {
		t.Fatalf("etag changed on error: %q", cur.ETag)
	}
	if !cur.LastErrorAt.Valid || !cur.ErrorJSON.Valid {
		t.Fatalf("expected error metadata to be recorded: %+v", cur)
	}
}
