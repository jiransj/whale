package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/usewhale/whale/internal/app"
)

func TestConfigServiceDispatchPersists(t *testing.T) {
	dir := t.TempDir()

	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	cfg := app.DefaultConfig()
	cfg.DataDir = dir

	svc, err := New(t.Context(), cfg, app.StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	svc.Dispatch(Intent{
		Kind:     IntentSetModelAndEffort,
		Model:    "deepseek-v4-pro",
		Effort:   "max",
		Thinking: "off",
	})

	loaded, ok, err := app.LoadConfigFile(app.GlobalConfigPath(dir))
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if !ok {
		t.Fatal("expected config.toml to be written")
	}
	if loaded.Model != "deepseek-v4-pro" {
		t.Fatalf("persisted model: want deepseek-v4-pro, got %s", loaded.Model)
	}
	if loaded.ReasoningEffort != "max" {
		t.Fatalf("persisted effort: want max, got %s", loaded.ReasoningEffort)
	}
	if loaded.ThinkingEnabled == nil || *loaded.ThinkingEnabled {
		t.Fatal("persisted thinking_enabled: want false")
	}
}
