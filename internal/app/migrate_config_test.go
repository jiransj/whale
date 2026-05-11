package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateConfigConvertsLegacyPreferencesAndHooks(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".whale"), 0o755); err != nil {
		t.Fatalf("mkdir .whale: %v", err)
	}
	thinking := false
	if err := SavePreferences(dataDir, Preferences{
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "max",
		ThinkingEnabled: &thinking,
	}); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "settings.json"), []byte(`{"hooks":{"Stop":[{"command":"echo global"}]}}`), 0o600); err != nil {
		t.Fatalf("write global settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".whale", "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"command":"echo project"}]}}`), 0o600); err != nil {
		t.Fatalf("write project settings: %v", err)
	}

	report, err := MigrateConfig(dataDir, workspace)
	if err != nil {
		t.Fatalf("MigrateConfig: %v", err)
	}
	if len(report.Written) != 2 {
		t.Fatalf("written files: %+v", report.Written)
	}
	global, ok, err := LoadConfigFile(GlobalConfigPath(dataDir))
	if err != nil || !ok {
		t.Fatalf("load global config ok=%v err=%v", ok, err)
	}
	if global.Model != "deepseek-v4-pro" || global.ReasoningEffort != "max" {
		t.Fatalf("global config: %+v", global)
	}
	if global.ThinkingEnabled == nil || *global.ThinkingEnabled {
		t.Fatal("global thinking_enabled: want false")
	}
	if len(global.Hooks["Stop"]) != 1 {
		t.Fatalf("global hooks: %+v", global.Hooks)
	}
	project, ok, err := LoadConfigFile(ProjectConfigPath(workspace))
	if err != nil || !ok {
		t.Fatalf("load project config ok=%v err=%v", ok, err)
	}
	if len(project.Hooks["PreToolUse"]) != 1 {
		t.Fatalf("project hooks: %+v", project.Hooks)
	}
}

func TestMigrateConfigDoesNotOverwriteExistingConfig(t *testing.T) {
	dataDir := t.TempDir()
	if err := SaveConfigFile(GlobalConfigPath(dataDir), FileConfig{Model: "deepseek-v4-flash"}); err != nil {
		t.Fatalf("SaveConfigFile: %v", err)
	}
	if err := SavePreferences(dataDir, Preferences{Model: "deepseek-v4-pro"}); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}

	if _, err := MigrateConfig(dataDir, ""); err != nil {
		t.Fatalf("MigrateConfig: %v", err)
	}
	global, ok, err := LoadConfigFile(GlobalConfigPath(dataDir))
	if err != nil || !ok {
		t.Fatalf("load global ok=%v err=%v", ok, err)
	}
	if global.Model != "deepseek-v4-flash" {
		t.Fatalf("model should not be overwritten, got %s", global.Model)
	}
}
