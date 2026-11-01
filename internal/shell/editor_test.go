package shell

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateEditorRequest(t *testing.T) {
	root := t.TempDir()
	for _, tt := range []struct {
		name    string
		req     OpenInEditorRequest
		message string
	}{
		{"valid", OpenInEditorRequest{root, "src/a.ts", 3}, ""},
		{"omitted line", OpenInEditorRequest{root, "src/a.ts", 0}, ""},
		{"negative line", OpenInEditorRequest{root, "src/a.ts", -1}, "Invalid line number"},
		{"empty path", OpenInEditorRequest{root, "", 0}, "Invalid file path"},
		{"NUL path", OpenInEditorRequest{root, "a\x00b", 0}, "Invalid file path"},
		{"relative root", OpenInEditorRequest{"relative", "a", 0}, "Invalid repository root"},
		{"empty root", OpenInEditorRequest{"", "a", 0}, "Invalid repository root"},
		{"NUL root", OpenInEditorRequest{root + "\x00", "a", 0}, "Invalid repository root"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEditorRequest(tt.req)
			if tt.message == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, ErrEditorRequest) || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("got %v, want ErrEditorRequest with %q", err, tt.message)
			}
		})
	}
}

func editorFixture(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	for _, dir := range []string{filepath.Join(root, ".git"), filepath.Join(root, "src"), filepath.Join(base, "outside"), filepath.Join(base, "repo-evil", ".git")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join(root, "src", "app.ts"), filepath.Join(base, "outside", "secret.env"), filepath.Join(base, "repo-evil", "note.txt")} {
		if err := os.WriteFile(file, []byte("fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return base, root
}

func TestResolveEditorTarget(t *testing.T) {
	base, root := editorFixture(t)
	for _, tt := range []struct{ name, root, relative, message string }{
		{"contained", root, "src/app.ts", ""},
		{"root itself", root, ".", ""},
		{"parent traversal", root, "../outside/secret.env", editorOutsideRepo},
		{"absolute escape", root, filepath.Join(base, "outside", "secret.env"), editorOutsideRepo},
		{"absolute inside", root, filepath.Join(root, "src", "app.ts"), editorOutsideRepo},
		{"nested traversal", root, "src/../../outside/secret.env", editorOutsideRepo},
		{"sibling prefix trap", root, "../repo-evil/note.txt", editorOutsideRepo},
		{"non git root", filepath.Join(base, "outside"), "secret.env", editorNotCheckout},
		{"relative root", "repo", "src/app.ts", editorRootNotAbsolute},
		{"missing root", filepath.Join(base, "missing"), "src/app.ts", editorNotCheckout},
		{"missing target", root, "nope.ts", editorOutsideRepo},
		{"empty root", "", "src/app.ts", editorRootNotAbsolute},
		{"empty target", root, "", editorOutsideRepo},
		{"NUL target", root, "a\x00b", editorOutsideRepo},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ResolveEditorTarget(tt.root, tt.relative)
			if tt.message != "" {
				if !errors.Is(err, ErrEditorRequest) || !strings.Contains(err.Error(), tt.message) {
					t.Fatalf("got %v, want %q", err, tt.message)
				}
				return
			}
			want, resolveErr := filepath.EvalSymlinks(filepath.Join(tt.root, tt.relative))
			if resolveErr != nil || err != nil || target != want {
				t.Fatalf("got %q, %v; want %q (%v)", target, err, want, resolveErr)
			}
		})
	}
}

func TestResolveEditorTargetSymlinks(t *testing.T) {
	base, root := editorFixture(t)
	for _, tt := range []struct {
		name, source, link, relative string
		escapes                      bool
	}{
		{"file escape", filepath.Join(base, "outside", "secret.env"), "leak.env", "leak.env", true},
		{"directory escape", filepath.Join(base, "outside"), "linked", "linked/secret.env", true},
		{"contained symlink", filepath.Join(root, "src", "app.ts"), "app.ts", "app.ts", false},
		{"root symlink", root, "checkout", "src/app.ts", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			link := filepath.Join(root, tt.link)
			if tt.name == "root symlink" {
				link = filepath.Join(base, tt.link)
			}
			if err := os.Symlink(tt.source, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			repoRoot := root
			if tt.name == "root symlink" {
				repoRoot = link
			}
			target, err := ResolveEditorTarget(repoRoot, tt.relative)
			if tt.escapes {
				if !errors.Is(err, ErrEditorRequest) || !strings.Contains(err.Error(), editorOutsideRepo) {
					t.Fatalf("escape accepted: %q, %v", target, err)
				}
				return
			}
			want, resolveErr := filepath.EvalSymlinks(filepath.Join(root, "src", "app.ts"))
			if resolveErr != nil || err != nil || target != want {
				t.Fatalf("got %q, %v; want %q", target, err, want)
			}
		})
	}
}

func TestParseEditorCommand(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  []string
	}{
		{" /usr/bin/nvim -u NONE ", []string{"/usr/bin/nvim", "-u", "NONE"}},
		{`"C:\Program Files\Microsoft VS Code\Code.exe" --wait`, []string{`C:\Program Files\Microsoft VS Code\Code.exe`, "--wait"}},
		{`nvim -c 'set ft=go' /my\ path/x`, []string{"nvim", "-c", "set ft=go", "/my path/x"}},
		{`nvim "a\"b" "c\\d" "e\f"`, []string{"nvim", `a"b`, `c\d`, `e\f`}},
		{`nvim 'a\b' ""`, []string{"nvim", `a\b`, ""}},
		{`nvim trailing\`, []string{"nvim", `trailing\`}},
		{`nvim 'unfinished`, []string{"nvim", "unfinished"}},
		{`nvim a" b"c`, []string{"nvim", "a bc"}},
		{"", nil}, {"   \t\n", nil}, {`"" --wait`, nil},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseEditorCommand(tt.input)
			if tt.want == nil {
				if !errors.Is(err, ErrEditorNotSet) || err.Error() != "$EDITOR is not set" {
					t.Fatalf("got %v", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestBuildEditorArgs(t *testing.T) {
	for _, tt := range []struct {
		editor string
		line   int
		want   []string
	}{
		{"code", 42, []string{"-g", "src/foo.ts:42"}},
		{"code-insiders", 42, []string{"-g", "src/foo.ts:42"}},
		{"cursor", 42, []string{"-g", "src/foo.ts:42"}},
		{"vim", 7, []string{"+7", "src/foo.ts"}},
		{"nvim", 7, []string{"+7", "src/foo.ts"}},
		{"vi", 7, []string{"+7", "src/foo.ts"}},
		{"vim", 0, []string{"src/foo.ts"}},
		{"code", 0, []string{"-g", "src/foo.ts"}},
		{"code --wait", 3, []string{"--wait", "-g", "src/foo.ts:3"}},
		{"subl", 1, []string{"src/foo.ts"}},
		{`"C:\VS Code\Code.exe"`, 5, []string{"-g", "src/foo.ts:5"}},
		{`"C:\VS Code\CODE.CMD"`, 5, []string{"-g", "src/foo.ts:5"}},
		{`"C:\VS Code\Code.bat"`, 5, []string{"-g", "src/foo.ts:5"}},
	} {
		t.Run(tt.editor+"/"+strings.Join(tt.want, " "), func(t *testing.T) {
			editor, err := ParseEditorCommand(tt.editor)
			if err != nil {
				t.Fatal(err)
			}
			before := append([]string(nil), editor...)
			got := BuildEditorArgs(editor, "src/foo.ts", tt.line)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(editor, before) {
				t.Fatal("mutated editor arguments")
			}
		})
	}
}

func TestLaunch(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	if err := launch(goBinary, []string{"version"}); err != nil {
		t.Fatal(err)
	}
	if err := launch(filepath.Join(t.TempDir(), "missing-editor"), nil); !errors.Is(err, ErrEditorLaunch) {
		t.Fatalf("got %v, want ErrEditorLaunch", err)
	}
}
