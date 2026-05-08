package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigMissingFileIsEmpty(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("servers: %+v", cfg.Servers)
	}
}

func TestLoadConfigSupportsServersAndMCPServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(`{
		"servers": {"fs": {"command": "node", "args": ["server.js"], "disabled_tools": ["write"]}},
		"mcpServers": {"mem": {"command": "memory"}}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != path {
		t.Fatalf("path: %s", cfg.Path)
	}
	if cfg.Servers["fs"].Name != "fs" || cfg.Servers["mem"].Name != "mem" {
		t.Fatalf("servers: %+v", cfg.Servers)
	}
	if cfg.Servers["fs"].DisabledTools[0] != "write" {
		t.Fatalf("disabled tools: %+v", cfg.Servers["fs"].DisabledTools)
	}
}
