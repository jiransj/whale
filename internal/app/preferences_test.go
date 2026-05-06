package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Save preferences.
	enabled := true
	prefs := Preferences{
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "max",
		ThinkingEnabled: &enabled,
	}
	if err := SavePreferences(dir, prefs); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}

	// Verify file exists and is valid JSON.
	path := preferencesPath(dir)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("preferences.json not created: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read preferences.json: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Load and verify.
	loaded, err := LoadPreferences(dir)
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
	if loaded.Model != "deepseek-v4-pro" {
		t.Fatalf("model: want deepseek-v4-pro, got %s", loaded.Model)
	}
	if loaded.ReasoningEffort != "max" {
		t.Fatalf("reasoning_effort: want max, got %s", loaded.ReasoningEffort)
	}
	if loaded.ThinkingEnabled == nil || !*loaded.ThinkingEnabled {
		t.Fatal("thinking_enabled: want true")
	}
}

func TestLoadPreferencesMissingFile(t *testing.T) {
	dir := t.TempDir()
	prefs, err := LoadPreferences(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prefs.Model != "" {
		t.Fatalf("expected empty model, got %s", prefs.Model)
	}
	if prefs.ThinkingEnabled != nil {
		t.Fatal("expected nil ThinkingEnabled")
	}
}

func TestPreferencesNewAppPrefsOverrideDefaults(t *testing.T) {
	dir := t.TempDir()

	// Write preferences to disk.
	enabled := false
	prefs := Preferences{
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "max",
		ThinkingEnabled: &enabled,
	}
	if err := SavePreferences(dir, prefs); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}

	// Create sessions dir so New doesn't fail.
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	// Use default config — preferences should override.
	cfg := DefaultConfig()
	cfg.DataDir = dir

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if app.Model() != "deepseek-v4-pro" {
		t.Fatalf("model: want deepseek-v4-pro from prefs, got %s", app.Model())
	}
	if app.ReasoningEffort() != "max" {
		t.Fatalf("effort: want max from prefs, got %s", app.ReasoningEffort())
	}
	if app.ThinkingEnabled() {
		t.Fatal("thinking: want false from prefs, got true")
	}
}

func TestPreferencesCLIFlagOverridesPrefs(t *testing.T) {
	dir := t.TempDir()

	// Write preferences with non-default values.
	enabled := false
	prefs := Preferences{
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "max",
		ThinkingEnabled: &enabled,
	}
	if err := SavePreferences(dir, prefs); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}

	// Create sessions dir.
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	// When Config has non-default model (different from "deepseek-v4-flash"),
	// the Config value takes priority over preferences.
	cfg := DefaultConfig()
	cfg.DataDir = dir
	cfg.Model = "deepseek-v4-pro" // explicit CLI flag (non-default)
	cfg.ModelExplicit = true

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Config explicitly set to "deepseek-v4-pro" which differs from default,
	// so the Config value wins, even though prefs also say "deepseek-v4-pro".
	if app.Model() != "deepseek-v4-pro" {
		t.Fatalf("model: want deepseek-v4-pro from CLI, got %s", app.Model())
	}
	// Effort uses cfg default "high" since cfg didn't override it,
	// and preferences has "max" which is different from default "high",
	// so preferences override the default.
	if app.ReasoningEffort() != "max" {
		t.Fatalf("effort: want max from prefs, got %s", app.ReasoningEffort())
	}
}

func TestPreferencesExplicitDefaultModelOverridesPrefs(t *testing.T) {
	dir := t.TempDir()

	enabled := false
	prefs := Preferences{
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "max",
		ThinkingEnabled: &enabled,
	}
	if err := SavePreferences(dir, prefs); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
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

func TestSetModelAndEffortPersists(t *testing.T) {
	dir := t.TempDir()

	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	cfg := DefaultConfig()
	cfg.DataDir = dir

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Change model.
	if err := app.SetModelAndEffort("deepseek-v4-pro", "max"); err != nil {
		t.Fatalf("SetModelAndEffort: %v", err)
	}

	// Load preferences from disk to verify persistence.
	loaded, err := LoadPreferences(dir)
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
	if loaded.Model != "deepseek-v4-pro" {
		t.Fatalf("persisted model: want deepseek-v4-pro, got %s", loaded.Model)
	}
	if loaded.ReasoningEffort != "max" {
		t.Fatalf("persisted effort: want max, got %s", loaded.ReasoningEffort)
	}
}

func TestSetThinkingEnabledPersists(t *testing.T) {
	dir := t.TempDir()

	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	cfg := DefaultConfig()
	cfg.DataDir = dir
	cfg.ThinkingEnabled = true

	app, err := New(t.Context(), cfg, StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Toggle thinking off.
	app.SetThinkingEnabled(false)

	// Load preferences from disk.
	loaded, err := LoadPreferences(dir)
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
	if loaded.ThinkingEnabled == nil || *loaded.ThinkingEnabled {
		t.Fatal("persisted thinking_enabled: want false")
	}

	// Toggle thinking on.
	app.SetThinkingEnabled(true)
	loaded, err = LoadPreferences(dir)
	if err != nil {
		t.Fatalf("LoadPreferences: %v", err)
	}
	if loaded.ThinkingEnabled == nil || !*loaded.ThinkingEnabled {
		t.Fatal("persisted thinking_enabled: want true")
	}
}
