package session

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type SessionMeta struct {
	Branch       string    `json:"branch,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	TotalCostUSD float64   `json:"total_cost_usd,omitempty"`
	TurnCount    int       `json:"turn_count,omitempty"`
	Workspace    string    `json:"workspace,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func metaStatePath(sessionsDir, sessionID string) string {
	return filepath.Join(sessionsDir, sanitizeSessionID(sessionID)+".meta.json")
}

func LoadSessionMeta(sessionsDir, sessionID string) (SessionMeta, error) {
	path := metaStatePath(sessionsDir, sessionID)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionMeta{}, nil
		}
		return SessionMeta{}, fmt.Errorf("read session meta: %w", err)
	}
	var st SessionMeta
	if err := json.Unmarshal(b, &st); err != nil {
		return SessionMeta{}, fmt.Errorf("unmarshal session meta: %w", err)
	}
	return st, nil
}

func SaveSessionMeta(sessionsDir, sessionID string, st SessionMeta) error {
	st.UpdatedAt = time.Now()
	b, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal session meta: %w", err)
	}
	path := metaStatePath(sessionsDir, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir session meta dir: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write session meta: %w", err)
	}
	return nil
}

func PatchSessionMeta(sessionsDir, sessionID string, patch SessionMeta) (SessionMeta, error) {
	cur, err := LoadSessionMeta(sessionsDir, sessionID)
	if err != nil {
		return SessionMeta{}, err
	}
	if strings.TrimSpace(patch.Branch) != "" {
		cur.Branch = strings.TrimSpace(patch.Branch)
	}
	if strings.TrimSpace(patch.Summary) != "" {
		cur.Summary = strings.TrimSpace(patch.Summary)
	}
	if patch.TotalCostUSD != 0 {
		cur.TotalCostUSD = patch.TotalCostUSD
	}
	if patch.TurnCount != 0 {
		cur.TurnCount = patch.TurnCount
	}
	if strings.TrimSpace(patch.Workspace) != "" {
		cur.Workspace = strings.TrimSpace(patch.Workspace)
	}
	if err := SaveSessionMeta(sessionsDir, sessionID, cur); err != nil {
		return SessionMeta{}, err
	}
	return cur, nil
}

func DetectGitBranch(cwd string) string {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
