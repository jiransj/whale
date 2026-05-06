package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/core"
)

func tc(name string, in any) core.ToolCall {
	b, _ := json.Marshal(in)
	return core.ToolCall{ID: "tc-1", Name: name, Input: string(b)}
}

func TestViewWriteEdit(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}

	writeRes, err := ts.writeFile(context.Background(), tc("write", map[string]any{
		"file_path": "a.txt",
		"content":   "hello\nworld\n",
	}))
	if err != nil || writeRes.IsError {
		t.Fatalf("write failed: err=%v res=%+v", err, writeRes)
	}

	viewRes, err := ts.readFile(context.Background(), tc("read_file", map[string]any{
		"file_path": "a.txt",
		"offset":    1,
		"limit":     1,
	}))
	if err != nil || viewRes.IsError {
		t.Fatalf("view failed: err=%v res=%+v", err, viewRes)
	}
	if !strings.Contains(viewRes.Content, "world") {
		t.Fatalf("unexpected view content: %s", viewRes.Content)
	}

	editRes, err := ts.editFile(context.Background(), tc("edit", map[string]any{
		"file_path": "a.txt",
		"search":    "world",
		"replace":   "whale",
	}))
	if err != nil || editRes.IsError {
		t.Fatalf("edit failed: err=%v res=%+v", err, editRes)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if !strings.Contains(string(got), "whale") {
		t.Fatalf("edit not applied: %s", string(got))
	}
}

func TestPathEscapeDenied(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	res, err := ts.readFile(context.Background(), tc("read_file", map[string]any{
		"file_path": "../x",
	}))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "permission_denied") {
		t.Fatalf("expected permission_denied, got: %+v", res)
	}
}

func TestListDirAndExecShell(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	lsRes, err := ts.listDir(context.Background(), tc("list_dir", map[string]any{}))
	if err != nil || lsRes.IsError {
		t.Fatalf("ls failed: err=%v res=%+v", err, lsRes)
	}
	if !strings.Contains(lsRes.Content, "x.txt") {
		t.Fatalf("ls missing file: %s", lsRes.Content)
	}
	bashRes, err := ts.execShell(context.Background(), tc("exec_shell", map[string]any{
		"command": "echo hi",
	}))
	if err != nil || bashRes.IsError {
		t.Fatalf("bash failed: err=%v res=%+v", err, bashRes)
	}
	if !strings.Contains(bashRes.Content, "hi") {
		t.Fatalf("unexpected bash output: %s", bashRes.Content)
	}
}

func TestApplyPatchUpdateAddDelete(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Update File: a.txt",
		"@@",
		" hello",
		"-world",
		"+whale",
		"*** Add File: b.txt",
		"+new file",
		"*** Delete File: a.txt",
		"*** End Patch",
	}, "\n")
	res, err := ts.applyPatch(context.Background(), tc("apply_patch", map[string]any{"patch": patch}))
	if err != nil {
		t.Fatalf("apply patch err: %v", err)
	}
	if res.IsError {
		t.Fatalf("apply patch result error: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected a.txt deleted, stat err=%v", err)
	}
	gotB, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	if err != nil {
		t.Fatalf("read b.txt: %v", err)
	}
	if string(gotB) != "new file" {
		t.Fatalf("unexpected b.txt content: %q", string(gotB))
	}
}

func TestApplyPatchInvalidPatch(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	res, err := ts.applyPatch(context.Background(), tc("apply_patch", map[string]any{"patch": "bad patch"}))
	if err != nil {
		t.Fatalf("apply patch err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Content, "patch_parse_failed") {
		t.Fatalf("expected patch_parse_failed, got: %+v", res)
	}
}

func TestSearchFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "alpha.go"), []byte("package sub"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	res, err := ts.searchFiles(context.Background(), tc("search_files", map[string]any{
		"pattern": "alpha",
	}))
	if err != nil || res.IsError {
		t.Fatalf("search_files failed: err=%v res=%+v", err, res)
	}
	if !strings.Contains(res.Content, "alpha.go") {
		t.Fatalf("expected alpha.go in result: %s", res.Content)
	}
}

func TestExecShellBackgroundAndWait(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	startRes, err := ts.execShell(context.Background(), tc("exec_shell", map[string]any{
		"command":    "echo hello",
		"background": true,
	}))
	if err != nil || startRes.IsError {
		t.Fatalf("exec_shell background failed: err=%v res=%+v", err, startRes)
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Payload struct {
				TaskID string `json:"task_id"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(startRes.Content), &envelope); err != nil {
		t.Fatalf("unmarshal start result: %v", err)
	}
	if envelope.Data.Payload.TaskID == "" {
		t.Fatalf("expected task_id, got: %s", startRes.Content)
	}
	waitRes, err := ts.execShellWait(context.Background(), tc("exec_shell_wait", map[string]any{
		"task_id":    envelope.Data.Payload.TaskID,
		"timeout_ms": 5000,
	}))
	if err != nil || waitRes.IsError {
		t.Fatalf("exec_shell_wait failed: err=%v res=%+v", err, waitRes)
	}
	if !strings.Contains(waitRes.Content, "hello") {
		t.Fatalf("expected output in wait result: %s", waitRes.Content)
	}
}

func TestExecShellCWDStaysInsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}
	res, err := ts.execShell(context.Background(), tc("exec_shell", map[string]any{
		"command": "pwd",
		"cwd":     "sub",
	}))
	if err != nil || res.IsError {
		t.Fatalf("exec_shell cwd failed: err=%v res=%+v", err, res)
	}
	if !strings.Contains(res.Content, filepath.Join(dir, "sub")) || !strings.Contains(res.Content, `"cwd":"sub"`) {
		t.Fatalf("expected command to run in subdir with cwd metadata: %s", res.Content)
	}
	escaped, err := ts.execShell(context.Background(), tc("exec_shell", map[string]any{
		"command": "pwd",
		"cwd":     "../outside",
	}))
	if err != nil {
		t.Fatalf("exec_shell escaped cwd returned dispatch error: %v", err)
	}
	if !escaped.IsError || !strings.Contains(escaped.Content, "path escapes workspace") {
		t.Fatalf("expected escaped cwd to be rejected: %+v", escaped)
	}
}
