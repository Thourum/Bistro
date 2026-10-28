package repolocation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	storedb "github.com/itterno/bistro/internal/store/db"
)

// Location mirrors the location columns of one repos row.
type Location struct {
	RepoID                string
	Provider, Owner, Repo string
	LocalPath             string // "" when NULL
	Source                string // "local" | "cache" | ""
	LastUsedAt            *time.Time
	LocationStatus        string
	LocationErrorCode     string
	LocationError         string
}

// Store persists repo local/cache location metadata.
type Store struct{ queries *storedb.Queries }

// New creates a Store backed by the given *sql.DB.
func New(db *sql.DB) *Store {
	return &Store{queries: storedb.New(db)}
}

// UpsertLocal registers/updates a local repo. watch=true marks the repo
// watched (explicit user adds); watch=false only registers the location and
// never changes an existing watched state — dev-folder scans must not
// auto-watch every discovered checkout. Preserves an existing healthy
// local_path when it still exists on disk so duplicate checkout scans do not
// replace the user's chosen location.
func (s *Store) UpsertLocal(ctx context.Context, provider, owner, repo, canonicalPath string, watch bool) error {
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

	watched := int64(0)
	if watch {
		watched = 1
	}
	if err := s.queries.UpsertLocalRepoLocation(ctx, storedb.UpsertLocalRepoLocationParams{
		ID:         repoID,
		Provider:   normalizedProvider,
		Owner:      normalizedOwner,
		Repo:       normalizedRepo,
		Watched:    watched,
		LocalPath:  validNullString(localPath),
		LastUsedAt: currentNullTime(),
	}); err != nil {
		return fmt.Errorf("upsert local repo %s: %w", repoID, err)
	}
	return nil
}

// SetCache marks a repo as cache-cloned at path, last_used_at=now, status ok.
func (s *Store) SetCache(ctx context.Context, repoID, path string) error {
	if err := s.queries.SetCacheRepoLocation(ctx, storedb.SetCacheRepoLocationParams{
		LocalPath:  validNullString(path),
		LastUsedAt: currentNullTime(),
		ID:         repoID,
	}); err != nil {
		return fmt.Errorf("set cache location for %s: %w", repoID, err)
	}
	return nil
}

// Touch sets last_used_at=now. Called only after a successful use, so it also
// clears any stale degraded state left from a previous failure.
func (s *Store) Touch(ctx context.Context, repoID string) error {
	if err := s.queries.TouchRepoLocation(ctx, storedb.TouchRepoLocationParams{
		LastUsedAt: currentNullTime(),
		ID:         repoID,
	}); err != nil {
		return fmt.Errorf("touch repo location %s: %w", repoID, err)
	}
	return nil
}

// MarkDegraded sets location_status + error code/message. Keeps local_path.
func (s *Store) MarkDegraded(ctx context.Context, repoID, status, code, msg string) error {
	if err := s.queries.MarkRepoLocationDegraded(ctx, storedb.MarkRepoLocationDegradedParams{
		LocationStatus:    validNullString(status),
		LocationErrorCode: validNullString(code),
		LocationError:     validNullString(msg),
		ID:                repoID,
	}); err != nil {
		return fmt.Errorf("mark repo location degraded %s: %w", repoID, err)
	}
	return nil
}

// ClearDegraded resets status to 'ok' and clears error fields.
func (s *Store) ClearDegraded(ctx context.Context, repoID string) error {
	if err := s.queries.ClearRepoLocationDegraded(ctx, repoID); err != nil {
		return fmt.Errorf("clear repo location degraded state %s: %w", repoID, err)
	}
	return nil
}

// ClearCache nulls local_path/source/location fields (post-prune).
func (s *Store) ClearCache(ctx context.Context, repoID string) error {
	if err := s.queries.ClearCacheRepoLocation(ctx, repoID); err != nil {
		return fmt.Errorf("clear cache location for %s: %w", repoID, err)
	}
	return nil
}

// Get returns the row or (nil, nil) when absent.
func (s *Store) Get(ctx context.Context, repoID string) (*Location, error) {
	row, err := s.queries.GetRepoLocation(ctx, repoID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repo location %s: %w", repoID, err)
	}
	location := locationFromRow(locationDBRow(row))
	return &location, nil
}

// ListBySource returns all rows with the given source ("local"/"cache").
func (s *Store) ListBySource(ctx context.Context, source string) ([]Location, error) {
	rows, err := s.queries.ListRepoLocationsBySource(ctx, validNullString(source))
	if err != nil {
		return nil, fmt.Errorf("list repo locations by source %s: %w", source, err)
	}

	locations := make([]Location, 0, len(rows))
	for _, row := range rows {
		locations = append(locations, locationFromRow(locationDBRow(row)))
	}
	return locations, nil
}

// Both sqlc queries select the same columns, so their row structs are convertible.
type locationDBRow storedb.GetRepoLocationRow

func locationFromRow(row locationDBRow) Location {
	return Location{
		RepoID:            row.ID,
		Provider:          row.Provider,
		Owner:             row.Owner,
		Repo:              row.Repo,
		LocalPath:         row.LocalPath,
		Source:            row.Source,
		LastUsedAt:        nullTimeValue(row.LastUsedAt),
		LocationStatus:    row.LocationStatus,
		LocationErrorCode: row.LocationErrorCode,
		LocationError:     row.LocationError,
	}
}

const (
	sourceLocal = "local"
	statusOK    = "ok"
)

func shouldPreserveLocalPath(existing *Location, canonicalPath string) bool {
	if existing == nil || existing.Source != sourceLocal || existing.LocationStatus != statusOK {
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

func validNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: true}
}

func currentNullTime() sql.NullTime {
	return sql.NullTime{Time: time.Now().UTC(), Valid: true}
}

func nullTimeValue(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	timestamp := value.Time
	return &timestamp
}
