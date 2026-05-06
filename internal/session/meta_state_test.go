package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionMetaPatchAndLoad(t *testing.T) {
	dir := t.TempDir()
	_, err := PatchSessionMeta(dir, "s1", SessionMeta{
		Workspace: "/tmp/work",
		Branch:    "main",
		TurnCount: 2,
		Summary:   "hello",
	})
	if err != nil {
		t.Fatalf("patch meta: %v", err)
	}
	got, err := LoadSessionMeta(dir, "s1")
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if got.Workspace != "/tmp/work" || got.Branch != "main" || got.TurnCount != 2 || got.Summary != "hello" {
		t.Fatalf("unexpected meta: %+v", got)
	}
}

func TestListSessionsIncludesMeta(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSessionMeta(dir, "s1", SessionMeta{Branch: "dev", TurnCount: 3}); err != nil {
		t.Fatalf("save meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}
	out, err := ListSessions(dir, 10)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 session, got %d", len(out))
	}
	if out[0].Meta.Branch != "dev" || out[0].Meta.TurnCount != 3 {
		t.Fatalf("unexpected meta: %+v", out[0].Meta)
	}
}
