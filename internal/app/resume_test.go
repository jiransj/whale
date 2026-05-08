package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/usewhale/whale/internal/session"
)

func TestListResumeChoicesShowsReadableConversationTable(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	if err := session.SaveSessionMeta(sessionsDir, "s1", session.SessionMeta{Title: "Saved title", Branch: "main"}); err != nil {
		t.Fatalf("save s1 meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "s1.jsonl"), []byte("{\"Role\":\"user\",\"Text\":\"fallback\"}\n"), 0o600); err != nil {
		t.Fatalf("write s1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "s2.jsonl"), []byte("{\"Role\":\"user\",\"Text\":\"first prompt\"}\n"), 0o600); err != nil {
		t.Fatalf("write s2: %v", err)
	}
	now := time.Now()
	_ = os.Chtimes(filepath.Join(sessionsDir, "s1.jsonl"), now.Add(-2*time.Minute), now.Add(-2*time.Minute))
	_ = os.Chtimes(filepath.Join(sessionsDir, "s2.jsonl"), now.Add(-time.Minute), now.Add(-time.Minute))

	app := &App{
		sessionsDir: sessionsDir,
		sessionID:   "s2",
	}
	out, err := app.ListResumeChoices(10)
	if err != nil {
		t.Fatalf("list resume choices: %v", err)
	}
	rendered := strings.Join(out, "\n")
	if !strings.Contains(rendered, "Updated") || !strings.Contains(rendered, "Branch") || !strings.Contains(rendered, "Conversation") {
		t.Fatalf("expected readable table headers, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "*  1)") || !strings.Contains(rendered, "first prompt") {
		t.Fatalf("expected current latest session with fallback title, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Saved title") || !strings.Contains(rendered, "main") {
		t.Fatalf("expected saved title and branch, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, " - ") {
		t.Fatalf("expected empty branch placeholder, got:\n%s", rendered)
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("abcdef", 4); got != "abc…" {
		t.Fatalf("unexpected ascii truncation: %q", got)
	}
	if got := truncateRunes("中文标题", 3); got != "中文…" {
		t.Fatalf("unexpected unicode truncation: %q", got)
	}
}
