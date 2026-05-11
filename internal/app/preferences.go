package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Preferences is the legacy preferences.json shape used only by migrate-config.
type Preferences struct {
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	ThinkingEnabled *bool  `json:"thinking_enabled,omitempty"`
}

func preferencesPath(dataDir string) string {
	return filepath.Join(dataDir, "preferences.json")
}

// LoadPreferences reads the legacy preferences file for migration tests/helpers.
func LoadPreferences(dataDir string) (Preferences, error) {
	path := preferencesPath(dataDir)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Preferences{}, nil
		}
		return Preferences{}, fmt.Errorf("read preferences: %w", err)
	}
	var prefs Preferences
	if err := json.Unmarshal(b, &prefs); err != nil {
		return Preferences{}, fmt.Errorf("unmarshal preferences: %w", err)
	}
	return prefs, nil
}

// SavePreferences writes the legacy preferences file for migration tests/helpers.
func SavePreferences(dataDir string, prefs Preferences) error {
	b, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("marshal preferences: %w", err)
	}
	path := preferencesPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir preferences dir: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write preferences: %w", err)
	}
	return nil
}
