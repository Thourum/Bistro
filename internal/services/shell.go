package services

import (
	"context"
	"errors"
	"net/url"
	"os"
	"runtime"

	"github.com/itterno/bistro/internal/shell"
)

// Shell exposes desktop settings and OS operations; the engine does not own it.
type Shell struct {
	Settings  *shell.SettingsStore
	Platform  shell.Platform
	EditorEnv func() string
	Version   string
	LogDir    string
	Panics    PanicReporter
}

// AppInfo identifies the running desktop build.
type AppInfo struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

func (s *Shell) GetSettings(ctx context.Context) (shell.Settings, error) {
	return guard(s.Panics, "Shell.GetSettings", func() (shell.Settings, error) {
		return s.Settings.Get(), nil
	})
}

func (s *Shell) SetSettings(ctx context.Context, settings shell.Settings) (shell.Settings, error) {
	return guard(s.Panics, "Shell.SetSettings", func() (shell.Settings, error) {
		return s.Settings.Set(settings)
	})
}

func (s *Shell) ResetSettings(ctx context.Context) (shell.Settings, error) {
	return guard(s.Panics, "Shell.ResetSettings", func() (shell.Settings, error) {
		return s.Settings.Reset()
	})
}

func (s *Shell) OpenSettingsFile(ctx context.Context) error {
	return guardErr(s.Panics, "Shell.OpenSettingsFile", func() error {
		_, err := os.Stat(s.Settings.Path())
		if errors.Is(err, os.ErrNotExist) {
			if _, err := s.Settings.Set(s.Settings.Get()); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		return s.Platform.OpenPath(s.Settings.Path())
	})
}

func (s *Shell) PickFolder(ctx context.Context) (string, error) {
	return guard(s.Panics, "Shell.PickFolder", func() (string, error) {
		return s.Platform.PickFolder("")
	})
}

func (s *Shell) OpenExternal(ctx context.Context, address string) error {
	return guardErr(s.Panics, "Shell.OpenExternal", func() error {
		parsed, err := url.Parse(address)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			return Errorf(CodeInvalid, "Invalid external URL")
		}
		return s.Platform.OpenURL(address)
	})
}

func (s *Shell) OpenInEditor(ctx context.Context, req shell.OpenInEditorRequest) error {
	return guardErr(s.Panics, "Shell.OpenInEditor", func() error {
		err := shell.OpenInEditor(req, s.EditorEnv())
		switch {
		case errors.Is(err, shell.ErrEditorNotSet):
			return WithReason(CodeInvalid, "editor_not_set", "Set $EDITOR")
		case errors.Is(err, shell.ErrEditorRequest):
			return Errorf(CodeInvalid, "%s", err)
		case errors.Is(err, shell.ErrEditorLaunch):
			return WithReason(CodeInternal, "editor_launch_failed", err.Error())
		default:
			return err
		}
	})
}

func (s *Shell) Quit(ctx context.Context) error {
	return guardErr(s.Panics, "Shell.Quit", func() error {
		s.Platform.Quit()
		return nil
	})
}

func (s *Shell) AppInfo(ctx context.Context) (AppInfo, error) {
	return guard(s.Panics, "Shell.AppInfo", func() (AppInfo, error) {
		return AppInfo{Version: s.Version, Platform: runtime.GOOS}, nil
	})
}

func (s *Shell) OpenLogFolder(ctx context.Context) error {
	return guardErr(s.Panics, "Shell.OpenLogFolder", func() error {
		return s.Platform.OpenPath(s.LogDir)
	})
}
