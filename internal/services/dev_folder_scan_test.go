package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestDevFolderScanCancellation(t *testing.T) {
	for _, superseded := range []bool{false, true} {
		t.Run(fmt.Sprintf("superseded=%v", superseded), func(t *testing.T) {
			t.Setenv("BISTRO_REPO_DIR", "")
			entered := make(chan struct{})
			done := make(chan error, 1)
			database := storetest.Open(t)
			root := t.TempDir()
			service := &DevFolderScan{Locations: repolocation.New(database.DB), ScanFunc: func(ctx context.Context, path string) (scan.Result, error) {
				if path != root {
					return scan.Result{}, nil
				}
				close(entered)
				<-ctx.Done()
				return scan.Result{}, ctx.Err()
			}}
			// Temp directories can have symlinked ancestors on macOS.
			canonical, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			root = canonical
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _, err := service.Scan(ctx, ScanDevFolderRequest{Path: root}); done <- err }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("scan did not start")
			}
			if superseded {
				if _, err := service.Scan(context.Background(), ScanDevFolderRequest{Path: t.TempDir()}); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-done:
				assertServiceCode(t, err, CodeCancelled)
			case <-time.After(time.Second):
				t.Fatal("cancelled scan did not return")
			}
		})
	}
}

func TestDevFolderScanValidationAndOverride(t *testing.T) {
	service := &DevFolderScan{}
	root := t.TempDir()
	result, err := service.Validate(context.Background(), ValidateDevFolderRequest{Path: root})
	canonical, canonicalErr := filepath.EvalSymlinks(root)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if err != nil || result.CanonicalPath != canonical {
		t.Fatalf("validate = %+v, %v", result, err)
	}
	_, err = service.Scan(context.Background(), ScanDevFolderRequest{Path: filepath.Join(root, "missing")})
	assertLocationError(t, err, "invalid_path")
	t.Setenv("BISTRO_REPO_DIR", root)
	scanResult, err := service.Scan(context.Background(), ScanDevFolderRequest{Path: "missing"})
	if err != nil || scanResult != (ScanDevFolderResult{}) {
		t.Fatalf("override = %+v, %v", scanResult, err)
	}
}

func TestDevFolderScanPersistsAndReconcilesHealth(t *testing.T) {
	t.Setenv("BISTRO_REPO_DIR", "")
	database := storetest.Open(t)
	locations := repolocation.New(database.DB)
	root := t.TempDir()
	healthy := filepath.Join(root, "acme", "api")
	mismatch := filepath.Join(root, "other")
	if err := createRepo(healthy, "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatal(err)
	}
	if err := createRepo(mismatch, "origin", "https://github.com/acme/other.git"); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ name, path string }{{"missing", filepath.Join(root, "missing")}, {"mismatch", mismatch}} {
		if err := locations.UpsertLocal(context.Background(), "github", "acme", row.name, row.path, false); err != nil {
			t.Fatal(err)
		}
	}
	syncs := make(chan string, 1)
	service := &DevFolderScan{Locations: locations, SyncTrigger: syncs, ScanFunc: func(context.Context, string) (scan.Result, error) {
		return scan.Result{Found: []scan.Found{{Identity: remote.Identity{Provider: "github", Owner: "acme", Repo: "api"}, Path: healthy}}, Skipped: 1, Partial: true}, nil
	}}
	result, err := service.Scan(context.Background(), ScanDevFolderRequest{Path: root})
	if err != nil || result != (ScanDevFolderResult{Discovered: 1, Healthy: 1, Degraded: 2, Skipped: 1, Partial: true}) {
		t.Fatalf("scan = %+v, %v", result, err)
	}
	assertLocationStatus(t, locations, "github:acme/api", "ok", "")
	assertLocationStatus(t, locations, "github:acme/missing", "missing", "path_missing")
	assertLocationStatus(t, locations, "github:acme/mismatch", "remote_mismatch", "remote_mismatch")
	var watched int
	if err := database.QueryRow("SELECT watched FROM repos WHERE id='github:acme/api'").Scan(&watched); err != nil || watched != 0 {
		t.Fatalf("scan auto-watched repo: %d, %v", watched, err)
	}
	select {
	case id := <-syncs:
		if id != "" {
			t.Fatalf("sync %q", id)
		}
	default:
		t.Fatal("missing sync")
	}
}

func TestDevFolderScanErrors(t *testing.T) {
	for _, failure := range []string{"scan_failed", "persistence_failed", "cancelled", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("BISTRO_REPO_DIR", "")
			database := storetest.Open(t)
			if failure == "persistence_failed" {
				if _, err := database.Exec("DROP TABLE repos"); err != nil {
					t.Fatal(err)
				}
			}
			syncs := make(chan string, 1)
			service := &DevFolderScan{Locations: repolocation.New(database.DB), SyncTrigger: syncs, ScanFunc: func(context.Context, string) (scan.Result, error) {
				switch failure {
				case "scan_failed":
					return scan.Result{}, errors.New("filesystem failure")
				case "cancelled":
					return scan.Result{}, fmt.Errorf("walk: %w", context.Canceled)
				case "timeout":
					return scan.Result{}, fmt.Errorf("walk: %w", context.DeadlineExceeded)
				default:
					return scan.Result{}, nil
				}
			}}
			_, err := service.Scan(context.Background(), ScanDevFolderRequest{Path: t.TempDir()})
			if failure == "cancelled" {
				assertServiceCode(t, err, CodeCancelled)
			} else if failure == "timeout" {
				assertServiceCode(t, err, CodeTimeout)
			} else {
				assertLocationError(t, err, failure)
			}
			select {
			case <-syncs:
				t.Fatal("failed scan triggered sync")
			default:
			}
		})
	}
}
