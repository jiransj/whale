package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/usewhale/whale/internal/agent"
)

type MigrateConfigReport struct {
	Written []string
	Skipped []string
}

func MigrateConfig(dataDir, workspaceRoot string) (MigrateConfigReport, error) {
	var report MigrateConfigReport
	if strings.TrimSpace(dataDir) == "" {
		dataDir = GlobalConfigPath("")
		dataDir = filepath.Dir(dataDir)
	}
	globalPath := GlobalConfigPath(dataDir)
	globalCfg, _, err := LoadConfigFile(globalPath)
	if err != nil {
		return report, err
	}
	changedGlobal := false

	prefs, prefsLoaded, err := loadLegacyPreferences(dataDir)
	if err != nil {
		return report, err
	}
	if prefsLoaded {
		changedGlobal = mergeLegacyPreferences(&globalCfg, prefs) || changedGlobal
		report.Skipped = append(report.Skipped, preferencesPath(dataDir)+" (obsolete)")
	}

	globalSettings := filepath.Join(dataDir, "settings.json")
	globalHooks, globalHooksLoaded, err := loadLegacyHookSettings(globalSettings)
	if err != nil {
		return report, err
	}
	if globalHooksLoaded {
		changedGlobal = mergeHooks(&globalCfg, globalHooks) || changedGlobal
		report.Skipped = append(report.Skipped, globalSettings+" (obsolete)")
	}
	if changedGlobal {
		if err := SaveConfigFile(globalPath, globalCfg); err != nil {
			return report, err
		}
		report.Written = append(report.Written, globalPath)
	}

	if strings.TrimSpace(workspaceRoot) != "" {
		projectSettings := filepath.Join(workspaceRoot, ".whale", "settings.json")
		projectHooks, projectLoaded, err := loadLegacyHookSettings(projectSettings)
		if err != nil {
			return report, err
		}
		if projectLoaded {
			projectPath := ProjectConfigPath(workspaceRoot)
			projectCfg, _, err := LoadConfigFile(projectPath)
			if err != nil {
				return report, err
			}
			if mergeHooks(&projectCfg, projectHooks) {
				if err := SaveConfigFile(projectPath, projectCfg); err != nil {
					return report, err
				}
				report.Written = append(report.Written, projectPath)
			}
			report.Skipped = append(report.Skipped, projectSettings+" (obsolete)")
		}
	}

	return report, nil
}

func loadLegacyPreferences(dataDir string) (Preferences, bool, error) {
	path := preferencesPath(dataDir)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Preferences{}, false, nil
		}
		return Preferences{}, true, fmt.Errorf("read legacy preferences: %w", err)
	}
	var prefs Preferences
	if err := json.Unmarshal(b, &prefs); err != nil {
		return Preferences{}, true, fmt.Errorf("parse legacy preferences: %w", err)
	}
	return prefs, true, nil
}

func loadLegacyHookSettings(path string) (agent.HookSettings, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return agent.HookSettings{}, false, nil
		}
		return agent.HookSettings{}, true, fmt.Errorf("read legacy hooks: %w", err)
	}
	var st agent.HookSettings
	if err := json.Unmarshal(b, &st); err != nil {
		return agent.HookSettings{}, true, fmt.Errorf("parse legacy hooks: %w", err)
	}
	return st, true, nil
}

func mergeLegacyPreferences(dst *FileConfig, prefs Preferences) bool {
	changed := false
	if strings.TrimSpace(dst.Model) == "" && strings.TrimSpace(prefs.Model) != "" {
		dst.Model = strings.TrimSpace(prefs.Model)
		changed = true
	}
	if strings.TrimSpace(dst.ReasoningEffort) == "" && strings.TrimSpace(prefs.ReasoningEffort) != "" {
		dst.ReasoningEffort = strings.TrimSpace(prefs.ReasoningEffort)
		changed = true
	}
	if dst.ThinkingEnabled == nil && prefs.ThinkingEnabled != nil {
		dst.ThinkingEnabled = prefs.ThinkingEnabled
		changed = true
	}
	return changed
}

func mergeHooks(dst *FileConfig, st agent.HookSettings) bool {
	changed := false
	if dst.Hooks == nil {
		dst.Hooks = map[string][]agent.HookConfig{}
	}
	for ev, hooks := range st.Hooks {
		key := string(ev)
		for _, h := range hooks {
			if strings.TrimSpace(h.Command) == "" {
				continue
			}
			if hookExists(dst.Hooks[key], h) {
				continue
			}
			dst.Hooks[key] = append(dst.Hooks[key], h)
			changed = true
		}
	}
	return changed
}

func hookExists(list []agent.HookConfig, h agent.HookConfig) bool {
	for _, existing := range list {
		if existing.Match == h.Match &&
			existing.Command == h.Command &&
			existing.Description == h.Description &&
			existing.TimeoutMS == h.TimeoutMS &&
			existing.CWD == h.CWD {
			return true
		}
	}
	return false
}
