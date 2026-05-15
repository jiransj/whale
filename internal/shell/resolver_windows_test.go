//go:build windows

package shell

import (
	"os/exec"
	"strings"
	"testing"
)

func TestWindowsResolveRunsCommand(t *testing.T) {
	const marker = "whale_windows_shell_resolver"

	spec, err := Resolve("echo " + marker)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if spec.Kind != KindPowerShell && spec.Kind != KindCmd {
		t.Fatalf("Kind = %q, want %q or %q", spec.Kind, KindPowerShell, KindCmd)
	}

	out, err := exec.Command(spec.Bin, spec.Args...).CombinedOutput()
	if err != nil {
		t.Fatalf("resolved shell command failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(string(out), marker) {
		t.Fatalf("output %q does not contain marker %q", string(out), marker)
	}
}
