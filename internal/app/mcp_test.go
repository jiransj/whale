package app

import (
	"strings"
	"testing"
)

func TestHandleLocalCommandMCPShowsEmptyStatus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DataDir = t.TempDir()
	a, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	handled, out, err := a.HandleLocalCommand("/mcp")
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("expected /mcp handled")
	}
	if !strings.Contains(out, "MCP") || !strings.Contains(out, "servers: none") {
		t.Fatalf("output:\n%s", out)
	}
}
