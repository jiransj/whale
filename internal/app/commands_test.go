package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/store"
)

func TestResolveInitialSessionID(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "recent.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := resolveInitialSessionID(sessionsDir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "recent" {
		t.Fatalf("want recent, got %s", got)
	}
}

func TestHandleCommandResumeAndNew(t *testing.T) {
	now := time.Date(2026, 5, 2, 10, 20, 30, 0, time.UTC)

	_, err := handleCommand("/resume abc", "cur", now)
	if err == nil {
		t.Fatal("expected /resume usage error")
	}

	res, err := handleCommand("/new", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.SessionID != "20260502-102030" {
		t.Fatalf("unexpected generated id: %s", res.SessionID)
	}

	res, err = handleCommand("/new s2", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.SessionID != "s2" {
		t.Fatalf("unexpected new id: %s", res.SessionID)
	}

	_, err = handleCommand("/new a b", "cur", now)
	if err == nil {
		t.Fatal("expected /new usage error")
	}
}

func TestExpandUniqueSlashPrefix(t *testing.T) {
	if got := expandUniqueSlashPrefix("/com"); got != "/compact" {
		t.Fatalf("expected /compact, got %q", got)
	}
	if got := expandUniqueSlashPrefix("/tool"); got != "/tool" {
		t.Fatalf("ambiguous exact command should stay unchanged, got %q", got)
	}
	if got := expandUniqueSlashPrefix("/plan inspect"); got != "/plan inspect" {
		t.Fatalf("commands with args should stay unchanged, got %q", got)
	}
	if got := expandUniqueSlashPrefix("/as"); got != "/ask" {
		t.Fatalf("expected /ask, got %q", got)
	}
}

func TestResolveCLIResumeID(t *testing.T) {
	got, matched, err := resolveCLIResumeID([]string{"resume", "s-1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !matched || got != "s-1" {
		t.Fatalf("unexpected result: got=%q matched=%v", got, matched)
	}

	_, matched, err = resolveCLIResumeID([]string{"resume"})
	if err == nil || !matched {
		t.Fatalf("expected usage error for missing id, matched=%v err=%v", matched, err)
	}

	got, matched, err = resolveCLIResumeID([]string{"other", "x"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if matched || got != "" {
		t.Fatalf("unexpected non-resume parse: got=%q matched=%v", got, matched)
	}
}

func TestHandleCommandModeSwitch(t *testing.T) {
	now := time.Date(2026, 5, 3, 8, 0, 0, 0, time.UTC)
	res, err := handleCommand("/status", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Handled || !res.ShowStatus {
		t.Fatalf("unexpected /status result: %+v", res)
	}

	res, err = handleCommand("/context", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Handled || !res.ShowContext {
		t.Fatalf("unexpected /context result: %+v", res)
	}

	res, err = handleCommand("/plan", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Handled || res.Mode != "plan" {
		t.Fatalf("unexpected /plan result: %+v", res)
	}

	if _, err = handleCommand("/plan show", "cur", now); err == nil {
		t.Fatal("expected /plan show usage error")
	}

	res, err = handleCommand("/plan implement tests", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Handled || res.Mode != "plan" || res.PlanPrompt != "implement tests" {
		t.Fatalf("unexpected /plan prompt result: %+v", res)
	}

	if _, err = handleCommand("/plan on", "cur", now); err == nil {
		t.Fatal("expected /plan on usage error")
	}
	if _, err = handleCommand("/plan off", "cur", now); err == nil {
		t.Fatal("expected /plan off usage error")
	}

	res, err = handleCommand("/ask", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Handled || res.Mode != "ask" {
		t.Fatalf("unexpected /ask result: %+v", res)
	}

	res, err = handleCommand("/ask inspect the parser", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Handled || res.Mode != "ask" || res.AskPrompt != "inspect the parser" {
		t.Fatalf("unexpected /ask prompt result: %+v", res)
	}

	for _, old := range []string{"/step", "/checkpoint", "/continue", "/stop", "/revise add retry"} {
		res, err = handleCommand(old, "cur", now)
		if err != nil || res.Handled {
			t.Fatalf("expected %s to be unhandled, got %+v err=%v", old, res, err)
		}
	}
	res, err = handleCommand("/init", "cur", now)
	if err != nil || !res.Handled || !res.InitMemory {
		t.Fatalf("unexpected /init result: %+v err=%v", res, err)
	}
	res, err = handleCommand("/memory", "cur", now)
	if err != nil || !res.Handled || !res.ShowMemory {
		t.Fatalf("unexpected /memory result: %+v err=%v", res, err)
	}
}

func TestHandleSlashInitReturnsSyntheticPrompt(t *testing.T) {
	dir := t.TempDir()
	app := &App{workspaceRoot: dir, sessionID: "cur"}
	handled, output, synthetic, shouldExit, _, err := app.HandleSlash("/init")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !handled || shouldExit {
		t.Fatalf("unexpected handled=%v shouldExit=%v", handled, shouldExit)
	}
	if !strings.Contains(output, "Initializing AGENTS.md") {
		t.Fatalf("unexpected output: %q", output)
	}
	if !strings.Contains(synthetic, "Generate a file named AGENTS.md") {
		t.Fatalf("missing synthetic init prompt: %q", synthetic)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected /init not to write AGENTS.md directly, err=%v", err)
	}
}

func TestHandleCommandClear(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

	res, err := handleCommand("/clear", "cur", now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Handled {
		t.Fatal("expected /clear to be handled")
	}
	if !res.ClearScreen {
		t.Fatal("expected clearScreen=true for /clear")
	}
	if res.SessionID != "cur" {
		t.Fatalf("expected session unchanged, got %s", res.SessionID)
	}
}

func TestHandleSlashClearReturnsClearScreenFlag(t *testing.T) {
	app := &App{sessionID: "sess-1", workspaceRoot: t.TempDir()}
	handled, out, synthetic, shouldExit, clearScreen, err := app.HandleSlash("/clear")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !handled {
		t.Fatal("expected /clear to be handled")
	}
	if !clearScreen {
		t.Fatal("expected clearScreen=true")
	}
	if shouldExit {
		t.Fatal("expected shouldExit=false")
	}
	if synthetic != "" {
		t.Fatal("expected no synthetic prompt")
	}
	if !strings.Contains(out, "terminal cleared") {
		t.Fatalf("expected output to mention terminal cleared, got: %q", out)
	}
}

func TestHandleSlashNewIncludesResumeHint(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a message so old session has content.
	store, err := store.NewJSONLStore(sessionsDir)
	if err != nil {
		t.Fatalf("store init: %v", err)
	}
	if _, err := store.Create(context.Background(), core.Message{SessionID: "sess-1", Role: core.RoleUser, Text: "hello"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	app := &App{
		sessionsDir:   sessionsDir,
		workspaceRoot: dir,
		sessionID:     "sess-1",
		msgStore:      store,
		ctx:           context.Background(),
	}
	handled, out, synthetic, shouldExit, clearScreen, err := app.HandleSlash("/new")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !handled {
		t.Fatal("expected /new to be handled")
	}
	if clearScreen {
		t.Fatal("expected clearScreen=false for /new")
	}
	if shouldExit {
		t.Fatal("expected shouldExit=false")
	}
	if synthetic != "" {
		t.Fatal("expected no synthetic prompt")
	}
	if app.SessionID() == "sess-1" {
		t.Fatal("expected new session id, still on sess-1")
	}
	if !strings.Contains(out, "new session: ") {
		t.Fatalf("expected output to contain new session, got: %q", out)
	}
	if !strings.Contains(out, "dropped 1 message") {
		t.Fatalf("expected output to mention dropped messages, got: %q", out)
	}
	if !strings.Contains(out, "whale resume sess-1") {
		t.Fatalf("expected output to include resume hint, got: %q", out)
	}
}
