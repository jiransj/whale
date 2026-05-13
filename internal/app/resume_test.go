package app

import (
	"context"
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

func TestApplyResumeChoiceBlocksCrossWorkspace(t *testing.T) {
	current := t.TempDir()
	other := t.TempDir()
	sessionsDir := filepath.Join(t.TempDir(), "sessions")
	writeResumeTestSession(t, sessionsDir, "s1", "from another workspace")
	if err := session.SaveSessionMeta(sessionsDir, "s1", session.SessionMeta{Workspace: other, Branch: "main"}); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	app := &App{
		sessionsDir:   sessionsDir,
		workspaceRoot: current,
		sessionID:     "current",
	}
	out, err := app.ApplyResumeChoice("1")
	if err != nil {
		t.Fatalf("ApplyResumeChoice: %v", err)
	}
	if out.Resumed {
		t.Fatal("expected cross-workspace resume to be blocked")
	}
	if app.SessionID() != "current" {
		t.Fatalf("session changed to %q", app.SessionID())
	}
	if !strings.Contains(out.Message, "This conversation is from a different directory.") ||
		!strings.Contains(out.Message, "To resume, run:") ||
		!strings.Contains(out.Message, "cd ") ||
		!strings.Contains(out.Message, " resume s1") {
		t.Fatalf("unexpected cross-workspace message:\n%s", out.Message)
	}
	meta, err := session.LoadSessionMeta(sessionsDir, "s1")
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if meta.Workspace != other {
		t.Fatalf("workspace was mutated to %q, want %q", meta.Workspace, other)
	}
}

func TestApplyResumeChoiceAllowsSameWorkspace(t *testing.T) {
	current := t.TempDir()
	sessionsDir := filepath.Join(t.TempDir(), "sessions")
	writeResumeTestSession(t, sessionsDir, "s1", "same workspace")
	if err := session.SaveSessionMeta(sessionsDir, "s1", session.SessionMeta{Workspace: current, Branch: "main"}); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	app := &App{
		sessionsDir:   sessionsDir,
		workspaceRoot: current,
		sessionID:     "current",
	}
	out, err := app.ApplyResumeChoice("1")
	if err != nil {
		t.Fatalf("ApplyResumeChoice: %v", err)
	}
	if !out.Resumed {
		t.Fatalf("expected same-workspace resume, got message:\n%s", out.Message)
	}
	if app.SessionID() != "s1" {
		t.Fatalf("session = %q, want s1", app.SessionID())
	}
}

func TestApplyResumeChoiceAllowsLegacySessionWithoutWorkspace(t *testing.T) {
	current := t.TempDir()
	sessionsDir := filepath.Join(t.TempDir(), "sessions")
	writeResumeTestSession(t, sessionsDir, "s1", "legacy workspace")

	app := &App{
		sessionsDir:   sessionsDir,
		workspaceRoot: current,
		sessionID:     "current",
	}
	out, err := app.ApplyResumeChoice("1")
	if err != nil {
		t.Fatalf("ApplyResumeChoice: %v", err)
	}
	if !out.Resumed {
		t.Fatalf("expected legacy session to resume, got message:\n%s", out.Message)
	}
	meta, err := session.LoadSessionMeta(sessionsDir, "s1")
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if meta.Workspace != current {
		t.Fatalf("legacy workspace = %q, want %q", meta.Workspace, current)
	}
}

func TestNewResumeMenuDoesNotPatchMostRecentSessionWorkspace(t *testing.T) {
	current := t.TempDir()
	other := t.TempDir()
	dataDir := t.TempDir()
	sessionsDir := filepath.Join(dataDir, "sessions")
	writeResumeTestSession(t, sessionsDir, "s1", "do not mutate")
	if err := session.SaveSessionMeta(sessionsDir, "s1", session.SessionMeta{Workspace: other, Branch: "main"}); err != nil {
		t.Fatalf("save meta: %v", err)
	}
	t.Chdir(current)
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")

	cfg := DefaultConfig()
	cfg.DataDir = dataDir
	app, err := New(context.Background(), cfg, StartOptions{ResumeMenu: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer app.Close()

	meta, err := session.LoadSessionMeta(sessionsDir, "s1")
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if meta.Workspace != other {
		t.Fatalf("resume menu mutated workspace to %q, want %q", meta.Workspace, other)
	}
}

func TestNewDirectResumeBlocksCrossWorkspace(t *testing.T) {
	current := t.TempDir()
	other := t.TempDir()
	dataDir := t.TempDir()
	sessionsDir := filepath.Join(dataDir, "sessions")
	writeResumeTestSession(t, sessionsDir, "s1", "direct resume")
	if err := session.SaveSessionMeta(sessionsDir, "s1", session.SessionMeta{Workspace: other}); err != nil {
		t.Fatalf("save meta: %v", err)
	}
	t.Chdir(current)
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")

	cfg := DefaultConfig()
	cfg.DataDir = dataDir
	_, err := New(context.Background(), cfg, StartOptions{SessionID: "s1"})
	if err == nil {
		t.Fatal("expected cross-workspace direct resume to be blocked")
	}
	if !IsCrossWorkspaceResumeError(err) {
		t.Fatalf("expected cross-workspace error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "This conversation is from a different directory.") {
		t.Fatalf("unexpected error message:\n%s", err)
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

func writeResumeTestSession(t *testing.T, sessionsDir, id, text string) {
	t.Helper()
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, id+".jsonl"), []byte(`{"Role":"user","Text":"`+text+`"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}
}
