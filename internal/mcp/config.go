package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultConfigFile = "mcp.json"

type Config struct {
	Servers map[string]ServerConfig
	Path    string
}

type ServerConfig struct {
	Name          string            `json:"-"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Disabled      bool              `json:"disabled,omitempty"`
	DisabledTools []string          `json:"disabled_tools,omitempty"`
	Timeout       int               `json:"timeout,omitempty"`
}

func DefaultConfigPath(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		return DefaultConfigFile
	}
	return filepath.Join(dataDir, DefaultConfigFile)
}

func LoadConfig(path string) (Config, error) {
	path = strings.TrimSpace(path)
	cfg := Config{Servers: map[string]ServerConfig{}, Path: path}
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	var raw struct {
		Servers    map[string]ServerConfig `json:"servers"`
		MCPServers map[string]ServerConfig `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return cfg, fmt.Errorf("parse mcp config: %w", err)
	}
	mergeServers(cfg.Servers, raw.Servers)
	mergeServers(cfg.Servers, raw.MCPServers)
	return cfg, nil
}

func mergeServers(dst map[string]ServerConfig, src map[string]ServerConfig) {
	for name, srv := range src {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		srv.Name = name
		dst[name] = srv
	}
}

func (s ServerConfig) TimeoutDuration() time.Duration {
	if s.Timeout <= 0 {
		return 15 * time.Second
	}
	return time.Duration(s.Timeout) * time.Second
}

func (s ServerConfig) disabledToolSet() map[string]bool {
	out := make(map[string]bool, len(s.DisabledTools))
	for _, name := range s.DisabledTools {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = true
		}
	}
	return out
}
