package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleasePackagingResolvesVersionScopeAndArchitecture(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS package task is platform gated")
	}
	cli, err := exec.LookPath("wails3")
	if err != nil {
		t.Skip("pinned Wails CLI unavailable")
	}
	fixture := t.TempDir()
	// Dry runs still execute Docker preconditions. This fixture only supplies
	// discovery, never a build, so target commands can be asserted without Docker.
	if err := os.WriteFile(filepath.Join(fixture, "docker"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		task       string
		args       []string
		want       []string
		wantAbsent string
	}{
		{"package:darwin", nil, []string{"-X main.version=0.2.0-alpha.2", "s/@NUMERIC_VERSION@/0.2.0/g", "s/@VERSION@/0.2.0-alpha.2/g"}, "embeddedGitHubClientID="},
		{"package:windows", []string{"CGO_ENABLED=0"}, []string{"-X main.version=0.2.0-alpha.2", "-DINFO_PRODUCTVERSION=0.2.0", "-DWAILS_INSTALL_SCOPE=user -DREQUEST_EXECUTION_LEVEL=user", "s/@NUMERIC_VERSION@/0.2.0/g"}, ""},
		{"package:linux", []string{"DOCKER_MOUNTS="}, []string{"--platform linux/amd64", "-X main.version=0.2.0-alpha.2"}, ""},
		{"linux:build:docker", []string{"ARCH=arm64", "DOCKER_MOUNTS="}, []string{"--platform linux/arm64"}, ""},
		{"setup:docker", []string{"ARCH=amd64"}, []string{"--platform linux/amd64"}, ""},
	} {
		t.Run(test.task+strings.Join(test.args, ","), func(t *testing.T) {
			args := append([]string{"task", "-dry", test.task}, test.args...)
			cmd := exec.Command(cli, args...)
			cmd.Env = append(os.Environ(), "PATH="+fixture+string(os.PathListSeparator)+os.Getenv("PATH"), "BISTRO_VERSION=0.2.0-alpha.2", "GITHUB_CLIENT_ID=", "SENTRY_DSN=")
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("dry run: %v\n%s", err, output)
			}
			if test.wantAbsent != "" && strings.Contains(string(output), test.wantAbsent) {
				t.Fatalf("unexpected %q in resolved task\n%s", test.wantAbsent, output)
			}
			for _, want := range test.want {
				if !strings.Contains(string(output), want) {
					t.Fatalf("missing %q in resolved task\n%s", want, output)
				}
			}
		})
	}
}

func TestSmokeRejectsWindowsTimeoutAndSelectsGNU(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires Unix")
	}
	fixture := t.TempDir()
	trace := filepath.Join(fixture, "trace")
	for name, script := range map[string]string{
		"timeout":  "#!/bin/sh\nif [ \"$1\" = --version ]; then echo Windows; exit 1; fi\necho wrong >> \"$TRACE\"\nexit 1\n",
		"gtimeout": "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'timeout (GNU coreutils)'; exit 0; fi\necho gnu >> \"$TRACE\"\nshift\nexec \"$@\"\n",
		"uname":    "#!/bin/sh\necho Darwin\n",
		"bistro":   "#!/bin/sh\n[ \"$1\" = --smoke ]\n",
	} {
		if err := os.WriteFile(filepath.Join(fixture, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "scripts/smoke.sh", filepath.Join(fixture, "bistro"))
	cmd.Env = append(os.Environ(), "PATH="+fixture+string(os.PathListSeparator)+os.Getenv("PATH"), "TRACE="+trace)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("smoke: %v\n%s", err, output)
	}
	got, err := os.ReadFile(trace)
	if err != nil || string(got) != "gnu\n" {
		t.Fatalf("runner = %q, %v", got, err)
	}
}
