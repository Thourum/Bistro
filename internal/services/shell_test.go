package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/itterno/bistro/internal/shell"
)

type shellPlatform struct {
	url    string
	path   string
	folder string
	err    error
}

func (p *shellPlatform) PickFolder(title string) (string, error) { return p.folder, p.err }
func (p *shellPlatform) OpenURL(address string) error            { p.url = address; return p.err }
func (p *shellPlatform) OpenPath(path string) error              { p.path = path; return p.err }
func (p *shellPlatform) Quit()                                   {}

func TestShellSetMenuActions(t *testing.T) {
	var enabled []string
	service := &Shell{OnMenuActions: func(ids []string) { enabled = ids }}
	request := MenuActionsRequest{Enabled: append(append([]string{}, MenuActionIDs...), "unknown")}
	if err := service.SetMenuActions(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(enabled, MenuActionIDs) {
		t.Fatalf("enabled = %v, want %v", enabled, MenuActionIDs)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.SetMenuActions(ctx, MenuActionsRequest{}); err == nil {
		t.Fatal("cancelled request succeeded")
	}
	if !reflect.DeepEqual(enabled, MenuActionIDs) {
		t.Fatal("cancelled request changed enabled actions")
	}
	if err := service.SetMenuActions(context.Background(), MenuActionsRequest{}); err != nil || len(enabled) != 0 {
		t.Fatalf("empty report did not clear enablement: %v, %v", enabled, err)
	}
	if err := (&Shell{}).SetMenuActions(context.Background(), request); err != nil {
		t.Fatal(err)
	}
}

func TestShellOpenExternal(t *testing.T) {
	for _, tt := range []struct {
		address string
		invalid bool
	}{
		{"https://x", false}, {"http://x", false},
		{"javascript:alert(1)", true}, {"file:///etc/passwd", true}, {"https://", true},
		{"https://:443", true}, {"https://x/%zz", true}, {"//x", true},
	} {
		t.Run(tt.address, func(t *testing.T) {
			platform := &shellPlatform{}
			service := &Shell{Platform: platform}
			err := service.OpenExternal(context.Background(), tt.address)
			if tt.invalid {
				var serviceErr *ServiceError
				if !errors.As(err, &serviceErr) || serviceErr.Code != CodeInvalid {
					t.Fatalf("got %v, want CodeInvalid", err)
				}
				if platform.url != "" {
					t.Fatal("invalid URL reached platform")
				}
				return
			}
			if err != nil || platform.url != tt.address {
				t.Fatalf("got URL %q, error %v", platform.url, err)
			}
		})
	}
}

func TestShellOpenSettingsFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	store := shell.NewSettingsStore(path, nil)
	platform := &shellPlatform{}
	service := &Shell{Settings: store, Platform: platform}
	if err := service.OpenSettingsFile(ctx); err != nil {
		t.Fatal(err)
	}
	if platform.path != path {
		t.Fatalf("opened %q, want %q", platform.path, path)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted shell.Settings
	if err := json.Unmarshal(contents, &persisted); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, store.Get()) {
		t.Fatal("missing file did not receive current settings")
	}

	// Opening an existing file must not discard edits awaiting the store watcher.
	const externalEdit = `{"appearance":{"theme":"dark"}}`
	if err := os.WriteFile(path, []byte(externalEdit), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.OpenSettingsFile(ctx); err != nil {
		t.Fatal(err)
	}
	contents, err = os.ReadFile(path)
	if err != nil || string(contents) != externalEdit {
		t.Fatalf("existing file overwritten: %q, %v", contents, err)
	}

	service.Settings = shell.NewSettingsStore(filepath.Join(path, "cannot-write.json"), nil)
	platform.path = ""
	if err := service.OpenSettingsFile(ctx); err == nil || platform.path != "" {
		t.Fatalf("failed persistence opened path %q: %v", platform.path, err)
	}
}

func TestShellEditorErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, editor    string
		req             shell.OpenInEditorRequest
		code            Code
		reason, message string
	}{
		{"not configured", "", shell.OpenInEditorRequest{RepoRoot: root, RelativePath: "file.go"}, CodeInvalid, "editor_not_set", "Set $EDITOR"},
		{"invalid request", "", shell.OpenInEditorRequest{RepoRoot: root, RelativePath: "file.go", Line: -1}, CodeInvalid, "", ""},
		{"launch failure", filepath.Join(root, "missing-editor"), shell.OpenInEditorRequest{RepoRoot: root, RelativePath: "file.go"}, CodeInternal, "editor_launch_failed", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := &Shell{EditorEnv: func() string { return tt.editor }}
			err := service.OpenInEditor(context.Background(), tt.req)
			var serviceErr *ServiceError
			if !errors.As(err, &serviceErr) || serviceErr.Code != tt.code || serviceErr.Reason != tt.reason {
				t.Fatalf("got %v, want %s/%s", err, tt.code, tt.reason)
			}
			if tt.message != "" && serviceErr.Message != tt.message {
				t.Fatalf("got message %q", serviceErr.Message)
			}
		})
	}
}
