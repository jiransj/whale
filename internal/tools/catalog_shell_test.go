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

func TestShellReadOnlyCheckForGOOSUnixCommands(t *testing.T) {
	check := shellReadOnlyCheckForGOOS("linux")

	for _, command := range []string{
		"ls -la",
		"cat a.txt",
		"git diff --stat",
	} {
		if !check(map[string]any{"command": command}) {
			t.Fatalf("expected %q to be read-only on unix", command)
		}
	}

	for _, command := range []string{
		"dir",
		"cat a.txt > b.txt",
		"pwd; touch x",
		"git diff | tee out.txt",
	} {
		if check(map[string]any{"command": command}) {
			t.Fatalf("did not expect %q to be read-only on unix", command)
		}
	}
}

func TestShellReadOnlyCheckForGOOSWindowsCmdCommands(t *testing.T) {
	check := shellReadOnlyCheckForGOOS("windows")

	for _, command := range []string{
		"dir",
		"type file.txt",
		"where git",
		`findstr "foo|bar" file.txt`,
		"git status --short",
	} {
		if !check(map[string]any{"command": command}) {
			t.Fatalf("expected %q to be read-only on windows", command)
		}
	}
}

func TestShellReadOnlyCheckForGOOSWindowsPowerShellCommands(t *testing.T) {
	check := shellReadOnlyCheckForGOOS("windows")

	for _, command := range []string{
		"Get-ChildItem",
		"Get-Content file.txt",
		"Get-Location",
		"Select-String foo file.txt",
		`Select-String "foo|bar" file.txt`,
		"Get-Command git",
	} {
		if !check(map[string]any{"command": command}) {
			t.Fatalf("expected %q to be read-only on windows", command)
		}
	}
}

func TestShellReadOnlyCheckForGOOSBlocksUnsafeCompositions(t *testing.T) {
	tests := []struct {
		goos    string
		command string
	}{
		{goos: "windows", command: "dir | Remove-Item file"},
		{goos: "windows", command: "cd subdir; Remove-Item file"},
		{goos: "windows", command: "type file.txt > out.txt"},
		{goos: "windows", command: "echo x > out.txt"},
		{goos: "windows", command: "echo hello"},
		{goos: "linux", command: "cat a.txt > b.txt"},
		{goos: "linux", command: "pwd; touch x"},
		{goos: "linux", command: "git diff | tee out.txt"},
	}

	for _, tt := range tests {
		check := shellReadOnlyCheckForGOOS(tt.goos)
		if check(map[string]any{"command": tt.command}) {
			t.Fatalf("did not expect %q to be read-only on %s", tt.command, tt.goos)
		}
	}
}
