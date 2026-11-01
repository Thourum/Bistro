package services

import (
	"context"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/storetest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRepoLocationScanDevFolderReconcilesHealthAndTriggersSync(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 permission checks are not reliable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission-denied checks are not meaningful when running as root")
	}
	t.Setenv("BISTRO_REPO_DIR", "")
	// Arrange
	db := storetest.Open(t)
	store := repolocation.New(db.DB)
	dev := filepath.Join(t.TempDir(), "dev")
	mustMkdirAll(dev)

	healthyPath := filepath.Join(dev, "acme", "healthy")
	if err := createRepo(healthyPath, "origin", "https://github.com/acme/healthy.git"); err != nil {
		t.Fatalf("create healthy repo: %v", err)
	}
	missingPath := filepath.Join(dev, "acme", "missing")
	if err := store.UpsertLocal(context.Background(), "github", "acme", "missing", missingPath, false); err != nil {
		t.Fatalf("upsert missing: %v", err)
	}
	permissionPath := filepath.Join(dev, "acme", "permission")
	if err := createRepo(permissionPath, "origin", "https://github.com/acme/permission.git"); err != nil {
		t.Fatalf("create permission repo: %v", err)
	}
	if err := store.UpsertLocal(context.Background(), "github", "acme", "permission", permissionPath, false); err != nil {
		t.Fatalf("upsert permission: %v", err)
	}
	mustChmod(t, permissionPath, 0)
	if _, err := os.Open(permissionPath); !os.IsPermission(err) {
		t.Skipf("chmod 000 did not produce permission denial: %v", err)
	}
	mismatchPath := filepath.Join(dev, "acme", "mismatch")
	if err := createRepo(mismatchPath, "origin", "https://github.com/acme/other.git"); err != nil {
		t.Fatalf("create mismatch repo: %v", err)
	}
	if err := store.UpsertLocal(context.Background(), "github", "acme", "mismatch", mismatchPath, false); err != nil {
		t.Fatalf("upsert mismatch: %v", err)
	}
	syncTrigger := make(chan string, 1)
	service := &DevFolderScan{Locations: store, SyncTrigger: syncTrigger, ScanFunc: scan.Scan}

	// Act
	result, err := service.Scan(context.Background(), ScanDevFolderRequest{Path: dev, Reason: "test"})

	// Assert
	if err != nil {
		t.Fatalf("expected ok response, got %#v", result)
	}
	if result.Healthy != 2 || result.Degraded != 3 {
		t.Fatalf("expected 2 healthy and 3 degraded, got %#v", result)
	}
	assertLocationStatus(t, store, "github:acme/healthy", "ok", "")
	assertLocationStatus(t, store, "github:acme/missing", "missing", "path_missing")
	assertLocationStatus(t, store, "github:acme/permission", "permission_denied", "permission_denied")
	assertLocationStatus(t, store, "github:acme/mismatch", "remote_mismatch", "remote_mismatch")
	select {
	case <-syncTrigger:
	case <-time.After(time.Second):
		t.Fatal("expected scan to trigger sync loop")
	}
}

func TestRepoLocationRecoverPermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission checks unavailable")
	}
	database := storetest.Open(t)
	path := filepath.Join(t.TempDir(), "api")
	if err := createRepo(path, "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatal(err)
	}
	insertRepoWithLocation(t, database, "github:acme/api", "github", "acme", "api", "local", path, nil)
	mustChmod(t, path, 0)
	dir, err := os.Open(path)
	if err == nil {
		_ = dir.Close()
		t.Skip("chmod did not deny access")
	}
	if !os.IsPermission(err) {
		t.Fatalf("unexpected access error: %v", err)
	}
	service := &RepoLocation{Store: repolocation.New(database.DB)}
	assertLocationError(t, service.RecoverPath(context.Background(), RecoverPathRequest{RepoID: "github:acme/api", Path: path}), "permission_denied")
}
