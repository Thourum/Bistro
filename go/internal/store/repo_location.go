package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// RepoLocation mirrors the location columns of one repos row.
type RepoLocation struct {
	RepoID                string
	Provider, Owner, Repo string
	LocalPath             string // "" when NULL
	Source                string // "local" | "cache" | ""
	LastUsedAt            *time.Time
	LocationStatus        string
	LocationErrorCode     string
	LocationError         string
}

// RepoLocationStore persists repo local/cache location metadata.
type RepoLocationStore struct{ db *sql.DB }

// NewRepoLocationStore creates a RepoLocationStore backed by the given *sql.DB.
func NewRepoLocationStore(db *sql.DB) *RepoLocationStore {
	return &RepoLocationStore{db: db}
}

// UpsertLocal registers/updates a local repo. Creates the repos row when missing
// (watched=1). Preserves an existing healthy local_path when it still exists on
// disk so duplicate checkout scans do not replace the user's chosen location.
func (s *RepoLocationStore) UpsertLocal(ctx context.Context, provider, owner, repo, canonicalPath string) error {
	normalizedProvider := strings.ToLower(provider)
	normalizedOwner := strings.ToLower(owner)
	normalizedRepo := strings.ToLower(repo)
	repoID := fmt.Sprintf("%s:%s/%s", normalizedProvider, normalizedOwner, normalizedRepo)

	existing, err := s.Get(ctx, repoID)
	if err != nil {
		return fmt.Errorf("get existing repo location %s: %w", repoID, err)
	}

	localPath := canonicalPath
	if shouldPreserveLocalPath(existing, canonicalPath) {
		localPath = existing.LocalPath
	}

	const query = `
		INSERT INTO repos (
			id, provider, owner, repo, watched, local_path, source, last_used_at,
			location_status, location_error_code, location_error
		)
		VALUES (?, ?, ?, ?, 1, ?, 'local', ?, 'ok', NULL, NULL)
		ON CONFLICT(id) DO UPDATE SET
			provider = excluded.provider,
			owner = excluded.owner,
			repo = excluded.repo,
			watched = 1,
			local_path = excluded.local_path,
			source = excluded.source,
			last_used_at = excluded.last_used_at,
			location_status = excluded.location_status,
			location_error_code = NULL,
			location_error = NULL
	`
	if _, err := s.db.ExecContext(ctx, query, repoID, normalizedProvider, normalizedOwner, normalizedRepo, localPath, time.Now().UTC()); err != nil {
		return fmt.Errorf("upsert local repo %s: %w", repoID, err)
	}
	return nil
}

// SetCache marks a repo as cache-cloned at path, last_used_at=now, status ok.
func (s *RepoLocationStore) SetCache(ctx context.Context, repoID, path string) error {
	const query = `
		UPDATE repos
		SET local_path = ?, source = 'cache', last_used_at = ?, location_status = 'ok',
			location_error_code = NULL, location_error = NULL
		WHERE id = ?
	`
	if _, err := s.db.ExecContext(ctx, query, path, time.Now().UTC(), repoID); err != nil {
		return fmt.Errorf("set cache location for %s: %w", repoID, err)
	}
	return nil
}

// Touch sets last_used_at=now. Called only after a successful use, so it also
// clears any stale degraded state left from a previous failure.
func (s *RepoLocationStore) Touch(ctx context.Context, repoID string) error {
	const query = `
		UPDATE repos
		SET last_used_at = ?, location_status = 'ok',
			location_error_code = NULL, location_error = NULL
		WHERE id = ?
	`
	if _, err := s.db.ExecContext(ctx, query, time.Now().UTC(), repoID); err != nil {
		return fmt.Errorf("touch repo location %s: %w", repoID, err)
	}
	return nil
}

