package agent

import (
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/core"
)

func TestRuntimeEnvironmentBlockIncludesWorkspaceAndShellRunCWD(t *testing.T) {
	block := renderRuntimeEnvironmentBlock("linux", "/repo")

	for _, want := range []string{
		"Runtime environment.",
		"OS: linux",
		"Current Whale workspace root: /repo",
		"shell_run commands run from the workspace root by default",
		"shell_run cwd parameter",
		"/bin/sh",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("runtime block missing %q:\n%s", want, block)
		}
	}
}

func TestRuntimeEnvironmentBlockWindowsUsesCurrentShellRunName(t *testing.T) {
	block := renderRuntimeEnvironmentBlock("windows", `C:\repo`)

	for _, want := range []string{
		"OS: windows",
		`shell_run commands run from the workspace root by default`,
		"prefers pwsh when available",
		"falls back to ComSpec or cmd.exe",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("windows runtime block missing %q:\n%s", want, block)
		}
	}
	for _, old := range []string{"exec" + "_shell", "always " + "PowerShell"} {
		if strings.Contains(block, old) {
			t.Fatalf("windows runtime block contains stale wording %q:\n%s", old, block)
		}
	}
}

func TestImmutableSystemBlocksIncludeRuntimeEnvironment(t *testing.T) {
	a := NewAgentWithRegistry(nil, nil, core.NewToolRegistry(nil), WithProjectMemory(false, 0, nil, "/repo"))
	joined := strings.Join(a.buildImmutableSystemBlocks(), "\n\n")

	if !strings.Contains(joined, "Runtime environment.") {
		t.Fatalf("system blocks missing runtime environment:\n%s", joined)
	}
	if !strings.Contains(joined, "Current Whale workspace root: /repo") {
		t.Fatalf("system blocks missing workspace root:\n%s", joined)
	}
	if !strings.Contains(joined, "shell_run cwd parameter") {
		t.Fatalf("system blocks missing shell_run cwd guidance:\n%s", joined)
	}
}
