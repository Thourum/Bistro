package reviewdraft

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

// ErrNotFound reports a draft id the account does not own.
var ErrNotFound = errors.New("review draft not found")

// Draft is a pending line comment that has not been submitted to GitHub.
type Draft struct {
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

// Store persists pending review comments per account.
type Store struct {
	db      *sql.DB
	queries *storedb.Queries
}

// New creates a draft store backed by the given database.
func New(db *sql.DB) *Store {
	return &Store{db: db, queries: storedb.New(db)}
}

// Create inserts a new draft for login and returns it with its generated id.
func (d *Store) Create(ctx context.Context, login string, draft Draft) (Draft, error) {
	id, err := newDraftID()
	if err != nil {
		return Draft{}, err
	}
	row, err := d.queries.InsertReviewDraft(ctx, storedb.InsertReviewDraftParams{
		ID: id, PrID: draft.PRID, Login: login, Path: draft.Path, Side: draft.Side,
		StartLine: sql.NullInt64{Int64: int64(draft.StartLine), Valid: draft.StartLine != 0},
		Line:      int64(draft.Line), Body: draft.Body, CommitSha: draft.CommitSHA,
	})
	if err != nil {
		return Draft{}, fmt.Errorf("insert review draft (%s, %s): %w", draft.PRID, login, err)
	}
	return draftFromRow(row), nil
}

// UpdateBody replaces the body of login's draft id.
func (d *Store) UpdateBody(ctx context.Context, login, id, body string) (Draft, error) {
	row, err := d.queries.UpdateReviewDraftBody(ctx, storedb.UpdateReviewDraftBodyParams{Body: body, ID: id, Login: login})
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{}, ErrNotFound
	}
	if err != nil {
		return Draft{}, fmt.Errorf("update review draft %s: %w", id, err)
	}
	return draftFromRow(row), nil
}

// List returns login's drafts on prID, oldest first.
func (d *Store) List(ctx context.Context, prID, login string) ([]Draft, error) {
	rows, err := d.queries.ListReviewDrafts(ctx, storedb.ListReviewDraftsParams{PrID: prID, Login: login})
	if err != nil {
		return nil, fmt.Errorf("list review drafts (%s, %s): %w", prID, login, err)
	}
	drafts := make([]Draft, 0, len(rows))
	for _, row := range rows {
		drafts = append(drafts, draftFromRow(row))
	}
	return drafts, nil
}

// Delete removes login's drafts in one transaction and returns the committed
// row count. Missing IDs and IDs owned by another account are ignored.
func (d *Store) Delete(ctx context.Context, login string, ids []string) (int64, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin delete review drafts: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := d.queries.WithTx(tx)
	var deleted int64
	for _, id := range ids {
		count, err := qtx.DeleteReviewDraft(ctx, storedb.DeleteReviewDraftParams{ID: id, Login: login})
		if err != nil {
			return 0, fmt.Errorf("delete review draft %s: %w", id, err)
		}
		deleted += count
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit delete review drafts: %w", err)
	}
	return deleted, nil
}

func draftFromRow(row storedb.ReviewDraft) Draft {
	return Draft{
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
