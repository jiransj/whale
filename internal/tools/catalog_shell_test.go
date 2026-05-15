package tools

import (
	"strings"
	"testing"
)

func TestShellRunDescriptionMatchesGOOS(t *testing.T) {
	unix := shellRunDescription("linux")
	if !strings.Contains(unix, "shell_run") || !strings.Contains(unix, "/bin/sh") {
		t.Fatalf("unix shell_run description missing expected wording: %s", unix)
	}
	if strings.Contains(unix, "exec"+"_shell") {
		t.Fatalf("unix shell_run description contains stale tool name: %s", unix)
	}

	windows := shellRunDescription("windows")
	for _, want := range []string{"shell_run", "pwsh", "ComSpec", "cmd.exe"} {
		if !strings.Contains(windows, want) {
			t.Fatalf("windows shell_run description missing %q: %s", want, windows)
		}
	}
	if strings.Contains(windows, "always "+"PowerShell") || strings.Contains(windows, "exec"+"_shell") {
		t.Fatalf("windows shell_run description contains stale wording: %s", windows)
	}
}

func TestShellReadOnlyCheckForGOOS(t *testing.T) {
	unix := shellReadOnlyCheckForGOOS("linux")
	if !unix(map[string]any{"command": "ls -la"}) {
		t.Fatal("expected ls to be read-only on unix")
	}
	if unix(map[string]any{"command": "dir"}) {
		t.Fatal("did not expect dir to be read-only on unix")
	}

	windows := shellReadOnlyCheckForGOOS("windows")
	if !windows(map[string]any{"command": "dir"}) {
		t.Fatal("expected dir to be read-only on windows")
	}
	if !windows(map[string]any{"command": "git status --short"}) {
		t.Fatal("expected git status to be read-only on windows")
	}
	if windows(map[string]any{"command": "rm -rf /tmp/x"}) {
		t.Fatal("did not expect mutating command to be read-only on windows")
	}
}
