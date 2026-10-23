# Review Without Leaving the Keyboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A complete GitHub review from the keyboard: line-comment drafts submitted as one review, review actions in the command palette, a resolve toggle, and (phase 2) a key-remapping UI.

**Architecture:** The Go engine owns a new `review_drafts` table and a `submit_review` action that turns the current user's drafts into one `CreateReview` call per commit SHA. `approve` / `request_changes` absorb pending drafts inside `Orchestrator.ApplyAction`. The renderer adds a `lineComment` overlay driven by a pure cursor module (`utils/lineCommentCursor.ts`). Drafts render as `@pierre/diffs` annotations. Palette entries call the keybinding handler by `actionId`.

**Tech Stack:** Go 1.26 (go-github v84, sqlc, goose migrations, SQLite), Electron + React 19 + TypeScript, TanStack Hotkeys 0.11 / react-hotkeys 0.13, @pierre/diffs 1.5.2, zod 4, `@thourum/fault`, Vitest, Biome.

**Spec:** `docs/superpowers/specs/2026-10-23-review-without-leaving-keyboard-design.md`

## Global Constraints

- Go owns all SQLite writes; the renderer never writes the DB directly.
- Electron TS: **no `try`/`catch`/`throw`**. Use `@thourum/fault` `Result` / `ResultAsync` with tagged faults. Use `fault.message` only at display/log boundaries.
- Validate external data with zod at the edge (`types/wsSchemas.ts`, `types/index.ts` + `parseResponse`). No `as` casts on WS data.
- Keybindings are pure config (keys → `actionId`) in `utils/keybindings.ts`. Handlers live in `utils/keybindingActions.ts`. Registration happens only in `hooks/useAppHotkeys.ts`.
- Plain WS requests and appStore writes live in `services/`. Hooks own React state, effects, and data-fetching lifecycles.
- Generated sqlc code in `go/internal/store/db/` is never hand-edited. Regenerate with `cd go && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`.
- No React component tests. Only utility/hook tests, and only where they would fail if the logic broke.
- Submissions are never queued for offline/rate-limited replay. They fail and the drafts stay intact.
- Package manager: pnpm. Run pnpm from `electron/`. Deps are hoisted to the repo-root `node_modules/`.
- Verify commands — Go: `cd go && go vet ./... && go test -race ./...`. Electron: `cd electron && pnpm type-check && pnpm lint && pnpm test`.
- Commit messages: `<type>: <description>` (feat, fix, refactor, docs, test, chore).

## Decisions the spec left open (made here)

1. **Mixed commit SHAs.** One GitHub review has one `commit_id`. Drafts are grouped by `commit_sha`. Each older group goes out as a body-less `COMMENT` review. The group on the current head goes last and carries the user's event and body. A failed group stops the run. Its drafts stay, and the error names them (`path:line`). GitHub's 422 does not say which comment it rejected, so the whole failed group is named.
2. **Approve / request-changes are broken today.** `services/prActions.ts` sends `APPROVE_PR` / `REQUEST_CHANGES`, and no Go handler exists for either. `toggleApproval` also needs two `a a` sequences. Task 6 routes both through `SUBMIT_ACTION`, and `a a` approves at once. The draft absorption depends on this fix.
3. **`enableLineSelection` is not used.** That option enables *pointer* selection. Programmatic highlight works through CodeView's controlled `selectedLines` prop (`CodeView.applySelectedLines` runs regardless of the option). Keyboard-first means pointer selection stays off.
4. **`x` on the detail page.** The detail page has no active inline thread. `x` there (and with no thread selected in the diff) shows a toast that points the user to `n`/`p` in the diff.
5. **Line ranges** stay within one file, one side, and one hunk. GitHub rejects ranges across hunks.
6. **Merge-base dependency.** `2026-10-23-since-last-review-design.md` is not in the repo yet. The diff is `base_sha..head_sha`, so `LEFT`-side line numbers can be wrong when the base moved. `RIGHT`-side comments are unaffected. This plan does not block on it.

---

## File map

**Go**
- Create `go/internal/store/migrations/00035_add_review_drafts.sql`: table + index.
- Create `go/internal/store/queries/review_drafts.sql`: sqlc queries. Regenerate `go/internal/store/db/`.
- Create `go/internal/store/review_drafts.go` (+ `_test.go`): `store.ReviewDrafts`.
- Modify `go/internal/provider/models.go`, `interface.go`: `ReviewComment`, `ReviewSubmission`, `ReviewSubmitter`.
- Modify `go/internal/provider/github/reviews.go` (+ `reviews_test.go`): `SubmitReview`.
- Modify `go/internal/actions/types.go`, `handlers.go`, `executor.go` (+ tests): `submit_review`.
- Create `go/internal/sync/orchestrator_review.go` (+ `_test.go`): absorb, group, submit, delete, push.
- Modify `go/internal/sync/orchestrator.go`, `orchestrator_actions.go`: `WithReviewDrafts`, hook in `ApplyAction`.
- Create `go/internal/server/handler/drafts.go` (+ `_test.go`): `DRAFT_SAVE`, `DRAFT_DELETE`, `DRAFTS_LIST`.
- Modify `go/internal/server/handler/threads.go`: `review_event` on `SUBMIT_ACTION`.
- Modify `go/internal/server/app.go`: wiring.

**Renderer** (`electron/src/renderer/`)
- Modify `types/index.ts`, `types/wsSchemas.ts`, `services/ws/events/index.ts`, `services/ws/events/review.ts`.
- Create `services/reviewDrafts.ts`, `hooks/useReviewDrafts.ts`.
- Modify `services/prActions.ts` (+ test), `store/ui.ts`, `components/ActionFeedback.tsx`.
- Create `utils/lineCommentCursor.ts` (+ test).
- Modify `overlays/OverlayContext.tsx`, `utils/keybindings.ts`, `utils/keybindingActions.ts`, `hooks/useAppHotkeys.ts`.
- Create `hooks/useKeyActionContext.ts`.
- Modify `components/diff/diffAnnotations.ts` (+ test), `diffCodeViewItems.ts`, `diffViewNavigation.ts`, `diffCodeViewNavigation.ts`, `DiffView.tsx`.
- Create `components/diff/useLineCommentMode.ts`, `components/diff/diffDraftAnnotation.tsx`.
- Modify `pages/DiffPage.tsx`, `pages/DiffPageChrome.tsx`, `pages/useDiffPageActions.ts`, `pages/PRDetailPage.tsx`, `pages/PRDetailSections.tsx`, `components/CommandPalette.tsx`.
- Phase 2: create `utils/keybindingConflicts.ts` (+ test); modify `pages/settings/KeyboardSection.tsx`.

**Docs:** `CLAUDE.md`, `docs/keyboard-shortcuts.md`, `website/src/pages/docs/keybindings.mdx`, `docs/ROADMAP.md`.

---

## Task 1: `review_drafts` table and store

**Files:**
- Create: `go/internal/store/migrations/00035_add_review_drafts.sql`
- Create: `go/internal/store/queries/review_drafts.sql`
- Regenerate: `go/internal/store/db/` (sqlc)
- Create: `go/internal/store/review_drafts.go`
- Test: `go/internal/store/review_drafts_test.go`

**Interfaces:**
- Produces:
  ```go
  type ReviewDraft struct {
      ID, PRID, Path, Side, Body, CommitSHA string
      StartLine int // 0 = single-line comment
      Line      int
      CreatedAt time.Time
  }  // JSON: id, pr_id, path, side, start_line, line, body, commit_sha, created_at
  var ErrDraftNotFound error
  func NewReviewDrafts(db *sql.DB) *ReviewDrafts
  func (d *ReviewDrafts) Create(ctx context.Context, login string, draft ReviewDraft) (ReviewDraft, error)
  func (d *ReviewDrafts) UpdateBody(ctx context.Context, login, id, body string) (ReviewDraft, error)
  func (d *ReviewDrafts) List(ctx context.Context, prID, login string) ([]ReviewDraft, error) // ORDER BY created_at, id
  func (d *ReviewDrafts) Delete(ctx context.Context, login string, ids []string) error
  ```

- [ ] **Step 1: Write the migration**

`go/internal/store/migrations/00035_add_review_drafts.sql`:
```sql
-- +goose Up
-- Migration: 00035_add_review_drafts
-- Description: Pending line comments, per account, submitted as one GitHub
-- review. No FK to prs: a cache eviction must never drop user-written text.
CREATE TABLE IF NOT EXISTS review_drafts (
    id         TEXT PRIMARY KEY,
    pr_id      TEXT NOT NULL,
    login      TEXT NOT NULL,
    path       TEXT NOT NULL,
    side       TEXT NOT NULL CHECK (side IN ('LEFT', 'RIGHT')),
    start_line INTEGER CHECK (start_line IS NULL OR start_line > 0),
    line       INTEGER NOT NULL CHECK (line > 0),
    body       TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    CHECK (start_line IS NULL OR start_line < line)
);
CREATE INDEX IF NOT EXISTS idx_review_drafts_pr_login ON review_drafts(pr_id, login);

-- +goose Down
-- This migration is irreversible.
```

- [ ] **Step 2: Write the queries and regenerate**

`go/internal/store/queries/review_drafts.sql`:
```sql
-- name: InsertReviewDraft :one
INSERT INTO review_drafts (id, pr_id, login, path, side, start_line, line, body, commit_sha)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateReviewDraftBody :one
UPDATE review_drafts SET body = ?
WHERE id = ? AND login = ?
RETURNING *;

-- name: ListReviewDrafts :many
SELECT * FROM review_drafts
WHERE pr_id = ? AND login = ?
ORDER BY created_at, id;

-- name: DeleteReviewDraft :exec
DELETE FROM review_drafts WHERE id = ? AND login = ?;
```
Run: `cd go && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`
Expected: `internal/store/db/review_drafts.sql.go` is created and `models.go` gains a `ReviewDraft` struct (`StartLine sql.NullInt64`, `Line int64`). Read the generated param struct names before writing Step 4. They are expected to be `InsertReviewDraftParams`, `UpdateReviewDraftBodyParams`, `ListReviewDraftsParams`, and `DeleteReviewDraftParams`.

- [ ] **Step 3: Write the failing test**

`go/internal/store/review_drafts_test.go` (the `setupTestDB` helper lives in `diff_store_test.go`, same package):
```go
package store

import (
	"context"
	"errors"
	"testing"
)

func TestReviewDrafts_CreateListUpdateDelete_ScopedToLogin(t *testing.T) {
	ctx := context.Background()
	drafts := NewReviewDrafts(setupTestDB(t).DB)

	single, err := drafts.Create(ctx, "me", ReviewDraft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
	if err != nil {
		t.Fatalf("create single: %v", err)
	}
	if single.ID == "" || single.StartLine != 0 {
		t.Fatalf("single draft = %+v, want generated id and StartLine 0", single)
	}
	ranged, err := drafts.Create(ctx, "me", ReviewDraft{PRID: "pr-1", Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "range", CommitSHA: "old"})
	if err != nil {
		t.Fatalf("create range: %v", err)
	}
	if _, err := drafts.Create(ctx, "other", ReviewDraft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 1, Body: "theirs", CommitSHA: "head"}); err != nil {
		t.Fatalf("create other: %v", err)
	}

	mine, err := drafts.List(ctx, "pr-1", "me")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mine) != 2 || mine[1].StartLine != 3 || mine[1].Side != "LEFT" {
		t.Fatalf("list = %+v, want my two drafts with the range intact", mine)
	}

	if _, err := drafts.UpdateBody(ctx, "other", single.ID, "hijack"); !errors.Is(err, ErrDraftNotFound) {
		t.Fatalf("update by another login err = %v, want ErrDraftNotFound", err)
	}
	updated, err := drafts.UpdateBody(ctx, "me", single.ID, "edited")
	if err != nil || updated.Body != "edited" {
		t.Fatalf("update = %+v, %v", updated, err)
	}

	if err := drafts.Delete(ctx, "other", []string{ranged.ID}); err != nil {
		t.Fatalf("delete by other: %v", err)
	}
	if err := drafts.Delete(ctx, "me", []string{single.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	left, _ := drafts.List(ctx, "pr-1", "me")
	if len(left) != 1 || left[0].ID != ranged.ID {
		t.Fatalf("after delete = %+v, want only the range draft", left)
	}
}

func TestReviewDrafts_RejectsInvalidRange(t *testing.T) {
	drafts := NewReviewDrafts(setupTestDB(t).DB)
	_, err := drafts.Create(context.Background(), "me", ReviewDraft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", StartLine: 9, Line: 5, Body: "x", CommitSHA: "head"})
	if err == nil {
		t.Fatal("expected the start_line < line check to reject the draft")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd go && go test ./internal/store/ -run TestReviewDrafts -v`
Expected: FAIL — `undefined: NewReviewDrafts`.

- [ ] **Step 5: Implement the store**

`go/internal/store/review_drafts.go`:
```go
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// draftIDBytes gives 128-bit ids, unguessable across accounts.
const draftIDBytes = 16

// ErrDraftNotFound reports a draft id the account does not own.
var ErrDraftNotFound = errors.New("review draft not found")

// ReviewDraft is a pending line comment that has not been submitted to GitHub.
type ReviewDraft struct {
	ID        string    `json:"id"`
	PRID      string    `json:"pr_id"`
	Path      string    `json:"path"`
	Side      string    `json:"side"`
	StartLine int       `json:"start_line"` // 0 = single-line comment
	Line      int       `json:"line"`
	Body      string    `json:"body"`
	CommitSHA string    `json:"commit_sha"`
	CreatedAt time.Time `json:"created_at"`
}

// ReviewDrafts persists pending review comments per account.
type ReviewDrafts struct {
	db      *sql.DB
	queries *storedb.Queries
}

// NewReviewDrafts creates a ReviewDrafts store backed by the given database.
func NewReviewDrafts(db *sql.DB) *ReviewDrafts {
	return &ReviewDrafts{db: db, queries: storedb.New(db)}
}

// Create inserts a new draft for login and returns it with its generated id.
func (d *ReviewDrafts) Create(ctx context.Context, login string, draft ReviewDraft) (ReviewDraft, error) {
	id, err := newDraftID()
	if err != nil {
		return ReviewDraft{}, err
	}
	row, err := d.queries.InsertReviewDraft(ctx, storedb.InsertReviewDraftParams{
		ID: id, PrID: draft.PRID, Login: login, Path: draft.Path, Side: draft.Side,
		StartLine: sql.NullInt64{Int64: int64(draft.StartLine), Valid: draft.StartLine > 0},
		Line:      int64(draft.Line), Body: draft.Body, CommitSha: draft.CommitSHA,
	})
	if err != nil {
		return ReviewDraft{}, fmt.Errorf("insert review draft (%s, %s): %w", draft.PRID, login, err)
	}
	return draftFromRow(row), nil
}

// UpdateBody replaces the body of login's draft id.
func (d *ReviewDrafts) UpdateBody(ctx context.Context, login, id, body string) (ReviewDraft, error) {
	row, err := d.queries.UpdateReviewDraftBody(ctx, storedb.UpdateReviewDraftBodyParams{Body: body, ID: id, Login: login})
	if errors.Is(err, sql.ErrNoRows) {
		return ReviewDraft{}, ErrDraftNotFound
	}
	if err != nil {
		return ReviewDraft{}, fmt.Errorf("update review draft %s: %w", id, err)
	}
	return draftFromRow(row), nil
}

// List returns login's drafts on prID, oldest first.
func (d *ReviewDrafts) List(ctx context.Context, prID, login string) ([]ReviewDraft, error) {
	rows, err := d.queries.ListReviewDrafts(ctx, storedb.ListReviewDraftsParams{PrID: prID, Login: login})
	if err != nil {
		return nil, fmt.Errorf("list review drafts (%s, %s): %w", prID, login, err)
	}
	drafts := make([]ReviewDraft, 0, len(rows))
	for _, row := range rows {
		drafts = append(drafts, draftFromRow(row))
	}
	return drafts, nil
}

// Delete removes login's drafts with the given ids in one transaction. Ids the
// account does not own are ignored.
func (d *ReviewDrafts) Delete(ctx context.Context, login string, ids []string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete review drafts: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := d.queries.WithTx(tx)
	for _, id := range ids {
		if err := qtx.DeleteReviewDraft(ctx, storedb.DeleteReviewDraftParams{ID: id, Login: login}); err != nil {
			return fmt.Errorf("delete review draft %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete review drafts: %w", err)
	}
	return nil
}

func draftFromRow(row storedb.ReviewDraft) ReviewDraft {
	return ReviewDraft{
		ID: row.ID, PRID: row.PrID, Path: row.Path, Side: row.Side,
		StartLine: int(row.StartLine.Int64), Line: int(row.Line), Body: row.Body,
		CommitSHA: row.CommitSha, CreatedAt: row.CreatedAt,
	}
}

func newDraftID() (string, error) {
	var b [draftIDBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate review draft id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
```
If sqlc names differ from the ones above (Step 2 note), match the generated names.

- [ ] **Step 6: Run the tests**

Run: `cd go && go test -race ./internal/store/...`
Expected: PASS. If a test asserts the latest migration version or the table list (`db_test.go`, `migration_runner`), update it to include 35 / `review_drafts`.

- [ ] **Step 7: Commit**

```bash
git add go/internal/store
git commit -m "feat(go): add review_drafts table and store"
```

---

## Task 2: GitHub `SubmitReview` (one `CreateReview` with line comments)

**Files:**
- Modify: `go/internal/provider/models.go` (append types)
- Modify: `go/internal/provider/interface.go` (append capability)
- Modify: `go/internal/provider/github/reviews.go`
- Test: `go/internal/provider/github/reviews_test.go`

**Interfaces:**
- Produces:
  ```go
  // provider package
  type ReviewComment struct { Path, Side, Body string; StartLine, Line int } // StartLine 0 = single line
  type ReviewSubmission struct { Event, Body, CommitID string; Comments []ReviewComment }
  type ReviewSubmitter interface { SubmitReview(ctx context.Context, prID string, sub ReviewSubmission) error }
  // github.Client implements provider.ReviewSubmitter
  ```

