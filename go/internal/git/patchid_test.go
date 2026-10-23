package git

import (
	"strings"
	"testing"
)

const patchIDBase = "diff --git a/f.go b/f.go\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/f.go\n" +
	"+++ b/f.go\n" +
	"@@ -10,3 +10,4 @@ func A() {\n" +
	" a\n" +
	"+b\n" +
	" c\n"

func TestPatchIDIgnoresLineShiftsAndBlobHashes(t *testing.T) {
	shifted := strings.NewReplacer(
		"index 1111111..2222222", "index 3333333..4444444",
		"@@ -10,3 +10,4 @@ func A() {", "@@ -40,3 +42,4 @@ func Z() {",
	).Replace(patchIDBase)
	if PatchID(patchIDBase) != PatchID(shifted) {
		t.Error("PatchID changed when only hunk line numbers and blob hashes moved")
	}
}

func TestPatchIDChangesWhenContentChanges(t *testing.T) {
	edited := strings.Replace(patchIDBase, "+b\n", "+B\n", 1)
	if PatchID(patchIDBase) == PatchID(edited) {
		t.Error("PatchID did not change when an added line changed")
	}
}

func TestGenerateDiffBinaryPatchIDTracksContent(t *testing.T) {
	base := map[string][]byte{"img.bin": {0, 1, 2}}
	firstDir, firstBase, firstHead := makeDiffTestRepo(t, base, map[string][]byte{"img.bin": {0, 1, 3}})
	secondDir, secondBase, secondHead := makeDiffTestRepo(t, base, map[string][]byte{"img.bin": {0, 1, 4}})
	client := NewClient(nil)
	first, err := client.GenerateDiff(firstDir, firstBase, firstHead, DefaultDiffOptions())
	if err != nil {
		t.Fatalf("GenerateDiff first: %v", err)
	}
	second, err := client.GenerateDiff(secondDir, secondBase, secondHead, DefaultDiffOptions())
	if err != nil {
		t.Fatalf("GenerateDiff second: %v", err)
	}
	if first.Files[0].PatchID == "" || first.Files[0].PatchID == second.Files[0].PatchID {
		t.Errorf("binary PatchIDs should be set and differ: %q vs %q",
			first.Files[0].PatchID, second.Files[0].PatchID)
	}
}

func TestGenerateDiffPatchIDSurvivesBudgetTruncation(t *testing.T) {
	dir, baseSHA, headSHA := makeDiffTestRepo(t,
		map[string][]byte{"f.go": []byte("package main\n")},
		map[string][]byte{"f.go": []byte("package main\n// added\n")},
	)
	client := NewClient(nil)
	full, err := client.GenerateDiff(dir, baseSHA, headSHA, DiffOptions{})
	if err != nil {
		t.Fatalf("GenerateDiff full: %v", err)
	}
	if len(full.Files) != 1 || full.Files[0].Patch == "" {
		t.Fatalf("expected one full patch, got %+v", full.Files)
	}
	expectedID := PatchID(full.Files[0].Patch)
	if full.Files[0].PatchID != expectedID {
		t.Fatalf("full PatchID = %q, want %q", full.Files[0].PatchID, expectedID)
	}
	for _, test := range []struct {
		name   string
		opts   DiffOptions
		status string
	}{
		{name: "per-file budget", opts: DiffOptions{MaxFilePatchBytes: 1}, status: DiffStatusTruncated},
		{name: "PR budget", opts: DiffOptions{MaxTotalPatchBytes: 1}, status: DiffStatusTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			limited, err := client.GenerateDiff(dir, baseSHA, headSHA, test.opts)
			if err != nil {
				t.Fatalf("GenerateDiff limited: %v", err)
			}
			if len(limited.Files) != 1 {
				t.Fatalf("expected one limited file, got %d", len(limited.Files))
			}
			file := limited.Files[0]
			if file.Patch != "" || file.DiffStatus != test.status {
				t.Fatalf("expected empty %s patch, got %+v", test.status, file)
			}
			if file.PatchID != expectedID {
				t.Errorf("limited PatchID = %q, want full patch ID %q", file.PatchID, expectedID)
			}
		})
	}
}
