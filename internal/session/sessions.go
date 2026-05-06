package session

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SessionSummary struct {
	ID      string
	ModTime time.Time
	Size    int64
	Meta    SessionMeta
}

func ListSessions(sessionsDir string, limit int) ([]SessionSummary, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]SessionSummary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		if id == "" {
			continue
		}
		out = append(out, SessionSummary{
			ID:      id,
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
	}
	for i := range out {
		meta, err := LoadSessionMeta(sessionsDir, out[i].ID)
		if err == nil {
			out[i].Meta = meta
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func FindSessionPathByID(sessionsDir, sessionID string) string {
	id := sanitizeSessionID(sessionID)
	return filepath.Join(sessionsDir, id+".jsonl")
}

func sanitizeSessionID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "default"
	}
	v = strings.ReplaceAll(v, "/", "_")
	v = strings.ReplaceAll(v, "\\", "_")
	return v
}
