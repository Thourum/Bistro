# Task 1: `PatchID` fingerprint + `DiffFile.PatchID`

**Files:**
- Create: `go/internal/git/patchid.go`
- Create: `go/internal/git/patchid_test.go`
- Modify: `go/internal/git/types.go` (`DiffFile` struct)
- Modify: `go/internal/git/diff.go` (`renderDiffFile`)

**Interfaces:**
- Consumes: nothing.
- Produces: `func PatchID(patch string) string` (hex SHA-256); `git.DiffFile.PatchID string`, set for every file returned by `GenerateDiff`, computed from the **full** patch before budget truncation; binary files fingerprint their blob hashes.

**Why:** `patch_id` must ignore `@@` line-number shifts (spec §2) and the `index <old>..<new>` blob-hash line (it changes whenever the base version of the file changes, even if the PR's change is identical). Truncated/too-large files store an empty patch, so the fingerprint must come from the unrendered full patch or every big file would look "unchanged".

- [ ] **Step 1: Write the failing tests**

`go/internal/git/patchid_test.go`:

```go
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
```

(`makeDiffTestRepo` already exists in `diff_test.go`: `makeDiffTestRepo(t, baseFiles, headFiles map[string][]byte) (dir, baseSHA, headSHA string)`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go && go test ./internal/git/ -run 'PatchID' -v`
Expected: FAIL — `undefined: PatchID` / `Files[0].PatchID undefined`.

- [ ] **Step 3: Implement `PatchID`**

`go/internal/git/patchid.go`:

```go
package git

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// PatchID fingerprints a per-file patch so a rebase that only moves the change
// keeps the same ID, in the spirit of `git patch-id`. Hunk header ranges and
// the `index` blob-hash line are dropped; hunk boundaries and every changed or
// context line still count.
func PatchID(patch string) string {
	var normalized strings.Builder
	for _, line := range strings.SplitAfter(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "index "):
			continue
		case strings.HasPrefix(line, "@@"):
			normalized.WriteString("@@\n")
		default:
			normalized.WriteString(line)
		}
	}
	sum := sha256.Sum256([]byte(normalized.String()))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Add `PatchID` to `git.DiffFile` and set it in `renderDiffFile`**

In `go/internal/git/types.go`, add to `DiffFile` (after `Patch`/before `AddedLines`, keep the existing comment style):

```go
	PatchID      string // PatchID of the full patch (before budget truncation)
```

In `go/internal/git/diff.go` `renderDiffFile`, right after `patchStr, err := filePatchToString(filePatch)` and its error check, compute:

```go
	patchID := PatchID(patchStr)
	if isBinary {
		// Binary patches render as "Binary files differ"; the blob IDs are
		// what identify the content.
		patchID = PatchID(fmt.Sprintf("binary %s %s\n", fileHash(from), fileHash(to)))
	}
```

and set `PatchID: patchID,` in the returned `&DiffFile{...}` literal. Add the helper in `diff.go`:

```go
// fileHash returns the blob ID of one side of a file patch; "" when the side
// does not exist (added or deleted file).
func fileHash(file fdiff.File) string {
	if file == nil {
		return ""
	}
	return file.Hash().String()
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd go && go test ./internal/git/ -run 'PatchID' -v`
Expected: PASS (3 tests).

- [ ] **Step 6: Full Go verification**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go/internal/git/patchid.go go/internal/git/patchid_test.go go/internal/git/types.go go/internal/git/diff.go
git commit -m "feat: add line-shift-insensitive per-file patch ID"
```
