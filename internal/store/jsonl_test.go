package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMostRecentSessionIDIgnoresToolInputEventSidecars(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "s1.jsonl")
	sidecarPath := filepath.Join(dir, "s1.tool_input_events.jsonl")
	if err := os.WriteFile(sessionPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}
	if err := os.WriteFile(sidecarPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	now := time.Now()
	_ = os.Chtimes(sessionPath, now.Add(-time.Hour), now.Add(-time.Hour))
	_ = os.Chtimes(sidecarPath, now, now)

	got, err := MostRecentSessionID(dir)
	if err != nil {
		t.Fatalf("most recent session: %v", err)
	}
	if got != "s1" {
		t.Fatalf("expected s1, got %q", got)
	}
}