- [ ] **Step 1: Write the failing test** (append to `reviews_test.go`; add `encoding/json` and `github.com/itterno/bistro/internal/provider` imports)

```go
func TestSubmitReview_SendsOneReviewWithLineAndRangeComments(t *testing.T) {
	type sentComment struct {
		Path      string `json:"path"`
		Body      string `json:"body"`
		Side      string `json:"side"`
		Line      int    `json:"line"`
		StartSide string `json:"start_side"`
		StartLine int    `json:"start_line"`
		Position  *int   `json:"position"`
	}
	var sent struct {
		CommitID string        `json:"commit_id"`
		Body     string        `json:"body"`
		Event    string        `json:"event"`
		Comments []sentComment `json:"comments"`
	}
	var calls int
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewClient("test-token")
	ghClient := ghlib.NewClient(nil)
	ghClient.BaseURL, _ = ghClient.BaseURL.Parse(srv.URL + "/")
	client.gh = ghClient

	err := client.SubmitReview(context.Background(), "github:owner/repo:42", provider.ReviewSubmission{
		Event: "REQUEST_CHANGES", Body: "Needs work", CommitID: "abc123",
		Comments: []provider.ReviewComment{
			{Path: "a.go", Side: "RIGHT", Line: 10, Body: "single"},
			{Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "range"},
		},
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if calls != 1 || gotPath != "/repos/owner/repo/pulls/42/reviews" {
		t.Fatalf("calls=%d path=%s, want one POST to the reviews endpoint", calls, gotPath)
	}
	if sent.Event != "REQUEST_CHANGES" || sent.Body != "Needs work" || sent.CommitID != "abc123" {
		t.Fatalf("review = %+v", sent)
	}
	want := []sentComment{
		{Path: "a.go", Body: "single", Side: "RIGHT", Line: 10},
		{Path: "b.go", Body: "range", Side: "LEFT", Line: 5, StartSide: "LEFT", StartLine: 3},
	}
	if len(sent.Comments) != len(want) {
		t.Fatalf("comments = %+v", sent.Comments)
	}
	for i, got := range sent.Comments {
		if got.Position != nil {
			t.Errorf("comment %d sent legacy position", i)
		}
		got.Position = nil
		if got != want[i] {
			t.Errorf("comment %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestSubmitReview_RejectsUnknownEventWithoutCallingGitHub(t *testing.T) {
	client := NewClient("test-token")
	ghClient := ghlib.NewClient(nil)
	ghClient.BaseURL, _ = ghClient.BaseURL.Parse("http://127.0.0.1:1/")
	client.gh = ghClient
	err := client.SubmitReview(context.Background(), "github:owner/repo:42", provider.ReviewSubmission{Event: "MERGE"})
	if err == nil || !strings.Contains(err.Error(), "invalid review event") {
		t.Fatalf("err = %v, want invalid review event", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go && go test ./internal/provider/github/ -run TestSubmitReview -v`
Expected: FAIL — `client.SubmitReview undefined`.

- [ ] **Step 3: Add the provider types**

Append to `go/internal/provider/models.go`:
```go
// ReviewComment is one inline comment inside a submitted review. StartLine 0
// means a single-line comment; a range covers StartLine..Line on Side.
type ReviewComment struct {
	Path      string
	Side      string // "LEFT" (base) or "RIGHT" (head)
	StartLine int
	Line      int
	Body      string
}

// ReviewSubmission is one pull-request review: an event, an optional body, and
// inline comments anchored to CommitID.
type ReviewSubmission struct {
	Event    string // "APPROVE", "REQUEST_CHANGES" or "COMMENT"
	Body     string
	CommitID string
	Comments []ReviewComment
}
```
Append to `go/internal/provider/interface.go`:
```go
// ReviewSubmitter is an optional capability for providers that can submit a
// review with inline comments in one call.
type ReviewSubmitter interface {
	SubmitReview(ctx context.Context, prID string, sub ReviewSubmission) error
}
```

- [ ] **Step 4: Implement `SubmitReview`** (in `reviews.go`, after `requestChanges`)

```go
// reviewEvents are the CreateReview events submit_review may send.
var reviewEvents = map[string]bool{"APPROVE": true, "REQUEST_CHANGES": true, "COMMENT": true}

// submitReview creates one review carrying every inline comment.
func (c *Client) submitReview(ctx context.Context, owner, repo string, number int, sub provider.ReviewSubmission) error {
	if !reviewEvents[sub.Event] {
		return fmt.Errorf("invalid review event %q", sub.Event)
	}
	review := &ghlib.PullRequestReviewRequest{Event: ghlib.Ptr(sub.Event)}
	if sub.Body != "" {
		review.Body = ghlib.Ptr(sub.Body)
	}
	if sub.CommitID != "" {
		review.CommitID = ghlib.Ptr(sub.CommitID)
	}
	for _, comment := range sub.Comments {
		draft := &ghlib.DraftReviewComment{
			Path: ghlib.Ptr(comment.Path), Body: ghlib.Ptr(comment.Body),
			Side: ghlib.Ptr(comment.Side), Line: ghlib.Ptr(comment.Line),
		}
		if comment.StartLine > 0 {
			draft.StartLine = ghlib.Ptr(comment.StartLine)
			draft.StartSide = ghlib.Ptr(comment.Side)
		}
		review.Comments = append(review.Comments, draft)
	}
	if _, _, err := c.gh.PullRequests.CreateReview(ctx, owner, repo, number, review); err != nil {
		return fmt.Errorf("failed to submit review on PR %s/%s#%d: %w", owner, repo, number, err)
	}
	return nil
}

// SubmitReview implements provider.ReviewSubmitter, parsing the "github:owner/repo:number" ID format.
func (c *Client) SubmitReview(ctx context.Context, prID string, sub provider.ReviewSubmission) error {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return err
	}
	return c.submitReview(ctx, owner, repo, number, sub)
}
```
`reviewEvents` is a read-only lookup table, set once and never written.

- [ ] **Step 5: Run the tests**

Run: `cd go && go test -race ./internal/provider/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/internal/provider
git commit -m "feat(go): submit a review with line comments in one CreateReview call"
```

---

## Task 3: `submit_review` action

**Files:**
- Modify: `go/internal/actions/types.go`, `handlers.go`, `executor.go`
- Test: `go/internal/actions/types_test.go`, `go/internal/actions/handlers_test.go`

**Interfaces:**
- Consumes: `provider.ReviewSubmitter`, `provider.ReviewSubmission`, `provider.ReviewComment` (Task 2).
- Produces:
  ```go
  const SubmitReviewCommand ActionCommand = "submit_review"
  const ReviewEventApprove, ReviewEventRequestChanges, ReviewEventComment = "APPROVE", "REQUEST_CHANGES", "COMMENT"
  // ActionRequest new fields:
  ReviewEvent    string
  CommitID       string
  ReviewComments []provider.ReviewComment
  ```
  `Execute` of `submit_review` has the same local side effects as the plain action for its event: APPROVE → approve, REQUEST_CHANGES → request_changes, COMMENT → post_comment. That covers events, `review_status`, and last-reviewed SHA.

- [ ] **Step 1: Write the failing tests**

Append to `types_test.go`:
```go
func TestSubmitReviewValidation(t *testing.T) {
	comments := []provider.ReviewComment{{Path: "a.go", Side: "RIGHT", Line: 1, Body: "x"}}
	cases := []struct {
		name    string
		req     ActionRequest
		wantErr bool
	}{
		{"approve without body", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove}, false},
		{"comment with drafts only", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventComment, CommitID: "c", ReviewComments: comments}, false},
		{"request changes needs body", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventRequestChanges}, true},
		{"unknown event", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: "MERGE"}, true},
		{"comments need commit id", ActionRequest{PRID: "p", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove, ReviewComments: comments}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
```
(Add the `provider` import to `types_test.go` if it is missing.)

Append to `handlers_test.go` (add imports as needed: `context`, `strings`, `testing`, `provider`, `provider/mock`):
```go
type reviewSubmitterProvider struct {
	*mock.MockProvider
	submissions []provider.ReviewSubmission
}

func (p *reviewSubmitterProvider) SubmitReview(_ context.Context, _ string, sub provider.ReviewSubmission) error {
	p.submissions = append(p.submissions, sub)
	return nil
}

func TestExecute_SubmitReviewSendsTheReviewSubmission(t *testing.T) {
	prov := &reviewSubmitterProvider{MockProvider: mock.NewMockProvider()}
	comments := []provider.ReviewComment{{Path: "a.go", Side: "LEFT", StartLine: 2, Line: 4, Body: "range"}}
	result, _ := NewActionExecutor(nil, prov, nil, nil).Execute(context.Background(), &ActionRequest{
		PRID: "github:o/r:1", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove,
		Body: "LGTM", CommitID: "abc", ReviewComments: comments,
	})
	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	if len(prov.submissions) != 1 {
		t.Fatalf("submissions = %d, want 1", len(prov.submissions))
	}
	got := prov.submissions[0]
	if got.Event != ReviewEventApprove || got.Body != "LGTM" || got.CommitID != "abc" || len(got.Comments) != 1 || got.Comments[0] != comments[0] {
		t.Fatalf("submission = %+v", got)
	}
}

func TestExecute_SubmitReviewWithoutCapabilityFailsPermanently(t *testing.T) {
	result, _ := NewActionExecutor(nil, mock.NewMockProvider(), nil, nil).Execute(context.Background(), &ActionRequest{
		PRID: "github:o/r:1", Action: SubmitReviewCommand, ReviewEvent: ReviewEventApprove,
	})
	if result.Success || result.IsRecoverable() || !strings.Contains(result.Error, "permanent") {
		t.Fatalf("result = %+v, want a permanent failure", result)
	}
}
```
If `mock.MockProvider` already has a `SubmitReview` method, the second test is invalid; it does not have one today.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go && go test ./internal/actions/ -run 'SubmitReview' -v`
Expected: FAIL — `undefined: SubmitReviewCommand`.

- [ ] **Step 3: Implement types**

In `types.go`:
- Add `"github.com/itterno/bistro/internal/provider"` to the imports.
- Add the command constant after `UnresolveThreadCommand`:
```go
	// SubmitReviewCommand submits one review carrying the user's line-comment drafts.
	SubmitReviewCommand ActionCommand = "submit_review"
```
- Add the event constants:
```go
// Review events for submit_review, matching GitHub's CreateReview event values.
const (
	ReviewEventApprove        = "APPROVE"
	ReviewEventRequestChanges = "REQUEST_CHANGES"
	ReviewEventComment        = "COMMENT"
)
```
- Add `SubmitReviewCommand` to the `isNonIdempotent` case list. Add it to the doc bullet list too: `submit_review → each call creates a new review`.
- Add the fields at the end of `ActionRequest`:
```go
	// ReviewEvent, CommitID and ReviewComments carry a submit_review request.
	// The sync orchestrator fills CommitID/ReviewComments from stored drafts.
	ReviewEvent    string
	CommitID       string
	ReviewComments []provider.ReviewComment
```
- Add a `Validate` case before `default`:
```go
	case SubmitReviewCommand:
		switch ar.ReviewEvent {
		case ReviewEventApprove, ReviewEventComment:
		case ReviewEventRequestChanges:
			if ar.Body == "" {
				return fmt.Errorf("body is required for submit_review with %s", ReviewEventRequestChanges)
			}
		default:
			return fmt.Errorf("invalid review event %q for submit_review", ar.ReviewEvent)
		}
		if len(ar.ReviewComments) > 0 && ar.CommitID == "" {
			return fmt.Errorf("commit id is required for submit_review with comments")
		}
		return nil
```
(A COMMENT event with no body and no drafts is rejected by the orchestrator in Task 4. The SUBMIT_ACTION handler validates before drafts are loaded.)

- [ ] **Step 4: Implement handler and executor mapping**

Append to `handlers.go`:
```go
// SubmitReviewHandler executes submit_review via the ReviewSubmitter capability.
type SubmitReviewHandler struct {
	provider provider.Provider
}

// NewSubmitReviewHandler creates a SubmitReviewHandler.
func NewSubmitReviewHandler(prov provider.Provider) *SubmitReviewHandler {
	return &SubmitReviewHandler{provider: prov}
}

// Handle submits one review with the request's event, body and inline comments.
func (h *SubmitReviewHandler) Handle(ctx context.Context, req *ActionRequest) (*ActionResult, error) {
	result := &ActionResult{PRID: req.PRID, Action: SubmitReviewCommand, Timestamp: time.Now()}
	submitter, ok := h.provider.(provider.ReviewSubmitter)
	if !ok {
		result.Error = "provider does not support review submission (403 permanent)"
		return result, nil
	}
	err := submitter.SubmitReview(ctx, req.PRID, provider.ReviewSubmission{
		Event: req.ReviewEvent, Body: req.Body, CommitID: req.CommitID, Comments: req.ReviewComments,
	})
	if err != nil {
		result.Error = fmt.Sprintf("failed to submit review: %v", err)
		return result, nil
	}
	result.Success = true
	return result, nil
}
```
In `executor.go`:
- Add to `handlerFor`: `case SubmitReviewCommand: return NewSubmitReviewHandler(ae.provider), true`
- Add:
```go
// reviewOutcome maps submit_review onto the plain action whose local side
// effects (events, review_status, last-reviewed SHA) it shares.
func reviewOutcome(req *ActionRequest) ActionCommand {
	if req.Action != SubmitReviewCommand {
		return req.Action
	}
	switch req.ReviewEvent {
	case ReviewEventApprove:
		return ApproveCommand
	case ReviewEventRequestChanges:
		return RequestChangesCommand
	default:
		return PostCommentCommand
	}
}
```
- In `emitEvent`, change `switch req.Action {` (the success switch) to `switch reviewOutcome(req) {`.
- In `updatePRReviewStatus`, change both `switch req.Action {` statements to `switch reviewOutcome(req) {`.

- [ ] **Step 5: Run the tests**

Run: `cd go && go test -race ./internal/actions/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/internal/actions
git commit -m "feat(go): add submit_review action"
```

---

## Task 4: Orchestrator — absorb drafts, group by commit, submit, delete

**Files:**
- Create: `go/internal/sync/orchestrator_review.go`
- Modify: `go/internal/sync/orchestrator.go` (field + `WithReviewDrafts`)
- Modify: `go/internal/sync/orchestrator_actions.go` (`ApplyAction` hook; `SubmitReviewCommand` in `executeAction`'s success switch)
- Test: `go/internal/sync/orchestrator_review_test.go`

**Interfaces:**
- Consumes: `store.ReviewDraft`, `(*store.ReviewDrafts).List/Delete` (Task 1); `actions.SubmitReviewCommand`, `ReviewEvent*`, `ActionRequest.ReviewEvent/CommitID/ReviewComments` (Task 3).
- Produces:
  ```go
  func (o *Orchestrator) WithReviewDrafts(d reviewDraftStore) *Orchestrator
  // push event emitted after drafts are deleted:
  events.Event{Type: "DRAFTS_CHANGED", Data: map[string]interface{}{"pr_id": prID}}
  ```
  `ApplyAction` behavior: if `approve` / `request_changes` arrives and the current user has drafts on the PR, it becomes `submit_review` with event `APPROVE` / `REQUEST_CHANGES`. A `submit_review` is never queued.

- [ ] **Step 1: Write the failing tests**

`go/internal/sync/orchestrator_review_test.go`:
```go
package sync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
)

type reviewRecorder struct {
	*mock.MockProvider
	submissions []provider.ReviewSubmission
	err         error
}

func (r *reviewRecorder) SubmitReview(_ context.Context, _ string, sub provider.ReviewSubmission) error {
	if r.err != nil {
		return r.err
	}
	r.submissions = append(r.submissions, sub)
	return nil
}

type staticLogin string

func (s staticLogin) GetCurrentUser(context.Context, string) (string, bool, error) {
	return string(s), true, nil
}

func newReviewTestOrchestrator(t *testing.T, prov provider.Provider) (*Orchestrator, *store.ReviewDrafts) {
	t.Helper()
	db := openTestDB(t)
	insertPRForSync(t, db.DB, "pr-r") // head_sha = 'head'
	drafts := store.NewReviewDrafts(db.DB)
	orch := NewOrchestrator(nil, prov, db, nil).
		WithReviewTracking(staticLogin("me"), db).
		WithReviewDrafts(drafts)
	return orch, drafts
}

func mustDraft(t *testing.T, drafts *store.ReviewDrafts, draft store.ReviewDraft) store.ReviewDraft {
	t.Helper()
	draft.PRID = "pr-r"
	created, err := drafts.Create(context.Background(), "me", draft)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	return created
}

func TestApplyAction_ApproveAbsorbsDraftsIntoOneReview(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
	mustDraft(t, drafts, store.ReviewDraft{Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "two", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.ApproveCommand})

	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	if len(prov.submissions) != 1 {
		t.Fatalf("submissions = %d, want 1", len(prov.submissions))
	}
	got := prov.submissions[0]
	if got.Event != actions.ReviewEventApprove || got.CommitID != "head" || len(got.Comments) != 2 {
		t.Fatalf("submission = %+v", got)
	}
	if got.Comments[1] != (provider.ReviewComment{Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "two"}) {
		t.Fatalf("range comment = %+v", got.Comments[1])
	}
	left, _ := drafts.List(context.Background(), "pr-r", "me")
	if len(left) != 0 {
		t.Fatalf("drafts left = %d, want submitted drafts deleted", len(left))
	}
}

func TestApplyAction_FailedSubmitKeepsDraftsAndNamesThem(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider(), err: errors.New("POST .../reviews: 422 Unprocessable Entity")}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 10, Body: "stale", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.SubmitReviewCommand, ReviewEvent: actions.ReviewEventComment})

	if result.Success || !strings.Contains(result.Error, "a.go:10") || !strings.Contains(result.Error, "422") {
		t.Fatalf("result = %+v, want a failure naming a.go:10 and the 422", result)
	}
	left, _ := drafts.List(context.Background(), "pr-r", "me")
	if len(left) != 1 {
		t.Fatalf("drafts left = %d, want the draft kept", len(left))
	}
}

