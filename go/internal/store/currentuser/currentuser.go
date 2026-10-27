package currentuser

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/itterno/bistro/internal/store"
	"log/slog"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// Store persists the authenticated user identity per provider.
type Store struct {
	log     *slog.Logger
	queries *storedb.Queries
}

// New creates a Store using slog.Default() as the logger.
func New(db *store.DB) *Store {
	return NewWithLogger(nil, db)
}

// NewWithLogger creates a Store with an explicit logger.
func NewWithLogger(log *slog.Logger, db *store.DB) *Store {
	if log == nil {
		log = slog.Default()
	}
	return &Store{log: log, queries: storedb.New(db)}
}

// Set upserts the login for the given provider.
func (s *Store) Set(ctx context.Context, providerName, login string) error {
	if err := s.queries.UpsertCurrentUser(ctx, storedb.UpsertCurrentUserParams{
		Provider: providerName,
		Login:    login,
	}); err != nil {
		return fmt.Errorf("set current user %s/%s: %w", providerName, login, err)
	}
	s.log.Debug("current user stored", "provider", providerName, "login", login)
	return nil
}

// Delete removes the stored current login for the given provider.
func (s *Store) Delete(ctx context.Context, providerName string) error {
	if err := s.queries.DeleteCurrentUser(ctx, providerName); err != nil {
		return fmt.Errorf("delete current user %s: %w", providerName, err)
	}
	s.log.Debug("current user cleared", "provider", providerName)
	return nil
}

// Get returns the stored login and true, or ("", false, nil) if not set.
// A non-nil error indicates a DB failure distinct from the not-set case.
func (s *Store) Get(ctx context.Context, providerName string) (string, bool, error) {
	login, err := s.queries.GetCurrentUserLogin(ctx, providerName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get current user %s: %w", providerName, err)
	}
	return login, true, nil
}
