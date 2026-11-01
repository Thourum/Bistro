package shell

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Platform supplies desktop operations to services without owning a Wails app.
type Platform interface {
	PickFolder(title string) (string, error)
	OpenURL(url string) error
	OpenPath(path string) error
	Quit()
}

type desktopPlatform struct{}

// NewPlatform resolves application.Get at call time, after shell startup.
func NewPlatform() Platform { return desktopPlatform{} }

func (desktopPlatform) PickFolder(title string) (string, error) {
	return application.Get().Dialog.OpenFile().
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
		SetTitle(title).PromptForSingleSelection()
}

func (desktopPlatform) OpenURL(url string) error {
	return application.Get().Browser.OpenURL(url)
}

func (desktopPlatform) OpenPath(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", path)
	case "linux":
		command = exec.Command("xdg-open", path)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		return fmt.Errorf("opening paths is unsupported on %s", runtime.GOOS)
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("open path: %w", err)
	}
	return nil
}

func (desktopPlatform) Quit() { application.Get().Quit() }