func TestApplyAction_DraftsOnOlderCommitGoFirstAsCommentReview(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "new.go", Side: "RIGHT", Line: 1, Body: "on head", CommitSHA: "head"})
	mustDraft(t, drafts, store.ReviewDraft{Path: "old.go", Side: "RIGHT", Line: 2, Body: "outdated", CommitSHA: "old"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.RequestChangesCommand, Body: "Please fix"})

	if !result.Success || len(prov.submissions) != 2 {
		t.Fatalf("result = %+v, submissions = %d", result, len(prov.submissions))
	}
	first, last := prov.submissions[0], prov.submissions[1]
	if first.CommitID != "old" || first.Event != actions.ReviewEventComment || first.Body != "" {
		t.Fatalf("first = %+v, want a body-less COMMENT review on the old commit", first)
	}
	if last.CommitID != "head" || last.Event != actions.ReviewEventRequestChanges || last.Body != "Please fix" {
		t.Fatalf("last = %+v, want the user's event and body on head", last)
	}
}

func TestApplyAction_RateLimitedReviewIsNotQueued(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	orch.rateLimitState = &provider.RateLimitState{ResetAt: time.Now().Add(5 * time.Minute)}
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 1, Body: "x", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.ApproveCommand})

	if result.Success || !strings.Contains(result.Error, "draft(s) kept") || len(prov.submissions) != 0 {
		t.Fatalf("result = %+v, submissions = %d", result, len(prov.submissions))
	}
	left, _ := drafts.List(context.Background(), "pr-r", "me")
	if len(left) != 1 {
		t.Fatalf("drafts left = %d, want 1", len(left))
	}
}

func TestApplyAction_CommentReviewWithNothingToSubmitFails(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, _ := newReviewTestOrchestrator(t, prov)
	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.SubmitReviewCommand, ReviewEvent: actions.ReviewEventComment})
	if result.Success || len(prov.submissions) != 0 {
		t.Fatalf("result = %+v, want failure without a provider call", result)
	}
}
```
Check that `rateLimitState` with only `ResetAt` makes `IsRateLimited()` true. `blocker_fixes_test.go:780` uses the same shape. If `IsRateLimited` also needs `Remaining == 0`, set it the way that test does.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go && go test ./internal/sync/ -run 'TestApplyAction_(Approve|Failed|Drafts|RateLimitedReview|CommentReview)' -v`
Expected: FAIL — `orch.WithReviewDrafts undefined`.

- [ ] **Step 3: Add the field and option**

In `orchestrator.go`, add a field to `Orchestrator`:
```go
	// drafts is optional; when set, approve/request_changes absorb the current
	// user's line-comment drafts and submit_review can run.
	drafts reviewDraftStore
```
Add the option next to `WithPostReview`:
```go
// WithReviewDrafts attaches the draft store used for review submission.
func (o *Orchestrator) WithReviewDrafts(d reviewDraftStore) *Orchestrator {
	o.drafts = d
	return o
}
```

- [ ] **Step 4: Implement `orchestrator_review.go`**

```go
package sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

// reviewDraftStore is the subset of store.ReviewDrafts review submission uses.
type reviewDraftStore interface {
	List(ctx context.Context, prID, login string) ([]store.ReviewDraft, error)
	Delete(ctx context.Context, login string, ids []string) error
}

// draftGroup holds drafts submitted together: GitHub anchors every comment of
// a review to the review's single commit_id.
type draftGroup struct {
	commitSHA string
	drafts    []store.ReviewDraft
}

// applyReviewWithDrafts handles approve, request_changes and submit_review when
// drafts are involved. handled=false means ApplyAction continues normally:
// the request is not a review, or a plain approve/request_changes has no drafts.
func (o *Orchestrator) applyReviewWithDrafts(ctx context.Context, req *actions.ActionRequest) (result *actions.ActionResult, handled bool) {
	switch req.Action {
	case actions.ApproveCommand, actions.RequestChangesCommand, actions.SubmitReviewCommand:
	default:
		return nil, false
	}
	isExplicitSubmit := req.Action == actions.SubmitReviewCommand
	if o.drafts == nil || o.reviewLogin == nil {
		if isExplicitSubmit {
			return failedActionResult(req, "review drafts are unavailable"), true
		}
		return nil, false
	}
	login, ok, err := o.reviewLogin.GetCurrentUser(ctx, githubProvider)
	if err != nil {
		return failedActionResult(req, fmt.Sprintf("resolve current user for review: %v", err)), true
	}
	if !ok || login == "" {
		if isExplicitSubmit {
			return failedActionResult(req, "no current GitHub account: review not submitted"), true
		}
		return nil, false
	}
	drafts, err := o.drafts.List(ctx, req.PRID, login)
	if err != nil {
		return failedActionResult(req, fmt.Sprintf("load review drafts: %v", err)), true
	}
	if len(drafts) == 0 && !isExplicitSubmit {
		return nil, false
	}
	return o.submitReview(ctx, asSubmitReview(req), login, drafts), true
}

// asSubmitReview turns approve/request_changes into the equivalent submit_review.
func asSubmitReview(req *actions.ActionRequest) *actions.ActionRequest {
	converted := *req
	switch req.Action {
	case actions.ApproveCommand:
		converted.ReviewEvent = actions.ReviewEventApprove
	case actions.RequestChangesCommand:
		converted.ReviewEvent = actions.ReviewEventRequestChanges
	}
	converted.Action = actions.SubmitReviewCommand
	return &converted
}

// submitReview sends one review per commit group, oldest groups first as
// COMMENT reviews, and deletes each group's drafts once GitHub accepts it.
// Never queued: drafts are already persisted, so failure just keeps them.
func (o *Orchestrator) submitReview(ctx context.Context, req *actions.ActionRequest, login string, drafts []store.ReviewDraft) *actions.ActionResult {
	if len(drafts) == 0 && req.ReviewEvent == actions.ReviewEventComment && req.Body == "" {
		return failedActionResult(req, "nothing to submit: no drafts and no review body")
	}
	if o.IsRateLimited() {
		return failedActionResult(req, fmt.Sprintf("rate limited: review not submitted; %d draft(s) kept", len(drafts)))
	}
	groups := groupDraftsByCommit(drafts, o.prHeadSHA(ctx, req.PRID))
	if len(groups) == 0 {
		groups = []draftGroup{{}} // body-only review
	}
	var result *actions.ActionResult
	for i, group := range groups {
		result = o.executeAction(ctx, reviewRequestForGroup(req, group, i == len(groups)-1))
		if !result.Success {
			if len(group.drafts) == 0 {
				return result
			}
			return failedActionResult(req, fmt.Sprintf("review not submitted; %d draft(s) kept (%s): %s",
				len(group.drafts), describeDrafts(group.drafts), result.Error))
		}
		o.deleteSubmittedDrafts(ctx, req.PRID, login, group.drafts)
	}
	return result
}

// groupDraftsByCommit keeps first-seen order and moves the head commit's group
// last, so the review carrying the user's event lands on the current head.
func groupDraftsByCommit(drafts []store.ReviewDraft, headSHA string) []draftGroup {
	var groups []draftGroup
	indexBySHA := map[string]int{}
	for _, draft := range drafts {
		i, ok := indexBySHA[draft.CommitSHA]
		if !ok {
			i = len(groups)
			indexBySHA[draft.CommitSHA] = i
			groups = append(groups, draftGroup{commitSHA: draft.CommitSHA})
		}
		groups[i].drafts = append(groups[i].drafts, draft)
	}
	if i, ok := indexBySHA[headSHA]; ok && i != len(groups)-1 {
		head := groups[i]
		groups = append(append(groups[:i:i], groups[i+1:]...), head)
	}
	return groups
}

// reviewRequestForGroup builds one CreateReview request. Only the last group
// carries the user's event and body; earlier groups are plain COMMENT reviews.
func reviewRequestForGroup(req *actions.ActionRequest, group draftGroup, isLast bool) *actions.ActionRequest {
	groupReq := &actions.ActionRequest{
		PRID: req.PRID, Action: actions.SubmitReviewCommand,
		ReviewEvent: actions.ReviewEventComment, CommitID: group.commitSHA,
		ReviewComments: toReviewComments(group.drafts),
	}
	if isLast {
		groupReq.ReviewEvent = req.ReviewEvent
		groupReq.Body = req.Body
	}
	return groupReq
}

func toReviewComments(drafts []store.ReviewDraft) []provider.ReviewComment {
	comments := make([]provider.ReviewComment, 0, len(drafts))
	for _, draft := range drafts {
		comments = append(comments, provider.ReviewComment{
			Path: draft.Path, Side: draft.Side, StartLine: draft.StartLine, Line: draft.Line, Body: draft.Body,
		})
	}
	return comments
}

// describeDrafts names drafts as path:line or path:start-end for error messages.
func describeDrafts(drafts []store.ReviewDraft) string {
	names := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		if draft.StartLine > 0 {
			names = append(names, fmt.Sprintf("%s:%d-%d", draft.Path, draft.StartLine, draft.Line))
			continue
		}
		names = append(names, fmt.Sprintf("%s:%d", draft.Path, draft.Line))
	}
	return strings.Join(names, ", ")
}

// prHeadSHA returns the cached head SHA, or "" when unknown (no reordering).
func (o *Orchestrator) prHeadSHA(ctx context.Context, prID string) string {
	if o.store == nil {
		return ""
	}
	var headSHA string
	if err := o.store.QueryRowContext(ctx, `SELECT head_sha FROM prs WHERE id = ?`, prID).Scan(&headSHA); err != nil {
		logging.FromContext(ctx).Debug("prHeadSHA: lookup failed", "pr_id", prID, "error", err)
		return ""
	}
	return headSHA
}

// deleteSubmittedDrafts removes drafts GitHub accepted and tells the UI.
func (o *Orchestrator) deleteSubmittedDrafts(ctx context.Context, prID, login string, drafts []store.ReviewDraft) {
	if len(drafts) == 0 {
		return
	}
	ids := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		ids = append(ids, draft.ID)
	}
	if err := o.drafts.Delete(ctx, login, ids); err != nil {
		// The review is on GitHub; resubmitting these drafts would duplicate it.
		logging.FromContext(ctx).Error("submitted drafts could not be deleted", "pr_id", prID, "count", len(ids), "error", err)
	}
	if o.emitter != nil {
		o.emitter.Emit(&events.Event{
			Type: "DRAFTS_CHANGED", Data: map[string]interface{}{"pr_id": prID}, Timestamp: time.Now(),
		})
	}
}
```

- [ ] **Step 5: Hook into `ApplyAction` and `executeAction`**

In `orchestrator_actions.go` `ApplyAction`, insert right after the `validateThreadPreflight` block, *before* `prepareReplyCommentTarget` and the rate-limit check:
```go
	// Review submissions with drafts bypass the queue: drafts are already
	// persisted, so a rate-limited or offline submit fails with them intact.
	if result, handled := o.applyReviewWithDrafts(ctx, req); handled {
		return result
	}
```
In `executeAction`'s success switch, extend the review case:
`case actions.ApproveCommand, actions.RequestChangesCommand, actions.PostCommentCommand, actions.SubmitReviewCommand:`

- [ ] **Step 6: Run the tests**

Run: `cd go && go test -race ./internal/sync/...`
Expected: PASS, including all pre-existing ApplyAction/queue tests. Plain approve has no drafts store attached in those tests, so its path is unchanged.

- [ ] **Step 7: Commit**

```bash
git add go/internal/sync
git commit -m "feat(go): submit drafts as reviews and absorb them into approve/request changes"
```

---

## Task 5: WS handlers for drafts + `review_event` on SUBMIT_ACTION + wiring

**Files:**
- Create: `go/internal/server/handler/drafts.go`
- Test: `go/internal/server/handler/drafts_test.go`
- Modify: `go/internal/server/handler/threads.go` (`NewSubmitActionHandler` request struct)
- Modify: `go/internal/server/app.go` (`registerDiffAndCheckoutHandlers`, `attachOrchestratorLocked`)

**Interfaces:**
- Consumes: `store.ReviewDrafts`, `store.ErrDraftNotFound` (Task 1); `Orchestrator.WithReviewDrafts` (Task 4).
- Produces (WS contract the renderer uses):
  - `DRAFTS_LIST {pr_id}` → `DRAFTS_READY {pr_id, drafts: ReviewDraft[]}`
  - `DRAFT_SAVE {pr_id, id?, path, side, start_line, line, body, commit_sha}` → `DRAFT_SAVED {draft}`. With an `id`, only `body` is updated.
  - `DRAFT_DELETE {pr_id, draft_ids: string[]}` → `DRAFTS_DELETED {pr_id, deleted: N}`
  - Push after save/delete: `DRAFTS_CHANGED {pr_id}`
  - `SUBMIT_ACTION` accepts `review_event` for `action: "submit_review"`.
  - Recoverable failures return the response type with `data.error` set. `wsClient.request` turns a non-empty `error` into a failed result.

- [ ] **Step 1: Write the failing tests**

`go/internal/server/handler/drafts_test.go` (`newReadStateTestDB` and `spyBroadcaster` live in `readstate_test.go`, same package):
```go
package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type fixedUser string

func (f fixedUser) GetCurrentUser(context.Context, string) (string, bool, error) {
	return string(f), f != "", nil
}

func draftMessage(t *testing.T, msgType string, data map[string]any) ws.InboundMessage {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return ws.InboundMessage{ID: "req-1", Type: msgType, Data: raw}
}

func responseData(t *testing.T, resp any) map[string]any {
	t.Helper()
	envelope, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response = %#v", resp)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data = %#v", envelope["data"])
	}
	return data
}

func TestDraftSave_RejectsInvalidPositions(t *testing.T) {
	deps := DraftDeps{Drafts: store.NewReviewDrafts(newReadStateTestDB(t).DB), CurrentUser: fixedUser("me"), Broadcaster: &spyBroadcaster{}}
	save := NewDraftSaveHandler(deps)
	valid := map[string]any{"pr_id": "pr-1", "path": "a.go", "side": "RIGHT", "line": 4, "body": "x", "commit_sha": "head"}
	cases := map[string]func(map[string]any){
		"bad side":       func(m map[string]any) { m["side"] = "MIDDLE" },
		"start >= line":  func(m map[string]any) { m["start_line"] = 4 },
		"blank body":     func(m map[string]any) { m["body"] = "   " },
		"missing commit": func(m map[string]any) { m["commit_sha"] = "" },
		"zero line":      func(m map[string]any) { m["line"] = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			data := map[string]any{}
			for k, v := range valid {
				data[k] = v
			}
			mutate(data)
			resp, err := save(context.Background(), draftMessage(t, "DRAFT_SAVE", data))
			if err != nil {
				t.Fatalf("handler err: %v", err)
			}
			if responseData(t, resp)["error"] == nil {
				t.Fatalf("expected a validation error, got %#v", resp)
			}
		})
	}
}

func TestDraftSaveListDelete_BroadcastsDraftsChanged(t *testing.T) {
	spy := &spyBroadcaster{}
	deps := DraftDeps{Drafts: store.NewReviewDrafts(newReadStateTestDB(t).DB), CurrentUser: fixedUser("me"), Broadcaster: spy}
	ctx := context.Background()

	resp, err := NewDraftSaveHandler(deps)(ctx, draftMessage(t, "DRAFT_SAVE", map[string]any{
		"pr_id": "pr-1", "path": "a.go", "side": "LEFT", "start_line": 2, "line": 4, "body": "range", "commit_sha": "head",
	}))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	saved, ok := responseData(t, resp)["draft"].(store.ReviewDraft)
	if !ok || saved.StartLine != 2 {
		t.Fatalf("saved = %#v", responseData(t, resp)["draft"])
	}

	listResp, _ := NewDraftsListHandler(deps)(ctx, draftMessage(t, "DRAFTS_LIST", map[string]any{"pr_id": "pr-1"}))
	if drafts, _ := responseData(t, listResp)["drafts"].([]store.ReviewDraft); len(drafts) != 1 {
		t.Fatalf("list = %#v", responseData(t, listResp))
	}

	if _, err := NewDraftDeleteHandler(deps)(ctx, draftMessage(t, "DRAFT_DELETE", map[string]any{"pr_id": "pr-1", "draft_ids": []string{saved.ID}})); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(spy.messages) != 2 {
		t.Fatalf("broadcasts = %d, want DRAFTS_CHANGED after save and delete", len(spy.messages))
	}
}

func TestDraftSave_RequiresSignedInUser(t *testing.T) {
	deps := DraftDeps{Drafts: store.NewReviewDrafts(newReadStateTestDB(t).DB), CurrentUser: fixedUser(""), Broadcaster: &spyBroadcaster{}}
	resp, _ := NewDraftSaveHandler(deps)(context.Background(), draftMessage(t, "DRAFT_SAVE", map[string]any{
		"pr_id": "pr-1", "path": "a.go", "side": "RIGHT", "line": 1, "body": "x", "commit_sha": "head",
	}))
	if responseData(t, resp)["error"] == nil {
		t.Fatal("expected an error without a signed-in user")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go && go test ./internal/server/handler/ -run 'TestDraft' -v`
Expected: FAIL — `undefined: DraftDeps`.

- [ ] **Step 3: Implement `drafts.go`**

```go
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// maxDraftBodyLength matches GitHub's 65,536-character comment body limit.
const maxDraftBodyLength = 65536

// draftStore is the subset of store.ReviewDrafts the draft handlers use.
type draftStore interface {
	Create(ctx context.Context, login string, draft store.ReviewDraft) (store.ReviewDraft, error)
	UpdateBody(ctx context.Context, login, id, body string) (store.ReviewDraft, error)
	List(ctx context.Context, prID, login string) ([]store.ReviewDraft, error)
	Delete(ctx context.Context, login string, ids []string) error
}

// DraftDeps carries the draft handlers' dependencies.
type DraftDeps struct {
	Drafts      draftStore
	CurrentUser currentUserReader
	Broadcaster broadcaster
	Log         *slog.Logger
}

type draftSaveRequest struct {
	PRID      string `json:"pr_id"`
	ID        string `json:"id"`
	Path      string `json:"path"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line"`
	Line      int    `json:"line"`
	Body      string `json:"body"`
	CommitSHA string `json:"commit_sha"`
}

