package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrEditorRequest = errors.New("invalid editor request")
	ErrEditorNotSet  = errors.New("$EDITOR is not set")
	ErrEditorLaunch  = errors.New("editor launch failed")
)

const (
	editorOutsideRepo     = "Access denied: path outside repository"
	editorNotCheckout     = "Access denied: repo root is not a git checkout"
	editorRootNotAbsolute = "Access denied: repo root must be an absolute path"
)

// OpenInEditorRequest identifies a checkout file and an optional positive line.
type OpenInEditorRequest struct {
	RepoRoot     string `json:"repoRoot"`
	RelativePath string `json:"relativePath"`
	Line         int    `json:"line,omitempty"`
}

func validateEditorRequest(req OpenInEditorRequest) error {
	if req.RepoRoot == "" || !filepath.IsAbs(req.RepoRoot) || strings.ContainsRune(req.RepoRoot, 0) {
		return fmt.Errorf("%w: Invalid repository root", ErrEditorRequest)
	}
	if req.RelativePath == "" || strings.ContainsRune(req.RelativePath, 0) {
		return fmt.Errorf("%w: Invalid file path", ErrEditorRequest)
	}
	if req.Line < 0 {
		return fmt.Errorf("%w: Invalid line number", ErrEditorRequest)
	}
	return nil
}

// ResolveEditorTarget follows symlinks before enforcing the checkout boundary.
func ResolveEditorTarget(repoRoot, relativePath string) (string, error) {
	if repoRoot == "" || !filepath.IsAbs(repoRoot) || strings.ContainsRune(repoRoot, 0) {
		return "", fmt.Errorf("%w: %s", ErrEditorRequest, editorRootNotAbsolute)
	}
	if relativePath == "" || filepath.IsAbs(relativePath) || strings.ContainsRune(relativePath, 0) {
		return "", fmt.Errorf("%w: %s", ErrEditorRequest, editorOutsideRepo)
	}
	root, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrEditorRequest, editorNotCheckout)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return "", fmt.Errorf("%w: %s", ErrEditorRequest, editorNotCheckout)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(root, relativePath))
	if err != nil || (target != root && !strings.HasPrefix(target, root+string(filepath.Separator))) {
		return "", fmt.Errorf("%w: %s", ErrEditorRequest, editorOutsideRepo)
	}
	return target, nil
}

// ParseEditorCommand tokenizes EDITOR without evaluating shell syntax.
func ParseEditorCommand(s string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	var quote rune
	inToken := false
	chars := []rune(s)
	for i := 0; i < len(chars); i++ {
		ch := chars[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
				continue
			}
			if ch == '\\' && quote == '"' && i+1 < len(chars) && (chars[i+1] == '"' || chars[i+1] == '\\') {
				i++
				ch = chars[i]
			}
			current.WriteRune(ch)
			continue
		}
		switch {
		case ch == '"' || ch == '\'':
			quote, inToken = ch, true
		case ch == '\\' && i+1 < len(chars):
			i++
			current.WriteRune(chars[i])
			inToken = true
		case unicode.IsSpace(ch):
			if inToken {
				tokens = append(tokens, current.String())
				current.Reset()
				inToken = false
			}
		default:
			current.WriteRune(ch)
			inToken = true
		}
	}
	if inToken {
		tokens = append(tokens, current.String())
	}
	if len(tokens) == 0 || tokens[0] == "" {
		return nil, ErrEditorNotSet
	}
	return tokens, nil
}

// BuildEditorArgs preserves leading arguments and adds the editor's file hint.
func BuildEditorArgs(editor []string, file string, line int) []string {
	if len(editor) == 0 {
		return nil
	}
	args := append([]string(nil), editor[1:]...)
	// Match Windows executable paths even when parsing settings on Unix.
	binary := strings.ToLower(filepath.Base(strings.ReplaceAll(editor[0], "\\", "/")))
	for _, suffix := range []string{".exe", ".cmd", ".bat"} {
		if strings.HasSuffix(binary, suffix) {
			binary = strings.TrimSuffix(binary, suffix)
			break
		}
	}
	switch binary {
	case "code", "code-insiders", "cursor":
		if line > 0 {
			file += ":" + strconv.Itoa(line)
		}
		return append(args, "-g", file)
	case "vim", "nvim", "vi":
		if line > 0 {
			args = append(args, "+"+strconv.Itoa(line))
		}
	}
	return append(args, file)
}

// OpenInEditor validates the target before starting a detached editor process.
func OpenInEditor(req OpenInEditorRequest, editorEnv string) error {
	if err := validateEditorRequest(req); err != nil {
		return err
	}
	target, err := ResolveEditorTarget(req.RepoRoot, req.RelativePath)
	if err != nil {
		return err
	}
	editor, err := ParseEditorCommand(editorEnv)
	if err != nil {
		return err
	}
	return launch(editor[0], BuildEditorArgs(editor, target, req.Line))
}

func launch(cmd string, args []string) error {
	command := exec.Command(cmd, args...)
	detach(command)
	if err := command.Start(); err != nil {
		return fmt.Errorf("%w: %v", ErrEditorLaunch, err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("%w: %v", ErrEditorLaunch, err)
	}
	return nil
}
