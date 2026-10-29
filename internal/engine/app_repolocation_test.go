package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestAppBuildsRepoLocationServices(t *testing.T) {
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)
	app := NewWithLogger(0, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	app.db = storetest.Open(t)
	app.prepareStores()
	set := app.buildServices(context.Background())
	if set.RepoLocation == nil || set.DevFolderScan == nil {
		t.Fatal("missing repo-location services")
	}
	if set.RepoLocation.Store != set.DevFolderScan.Locations || set.RepoLocation.Locks != app.repoLocks || set.RepoLocation.Clone == nil || set.DevFolderScan.ScanFunc == nil {
		t.Fatal("repo-location dependencies not shared/wired")
	}
	root, err := set.RepoLocation.CacheRoot()
	if err != nil || root != cacheRoot {
		t.Fatalf("cache root = %q, %v", root, err)
	}
	result, err := set.DevFolderScan.Scan(context.Background(), services.ScanDevFolderRequest{Path: "missing"})
	if err != nil || result != (services.ScanDevFolderResult{}) {
		t.Fatalf("override scan = %+v, %v", result, err)
	}
	_, err = set.RepoLocation.AddByURL(context.Background(), services.AddRepoByURLRequest{URL: "not a url"})
	var serviceErr *services.ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != services.CodeInvalid || serviceErr.Reason != "invalid_url" {
		t.Fatalf("invalid URL = %v", err)
	}
	stats, err := set.RepoLocation.CacheStats(context.Background())
	if err != nil || stats.TotalBytes != 0 || len(stats.Repos) != 0 {
		t.Fatalf("empty cache stats = %+v, %v", stats, err)
	}
}