// MarkDegraded sets location_status + error code/message. Keeps local_path.
func (s *RepoLocationStore) MarkDegraded(ctx context.Context, repoID, status, code, msg string) error {
	const query = `
		UPDATE repos
		SET location_status = ?, location_error_code = ?, location_error = ?
		WHERE id = ?
	`
	if _, err := s.db.ExecContext(ctx, query, status, code, msg, repoID); err != nil {
		return fmt.Errorf("mark repo location degraded %s: %w", repoID, err)
	}
	return nil
}

// ClearDegraded resets status to 'ok' and clears error fields.
func (s *RepoLocationStore) ClearDegraded(ctx context.Context, repoID string) error {
	const query = `
		UPDATE repos
		SET location_status = 'ok', location_error_code = NULL, location_error = NULL
		WHERE id = ?
	`
	if _, err := s.db.ExecContext(ctx, query, repoID); err != nil {
		return fmt.Errorf("clear repo location degraded state %s: %w", repoID, err)
	}
	return nil
}

// ClearCacheLocation nulls local_path/source/location fields (post-prune).
func (s *RepoLocationStore) ClearCacheLocation(ctx context.Context, repoID string) error {
	const query = `
		UPDATE repos
		SET local_path = NULL, source = NULL, last_used_at = NULL, location_status = NULL,
			location_error_code = NULL, location_error = NULL
		WHERE id = ?
	`
	if _, err := s.db.ExecContext(ctx, query, repoID); err != nil {
		return fmt.Errorf("clear cache location for %s: %w", repoID, err)
	}
	return nil
}

// Get returns the row or (nil, nil) when absent.
func (s *RepoLocationStore) Get(ctx context.Context, repoID string) (*RepoLocation, error) {
	const query = `
		SELECT id, provider, owner, repo, local_path, source, last_used_at,
			location_status, location_error_code, location_error
		FROM repos
		WHERE id = ?
	`
	location, err := scanRepoLocation(s.db.QueryRowContext(ctx, query, repoID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repo location %s: %w", repoID, err)
	}
	return location, nil
}

// ListBySource returns all rows with the given source ("local"/"cache").
func (s *RepoLocationStore) ListBySource(ctx context.Context, source string) ([]RepoLocation, error) {
	const query = `
		SELECT id, provider, owner, repo, local_path, source, last_used_at,
			location_status, location_error_code, location_error
		FROM repos
		WHERE source = ?
		ORDER BY id
	`
	rows, err := s.db.QueryContext(ctx, query, source)
	if err != nil {
		return nil, fmt.Errorf("list repo locations by source %s: %w", source, err)
	}
	defer rows.Close()

	locations := make([]RepoLocation, 0)
	for rows.Next() {
		location, err := scanRepoLocation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan repo location by source %s: %w", source, err)
		}
		locations = append(locations, *location)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repo locations by source %s: %w", source, err)
	}
	return locations, nil
}

type repoLocationScanner interface {
	Scan(dest ...any) error
}

func scanRepoLocation(scanner repoLocationScanner) (*RepoLocation, error) {
	var location RepoLocation
	var localPath, source, status, errorCode, locationError sql.NullString
	var lastUsedAt sql.NullTime
	if err := scanner.Scan(
		&location.RepoID,
		&location.Provider,
		&location.Owner,
		&location.Repo,
		&localPath,
		&source,
		&lastUsedAt,
		&status,
		&errorCode,
		&locationError,
	); err != nil {
		return nil, err
	}
	location.LocalPath = nullStringValue(localPath)
	location.Source = nullStringValue(source)
	location.LastUsedAt = nullTimeValue(lastUsedAt)
	location.LocationStatus = nullStringValue(status)
	location.LocationErrorCode = nullStringValue(errorCode)
	location.LocationError = nullStringValue(locationError)
	return &location, nil
}

func shouldPreserveLocalPath(existing *RepoLocation, canonicalPath string) bool {
	if existing == nil || existing.Source != "local" || existing.LocationStatus != "ok" {
		return false
	}
	if existing.LocalPath == "" || existing.LocalPath == canonicalPath {
		return false
	}
	if _, err := os.Stat(existing.LocalPath); err != nil {
		return false
	}
	return true
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullTimeValue(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}
