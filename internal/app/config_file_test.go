package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := GlobalConfigPath(dir)
	enabled := true
	cfg := FileConfig{
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "max",
		ThinkingEnabled: &enabled,
		Permissions: FilePermissionsConfig{
			Mode:               "never",
			AllowShellPrefixes: []string{"git status", "go test"},
		},
	}
	if err := SaveConfigFile(path, cfg); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	if !strings.Contains(string(raw), `model = "deepseek-v4-pro"`) {
		t.Fatalf("unexpected config TOML:\n%s", raw)
	}
	if !strings.Contains(string(raw), "[permissions]") || strings.Contains(string(raw), "allow_prefixes") {
		t.Fatalf("expected grouped config TOML, got:\n%s", raw)
	}

	loaded, ok, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if !ok {
		t.Fatal("expected config file to be loaded")
	}
	if loaded.Model != "deepseek-v4-pro" || loaded.ReasoningEffort != "max" {
		t.Fatalf("loaded config: %+v", loaded)
	}
	if loaded.ThinkingEnabled == nil || !*loaded.ThinkingEnabled {
		t.Fatal("thinking_enabled: want true")
	}
	if loaded.Permissions.Mode != "never" || len(loaded.Permissions.AllowShellPrefixes) != 2 {
		t.Fatalf("permissions config: %+v", loaded.Permissions)
	}
}

func TestConfigNewAppLoadsGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	enabled := false
	if err := SaveConfigFile(GlobalConfigPath(dir), FileConfig{
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "max",
		ThinkingEnabled: &enabled,
	}); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}

	cfg := DefaultConfig()
	cfg.DataDir = dir

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if app.Model() != "deepseek-v4-pro" {
		t.Fatalf("model: want deepseek-v4-pro from config, got %s", app.Model())
	}
	if app.ReasoningEffort() != "max" {
		t.Fatalf("effort: want max from config, got %s", app.ReasoningEffort())
	}
	if app.ThinkingEnabled() {
		t.Fatal("thinking: want false from config")
	}
}

func TestConfigProjectOverridesGlobal(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".whale"), 0o755); err != nil {
		t.Fatalf("mkdir .whale: %v", err)
	}
	if err := SaveConfigFile(GlobalConfigPath(dataDir), FileConfig{Model: "deepseek-v4-flash"}); err != nil {
		t.Fatalf("save global: %v", err)
	}
	if err := SaveConfigFile(ProjectConfigPath(workspace), FileConfig{Model: "deepseek-v4-pro"}); err != nil {
		t.Fatalf("save project: %v", err)
	}

	cfg := DefaultConfig()
	cfg.DataDir = dataDir
	loaded, err := LoadAndApplyConfig(cfg, workspace)
	if err != nil {
		t.Fatalf("LoadAndApplyConfig: %v", err)
	}
	if loaded.Model != "deepseek-v4-pro" {
		t.Fatalf("model: want project override, got %s", loaded.Model)
	}
}

func TestApplyFileConfigUsesGroupedConfig(t *testing.T) {
	autoCompact := false
	compactThreshold := 0.7
	contextWindow := 256000
	projectDocEnabled := false
	projectDocMaxBytes := 12000
	budgetLimit := 1.25
	cfg := DefaultConfig()
	ApplyFileConfig(&cfg, FileConfig{
		Permissions: FilePermissionsConfig{
			Mode:               "never",
			AllowShellPrefixes: []string{"git status"},
			DenyShellPrefixes:  []string{"rm -rf"},
		},
		Budget: FileBudgetConfig{SessionLimitUSD: &budgetLimit},
		MCP:    FileMCPConfig{ConfigPath: "~/custom-mcp.json"},
		Context: FileContextConfig{
			AutoCompact:        &autoCompact,
			CompactThreshold:   &compactThreshold,
			ModelContextWindow: &contextWindow,
		},
		ProjectDoc: FileProjectDocConfig{
			Enabled:           &projectDocEnabled,
			MaxBytes:          &projectDocMaxBytes,
			FallbackFilenames: []string{"AGENTS.md", "TEAM.md"},
		},
	})

	if cfg.ApprovalMode != "never" || cfg.AllowPrefixes != "git status" || cfg.DenyPrefixes != "rm -rf" {
		t.Fatalf("permissions not applied: %+v", cfg)
	}
	if cfg.BudgetWarningUSD != budgetLimit {
		t.Fatalf("budget not applied: %+v", cfg)
	}
	if !strings.HasSuffix(cfg.MCPConfigPath, "custom-mcp.json") {
		t.Fatalf("mcp path not applied: %s", cfg.MCPConfigPath)
	}
	if cfg.AutoCompact || cfg.AutoCompactThreshold != compactThreshold || cfg.ContextWindow != contextWindow {
		t.Fatalf("context not applied: %+v", cfg)
	}
	if cfg.MemoryEnabled || cfg.MemoryMaxChars != projectDocMaxBytes || cfg.MemoryFileOrder != "AGENTS.md,TEAM.md" {
		t.Fatalf("project doc not applied: %+v", cfg)
	}
}

func TestConfigExplicitModelOverridesFileConfig(t *testing.T) {
	dir := t.TempDir()
	if err := SaveConfigFile(GlobalConfigPath(dir), FileConfig{Model: "deepseek-v4-pro"}); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}

	cfg := DefaultConfig()
	cfg.DataDir = dir
	cfg.Model = "deepseek-v4-flash"
	cfg.ModelExplicit = true

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if app.Model() != "deepseek-v4-flash" {
		t.Fatalf("model: want explicit deepseek-v4-flash, got %s", app.Model())
	}
}

func TestSetModelAndThinkingPersistToConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.DataDir = dir

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := app.SetModelAndEffort("deepseek-v4-pro", "max"); err != nil {
		t.Fatalf("SetModelAndEffort: %v", err)
	}
	app.SetThinkingEnabled(false)

	loaded, ok, err := LoadConfigFile(GlobalConfigPath(dir))
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if !ok {
		t.Fatal("expected config.toml to be written")
	}
	if loaded.Model != "deepseek-v4-pro" || loaded.ReasoningEffort != "max" {
		t.Fatalf("persisted config: %+v", loaded)
	}
	if loaded.ThinkingEnabled == nil || *loaded.ThinkingEnabled {
		t.Fatal("persisted thinking_enabled: want false")
	}
	if _, err := os.Stat(preferencesPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("preferences.json should not be created, err=%v", err)
	}
}
