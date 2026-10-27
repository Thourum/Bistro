package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"github.com/itterno/bistro/internal/store"
)

func TestSeedPersistsHydratedBenchmarkDiffs(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "nested", "sync.db")
	// A second run must replace the cache without adding another patchset.
	for range 2 {
		if err := run(ctx, dbPath); err != nil {
			t.Fatalf("seed database: %v", err)
		}
	}
	db, err := store.OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	}()

	for _, expected := range []struct {
		prID       string
		title      string
		files      int
		lines      int
		diffSHA256 string
	}{
		{prID: "github:bench/large-diff:1", title: "bench: 1000 files x 10 changed lines", files: 1000, lines: 10, diffSHA256: "5855a0e49cfc7cf1b9d103e19016f7beb48157196e5e8c7f0135b0db8216241f"},
		{prID: "github:bench/large-diff:2", title: "bench: 1 file x 10000 changed lines", files: 1, lines: 10000, diffSHA256: "47165315dbad3fd48720d2714a7d6b1426bc53b35f46bb8ddd7fc574b3f1df84"},
	} {
		t.Run(expected.prID, func(t *testing.T) {
			var repoID, title, state, headSHA, baseSHA string
			var hydrated int
			if err := db.QueryRowContext(ctx, `SELECT repo_id, title, state, head_sha, base_sha, hydrated FROM prs WHERE id = ?`, expected.prID).
				Scan(&repoID, &title, &state, &headSHA, &baseSHA, &hydrated); err != nil {
				t.Fatal(err)
			}
			if repoID != "github:bench/large-diff" || title != expected.title || state != "open" || hydrated != 1 {
				t.Fatalf("unexpected PR metadata: repo=%q title=%q state=%q hydrated=%d", repoID, title, state, hydrated)
			}
			patchsets, err := db.ListPatchsets(ctx, expected.prID)
			if err != nil {
				t.Fatal(err)
			}
			if len(patchsets) != 1 || patchsets[0].HeadSHA != headSHA || patchsets[0].BaseSHA != baseSHA || patchsets[0].MergeBaseSHA != baseSHA || !patchsets[0].HasFingerprints {
				t.Fatalf("unexpected patchsets: %+v", patchsets)
			}
			fingerprints, err := db.ListPatchsetFiles(ctx, expected.prID, patchsets[0].Number)
			if err != nil {
				t.Fatal(err)
			}
			diffs, err := store.NewDiffStore(db.DB).GetDiffs(expected.prID)
			if err != nil {
				t.Fatal(err)
			}
			if len(diffs) != expected.files || len(fingerprints) != expected.files {
				t.Fatalf("file counts: diffs=%d fingerprints=%d want=%d", len(diffs), len(fingerprints), expected.files)
			}
			patches := make([]string, 0, len(diffs))
			for index, diff := range diffs {
				if diff.FilePath != fmt.Sprintf("src/generated/file_%05d.ts", index) || diff.Status != "modified" || diff.DiffStatus != "ok" || diff.IsBinary || diff.IsTruncated {
					t.Fatalf("unexpected diff metadata for file %d: %+v", index, diff)
				}
				if diff.Additions != expected.lines || diff.Deletions != expected.lines || diff.PatchBytes != len(diff.Patch) || diff.HunkCount != 1 || diff.PatchID == "" || fingerprints[diff.FilePath] != diff.PatchID {
					t.Fatalf("unexpected diff counts/fingerprint for %s", diff.FilePath)
				}
				patches = append(patches, diff.Patch)
			}
			// These hashes come from bench-large-diff.mjs generateDiff, without a trailing newline.
			if got := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(patches, "\n")))); got != expected.diffSHA256 {
				t.Fatalf("diff differs from JavaScript fixture: got %s want %s", got, expected.diffSHA256)
			}
		})
	}
}

func TestSeedRefusesEngineDatabaseLock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sync.db")
	lock := flock.New(dbPath + ".lock")
	locked, err := lock.TryLock()
	if err != nil || !locked {
		t.Fatalf("acquire engine lock: locked=%v err=%v", locked, err)
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			t.Errorf("unlock database: %v", err)
		}
	}()
	if err := run(context.Background(), dbPath); err == nil || !strings.Contains(err.Error(), "already using") {
		t.Fatalf("expected database lock rejection, got %v", err)
	}
}
