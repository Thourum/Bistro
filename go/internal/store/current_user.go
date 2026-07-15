package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// CurrentUserStore persists the authenticated user identity per provider.
type CurrentUserStore struct {
	log     *slog.Logger
	queries *storedb.Queries
}

// NewCurrentUserStore creates a CurrentUserStore using slog.Default() as the logger.
func NewCurrentUserStore(db *DB) *CurrentUserStore {
	return NewCurrentUserStoreWithLogger(nil, db)
}

// NewCurrentUserStoreWithLogger creates a CurrentUserStore with an explicit logger.
func NewCurrentUserStoreWithLogger(log *slog.Logger, db *DB) *CurrentUserStore {
	if log == nil {
		log = slog.Default()
	}
	return &CurrentUserStore{log: log, queries: storedb.New(db)}
}

// SetCurrentUser upserts the login for the given provider.
func (s *CurrentUserStore) SetCurrentUser(ctx context.Context, providerName, login string) error {
	if err := s.queries.UpsertCurrentUser(ctx, storedb.UpsertCurrentUserParams{
		Provider: providerName,
		Login:    login,
	}); err != nil {
		return fmt.Errorf("set current user %s/%s: %w", providerName, login, err)
	}
	s.log.Debug("current user stored", "provider", providerName, "login", login)
	return nil
}

// DeleteCurrentUser removes the stored current login for the given provider.
func (s *CurrentUserStore) DeleteCurrentUser(ctx context.Context, providerName string) error {
	if err := s.queries.DeleteCurrentUser(ctx, providerName); err != nil {
		return fmt.Errorf("delete current user %s: %w", providerName, err)
	}
	s.log.Debug("current user cleared", "provider", providerName)
	return nil
}

// GetCurrentUser returns the stored login and true, or ("", false, nil) if not set.
// A non-nil error indicates a DB failure distinct from the not-set case.
func (s *CurrentUserStore) GetCurrentUser(ctx context.Context, providerName string) (string, bool, error) {
	login, err := s.queries.GetCurrentUserLogin(ctx, providerName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		s.log.Error("get current user failed", "provider", providerName, "error", err)
		return "", false, err
	}
	return login, true, nil
}
