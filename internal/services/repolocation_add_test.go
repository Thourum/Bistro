package services

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/storetest"
	"os"
	"path/filepath"
	"testing"
)

func locationStringPtr(value string) *string { return &value }

func assertLocationError(t *testing.T, err error, reason string) {
	t.Helper()
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("expected ServiceError, got %v", err)
	}
	expected := map[string]Code{
		"invalid_url": CodeInvalid, "destination_required": CodeInvalid,
		"invalid_path": CodeInvalid, "destination_conflict": CodeInvalid,
		"remote_mismatch": CodeInvalid, "path_missing": CodeInvalid,
		"auth_failed": CodeUnauthenticated, "cancelled": CodeCancelled,
	}
	code, known := expected[reason]
	if !known {
		code = CodeInternal
	}
	if serviceErr.Code != code || serviceErr.Reason != reason || serviceErr.Message == "" {
		t.Fatalf("expected %s/%s, got %+v", code, reason, serviceErr)
	}
}

func TestRepoLocationAddRepoByURL(t *testing.T) {
	tests := []struct {
		name             string
		payload          func(root string) AddRepoByURLRequest
		arrange          func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner)
		wantOK           bool
		wantCode         string
		wantSource       string
		wantPartialClean string
		wantPreservePath string
		wantStoredSource string
		wantStoredPath   string
		wantCloneBare    *bool
	}{
		{
			name: "found local via targeted dev folder check",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: nil, DevFolderPath: locationStringPtr(filepath.Join(root, "dev"))}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				if err := createRepo(filepath.Join(root, "dev", "acme", "api"), "origin", "https://github.com/acme/api.git"); err != nil {
					t.Fatalf("create targeted repo: %v", err)
				}
			},
			wantOK:     true,
			wantSource: "local",
		},
		{
			name: "destination required",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: nil, DevFolderPath: nil}
			},
			wantOK:   false,
			wantCode: "destination_required",
		},
		{
			name: "invalid url",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "not a url", Destination: nil, DevFolderPath: nil}
			},
			wantOK:   false,
			wantCode: "invalid_url",
		},
		{
			name: "registered local wins",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("cache"), DevFolderPath: nil}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				localPath := filepath.Join(root, "dev", "acme", "api")
				if err := createRepo(localPath, "origin", "https://github.com/acme/api.git"); err != nil {
					t.Fatalf("create local repo: %v", err)
				}
				if err := repolocation.New(db.DB).UpsertLocal(context.Background(), "github", "acme", "api", localPath, true); err != nil {
					t.Fatalf("upsert local: %v", err)
				}
			},
			wantOK:     true,
			wantSource: "local",
		},
		{
			name: "degraded registered local returns structured error and preserves row",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("cache"), DevFolderPath: nil}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				localPath := filepath.Join(root, "dev", "acme", "api")
				insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "local", localPath, nil)
				if err := repolocation.New(db.DB).MarkDegraded(context.Background(), "github:acme/api", "missing", "path_missing", "folder not found"); err != nil {
					t.Fatalf("mark degraded local: %v", err)
				}
			},
			wantOK:           false,
			wantCode:         "path_missing",
			wantStoredSource: "local",
			wantStoredPath:   filepath.Join("dev", "acme", "api"),
		},
		{
			name: "cache existing match returns cache",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("cache"), DevFolderPath: nil}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cachePath := filepath.Join(root, "cache", "acme", "api")
				if err := createRepo(cachePath, "origin", "https://github.com/acme/api.git"); err != nil {
					t.Fatalf("create cache repo: %v", err)
				}
			},
			wantOK:           true,
			wantSource:       "cache",
			wantStoredSource: "cache",
		},
		{
			name: "auth failure maps auth_failed",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("cache"), DevFolderPath: nil}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = fmt.Errorf("clone repository: %w", transport.ErrAuthenticationRequired)
			},
			wantOK:        false,
			wantCode:      "auth_failed",
			wantCloneBare: boolPtr(true),
		},
		{
			name: "auth in clone failure message is not an auth failure",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/oauth-lib.git", Destination: locationStringPtr("cache")}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = errors.New("clone https://github.com/acme/oauth-lib.git: connection refused")
			},
			wantCode: "clone_failed",
		},
		{
			name: "unsupported destination is conflict",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("elsewhere"), DevFolderPath: nil}
			},
			wantOK:   false,
			wantCode: "destination_conflict",
		},
		{
			name: "clone failed preserves preexisting empty dir",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("dev_folder"), DevFolderPath: locationStringPtr(filepath.Join(root, "dev"))}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = errors.New("clone boom")
				mustMkdirAll(filepath.Join(root, "dev"))
				mustMkdirAll(filepath.Join(root, "dev", "acme", "api"))
			},
			wantOK:           false,
			wantCode:         "clone_failed",
			wantPreservePath: filepath.Join("dev", "acme", "api"),
		},
		{
			name: "missing dev folder path is invalid",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("dev_folder"), DevFolderPath: locationStringPtr(filepath.Join(root, "missing-dev"))}
			},
			wantOK:   false,
			wantCode: "invalid_path",
		},
		{
			name: "destination conflict",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("dev_folder"), DevFolderPath: locationStringPtr(filepath.Join(root, "dev"))}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				mustMkdirAll(filepath.Join(root, "dev"))
				conflict := filepath.Join(root, "dev", "acme", "api")
				mustMkdirAll(conflict)
				mustWriteFile(filepath.Join(conflict, "README.md"), "not this repo")
			},
			wantOK:   false,
			wantCode: "destination_conflict",
		},
		{
			name: "clone failed cleans partial dir",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("dev_folder"), DevFolderPath: locationStringPtr(filepath.Join(root, "dev"))}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = errors.New("clone boom")
				mustMkdirAll(filepath.Join(root, "dev"))
			},
			wantOK:           false,
			wantCode:         "clone_failed",
			wantPartialClean: filepath.Join("dev", "acme", "api"),
			wantCloneBare:    boolPtr(false),
		},
		{
			name: "clone failed preserves dev folder user file created during clone",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("dev_folder"), DevFolderPath: locationStringPtr(filepath.Join(root, "dev"))}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = errors.New("clone boom")
				mustMkdirAll(filepath.Join(root, "dev"))
				cloner.onFailure = func(destination string) {
					mustWriteFile(filepath.Join(destination, "user-notes.txt"), "created by user during clone")
				}
			},
			wantOK:           false,
			wantCode:         "clone_failed",
			wantPreservePath: filepath.Join("dev", "acme", "api"),
			wantCloneBare:    boolPtr(false),
		},
		{
			name: "post clone remote mismatch cleans partial dir",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("dev_folder"), DevFolderPath: locationStringPtr(filepath.Join(root, "dev"))}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				mustMkdirAll(filepath.Join(root, "dev"))
				cloner.overrideRemoteURL = "https://github.com/acme/other.git"
			},
			wantOK:           false,
			wantCode:         "remote_mismatch",
			wantPartialClean: filepath.Join("dev", "acme", "api"),
		},
		{
			name: "cancelled context",
			payload: func(root string) AddRepoByURLRequest {
				return AddRepoByURLRequest{URL: "https://github.com/acme/api.git", Destination: locationStringPtr("dev_folder"), DevFolderPath: locationStringPtr(filepath.Join(root, "dev"))}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = context.Canceled
				mustMkdirAll(filepath.Join(root, "dev"))
			},
			wantOK:   false,
			wantCode: "cancelled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			db := storetest.Open(t)
			cloner := &fakeRepoLocationCloner{}
			if tt.arrange != nil {
				tt.arrange(t, db, root, cloner)
			}
			service := &RepoLocation{
				DB:    db.DB,
				Store: repolocation.New(db.DB),
				Clone: cloner.CloneRepo,
				CacheRoot: func() (string, error) {
					cacheRoot := filepath.Join(root, "cache")
					mustMkdirAll(cacheRoot)
					return cacheRoot, nil
				},
				Log: testRepoLocationLogger(),
			}
			ctx := context.Background()

			// Act
			result, err := service.AddByURL(ctx, tt.payload(root))

			// Assert
			if (err == nil) != tt.wantOK {
				t.Fatalf("expected ok=%v, got %#v", tt.wantOK, result)
			}
			if tt.wantCloneBare != nil {
				calls := cloner.Calls()
				if len(calls) == 0 || calls[len(calls)-1].Bare != *tt.wantCloneBare {
					t.Fatalf("expected clone bare=%v, got calls=%#v", *tt.wantCloneBare, calls)
				}
			}
			if tt.wantOK {
				if result.RepoID != "github:acme/api" || result.Source != tt.wantSource {
					t.Fatalf("unexpected success payload %#v", result)
				}
				if tt.wantStoredSource != "" {
					storedPath := filepath.Join(root, "cache", "acme", "api")
					if tt.wantStoredPath != "" {
						storedPath = filepath.Join(root, tt.wantStoredPath)
					}
					assertSourcePath(t, db.DB, "github:acme/api", tt.wantStoredSource, storedPath)
				}
				return
			}
			assertLocationError(t, err, tt.wantCode)
			if tt.wantStoredSource != "" {
				storedPath := filepath.Join(root, "cache", "acme", "api")
				if tt.wantStoredPath != "" {
					storedPath = filepath.Join(root, tt.wantStoredPath)
				}
				assertSourcePath(t, db.DB, "github:acme/api", tt.wantStoredSource, storedPath)
			}
			if tt.wantPartialClean != "" {
				if _, err := os.Stat(filepath.Join(root, tt.wantPartialClean)); !os.IsNotExist(err) {
					t.Fatalf("expected partial dir cleanup, stat err=%v", err)
				}
			}
			if tt.wantPreservePath != "" {
				if _, err := os.Stat(filepath.Join(root, tt.wantPreservePath)); err != nil {
					t.Fatalf("expected preexisting dir preserved, stat err=%v", err)
				}
			}
		})
	}
}