func (r draftSaveRequest) validate() error {
	body := strings.TrimSpace(r.Body)
	switch {
	case r.PRID == "":
		return errors.New("pr_id is required")
	case body == "":
		return errors.New("body is required")
	case len(body) > maxDraftBodyLength:
		return fmt.Errorf("body exceeds %d characters", maxDraftBodyLength)
	case r.ID != "":
		return nil // edits only replace the body
	case r.Path == "":
		return errors.New("path is required")
	case r.Side != "LEFT" && r.Side != "RIGHT":
		return fmt.Errorf("side must be LEFT or RIGHT, got %q", r.Side)
	case r.Line <= 0:
		return errors.New("line must be positive")
	case r.StartLine < 0 || (r.StartLine > 0 && r.StartLine >= r.Line):
		return errors.New("start_line must be before line")
	case r.CommitSHA == "":
		return errors.New("commit_sha is required")
	}
	return nil
}

func (d DraftDeps) logger(ctx context.Context, msg ws.InboundMessage) *slog.Logger {
	if d.Log != nil {
		return d.Log.With("request_id", msg.ID, "msg_type", msg.Type)
	}
	return logging.FromContext(ctx)
}

// currentLogin resolves the account drafts are namespaced under.
func (d DraftDeps) currentLogin(ctx context.Context) (string, error) {
	login, ok, err := d.CurrentUser.GetCurrentUser(ctx, githubProvider)
	if err != nil {
		return "", fmt.Errorf("resolve current user: %w", err)
	}
	if !ok || login == "" {
		return "", errors.New("drafts require a signed-in account")
	}
	return login, nil
}

func (d DraftDeps) broadcastChanged(prID string) {
	d.Broadcaster.Broadcast(prsReadChangedMessage{Type: "DRAFTS_CHANGED", Data: map[string]interface{}{"pr_id": prID}})
}

func draftResponse(id, msgType string, data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"id": id, "type": msgType, "data": data}
}

// NewDraftSaveHandler returns the handler for DRAFT_SAVE (create, or update body when id is set).
// Response: DRAFT_SAVED {draft} or {error}.
func NewDraftSaveHandler(deps DraftDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req draftSaveRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse DRAFT_SAVE: %w", err)
		}
		if err := req.validate(); err != nil {
			return draftResponse(msg.ID, "DRAFT_SAVED", map[string]interface{}{"error": err.Error()}), nil
		}
		login, err := deps.currentLogin(ctx)
		if err != nil {
			return draftResponse(msg.ID, "DRAFT_SAVED", map[string]interface{}{"error": err.Error()}), nil
		}
		body := strings.TrimSpace(req.Body)
		var saved store.ReviewDraft
		if req.ID != "" {
			saved, err = deps.Drafts.UpdateBody(ctx, login, req.ID, body)
		} else {
			saved, err = deps.Drafts.Create(ctx, login, store.ReviewDraft{
				PRID: req.PRID, Path: req.Path, Side: req.Side, StartLine: req.StartLine,
				Line: req.Line, Body: body, CommitSHA: req.CommitSHA,
			})
		}
		if err != nil {
			deps.logger(ctx, msg).Error("DRAFT_SAVE failed", "pr_id", req.PRID, "error", err)
			return draftResponse(msg.ID, "DRAFT_SAVED", map[string]interface{}{"error": "unable to save draft"}), nil
		}
		deps.broadcastChanged(req.PRID)
		return draftResponse(msg.ID, "DRAFT_SAVED", map[string]interface{}{"draft": saved}), nil
	}
}

// NewDraftsListHandler returns the handler for DRAFTS_LIST.
// Response: DRAFTS_READY {pr_id, drafts} or {pr_id, drafts: [], error}.
func NewDraftsListHandler(deps DraftDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse DRAFTS_LIST: %w", err)
		}
		if req.PRID == "" {
			return nil, errors.New("DRAFTS_LIST requires pr_id")
		}
		login, err := deps.currentLogin(ctx)
		if err != nil {
			return draftResponse(msg.ID, "DRAFTS_READY", map[string]interface{}{"pr_id": req.PRID, "drafts": []store.ReviewDraft{}}), nil
		}
		drafts, err := deps.Drafts.List(ctx, req.PRID, login)
		if err != nil {
			deps.logger(ctx, msg).Error("DRAFTS_LIST failed", "pr_id", req.PRID, "error", err)
			return draftResponse(msg.ID, "DRAFTS_READY", map[string]interface{}{"pr_id": req.PRID, "drafts": []store.ReviewDraft{}, "error": "unable to load drafts"}), nil
		}
		return draftResponse(msg.ID, "DRAFTS_READY", map[string]interface{}{"pr_id": req.PRID, "drafts": drafts}), nil
	}
}

// NewDraftDeleteHandler returns the handler for DRAFT_DELETE.
// Response: DRAFTS_DELETED {pr_id, deleted} or {error}.
func NewDraftDeleteHandler(deps DraftDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var req struct {
			PRID     string   `json:"pr_id"`
			DraftIDs []string `json:"draft_ids"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse DRAFT_DELETE: %w", err)
		}
		if req.PRID == "" || len(req.DraftIDs) == 0 {
			return nil, errors.New("DRAFT_DELETE requires pr_id and draft_ids")
		}
		login, err := deps.currentLogin(ctx)
		if err != nil {
			return draftResponse(msg.ID, "DRAFTS_DELETED", map[string]interface{}{"error": err.Error()}), nil
		}
		if err := deps.Drafts.Delete(ctx, login, req.DraftIDs); err != nil {
			deps.logger(ctx, msg).Error("DRAFT_DELETE failed", "pr_id", req.PRID, "error", err)
			return draftResponse(msg.ID, "DRAFTS_DELETED", map[string]interface{}{"error": "unable to delete drafts"}), nil
		}
		deps.broadcastChanged(req.PRID)
		return draftResponse(msg.ID, "DRAFTS_DELETED", map[string]interface{}{"pr_id": req.PRID, "deleted": len(req.DraftIDs)}), nil
	}
}
```
Reusing `prsReadChangedMessage` for a different push type is misleading. Rename it to `pushMessage` in `readstate.go` (it is `{Type, Data}`) and update its one existing use there.

`UpdateBody` returning `store.ErrDraftNotFound` maps to the generic "unable to save draft" error. That is fine: the UI only edits drafts it listed.

- [ ] **Step 4: Accept `review_event` on SUBMIT_ACTION**

In `threads.go` `NewSubmitActionHandler`:
- Add `ReviewEvent string \`json:"review_event"\`` to the request struct.
- Set `ReviewEvent: req.ReviewEvent,` in the `actions.ActionRequest` literal.
- Add `"submit_review"` to the doc comment's action list.

- [ ] **Step 5: Wire into `app.go`**

In `registerDiffAndCheckoutHandlers`, after `SUBMIT_ACTION`:
```go
	draftDeps := handler.DraftDeps{
		Drafts:      store.NewReviewDrafts(a.db.DB),
		CurrentUser: stores.currentUser,
		Broadcaster: a.wsServer,
		Log:         a.log,
	}
	a.wsServer.RegisterHandler("DRAFT_SAVE", handler.NewDraftSaveHandler(draftDeps))
	a.wsServer.RegisterHandler("DRAFT_DELETE", handler.NewDraftDeleteHandler(draftDeps))
	a.wsServer.RegisterHandler("DRAFTS_LIST", handler.NewDraftsListHandler(draftDeps))
```
In `attachOrchestratorLocked`, add `WithReviewDrafts(store.NewReviewDrafts(a.db.DB)).` to the builder chain after `WithActionQueue(...)`.

- [ ] **Step 6: Run the Go suite**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go/internal/server
git commit -m "feat(go): expose review drafts over WebSocket"
```

---

## Task 6: Renderer data layer — drafts schemas, services, store; fix approve/request changes

**Files:**
- Modify: `electron/src/renderer/types/index.ts`, `types/wsSchemas.ts`
- Modify: `electron/src/renderer/services/ws/events/index.ts`, `services/ws/events/review.ts`
- Create: `electron/src/renderer/services/reviewDrafts.ts`, `hooks/useReviewDrafts.ts`
- Modify: `electron/src/renderer/services/prActions.ts`, `services/prActions.test.ts`
- Modify: `electron/src/renderer/store/ui.ts`, `components/ActionFeedback.tsx`
- Modify: `electron/src/renderer/utils/keybindingActions.ts` (`detail.approve`), `pages/PRDetailPage.tsx` (remove `approve_confirmed` effect)

**Interfaces:**
- Consumes: WS contract from Task 5.
- Produces:
  ```ts
  // types/index.ts
  export const reviewDraftSchema; export type ReviewDraft = { id; pr_id; path; side: 'LEFT'|'RIGHT'; start_line: number /*0=single*/; line; body; commit_sha; created_at: string }
  // services/reviewDrafts.ts
  export interface DraftInput { prId: string; id: string | null; path: string; side: ReviewDraft['side']; startLine: number | null; line: number; body: string; commitSha: string }
  export function loadDrafts(prId: string): ResultAsync<readonly ReviewDraft[], Fault>
  export function saveDraft(input: Readonly<DraftInput>): ResultAsync<ReviewDraft, Fault>
  export function deleteDrafts(prId: string, ids: readonly string[]): ResultAsync<void, Fault>
  export function isDraftOutdated(draft: ReviewDraft, headSha: string): boolean
  // services/prActions.ts
  export type ReviewEvent = 'APPROVE' | 'REQUEST_CHANGES' | 'COMMENT'
  export function submitReview(prId: string, event: ReviewEvent, body?: string): void
  // approvePR / requestChanges now send SUBMIT_ACTION (Go absorbs drafts)
  // hooks/useReviewDrafts.ts
  export function useReviewDrafts(prId: string | undefined): void  // keeps appStore.reviewDrafts for prId
  // store/ui.ts UIState additions
  reviewDrafts: ReviewDraft[]; reviewDraftsPrId: string | null;
  ```
  `PendingAction['type']` and `ActionResult['type']` gain `'submit_review'`. `ConfirmingAction` loses `'approve' | 'approve_confirmed'`.

- [ ] **Step 1: Update the failing test** (`services/prActions.test.ts`)

Replace the file. Approve and request changes now go through `wsClient.request`; the second test is the regression for the dead `APPROVE_PR` message:
```ts
import { errAsync, ServiceError } from '@thourum/fault';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { approvePR, requestChanges } from './prActions';

const request = vi.hoisted(() => vi.fn());
vi.mock('./ws/connection', () => ({
  wsClient: { send: vi.fn(), request },
}));

describe('PR actions', () => {
  afterEach(() => {
    appStore.setState(() => initialUIState);
    request.mockReset();
  });

  it.each([
    ['approve', () => approvePR('pr-1')],
    ['request_changes', () => requestChanges('pr-1', 'Please revise')],
  ] as const)(
    'clears pending %s when the WebSocket cannot send',
    async (type, dispatch) => {
      request.mockReturnValue(
        errAsync(ServiceError('CONNECTION_ERROR', 'WebSocket not connected')),
      );
      dispatch();
      await vi.waitFor(() => expect(appStore.state.pendingAction).toBeNull());

      expect(appStore.state.actionResult).toMatchObject({
        type,
        prId: 'pr-1',
        success: false,
        message: 'WebSocket not connected',
      });
    },
  );

  it('submits approve through SUBMIT_ACTION so the engine can absorb drafts', () => {
    request.mockReturnValue(errAsync(ServiceError('CONNECTION_ERROR', 'x')));
    approvePR('pr-1');
    expect(request).toHaveBeenCalledWith('SUBMIT_ACTION', {
      pr_id: 'pr-1',
      action: 'approve',
    });
  });
});
```
Confirm `errAsync` and `ServiceError` are exported by `@thourum/fault` (`wsSchemas.ts` imports `ServiceError`; check `node_modules/@thourum/fault` for `errAsync`). If `errAsync` is missing, use `new ResultAsync(Promise.resolve(err(ServiceError(...))))`.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd electron && pnpm vitest run src/renderer/services/prActions.test.ts`
Expected: FAIL — `request` not called / pending not cleared (approve still uses `send('APPROVE_PR')`).

- [ ] **Step 3: Types and schemas**

`types/index.ts` — append after the `ReviewThread` block:
```ts
/** A pending line comment the engine stores until it is submitted as a review. */
export const reviewDraftSchema = z.object({
  id: z.string(),
  pr_id: z.string(),
  path: z.string(),
  side: z.enum(['LEFT', 'RIGHT']),
  /** 0 = single-line comment; otherwise the first line of the range. */
  start_line: z.number().int().nonnegative(),
  line: z.number().int().positive(),
  body: z.string(),
  commit_sha: z.string(),
  created_at: z.string(),
});
export type ReviewDraft = z.infer<typeof reviewDraftSchema>;
```
Add `'submit_review'` to the `type` unions of `PendingAction` and `ActionResult`. Then add a `submit_review` label to `ACTION_LABELS` in `components/ActionFeedback.tsx` (`'Submitting review'`, matching the file's existing label style).

`types/wsSchemas.ts`:
- Add to `dataSchemas`: `DRAFTS_CHANGED: z.object({ pr_id: z.string() }),`, `DRAFTS_READY: z.unknown(),`, `DRAFT_SAVED: z.unknown(),`, `DRAFTS_DELETED: z.unknown(),`
- Add `eventSchema('DRAFTS_CHANGED')`, `eventSchema('DRAFTS_READY')`, `eventSchema('DRAFT_SAVED')`, and `eventSchema('DRAFTS_DELETED')` to `inboundMessageSchema`.

- [ ] **Step 4: Store fields**

`store/ui.ts`:
- Remove `'approve'` and `'approve_confirmed'` from `ConfirmingAction`.
- Import `ReviewDraft` from `../types`.
- Add to `UIState`:
```ts
  /** Current user's pending line-comment drafts for `reviewDraftsPrId`. */
  reviewDrafts: ReviewDraft[];
  /** PR whose drafts `reviewDrafts` holds; set by useReviewDrafts. */
  reviewDraftsPrId: string | null;
```
- Initial values: `reviewDrafts: []`, `reviewDraftsPrId: null`.

- [ ] **Step 5: Services**

`services/reviewDrafts.ts`:
```ts
import { Fault, type ResultAsync } from '@thourum/fault';
import { z } from 'zod';
import { appStore } from '../store';
import { type ReviewDraft, reviewDraftSchema } from '../types';
import { parseResponse } from '../utils/error';
import { wsClient } from './ws/connection';

const draftsReadySchema = z.object({
  pr_id: z.string(),
  drafts: z
    .array(reviewDraftSchema)
    .nullish()
    .transform((drafts) => drafts ?? []),
});
const draftSavedSchema = z.object({ draft: reviewDraftSchema });

export interface DraftInput {
  prId: string;
  /** Non-null edits that draft's body; position fields are then ignored. */
  id: string | null;
  path: string;
  side: ReviewDraft['side'];
  startLine: number | null;
  line: number;
  body: string;
  commitSha: string;
}

function networkFault(cause: Fault): Fault {
  return new Fault(cause).withTag('NETWORK_ERROR').withCause(cause);
}

/** Loads the current user's drafts and stores them if `prId` is still shown. */
export function loadDrafts(
  prId: string,
): ResultAsync<readonly ReviewDraft[], Fault> {
  return wsClient
    .request('DRAFTS_LIST', { pr_id: prId })
    .mapErr(networkFault)
    .andThen((data) => parseResponse(draftsReadySchema, data))
    .map(({ drafts }) => {
      appStore.setState((state) =>
        state.reviewDraftsPrId === prId
          ? { ...state, reviewDrafts: drafts }
          : state,
      );
      return drafts;
    });
}

/** Creates or edits a draft; the DRAFTS_CHANGED push refreshes the list. */
export function saveDraft(
  input: Readonly<DraftInput>,
): ResultAsync<ReviewDraft, Fault> {
  return wsClient
    .request('DRAFT_SAVE', {
      pr_id: input.prId,
      id: input.id ?? '',
      path: input.path,
      side: input.side,
      start_line: input.startLine ?? 0,
      line: input.line,
      body: input.body,
      commit_sha: input.commitSha,
    })
    .mapErr(networkFault)
    .andThen((data) => parseResponse(draftSavedSchema, data))
    .map(({ draft }) => draft);
}

/** Deletes drafts; the DRAFTS_CHANGED push refreshes the list. */
export function deleteDrafts(
  prId: string,
  ids: readonly string[],
): ResultAsync<void, Fault> {
  return wsClient
    .request('DRAFT_DELETE', { pr_id: prId, draft_ids: ids })
    .mapErr(networkFault)
    .map(() => undefined);
}

/** A draft written against an older head; GitHub still accepts its own commit. */
export function isDraftOutdated(draft: ReviewDraft, headSha: string): boolean {
  return headSha !== '' && draft.commit_sha !== headSha;
}
```
`deleteDrafts` with zero ids is a request the engine rejects. Callers guard with `ids.length > 0`.

`services/prActions.ts` — replace `approvePR`, `requestChanges`, `postComment`, and `replyComment` with one dispatch helper plus thin exports:
```ts
export type ReviewEvent = 'APPROVE' | 'REQUEST_CHANGES' | 'COMMENT';

interface ActionDispatch {
  type: ActionType;
  prId: string;
  payload: Readonly<Record<string, unknown>>;
  successMessage: string;
  failureMessage: string;
}

/** Sends SUBMIT_ACTION and settles pendingAction/actionResult from its reply. */
function dispatchAction({
  type,
  prId,
  payload,
  successMessage,
  failureMessage,
}: ActionDispatch): void {
  setPendingAction(type, prId);
  void wsClient
    .request('SUBMIT_ACTION', { pr_id: prId, action: type, ...payload })
    .mapErr((cause) =>
      new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
    )
    .match(
      (data) => {
        const parsed = actionResponseSchema.safeParse(data);
        const success = parsed.success && parsed.data.success;
        appStore.setState((s) => ({
          ...s,
          pendingAction: null,
          actionResult: {
            type,
            prId,
            success,
            message: success
              ? successMessage
              : ((parsed.success ? parsed.data.error : null) ??
                failureMessage),
          },
          commentsRevision: success
            ? s.commentsRevision + 1
            : s.commentsRevision,
          threadsRevision: success ? s.threadsRevision + 1 : s.threadsRevision,
        }));
      },
      (fault) => setFailedAction(type, prId, fault.message),
    );
}

/** Approves; the engine folds pending drafts into the same review. */
export function approvePR(prId: string): void {
  dispatchAction({ type: 'approve', prId, payload: {}, successMessage: 'Approved', failureMessage: 'Approval failed' });
}

/** Requests changes; the engine folds pending drafts into the same review. */
export function requestChanges(prId: string, comment: string): void {
  dispatchAction({ type: 'request_changes', prId, payload: { body: comment }, successMessage: 'Changes requested', failureMessage: 'Request changes failed' });
}

/** Submits pending drafts as one review with the given event. */
export function submitReview(prId: string, event: ReviewEvent, body = ''): void {
  dispatchAction({ type: 'submit_review', prId, payload: { review_event: event, body }, successMessage: 'Review submitted', failureMessage: 'Review failed' });
}

/** Posts a PR comment and refreshes comments after a successful response. */
export function postComment(prId: string, body: string): void {
  const trimmed = body.trim();
  if (trimmed.length === 0) return;
  dispatchAction({ type: 'post_comment', prId, payload: { body: trimmed }, successMessage: 'Comment posted', failureMessage: 'Comment failed' });
}

/** Replies to a comment and refreshes comments and threads on success. */
export function replyComment(prId: string, body: string, target: Readonly<ReplyTarget>): void {
  const trimmed = body.trim();
  if (trimmed.length === 0) return;
  dispatchAction({
    type: 'reply_comment',
    prId,
    payload: { body: trimmed, thread_id: target.threadId ?? '', comment_id: target.commentId ?? '' },
    successMessage: 'Reply posted',
    failureMessage: 'Reply failed',
  });
}
```
The test asserts `{ pr_id, action: 'approve' }` exactly, so `approvePR` must send an empty payload. Run Biome (`pnpm lint --write` or the project's format script) to restore formatting.

- [ ] **Step 6: Push handler and hook**

`services/ws/events/review.ts` — add:
```ts
/** Reloads drafts when the engine changed the shown PR's drafts. */
export function handleDraftsChanged({
  data,
}: Readonly<WSInboundMessage<'DRAFTS_CHANGED'>>): void {
  if (appStore.state.reviewDraftsPrId !== data.pr_id) return;
  void loadDrafts(data.pr_id).mapErr((fault) =>
    log.warn({ err: fault.message }, 'failed to reload review drafts'),
  );
}
```
Import `loadDrafts` from `../../reviewDrafts`. Use the file's logger if it has one; otherwise add `const log = createLogger('ReviewEvents');` from `../../../utils/logger`. In `services/ws/events/index.ts`, register `DRAFTS_CHANGED: handleDraftsChanged`, and register `resolveWsResponse` for `DRAFTS_READY`, `DRAFT_SAVED`, and `DRAFTS_DELETED`.

`hooks/useReviewDrafts.ts`:
```ts
import { useEffect } from 'react';
import { loadDrafts } from '../services/reviewDrafts';
import { appStore } from '../store';
import { createLogger } from '../utils/logger';

const log = createLogger('useReviewDrafts');

/** Keeps appStore.reviewDrafts loaded for the PR the page shows. */
export function useReviewDrafts(prId: string | undefined): void {
  useEffect(() => {
    if (prId === undefined) return;
    appStore.setState((s) => ({ ...s, reviewDraftsPrId: prId, reviewDrafts: [] }));
    void loadDrafts(prId).mapErr((fault) =>
      log.warn({ err: fault.message }, 'failed to load review drafts'),
    );
    return () => {
      appStore.setState((s) =>
        s.reviewDraftsPrId === prId
          ? { ...s, reviewDraftsPrId: null, reviewDrafts: [] }
          : s,
      );
    };
  }, [prId]);
}
```

- [ ] **Step 7: Make `a a` approve at once**

In `utils/keybindingActions.ts`:
- Delete `toggleApproval`.
- Import `approvePR` from `../services/prActions`.
- Set `'detail.approve': (context) => { if (context.prId !== undefined) approvePR(context.prId); },`.

In `pages/PRDetailPage.tsx`, delete the `approve_confirmed` effect and the now-unused `approvePR` import. In the same file, call `useReviewDrafts(detailPrId);` next to `usePatchsets`.

- [ ] **Step 8: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS. Type-check flags every exhaustive map over the action unions that needs a `submit_review` entry; fix each one.

- [ ] **Step 9: Commit**

```bash
git add electron/src/renderer
git commit -m "fix(electron): route approve and request changes through SUBMIT_ACTION; add review drafts data layer"
```

---

## Task 7: Pure line-comment cursor

**Files:**
- Create: `electron/src/renderer/utils/lineCommentCursor.ts`
- Test: `electron/src/renderer/utils/lineCommentCursor.test.ts`

**Interfaces:**
- Produces:
  ```ts
  export type ReviewSide = 'LEFT' | 'RIGHT';
  export type DiffLayout = 'unified' | 'split';
  export interface CursorHunk { deletionStart: number; additionStart: number; hunkContent: readonly ({ type: 'context'; lines: number } | { type: 'change'; deletions: number; additions: number })[] }
  export interface CommentableLine { side: ReviewSide; line: number; row: number; hunkIndex: number; isChange: boolean }
  export interface LineCursor { side: ReviewSide; anchor: number; head: number }
  export interface CommentRange { side: ReviewSide; startLine: number | null; line: number }
  export interface LineCommentState { prId: string; fileId: string; path: string; headSha: string; layout: DiffLayout; lines: readonly CommentableLine[]; cursor: LineCursor; composer: { draftId: string | null; initialBody: string } | null }
  export function listCommentableLines(hunks: readonly CursorHunk[], layout: DiffLayout): CommentableLine[]
  export function initialCursor(lines: readonly CommentableLine[]): LineCursor | null
  export function moveCursor(lines: readonly CommentableLine[], cursor: LineCursor, layout: DiffLayout, delta: 1 | -1): LineCursor
  export function extendCursor(lines: readonly CommentableLine[], cursor: LineCursor, delta: 1 | -1): LineCursor
  export function switchSide(lines: readonly CommentableLine[], cursor: LineCursor, target: ReviewSide): LineCursor
  export function cursorRange(cursor: LineCursor): CommentRange
  export function toAnnotationSide(side: ReviewSide): 'deletions' | 'additions'
  ```
  `@pierre/diffs` `Hunk` is structurally assignable to `CursorHunk`.

- [ ] **Step 1: Write the failing test**

`utils/lineCommentCursor.test.ts`:
```ts
import { describe, expect, it } from 'vitest';
import {
  type CursorHunk,
  cursorRange,
  extendCursor,
  initialCursor,
  type LineCursor,
  listCommentableLines,
  moveCursor,
  switchSide,
} from './lineCommentCursor';

// @@ -10,4 +10,3 @@ : context 10, -11, -12, +11, context 13→12
const HUNK: CursorHunk = {
  deletionStart: 10,
  additionStart: 10,
  hunkContent: [
    { type: 'context', lines: 1 },
    { type: 'change', deletions: 2, additions: 1 },
    { type: 'context', lines: 1 },
  ],
};
const SECOND_HUNK: CursorHunk = {
  deletionStart: 40,
  additionStart: 39,
  hunkContent: [{ type: 'change', deletions: 0, additions: 2 }],
};

const at = (side: 'LEFT' | 'RIGHT', line: number): LineCursor => ({
  side,
  anchor: line,
  head: line,
});
const spots = (lines: ReturnType<typeof listCommentableLines>) =>
  lines.map(({ side, line, row }) => `${side}${line}@${row}`);

describe('listCommentableLines', () => {
  it('lists unified rows in display order with context on the right side', () => {
    expect(spots(listCommentableLines([HUNK], 'unified'))).toEqual([
      'RIGHT10@0',
      'LEFT11@1',
      'LEFT12@2',
      'RIGHT11@3',
      'RIGHT12@4',
    ]);
  });

  it('pairs both sides of a split row', () => {
    expect(spots(listCommentableLines([HUNK], 'split'))).toEqual([
      'LEFT10@0',
      'RIGHT10@0',
      'LEFT11@1',
      'RIGHT11@1',
      'LEFT12@2',
      'LEFT13@3',
      'RIGHT12@3',
    ]);
  });
});

describe('cursor movement', () => {
  const unified = listCommentableLines([HUNK, SECOND_HUNK], 'unified');
  const split = listCommentableLines([HUNK], 'split');

  it('starts on the first changed line', () => {
    expect(initialCursor(unified)).toEqual(at('LEFT', 11));
    expect(initialCursor([])).toBeNull();
  });

  it('walks every unified row across sides and clamps at the file end', () => {
    let cursor = at('LEFT', 12);
    cursor = moveCursor(unified, cursor, 'unified', 1);
    expect(cursor).toEqual(at('RIGHT', 11));
    cursor = moveCursor(unified, at('RIGHT', 40), 'unified', 1);
    expect(cursor).toEqual(at('RIGHT', 40));
    expect(moveCursor(unified, at('RIGHT', 10), 'unified', -1)).toEqual(
      at('RIGHT', 10),
    );
  });

  it('stays on its side in split view', () => {
    expect(moveCursor(split, at('LEFT', 12), 'split', 1)).toEqual(
      at('LEFT', 13),
    );
  });

  it('collapses an extended range before moving', () => {
    const ranged: LineCursor = { side: 'RIGHT', anchor: 10, head: 11 };
    expect(moveCursor(unified, ranged, 'unified', 1)).toEqual(at('RIGHT', 12));
  });
});

describe('range extension', () => {
  const unified = listCommentableLines([HUNK, SECOND_HUNK], 'unified');

  it('extends on the anchor side in either direction', () => {
    const down = extendCursor(unified, at('RIGHT', 11), 1);
    expect(cursorRange(down)).toEqual({
      side: 'RIGHT',
      startLine: 11,
      line: 12,
    });
    const up = extendCursor(
      unified,
      extendCursor(unified, at('RIGHT', 11), -1),
      -1,
    );
    expect(cursorRange(up)).toEqual({ side: 'RIGHT', startLine: 10, line: 11 });
  });

  it('never crosses into the next hunk', () => {
    expect(extendCursor(unified, at('RIGHT', 12), 1)).toEqual(at('RIGHT', 12));
  });

  it('reports a single line without a start line', () => {
    expect(cursorRange(at('LEFT', 11))).toEqual({
      side: 'LEFT',
      startLine: null,
      line: 11,
    });
  });
});

describe('switchSide', () => {
  const split = listCommentableLines([HUNK], 'split');

  it('jumps to the nearest row on the other side, preferring the earlier row', () => {
    expect(switchSide(split, at('LEFT', 12), 'RIGHT')).toEqual(at('RIGHT', 11));
  });

  it('is a no-op when already on the target side', () => {
    expect(switchSide(split, at('RIGHT', 11), 'RIGHT')).toEqual(
      at('RIGHT', 11),
    );
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd electron && pnpm vitest run src/renderer/utils/lineCommentCursor.test.ts`
Expected: FAIL — cannot resolve `./lineCommentCursor`.

- [ ] **Step 3: Implement**

`utils/lineCommentCursor.ts`:
```ts
/**
 * Keyboard cursor for picking line-comment targets in a diff. Pure: the
 * comment overlay stores a LineCommentState and keybinding actions replace it.
 * Coordinates are GitHub's: LEFT = base file line, RIGHT = head file line.
 */

export type ReviewSide = 'LEFT' | 'RIGHT';
export type DiffLayout = 'unified' | 'split';

/** The parts of a parsed @pierre/diffs hunk the cursor needs. */
export interface CursorHunk {
  deletionStart: number;
  additionStart: number;
  hunkContent: readonly (
    | { type: 'context'; lines: number }
    | { type: 'change'; deletions: number; additions: number }
  )[];
}

export interface CommentableLine {
  side: ReviewSide;
  line: number;
  /** Display row; split view puts both sides of a row on the same number. */
  row: number;
  hunkIndex: number;
  isChange: boolean;
}

/** anchor/head are line numbers on `side`; anchor stays put while extending. */
export interface LineCursor {
  side: ReviewSide;
  anchor: number;
  head: number;
}

export interface CommentRange {
  side: ReviewSide;
  /** Null for a single-line comment. */
  startLine: number | null;
  line: number;
}

export interface LineCommentState {
  prId: string;
  fileId: string;
  path: string;
  /** PR head when the overlay opened; becomes the draft's commit_sha. */
  headSha: string;
  layout: DiffLayout;
  lines: readonly CommentableLine[];
  cursor: LineCursor;
  /** Set once Enter opens the composer; draftId is non-null when editing. */
  composer: { draftId: string | null; initialBody: string } | null;
}

export function listCommentableLines(
  hunks: readonly CursorHunk[],
  layout: DiffLayout,
): CommentableLine[] {
  const lines: CommentableLine[] = [];
  let row = 0;
  hunks.forEach((hunk, hunkIndex) => {
    let oldLine = hunk.deletionStart;
    let newLine = hunk.additionStart;
    for (const block of hunk.hunkContent) {
      if (block.type === 'context') {
        for (let offset = 0; offset < block.lines; offset += 1) {
          if (layout === 'split')
            lines.push({ side: 'LEFT', line: oldLine, row, hunkIndex, isChange: false });
          lines.push({ side: 'RIGHT', line: newLine, row, hunkIndex, isChange: false });
          oldLine += 1;
          newLine += 1;
          row += 1;
        }
        continue;
      }
      const blockStart = row;
      // Unified view shows a change block's deletions above its additions.
      const additionsStart =
        layout === 'split' ? blockStart : blockStart + block.deletions;
      for (let offset = 0; offset < block.deletions; offset += 1)
        lines.push({ side: 'LEFT', line: oldLine + offset, row: blockStart + offset, hunkIndex, isChange: true });
      for (let offset = 0; offset < block.additions; offset += 1)
        lines.push({ side: 'RIGHT', line: newLine + offset, row: additionsStart + offset, hunkIndex, isChange: true });
      oldLine += block.deletions;
      newLine += block.additions;
      row =
        layout === 'split'
          ? blockStart + Math.max(block.deletions, block.additions)
          : additionsStart + block.additions;
    }
  });
  return lines.sort(
    (left, right) =>
      left.row - right.row ||
      (left.side === right.side ? 0 : left.side === 'LEFT' ? -1 : 1),
  );
}

function collapsedAt(entry: CommentableLine): LineCursor {
  return { side: entry.side, anchor: entry.line, head: entry.line };
}

function stepWithin(
  candidates: readonly CommentableLine[],
  cursor: { side: ReviewSide; line: number },
  delta: 1 | -1,
): CommentableLine | undefined {
  const index = candidates.findIndex(
    (entry) => entry.side === cursor.side && entry.line === cursor.line,
  );
  if (index < 0) return candidates[0];
  const next = Math.min(Math.max(index + delta, 0), candidates.length - 1);
  return candidates[next];
}

export function initialCursor(
  lines: readonly CommentableLine[],
): LineCursor | null {
  const first = lines.find((entry) => entry.isChange) ?? lines[0];
  return first === undefined ? null : collapsedAt(first);
}

/** Moves the head one row and collapses the range; split view keeps the side. */
export function moveCursor(
  lines: readonly CommentableLine[],
  cursor: LineCursor,
  layout: DiffLayout,
  delta: 1 | -1,
): LineCursor {
  const candidates =
    layout === 'split'
      ? lines.filter((entry) => entry.side === cursor.side)
      : lines;
  const next = stepWithin(candidates, { side: cursor.side, line: cursor.head }, delta);
  return next === undefined ? cursor : collapsedAt(next);
}

/** Moves only the head, staying on the anchor's side and hunk (GitHub rule). */
export function extendCursor(
  lines: readonly CommentableLine[],
  cursor: LineCursor,
  delta: 1 | -1,
): LineCursor {
  const anchor = lines.find(
    (entry) => entry.side === cursor.side && entry.line === cursor.anchor,
  );
  if (anchor === undefined) return cursor;
  const candidates = lines.filter(
    (entry) => entry.side === cursor.side && entry.hunkIndex === anchor.hunkIndex,
  );
  const next = stepWithin(candidates, { side: cursor.side, line: cursor.head }, delta);
  return next === undefined ? cursor : { ...cursor, head: next.line };
}

/** Split view: jump to the nearest row on `target`; ties go to the earlier row. */
export function switchSide(
  lines: readonly CommentableLine[],
  cursor: LineCursor,
  target: ReviewSide,
): LineCursor {
  if (cursor.side === target) return cursor;
  const current = lines.find(
    (entry) => entry.side === cursor.side && entry.line === cursor.head,
  );
  const others = lines.filter((entry) => entry.side === target);
  const [first] = others;
  if (current === undefined || first === undefined) return cursor;
  const nearest = others.reduce(
    (best, entry) =>
      Math.abs(entry.row - current.row) < Math.abs(best.row - current.row)
        ? entry
        : best,
    first,
  );
  return collapsedAt(nearest);
}

export function cursorRange(cursor: LineCursor): CommentRange {
  const start = Math.min(cursor.anchor, cursor.head);
  const end = Math.max(cursor.anchor, cursor.head);
  return { side: cursor.side, startLine: start === end ? null : start, line: end };
}

export function toAnnotationSide(side: ReviewSide): 'deletions' | 'additions' {
  return side === 'LEFT' ? 'deletions' : 'additions';
}
```
`switchSide` is only bound in split view. In unified view a target side is still reachable with `j`/`k`, so the function does not check the layout.

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm vitest run src/renderer/utils/lineCommentCursor.test.ts && pnpm lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/utils/lineCommentCursor.ts electron/src/renderer/utils/lineCommentCursor.test.ts
git commit -m "feat(electron): add pure line-comment cursor"
```

---

## Task 8: Comment mode — overlay, bindings, highlight, composer

**Files:**
- Modify: `electron/src/renderer/overlays/OverlayContext.tsx`
- Modify: `electron/src/renderer/store/ui.ts`
- Modify: `electron/src/renderer/utils/keybindings.ts`, `utils/keybindingActions.ts`
- Modify: `electron/src/renderer/components/diff/diffAnnotations.ts` (+ `diffAnnotations.test.ts`), `diffCodeViewItems.ts`, `diffViewNavigation.ts`, `diffCodeViewNavigation.ts`, `DiffView.tsx`
- Create: `electron/src/renderer/components/diff/useLineCommentMode.ts`, `components/diff/diffDraftAnnotation.tsx`
- Modify: `electron/src/renderer/pages/DiffPage.tsx`

**Interfaces:**
- Consumes: Task 7 cursor API; `saveDraft`, `useReviewDrafts` (Task 6).
- Produces:
  ```ts
  // OverlayKind gains 'lineComment'; OverlayContextValue gains openLineComment(): void
  // UIState: lineComment: LineCommentState | null
  // diffAnnotations.ts
  export type DiffAnnotationMeta =
    | { kind: 'thread'; key: string; thread: ReviewThread }
    | { kind: 'draft'; key: string; draft: ReviewDraft; isOutdated: boolean }
    | { kind: 'composer'; key: string; lineComment: LineCommentState };
  export const COMPOSER_ANNOTATION_KEY = 'line-comment-composer';
  export function draftAnnotationKey(draftId: string): string   // `draft:${id}`
  export function draftIdFromKey(key: string | null): string | null
  export function lineExistsOnSide(fileDiff: FileDiffMetadata, line: number, side: 'deletions' | 'additions'): boolean
  // AnnotationEntry gains side: 'deletions' | 'additions'
  // app events: 'bistro:diff:line-comment' (open), 'bistro:diff:edit-draft' (Task 9)
  // DiffView props gain: prId?: string; headSha?: string; drafts?: readonly ReviewDraft[]
  ```
  Keybinding action ids: `diff.lineComment` (`c`, DIFF), and in the `lineComment` overlay `lineComment.down` `j`, `lineComment.up` `k`, `lineComment.extendDown` `Shift+J`, `lineComment.extendUp` `Shift+K`, `lineComment.left` `h`, `lineComment.right` `l`, `lineComment.compose` `Enter`.

- [ ] **Step 1: Update the failing annotation test**

Rewrite `components/diff/diffAnnotations.test.ts` around the new contract. Keep its existing cases, renamed from `lineExistsInAdditions` to `lineExistsOnSide(fileDiff, line, 'additions')`, and add:
```ts
it('checks deletion-side lines against the old file range', () => {
  const fileDiff = processFile(
    'diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n@@ -10,2 +10,1 @@\n-old\n-gone\n+new\n',
  );
  if (fileDiff === undefined) throw new Error('expected a parsed diff');
  expect(lineExistsOnSide(fileDiff, 11, 'deletions')).toBe(true);
  expect(lineExistsOnSide(fileDiff, 12, 'deletions')).toBe(false);
});

it('orders drafts with threads and keys them apart', () => {
  const entries = buildOrderedAnnotations([file], [thread], [draft]);
  expect(entries.map((entry) => entry.key)).toEqual([
    draftAnnotationKey(draft.id),
    thread.id,
  ]);
  expect(draftIdFromKey(entries[0]?.key ?? null)).toBe(draft.id);
  expect(draftIdFromKey(thread.id)).toBeNull();
});
```
Build `file`, `thread` (line 20, path `a.ts`), and `draft` (`side: 'LEFT'`, `line: 11`, path `a.ts`) with the fixtures already in this test file. If processFile usage in the existing test differs (e.g. imported from `@pierre/diffs`), match it. `throw` in a test fixture guard is test-only code, as in `keybindings.test.ts`.

- [ ] **Step 2: Run to verify it fails**

Run: `cd electron && pnpm vitest run src/renderer/components/diff/diffAnnotations.test.ts`
Expected: FAIL — `lineExistsOnSide` / `draftAnnotationKey` not exported.

- [ ] **Step 3: Generalize annotations** (`diffAnnotations.ts`)

```ts
import type { AnnotationSide, DiffLineAnnotation, FileDiffMetadata } from '@pierre/diffs';
import type { ReviewDraft, ReviewThread } from '../../types';
import { type LineCommentState, cursorRange, toAnnotationSide } from '../../utils/lineCommentCursor';
import type { ParsedDiffFile } from './diffPathUtils';

const DRAFT_KEY_PREFIX = 'draft:';
export const COMPOSER_ANNOTATION_KEY = 'line-comment-composer';

export type DiffAnnotationMeta =
  | { kind: 'thread'; key: string; thread: ReviewThread }
  | { kind: 'draft'; key: string; draft: ReviewDraft; isOutdated: boolean }
  | { kind: 'composer'; key: string; lineComment: LineCommentState };

export function draftAnnotationKey(draftId: string): string {
  return `${DRAFT_KEY_PREFIX}${draftId}`;
}

export function draftIdFromKey(key: string | null): string | null {
  return key?.startsWith(DRAFT_KEY_PREFIX) ? key.slice(DRAFT_KEY_PREFIX.length) : null;
}

/** A flattened, ordered annotation entry used for n/p keyboard navigation. */
export interface AnnotationEntry {
  /** Thread id, or draftAnnotationKey(draft.id). */
  key: string;
  fileIndex: number;
  fileId: string;
  line: number;
  side: AnnotationSide;
}
```
- Rewrite `buildOrderedAnnotations(files, threads, drafts: readonly ReviewDraft[] = [])` to emit entries for current-anchor threads (`side: 'additions'`) and for drafts on that file (`key: draftAnnotationKey`, `side: toAnnotationSide(draft.side)`). Sort by `line`, then `key`.
- Drop `threadId` from `AnnotationEntry`. Grep for `.threadId` users (`diffViewNavigation.ts`) and use `.key`.
- Rewrite `buildFileAnnotations(file, { threads, drafts, headSha, lineComment })` to return `DiffLineAnnotation<DiffAnnotationMeta>[]`:
  - threads → `{ side: 'additions', lineNumber: thread.line, metadata: { kind: 'thread', key: thread.id, thread } }`
  - drafts with `draft.path === file.path` → `{ side: toAnnotationSide(draft.side), lineNumber: draft.line, metadata: { kind: 'draft', key, draft, isOutdated: isDraftOutdated(draft, headSha) } }`. Skip the draft being edited (`lineComment?.composer?.draftId === draft.id`); the composer replaces it.
  - when `lineComment?.composer != null && lineComment.fileId === file.id` → `{ side: toAnnotationSide(range.side), lineNumber: range.line, metadata: { kind: 'composer', key: COMPOSER_ANNOTATION_KEY, lineComment } }` with `range = cursorRange(lineComment.cursor)`.
- Replace `lineExistsInAdditions` with:
```ts
export function lineExistsOnSide(
  fileDiff: FileDiffMetadata,
  lineNumber: number,
  side: AnnotationSide,
): boolean {
  return fileDiff.hunks.some((hunk) => {
    const start = side === 'additions' ? hunk.additionStart : hunk.deletionStart;
    const count = side === 'additions' ? hunk.additionCount : hunk.deletionCount;
    return lineNumber >= start && lineNumber < start + count;
  });
}
```
Keep the existing doc comment (reworded for both sides). Import `isDraftOutdated` from `../../services/reviewDrafts`.

- [ ] **Step 4: Thread the new metadata type through the diff view**

- `diffCodeViewItems.ts`: `CodeViewItemsOptions` gains `drafts: readonly ReviewDraft[]`, `headSha: string`, and `lineComment: LineCommentState | null`. `buildCodeViewItems` returns `CodeViewItem<DiffAnnotationMeta>[]` and calls the new `buildFileAnnotations`. In `buildItemVersion`, serialize each annotation by kind: threads as today (use `metadata.thread`); drafts as `[side, line, key, draft.body, isOutdated, activeAnnotationKey === key]`; composer as `[key, composer.draftId, range.side, range.startLine, range.line]`.
- `diffViewNavigation.ts` and `diffCodeViewNavigation.ts`: change `CodeViewHandle<ReviewThread, unknown>` to `CodeViewHandle<DiffAnnotationMeta, unknown>`. In `useInlineDiffAnnotations`, replace `lineExistsInAdditions(parsedFile.fileDiff, entry.line)` with `lineExistsOnSide(parsedFile.fileDiff, entry.line, entry.side)` and pass `side: entry.side` to `scrollTo`.
- `DiffView.tsx`:
  - `DiffViewProps` adds `prId?: string; headSha?: string; drafts?: readonly ReviewDraft[]`.
  - `useDiffViewData` passes `drafts` into `buildOrderedAnnotations` and `buildCodeViewItems`. It also reads `lineComment` from `appStore` and passes it to `buildCodeViewItems`.
  - `renderAnnotation` switches on `annotation.metadata?.kind`: `'thread'` → existing `DiffAnnotation` with `metadata.thread`; `'draft'` → `<DraftAnnotationCard … />`; `'composer'` → `<LineCommentComposer lineComment={metadata.lineComment} />`. The draft card is completed in Task 9; until then render `null` for `'draft'` so this task type-checks.
  - Pass `selectedLines={selectedLines}` to `<CodeView>`. It comes from `useLineCommentMode` (Step 6). Do **not** set `enableLineSelection`; it enables pointer selection only (see "Decisions" #3).

- [ ] **Step 5: Overlay, store, bindings, actions**

`OverlayContext.tsx`: add `'lineComment'` to `OverlayKind`. Add `openLineComment(): void` to the interface, a memoized `openLineComment = useCallback(() => setActiveOverlay('lineComment'), [])`, and include it in `value` and its deps.

`store/ui.ts`: import `LineCommentState` from `../utils/lineCommentCursor`. Add `/** Line-pick overlay state (diff \`c\`); null when comment mode is closed. */ lineComment: LineCommentState | null;` with initial value `null`.

`utils/keybindings.ts`:
- Add `'lineComment'` to `ALL_OVERLAYS` so `global.escape` and quit work in comment mode.
- Add `const LINE_COMMENT = { overlays: ['lineComment'] } as const;`.
- Append to `DETAIL_BINDINGS` (DIFF-activated entries already live there):
```ts
  {
    actionId: 'diff.lineComment',
    keys: 'c',
    description: 'Comment on line',
    activation: DIFF,
  },
```
- Add a new array and spread it into `DEFAULT_KEYBINDINGS`:
```ts
const LINE_COMMENT_BINDINGS: readonly KeyBinding[] = [
  { actionId: 'lineComment.down', keys: 'j', description: 'Next line', activation: LINE_COMMENT },
  { actionId: 'lineComment.up', keys: 'k', description: 'Previous line', activation: LINE_COMMENT },
  { actionId: 'lineComment.extendDown', keys: 'Shift+J', description: 'Extend range down', activation: LINE_COMMENT },
  { actionId: 'lineComment.extendUp', keys: 'Shift+K', description: 'Extend range up', activation: LINE_COMMENT },
  { actionId: 'lineComment.left', keys: 'h', description: 'Old side (split view)', activation: LINE_COMMENT },
  { actionId: 'lineComment.right', keys: 'l', description: 'New side (split view)', activation: LINE_COMMENT },
  { actionId: 'lineComment.compose', keys: 'Enter', description: 'Write comment', activation: LINE_COMMENT },
];
```

`utils/keybindingActions.ts` (import `extendCursor`, `moveCursor`, `switchSide`, and `type LineCommentState` from `./lineCommentCursor`):
```ts
/** Cursor keys only steer while the composer is closed. */
function updateLineComment(
  context: KeyActionContext,
  update: (state: LineCommentState) => LineCommentState,
): void {
  context.store.setState((state) =>
    state.lineComment === null || state.lineComment.composer !== null
      ? state
      : { ...state, lineComment: update(state.lineComment) },
  );
}

function composeLineComment(context: KeyActionContext): void {
  context.store.setState((state) =>
    state.lineComment === null
      ? state
      : {
          ...state,
          lineComment: {
            ...state.lineComment,
            composer: { draftId: null, initialBody: '' },
          },
        },
  );
}
```
ACTIONS entries:
```ts
  'diff.lineComment': () => emitAppEvent('bistro:diff:line-comment'),
  'lineComment.down': (context) =>
    updateLineComment(context, (lc) => ({ ...lc, cursor: moveCursor(lc.lines, lc.cursor, lc.layout, 1) })),
  'lineComment.up': (context) =>
    updateLineComment(context, (lc) => ({ ...lc, cursor: moveCursor(lc.lines, lc.cursor, lc.layout, -1) })),
  'lineComment.extendDown': (context) =>
    updateLineComment(context, (lc) => ({ ...lc, cursor: extendCursor(lc.lines, lc.cursor, 1) })),
  'lineComment.extendUp': (context) =>
    updateLineComment(context, (lc) => ({ ...lc, cursor: extendCursor(lc.lines, lc.cursor, -1) })),
  'lineComment.left': (context) =>
    updateLineComment(context, (lc) => lc.layout === 'split' ? { ...lc, cursor: switchSide(lc.lines, lc.cursor, 'LEFT') } : lc),
  'lineComment.right': (context) =>
    updateLineComment(context, (lc) => lc.layout === 'split' ? { ...lc, cursor: switchSide(lc.lines, lc.cursor, 'RIGHT') } : lc),
  'lineComment.compose': composeLineComment,
```

- [ ] **Step 6: `useLineCommentMode` hook**

`components/diff/useLineCommentMode.ts`:
```ts
import type { SelectedLineRange } from '@pierre/diffs';
import { useEffect, useMemo } from 'react';
import { addToast } from '../ToastContainer';
import { useStore } from '../../hooks/useStore';
import { useOverlays } from '../../overlays/OverlayContext';
import { appStore } from '../../store';
import {
  type DiffLayout,
  cursorRange,
  initialCursor,
  type LineCommentState,
  listCommentableLines,
  toAnnotationSide,
} from '../../utils/lineCommentCursor';
import type { ParsedCodeViewFile } from './diffCodeViewItems';

const OPEN_LINE_COMMENT_EVENT = 'bistro:diff:line-comment';

interface LineCommentModeOptions {
  prId: string | undefined;
  headSha: string | undefined;
  parsedFiles: readonly ParsedCodeViewFile[];
  activeFileId: string | null;
  layout: DiffLayout;
}

/** Opens comment mode on `c` and mirrors its cursor into CodeView's selection. */
export function useLineCommentMode({
  prId,
  headSha,
  parsedFiles,
  activeFileId,
  layout,
}: LineCommentModeOptions): { id: string; range: SelectedLineRange } | null {
  const { activeOverlay, openLineComment } = useOverlays();
  const lineComment = useStore(appStore, (state) => state.lineComment);

  useEffect(() => {
    const open = (): void => {
      if (prId === undefined || headSha === undefined) return;
      const parsed =
        parsedFiles.find(({ file }) => file.id === activeFileId) ?? parsedFiles[0];
      if (parsed === undefined) return;
      const lines = listCommentableLines(parsed.fileDiff.hunks, layout);
      const cursor = initialCursor(lines);
      if (cursor === null) {
        addToast({ type: 'warning', message: 'No commentable lines in this file', dismissible: true });
        return;
      }
      const state: LineCommentState = {
        prId, fileId: parsed.file.id, path: parsed.file.path, headSha,
        layout, lines, cursor, composer: null,
      };
      appStore.setState((s) => ({ ...s, lineComment: state }));
      openLineComment();
    };
    document.addEventListener(OPEN_LINE_COMMENT_EVENT, open);
    return () => document.removeEventListener(OPEN_LINE_COMMENT_EVENT, open);
  }, [prId, headSha, parsedFiles, activeFileId, layout, openLineComment]);

  // Every way out of the overlay (Esc, composer cancel or save) clears the state.
  useEffect(() => {
    if (activeOverlay === 'lineComment') return;
    appStore.setState((s) => (s.lineComment === null ? s : { ...s, lineComment: null }));
  }, [activeOverlay]);

  useEffect(
    () => () => appStore.setState((s) => ({ ...s, lineComment: null })),
    [],
  );

  return useMemo(() => {
    if (lineComment === null) return null;
    const range = cursorRange(lineComment.cursor);
    const side = toAnnotationSide(range.side);
    return {
      id: lineComment.fileId,
      range: { start: range.startLine ?? range.line, end: range.line, side, endSide: side },
    };
  }, [lineComment]);
}
```
Call it in `DiffView` with `activeFileId: activeId` (from `useDiffFileNavigation`) and `layout: diffStyle`. Pass the result to `<CodeView selectedLines>`. If `useOverlays` would throw outside a provider in any existing render path, use `useOptionalOverlays` and bail when it returns null.

Also scroll the cursor into view: when `lineComment` changes, call `codeViewRef.current?.scrollTo({ type: 'line', id: lineComment.fileId, lineNumber: cursorRange(lineComment.cursor).line, side, align: 'nearest', behavior: 'instant' })` in an effect inside `DiffView`. Guard with `lineExistsOnSide` the same way `useInlineDiffAnnotations` does.

- [ ] **Step 7: Composer component**

`components/diff/diffDraftAnnotation.tsx` (the draft card is added in Task 9):
```tsx
import { useEffect, useRef, useState } from 'react';
import { useOverlays } from '../../overlays/OverlayContext';
import { saveDraft } from '../../services/reviewDrafts';
import { isTextareaSubmit, TEXTAREA_SUBMIT_HOTKEY } from '../../utils/keybindings';
import { cursorRange, type LineCommentState } from '../../utils/lineCommentCursor';
import { addToast } from '../ToastContainer';
import { Button } from '../ui/button';
import { ShortcutBadge } from '../ui/shortcut-badge';
import { Textarea } from '../ui/textarea';

function rangeLabel(path: string, startLine: number | null, line: number): string {
  return startLine === null ? `${path}:${line}` : `${path}:${startLine}-${line}`;
}

/** Inline draft composer; Mod+Enter saves, Escape discards. */
export function LineCommentComposer({
  lineComment,
}: {
  lineComment: LineCommentState;
}): React.ReactElement {
  const { closeOverlay } = useOverlays();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [body, setBody] = useState(lineComment.composer?.initialBody ?? '');
  const [isSaving, setIsSaving] = useState(false);
  const range = cursorRange(lineComment.cursor);
  const trimmed = body.trim();

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const save = (): void => {
    if (trimmed.length === 0 || isSaving) return;
    setIsSaving(true);
    void saveDraft({
      prId: lineComment.prId,
      id: lineComment.composer?.draftId ?? null,
      path: lineComment.path,
      side: range.side,
      startLine: range.startLine,
      line: range.line,
      body: trimmed,
      commitSha: lineComment.headSha,
    }).match(
      () => closeOverlay(),
      (fault) => {
        setIsSaving(false);
        addToast({ type: 'error', message: `Draft not saved: ${fault.message}`, dismissible: true });
      },
    );
  };

  return (
    <div
      data-annotation-key="line-comment-composer"
      className="mt-2 space-y-2 border border-primary/60 bg-surface p-3"
    >
      <label
        htmlFor="line-comment-textarea"
        className="font-mono text-[9px] font-bold uppercase tracking-widest text-outline"
      >
        Draft comment · {rangeLabel(lineComment.path, range.startLine, range.line)} ({range.side === 'LEFT' ? 'old' : 'new'})
      </label>
      <Textarea
        ref={textareaRef}
        id="line-comment-textarea"
        value={body}
        onChange={(event) => setBody(event.target.value)}
        rows={3}
        placeholder="Write a comment…"
        className="bg-surface-container text-xs"
        aria-label="Draft line comment"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (isTextareaSubmit(event)) {
            event.preventDefault();
            save();
          }
          if (event.key === 'Escape') {
            event.preventDefault();
            closeOverlay();
          }
        }}
      />
      <div className="flex justify-end gap-2">
        <Button variant="outline" size="sm" onClick={closeOverlay}>
          Discard
        </Button>
        <Button variant="default" size="sm" onClick={save} disabled={trimmed.length === 0 || isSaving}>
          Save draft
          <ShortcutBadge hotkey={TEXTAREA_SUBMIT_HOTKEY} />
        </Button>
      </div>
    </div>
  );
}
```
Use the literal key from `COMPOSER_ANNOTATION_KEY` (imported from `./diffAnnotations`) instead of repeating the string.

- [ ] **Step 8: Wire DiffPage**

In `pages/DiffPage.tsx`:
- Call `useReviewDrafts(pr?.id);`.
- Add `const drafts = useStore(appStore, (s: UIState) => s.reviewDrafts);`.
- Pass `prId={pr?.id}`, `headSha={pr?.head_sha}`, and `drafts={drafts}` to `<DiffView>`.

- [ ] **Step 9: Verify, then check manually**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS. `keybindings.test.ts` "resolves an action for every configured binding" covers the new ids.

Manual check (`cd electron && pnpm start`, open a PR diff):
1. `c` highlights the first changed line of the top file.
2. `j`/`k` move it, and `Shift+J`/`Shift+K` grow a range without switching files.
3. In split view (`Shift+S`), `h`/`l` switch sides.
4. `Enter` opens the composer, `Mod+Enter` saves, and the overlay closes.
5. `Esc` at any point exits without saving.

Then, with the engine log open, confirm a `review_drafts` row exists: `sqlite3 <db> 'select path, side, start_line, line from review_drafts'`. The DB path is in the engine startup log.

- [ ] **Step 10: Commit**

```bash
git add electron/src/renderer
git commit -m "feat(electron): keyboard line-comment mode that saves review drafts"
```

---

## Task 9: Drafts in the diff — cards, `n`/`p`, edit, delete, counts

**Files:**
- Modify: `electron/src/renderer/components/diff/diffDraftAnnotation.tsx` (add `DraftAnnotationCard`)
- Modify: `electron/src/renderer/components/diff/DiffView.tsx`, `components/diff/useLineCommentMode.ts`
- Modify: `electron/src/renderer/utils/keybindings.ts`, `utils/keybindingActions.ts`
- Modify: `electron/src/renderer/pages/DiffPageChrome.tsx`, `pages/DiffPage.tsx`, `pages/PRDetailPage.tsx`, `pages/PRDetailSections.tsx`

**Interfaces:**
- Consumes: `DiffAnnotationMeta`, `draftIdFromKey`, `draftAnnotationKey` (Task 8); `deleteDrafts`, `isDraftOutdated` (Task 6).
- Produces: action id `diff.deleteDraft` (`d d`, DIFF). App event `bistro:diff:edit-draft`. `DiffPageStatusBar` props gain `draftCount: number; outdatedDraftCount: number`. `PendingDraftsNotice` component in `PRDetailSections.tsx`.

- [ ] **Step 1: Draft card**

Add to `diffDraftAnnotation.tsx`:
```tsx
interface DraftAnnotationCardProps {
  draft: ReviewDraft;
  annotationKey: string;
  isOutdated: boolean;
  isActive: boolean;
  onActivate: () => void;
}

/** A saved, unsubmitted line comment. Enter edits, d d deletes. */
export function DraftAnnotationCard({
  draft,
  annotationKey,
  isOutdated,
  isActive,
  onActivate,
}: DraftAnnotationCardProps): React.ReactElement {
  const startLine = draft.start_line === 0 ? null : draft.start_line;
  return (
    <div
      data-annotation-key={annotationKey}
      data-active={isActive ? 'true' : 'false'}
      className={`border-l-2 bg-surface-container-low px-3 py-2 space-y-2 text-[11px] ${
        isActive ? 'border-primary bg-surface-container' : 'border-outline-variant/30'
      }`}
    >
      <button
        type="button"
        onClick={onActivate}
        aria-label={`Focus draft at ${rangeLabel(draft.path, startLine, draft.line)}`}
        className="flex w-full items-center gap-2 text-left cursor-crosshair"
      >
        <Badge variant="outline">DRAFT</Badge>
        {isOutdated && <Badge variant="outdated">OUTDATED</Badge>}
        <span className="font-mono text-[10px] text-outline truncate">
          {rangeLabel(draft.path, startLine, draft.line)}
        </span>
      </button>
      <Markdown body={draft.body} compact />
      {isActive && (
        <p className="font-mono text-[9px] uppercase tracking-widest text-outline">
          Enter edit · d d delete
        </p>
      )}
    </div>
  );
}
```
(Imports: `ReviewDraft` from `../../types`, `Badge` from `../ui/badge`, `Markdown` from `../Markdown`. Check `Badge`'s variant union in `components/ui/badge.tsx`. `outline` and `outdated` exist for thread lifecycles. If `outdated` is missing, use the variant `ThreadAnnotationCard` uses for outdated threads.)

In `DiffView.tsx` `renderAnnotation`, the `'draft'` case renders `<DraftAnnotationCard draft={meta.draft} annotationKey={meta.key} isOutdated={meta.isOutdated} isActive={meta.key === activeAnnotationKey} onActivate={() => appStore.setState((s) => ({ ...s, activeInlineCommentThreadId: meta.key }))} />`.

- [ ] **Step 2: `n`/`p`, edit, delete**

`n`/`p` already walk `inlineCommentThreadIds`. Task 8 made `buildOrderedAnnotations` include draft keys, so drafts are in the walk. Confirm in the app.

`utils/keybindings.ts` — append to `DETAIL_BINDINGS`:
```ts
  {
    actionId: 'diff.deleteDraft',
    keys: ['d', 'd'],
    description: 'Delete draft',
    activation: DIFF,
  },
```
`utils/keybindingActions.ts`:
```ts
function deleteActiveDraft(context: KeyActionContext): void {
  const draftId = draftIdFromKey(context.store.state.activeInlineCommentThreadId);
  if (draftId === null || context.prId === undefined) return;
  void deleteDrafts(context.prId, [draftId]).match(
    () => addToast({ type: 'success', message: 'Draft deleted', dismissible: false }),
    (fault) => addToast({ type: 'error', message: `Draft not deleted: ${fault.message}`, dismissible: true }),
  );
}
```
Register `'diff.deleteDraft': deleteActiveDraft`. In `startInlineDiffReply`, after the `routeBase` guard, add:
```ts
  if (draftIdFromKey(context.store.state.activeInlineCommentThreadId) !== null) {
    emitAppEvent('bistro:diff:edit-draft');
    return;
  }
```
(Imports: `draftIdFromKey` from `../components/diff/diffAnnotations`, `deleteDrafts` from `../services/reviewDrafts`, `addToast` from `../components/ToastContainer`.)

`useLineCommentMode.ts` — add an `drafts: readonly ReviewDraft[]` option and an effect that listens for `bistro:diff:edit-draft`:
```ts
  useEffect(() => {
    const edit = (): void => {
      const draftId = draftIdFromKey(appStore.state.activeInlineCommentThreadId);
      const draft = drafts.find((entry) => entry.id === draftId);
      const parsed = parsedFiles.find(({ file }) => file.path === draft?.path);
      if (draft === undefined || parsed === undefined || prId === undefined) return;
      const state: LineCommentState = {
        prId,
        fileId: parsed.file.id,
        path: draft.path,
        headSha: draft.commit_sha,
        layout,
        lines: listCommentableLines(parsed.fileDiff.hunks, layout),
        cursor: { side: draft.side, anchor: draft.start_line === 0 ? draft.line : draft.start_line, head: draft.line },
        composer: { draftId: draft.id, initialBody: draft.body },
      };
      appStore.setState((s) => ({ ...s, lineComment: state }));
      openLineComment();
    };
    document.addEventListener(EDIT_DRAFT_EVENT, edit);
    return () => document.removeEventListener(EDIT_DRAFT_EVENT, edit);
  }, [prId, drafts, parsedFiles, layout, openLineComment]);
```
`headSha: draft.commit_sha` is ignored on edit (the engine updates only the body). It keeps the type honest. Pass `drafts` from `DiffView`.

- [ ] **Step 3: Counts**

`pages/DiffPageChrome.tsx` `DiffPageStatusBar`:
- Add `draftCount` and `outdatedDraftCount` props.
- In the left group, after the viewed indicator, add:
```tsx
        {draftCount > 0 && (
          <span className="text-primary">
            {draftCount} {draftCount === 1 ? 'draft' : 'drafts'}
            {outdatedDraftCount > 0 ? ` · ${outdatedDraftCount} outdated` : ''}
          </span>
        )}
```
`pages/DiffPage.tsx`: compute `outdatedDraftCount = drafts.filter((d) => isDraftOutdated(d, pr?.head_sha ?? '')).length` and pass both counts.

`pages/PRDetailSections.tsx` — add and export:
```tsx
/** Explains that pending drafts ride along with the next review action. */
export function PendingDraftsNotice({
  drafts,
  headSha,
}: {
  drafts: readonly ReviewDraft[];
  headSha: string;
}): React.ReactElement | null {
  if (drafts.length === 0) return null;
  const outdated = drafts.filter((draft) => isDraftOutdated(draft, headSha)).length;
  return (
    <p className="font-mono text-[10px] uppercase tracking-widest text-primary">
      {drafts.length} pending {drafts.length === 1 ? 'draft' : 'drafts'}
      {outdated > 0 ? ` (${outdated} outdated)` : ''} — sent with your next approve (a a)
      or request changes (⇧R), or submit from the command palette
    </p>
  );
}
```
Render `<PendingDraftsNotice drafts={reviewDrafts} headSha={pr.head_sha} />` in `PRDetailPage` right after `<ActionFeedback … />`, reading `reviewDrafts` from `appStore`.

- [ ] **Step 4: Verify, then check manually**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS.

Manual check:
1. Saved drafts show as DRAFT cards. `n`/`p` reach them.
2. `Enter` on a draft opens it filled in; saving updates it.
3. `d d` deletes it, and the status bar count drops.
4. The detail page shows the notice.
5. `a a` on the detail page with drafts posts one review on GitHub with the inline comments. The drafts disappear after the `DRAFTS_CHANGED` push.

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer
git commit -m "feat(electron): show, navigate, edit and delete review drafts"
```

---

## Task 10: Resolve toggle (`x`) and review commands in the palette

**Files:**
- Create: `electron/src/renderer/hooks/useKeyActionContext.ts`
- Modify: `electron/src/renderer/hooks/useAppHotkeys.ts`
- Modify: `electron/src/renderer/utils/keybindings.ts`, `utils/keybindingActions.ts`
- Modify: `electron/src/renderer/pages/useDiffPageActions.ts`, `pages/DiffPage.tsx`
- Modify: `electron/src/renderer/components/CommandPalette.tsx`

**Interfaces:**
- Consumes: `submitReview` (Task 6), `deleteDrafts` (Task 6), `draftIdFromKey` (Task 8), `useThreadActions` (existing).
- Produces:
  ```ts
  export function useKeyActionContext(): () => KeyActionContext   // stable factory
  // action ids: 'review.toggleResolve' (x, detail+diff), 'review.submit', 'review.discardDrafts' (palette-only, no default key)
  // app event: 'bistro:diff:toggle-resolve'
  ```

- [ ] **Step 1: Extract the action-context factory**

`hooks/useKeyActionContext.ts`:
```ts
import { useRouterState } from '@tanstack/react-router';
import { useCallback, useRef } from 'react';
import { useOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import type { KeyActionContext } from '../utils/keybindingActions';
import { routeBaseFromPath } from '../utils/keybindings';

function routePrId(
  matches: readonly { params?: Record<string, unknown> }[] | undefined,
): string | undefined {
  if (matches === undefined) return undefined;
  for (let i = matches.length - 1; i >= 0; i -= 1) {
    const prId = matches[i]?.params?.prId;
    if (typeof prId === 'string') return prId;
  }
  return undefined;
}

/**
 * Stable factory for the context keybinding actions receive. Reads the latest
 * overlay and router state through a ref, so callers never hold stale closures.
 * Shared by useAppHotkeys and the command palette (same handler, same context).
 */
export function useKeyActionContext(): () => KeyActionContext {
  const overlays = useOverlays();
  const routerState = useRouterState();
  const latestRef = useRef({ overlays, routerState });
  latestRef.current = { overlays, routerState };

  return useCallback((): KeyActionContext => {
    const { overlays: current, routerState: state } = latestRef.current;
    const context: KeyActionContext = {
      store: appStore,
      routeBase: routeBaseFromPath(state.location.pathname),
      activeOverlay: current.activeOverlay,
      overlays: current,
      location: state.location,
    };
    const prId = routePrId(state.matches);
    return prId === undefined ? context : { ...context, prId };
  }, []);
}
```
In `useAppHotkeys.ts`:
- Delete `routePrId`, the five refs, and `makeCtx`.
- Use `const makeCtx = useKeyActionContext();`.
- Keep `routeBase` / `activeOverlay` for `enabled`.

- [ ] **Step 2: `x` binding and actions**

`utils/keybindings.ts` — append to `DETAIL_BINDINGS`:
```ts
  {
    actionId: 'review.toggleResolve',
    keys: 'x',
    description: 'Resolve / unresolve thread',
    activation: SCROLL,
  },
```
(`SCROLL` is `{ routes: ['detail', 'diff'] }`.)

`utils/keybindingActions.ts`:
```ts
function toggleActiveThreadResolve(context: KeyActionContext): void {
  const active = context.store.state.activeInlineCommentThreadId;
  if (context.routeBase !== 'diff' || active === null || draftIdFromKey(active) !== null) {
    addToast({
      type: 'warning',
      message: 'Select a review thread in the diff (n / p) to resolve it',
      dismissible: true,
    });
    return;
  }
  emitAppEvent('bistro:diff:toggle-resolve');
}

function submitDraftsReview(context: KeyActionContext): void {
  if (context.prId === undefined) return;
  if (context.store.state.reviewDrafts.length === 0) {
    addToast({ type: 'warning', message: 'No drafts to submit', dismissible: true });
    return;
  }
  submitReview(context.prId, 'COMMENT');
}

function discardAllDrafts(context: KeyActionContext): void {
  const ids = context.store.state.reviewDrafts.map((draft) => draft.id);
  if (context.prId === undefined || ids.length === 0) return;
  void deleteDrafts(context.prId, ids).match(
    () => addToast({ type: 'success', message: `Discarded ${ids.length} drafts`, dismissible: false }),
    (fault) => addToast({ type: 'error', message: `Drafts not discarded: ${fault.message}`, dismissible: true }),
  );
}

/** The detail page owns the PR comment composers; the diff page jumps there. */
function openDetailComposer(
  context: KeyActionContext,
  confirmingAction: 'comment' | 'request_changes',
): void {
  context.store.setState((state) => ({ ...state, confirmingAction }));
  if (context.routeBase === 'diff' && context.prId !== undefined)
    void router.navigate({ to: '/prs/$prId', params: { prId: context.prId } });
}
```
Register `'review.toggleResolve'`, `'review.submit': submitDraftsReview`, and `'review.discardDrafts': discardAllDrafts`. Change `'detail.comment'` / `'detail.requestChanges'` to `(context) => openDetailComposer(context, 'comment' | 'request_changes')`. Import `submitReview`.

`pages/useDiffPageActions.ts` — add the consumer (import `useThreadActions`):
```ts
  const { resolveThread, unresolveThread } = useThreadActions(pr?.id);
  useEffect(() => {
    const toggle = (): void => {
      const thread = threads.find(
        (entry) => entry.id === appStore.state.activeInlineCommentThreadId,
      );
      if (thread === undefined) return;
      if (!thread.state_known) {
        addToast({ type: 'warning', message: 'Thread state unknown; sync before resolving', dismissible: true });
        return;
      }
      (thread.resolved ? unresolveThread : resolveThread)(thread.id);
    };
    document.addEventListener('bistro:diff:toggle-resolve', toggle);
    return () => document.removeEventListener('bistro:diff:toggle-resolve', toggle);
  }, [threads, resolveThread, unresolveThread]);
```
`pages/DiffPage.tsx`: add `useDismissals(pr?.id, comments);` so palette dismiss commands work on the diff page as well.

- [ ] **Step 3: Palette entries**

In `CommandPalette.tsx`:
- Extend `CommandItem` with `readonly actionId?: string;` and `/** Only listed on PR detail and diff routes. */ readonly prOnly?: boolean;`.
- Append to `COMMANDS` (before `repo-picker`):
```ts
  { id: 'review-approve', name: 'Approve', description: 'Includes pending drafts', icon: 'check_circle', actionId: 'detail.approve', prOnly: true },
  { id: 'review-request-changes', name: 'Request changes', description: 'Includes pending drafts', icon: 'rate_review', actionId: 'detail.requestChanges', prOnly: true },
  { id: 'review-comment', name: 'Comment on PR', icon: 'chat', actionId: 'detail.comment', prOnly: true },
  { id: 'review-submit', name: 'Submit review', description: 'Send pending drafts as one comment review', icon: 'send', actionId: 'review.submit', prOnly: true },
  { id: 'review-discard-drafts', name: 'Discard drafts', icon: 'delete_sweep', actionId: 'review.discardDrafts', prOnly: true },
  { id: 'review-line-comment', name: 'Start line comment', icon: 'add_comment', actionId: 'diff.lineComment', diffOnly: true },
  { id: 'review-dismiss-nits', name: 'Dismiss nits and info', icon: 'filter_list_off', actionId: 'detail.dismissNits', prOnly: true },
  { id: 'review-dismiss-bots', name: 'Dismiss bot comments', icon: 'smart_toy', actionId: 'detail.dismissBots', prOnly: true },
  { id: 'review-undo-dismiss', name: 'Undo dismissal', icon: 'undo', actionId: 'detail.undoDismiss', prOnly: true },
  { id: 'review-toggle-resolve', name: 'Resolve / unresolve thread', icon: 'task_alt', actionId: 'review.toggleResolve', prOnly: true },
```
- In the component:
  - Call `const makeCtx = useKeyActionContext();`.
  - Read `const overrides = useStore(settingsStore, (s) => s.keybindings);` and `const draftCount = useStore(appStore, (s) => s.reviewDrafts.length);`.
  - Compute `const bindings = useMemo(() => getEffectiveBindings(overrides), [overrides]);` and `const isPRRoute = routedPR !== false;`.
- In `filteredCommands`, skip `prOnly` commands when `!isPRRoute`. Map the list so that:
  - `review-submit`'s `name` becomes ``Submit review (${draftCount} ${draftCount === 1 ? 'draft' : 'drafts'})``.
  - Any command with `actionId` gets `shortcut: shortcutFor(command.actionId, bindings)`.
  - Add `draftCount` and `bindings` to the memo deps.
- Add the helper above the component:
```ts
/** Display chips for an action's effective key, so palette and keymap agree. */
function shortcutFor(
  actionId: string,
  bindings: readonly KeyBinding[],
): readonly string[] | undefined {
  const binding = bindings.find((entry) => entry.actionId === actionId);
  if (binding === undefined) return undefined;
  const steps = isSequenceBinding(binding) ? binding.keys : [binding.keys];
  return steps.map((step) => formatForDisplay(step));
}
```
  (`formatForDisplay` from `@tanstack/react-hotkeys`, which `KeyboardSection.tsx` already uses; `getEffectiveBindings`, `isSequenceBinding`, `type KeyBinding` from `../utils/keybindings`.)
- At the top of `handleExecuteCommand`:
```ts
    if (command.actionId !== undefined) {
      handleClose();
      getBindingAction(command.actionId)?.({ ...makeCtx(), activeOverlay: null });
      return;
    }
```
`useKeyActionContext` calls `useOverlays`, which the palette already requires, so no provider change is needed.

- [ ] **Step 4: Verify, then check manually**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS.

Manual check:
1. In a diff, `n` to a thread then `x` resolves it (toast). `x` again reopens it.
2. `x` on the detail page shows the "select a thread" toast.
3. `:` on a PR lists the review commands with key chips (`A A`, `⇧ R`, `C`, `X`, …). "Submit review (N drafts)" submits.
4. "Request changes" from the diff page opens the detail composer.
5. The list page palette shows no review commands.

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer
git commit -m "feat(electron): resolve toggle and review commands in the palette"
```

---

## Task 11: Phase 1 docs

**Files:**
- Modify: `CLAUDE.md` (Conventions → keybinding bullet)
- Modify: `docs/keyboard-shortcuts.md`, `website/src/pages/docs/keybindings.mdx`
- Modify: `docs/ROADMAP.md` (mark review-features #3 phase 1 done; note phase 2 status)

- [ ] **Step 1: Update CLAUDE.md**

Append to the keybinding convention bullet: "Command-palette entries dispatch the same handler by `actionId` (`getBindingAction`, with the context from `hooks/useKeyActionContext.ts`). Palette-only actions (no default key) live in `ACTIONS` without a binding." Add a Conventions bullet: "Review drafts: the engine owns `review_drafts`. `approve` / `request_changes` absorb the current user's drafts inside `Orchestrator.ApplyAction`, and `submit_review` is never queued."

- [ ] **Step 2: Update the shortcut docs**

In `docs/keyboard-shortcuts.md` and `website/src/pages/docs/keybindings.mdx`, follow each file's existing table format:
- Diff: `c` comment on line, `d d` delete draft, `x` resolve / unresolve the active thread. `Enter` on a draft edits it.
- New "Comment mode" table: `j`/`k`, `Shift+J`/`Shift+K` extend, `h`/`l` side (split), `Enter` write, `Mod+Enter` save, `Esc` exit.
- Detail: `x`. Fix the `a a` row: "Approve (includes pending drafts)". Fix the `Shift+R` row: "Request changes (includes pending drafts)".

- [ ] **Step 3: Update the roadmap and commit**

Edit the review-features #3 entry in `docs/ROADMAP.md`: phase 1 shipped 2026-10-23, remap UI in progress.
```bash
git add CLAUDE.md docs website/src/pages/docs/keybindings.mdx
git commit -m "docs: document keyboard line comments, drafts and resolve toggle"
```

---

## Task 12 (Phase 2): Keybinding conflict detection

**Files:**
- Modify: `electron/src/renderer/utils/keybindings.ts` (export `ALL_ROUTES`)
- Create: `electron/src/renderer/utils/keybindingConflicts.ts`
- Test: `electron/src/renderer/utils/keybindingConflicts.test.ts`

**Interfaces:**
- Produces:
  ```ts
  export function findKeybindingConflicts(bindings: readonly KeyBinding[], candidate: KeyBinding): KeyBinding[]
  ```
  Two bindings conflict when their key steps are equal or one is a prefix of the other, **and** some scope (`route:<r>` when no overlay is open, or `overlay:<o>`) activates both. Scopes are derived exactly like `isActive`. Routes undefined + overlays undefined → every route. Routes undefined + overlays set → overlays only. `routes: []` → none.

- [ ] **Step 1: Write the failing test**

`utils/keybindingConflicts.test.ts`:
```ts
import { describe, expect, it } from 'vitest';
import { findKeybindingConflicts } from './keybindingConflicts';
import { DEFAULT_KEYBINDINGS, type KeyBinding } from './keybindings';

function remap(actionId: string, keys: string | readonly string[]): KeyBinding {
  const binding = DEFAULT_KEYBINDINGS.find((entry) => entry.actionId === actionId);
  if (binding === undefined) throw new Error(`missing ${actionId}`);
  return { ...binding, keys };
}

const conflictIds = (candidate: KeyBinding): string[] =>
  findKeybindingConflicts(DEFAULT_KEYBINDINGS, candidate).map((b) => b.actionId);

describe('findKeybindingConflicts', () => {
  it('flags a key already bound on the same route', () => {
    expect(conflictIds(remap('diff.markViewed', 'n'))).toEqual(['diff.nextComment']);
  });

  it('ignores the same key on a route where the bindings never meet', () => {
    expect(conflictIds(remap('list.mute', 'x'))).toEqual([]);
  });

  it('keeps overlay scopes apart from route scopes', () => {
    expect(conflictIds(remap('threads.next', 'n'))).toEqual([]);
  });

  it('treats a single key that prefixes a sequence as a conflict', () => {
    expect(conflictIds(remap('diff.markViewed', ['d']))).toEqual(['diff.deleteDraft']);
  });

  it('allows sequences that only share a first step', () => {
    expect(conflictIds(remap('list.top', ['g', '7']))).toEqual([]);
  });

  it('does not report the binding being remapped', () => {
    expect(conflictIds(remap('diff.nextComment', 'n'))).toEqual([]);
  });

  it('sees multi-route bindings on each of their routes', () => {
    expect(conflictIds(remap('diff.toggleStyle', 'Shift+/'))).toEqual(['global.help']);
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd electron && pnpm vitest run src/renderer/utils/keybindingConflicts.test.ts`
Expected: FAIL — cannot resolve `./keybindingConflicts`.

- [ ] **Step 3: Implement**

In `keybindings.ts`, change `const ALL_ROUTES` to `export const ALL_ROUTES`.

`utils/keybindingConflicts.ts`:
```ts
import { ALL_ROUTES, isSequenceBinding, type KeyBinding } from './keybindings';

/** Scopes where a binding can fire; mirrors isActive in keybindingActivation.ts. */
function activeScopes(binding: KeyBinding): Set<string> {
  const { routes, overlays } = binding.activation;
  const routeScopes = routes ?? (overlays === undefined ? ALL_ROUTES : []);
  return new Set([
    ...routeScopes.map((route) => `route:${route}`),
    ...(overlays ?? []).map((overlay) => `overlay:${overlay}`),
  ]);
}

function keySteps(binding: KeyBinding): readonly string[] {
  return isSequenceBinding(binding) ? binding.keys : [binding.keys];
}

function isPrefixOrEqual(
  shorter: readonly string[],
  longer: readonly string[],
): boolean {
  return shorter.every((step, index) => longer[index] === step);
}

/**
 * Bindings that would fire alongside `candidate`: the same keys, or one
 * sequence prefixing the other, in a scope where both can be active.
 */
export function findKeybindingConflicts(
  bindings: readonly KeyBinding[],
  candidate: KeyBinding,
): KeyBinding[] {
  const scopes = activeScopes(candidate);
  const steps = keySteps(candidate);
  return bindings.filter((other) => {
    if (other.actionId === candidate.actionId) return false;
    const otherSteps = keySteps(other);
    const keysCollide =
      otherSteps.length <= steps.length
        ? isPrefixOrEqual(otherSteps, steps)
        : isPrefixOrEqual(steps, otherSteps);
    return keysCollide && [...activeScopes(other)].some((scope) => scopes.has(scope));
  });
}
```

- [ ] **Step 4: Run tests**

Run: `cd electron && pnpm vitest run src/renderer/utils/keybindingConflicts.test.ts && pnpm lint`
Expected: PASS. If a case fails because a default binding moved, fix the test's expectation only after confirming the real keymap in `keybindings.ts`.

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/utils
git commit -m "feat(electron): detect keybinding conflicts by activation scope"
```

---

## Task 13 (Phase 2): Remapping UI

**Files:**
- Modify: `electron/src/renderer/pages/settings/KeyboardSection.tsx`
- Modify: `electron/src/renderer/store/ui.ts` (`isRecordingKeybinding`)
- Modify: `electron/src/renderer/utils/keybindingActions.ts` (`handleEscape` guard)
- Modify: `CLAUDE.md`, `docs/keyboard-shortcuts.md`, `website/src/pages/docs/keybindings.mdx`, `docs/ROADMAP.md`

**Interfaces:**
- Consumes: `findKeybindingConflicts` (Task 12); `useHotkeySequenceRecorder` from `@tanstack/react-hotkeys` 0.13. Its options are `{ onRecord(sequence), onCancel?, recordBy?: 'code' | 'key', ignoreInputs? }`; Enter commits, Escape cancels, Backspace removes a step. It returns `{ isRecording, steps, startRecording, cancelRecording, ... }`.
- Produces: `UIState.isRecordingKeybinding: boolean`. The override shape is unchanged (`settings.keybindings: Record<actionId, string[]>`).

- [ ] **Step 1: Escape guard**

`store/ui.ts`:
```ts
  /** True while the keyboard settings page records a new binding; Esc then cancels recording instead of leaving settings. */
  isRecordingKeybinding: boolean;
```
Initial value: `false`. In `handleEscape` (`keybindingActions.ts`), as the first line, add `if (context.store.state.isRecordingKeybinding) return;`.

- [ ] **Step 2: Recording row**

In `KeyboardSection.tsx`, replace the static row with a `KeybindingRow` component in the same file:
```tsx
interface KeybindingRowProps {
  binding: KeyBinding;
  isOverridden: boolean;
  onRecord: (keys: readonly string[]) => void;
  onReset: () => void;
}

function KeybindingRow({ binding, isOverridden, onRecord, onReset }: KeybindingRowProps) {
  const recorder = useHotkeySequenceRecorder({
    recordBy: 'key',
    onRecord: (sequence) => onRecord([...sequence]),
  });

  useEffect(() => {
    appStore.setState((s) => ({ ...s, isRecordingKeybinding: recorder.isRecording }));
  }, [recorder.isRecording]);

  const keys = recorder.isRecording
    ? recorder.steps
    : isSequenceBinding(binding)
      ? binding.keys
      : [binding.keys];

  return (
    <button
      type="button"
      onClick={() => {
        if (!recorder.isRecording) recorder.startRecording();
      }}
      onKeyDown={(event) => {
        if (!recorder.isRecording && event.key === 'Backspace' && isOverridden) {
          event.preventDefault();
          onReset();
        }
      }}
      aria-label={`${binding.description}: press Enter to rebind${isOverridden ? ', Backspace to reset' : ''}`}
      className="flex w-full items-center justify-between py-2 gap-4 border-b border-outline-variant/10 last:border-b-0 text-left focus-visible:outline focus-visible:outline-1 focus-visible:outline-primary"
    >
      <span className="text-sm text-on-surface truncate">
        {binding.description}
        {isOverridden && <span className="ml-2 font-mono text-[9px] uppercase text-primary">custom</span>}
      </span>
      <span className="flex items-center gap-1 shrink-0">
        {recorder.isRecording && keys.length === 0 ? (
          <span className="font-mono text-[10px] uppercase tracking-widest text-primary">
            Press keys · Enter save · Esc cancel
          </span>
        ) : (
          keys.map((key, index) => (
            <Kbd
              // biome-ignore lint/suspicious/noArrayIndexKey: Sequence chords can repeat, and these labels have no state.
              key={index}
              className="px-2 text-[10px] font-bold text-on-surface"
            >
              {formatForDisplay(key)}
            </Kbd>
          ))
        )}
      </span>
    </button>
  );
}
```
The unmount cleanup must also reset `isRecordingKeybinding` to `false`. Add a cleanup effect: `useEffect(() => () => appStore.setState((s) => ({ ...s, isRecordingKeybinding: false })), [])`.

Verify `recordBy: 'key'` produces the same notation as `DEFAULT_KEYBINDINGS`: recording `j` gives `"j"` and `Shift+J` gives `"Shift+J"`. Log `sequence` once in dev and remove the log afterwards. If `'key'` yields e.g. `"Shift+?"` for `Shift+/` while the defaults use `"Shift+/"`, switch to the default `'code'` mode only if `'code'` matches the defaults. Either way, note the chosen mode in a one-line comment.

- [ ] **Step 3: Save, reset, conflicts**

In `KeyboardSection`:
```tsx
  const effective = useMemo(() => getEffectiveBindings(overrides), [overrides]);

  const saveBinding = (binding: KeyBinding, keys: readonly string[]): void => {
    const [first] = keys;
    if (first === undefined) return;
    const candidate: KeyBinding = { ...binding, keys: keys.length === 1 ? first : keys };
    const [conflict] = findKeybindingConflicts(effective, candidate);
    if (conflict !== undefined) {
      addToast({
        type: 'error',
        message: `${keys.map((key) => formatForDisplay(key)).join(' ')} already runs "${conflict.description}"`,
        dismissible: true,
      });
      return;
    }
    updateSettings({ keybindings: { ...overrides, [binding.actionId]: [...keys] } }).match(
      () => undefined,
      showSettingsError,
    );
  };

  const resetBinding = (actionId: string): void => {
    const { [actionId]: _removed, ...rest } = overrides;
    updateSettings({ keybindings: rest }).match(() => undefined, showSettingsError);
  };
```
The main process replaces `keybindings` wholesale (`main/settings.ts` `mergeSettings`), so dropping the key restores the default. Group rows from `effective` as today. Render each row with `isOverridden={overrides[b.actionId] !== undefined}`. `showSettingsError` takes a `Fault`; if its signature differs, adapt the `.match` error arm. Unbound actions (override `[]`) do not appear in `effective`. That matches today's behavior; resetting them stays a `settings.json` edit.

Replace the intro paragraph with: "Press Enter on an action to record a new key or chord (Enter saves, Esc cancels). Backspace restores the default. Conflicts with keys active in the same place are rejected." Keep the `settings.json` link as "Advanced: edit settings.json".

- [ ] **Step 4: Verify, then check manually**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS.

Manual check in Settings → Keyboard:
1. Tab to "Mark file viewed", press Enter, press `m`, Enter. The row shows `M` (custom), and `m` marks viewed in a diff.
2. Recording `n` for it is rejected and names "Next inline comment".
3. Esc while recording cancels and stays on Settings.
4. Backspace restores `V`.

- [ ] **Step 5: Docs and commit**

- CLAUDE.md: the override shape is unchanged. Add to the keybinding bullet: "Settings → Keyboard records overrides with `useHotkeySequenceRecorder` and rejects conflicts via `utils/keybindingConflicts.ts`."
- Docs: replace "edit settings.json" rebinding instructions in `docs/keyboard-shortcuts.md` and `website/src/pages/docs/keybindings.mdx` with the UI flow.
- `docs/ROADMAP.md`: phase 2 shipped.
```bash
git add electron/src/renderer CLAUDE.md docs website/src/pages/docs/keybindings.mdx
git commit -m "feat(electron): remap keybindings from the keyboard settings page"
```

---

## Final verification

- [ ] `cd go && go vet ./... && go test -race ./...` → PASS
- [ ] `cd electron && pnpm type-check && pnpm lint && pnpm test` → PASS
- [ ] Run the manual end-to-end review against a real test PR:
  1. Two single-line drafts and one multi-line draft (one of them on the LEFT side in split view).
  2. `Shift+R` with a body.
  3. GitHub shows one "changes requested" review with three inline comments.
  4. The drafts are gone locally.
  5. With the network off, `a a` with a draft fails, the draft stays, and the toast names it.
