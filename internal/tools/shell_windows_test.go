//go:build windows

package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWindowsShellRunForegroundAndBackground(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}

	const marker = "whale_windows_shell_tool"
	foreground, err := ts.shellRun(context.Background(), tc("shell_run", map[string]any{
		"command": "echo " + marker,
	}))
	if err != nil || foreground.IsError {
		t.Fatalf("shell_run foreground failed: err=%v res=%+v", err, foreground)
	}
	if !strings.Contains(foreground.Content, marker) {
		t.Fatalf("foreground result missing marker %q: %s", marker, foreground.Content)
	}

	start, err := ts.shellRun(context.Background(), tc("shell_run", map[string]any{
		"command":    "echo " + marker,
		"background": true,
	}))
	if err != nil || start.IsError {
		t.Fatalf("shell_run background failed: err=%v res=%+v", err, start)
	}

	var envelope struct {
		Data struct {
			Payload struct {
				TaskID string `json:"task_id"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(start.Content), &envelope); err != nil {
		t.Fatalf("unmarshal background result: %v", err)
	}
	if envelope.Data.Payload.TaskID == "" {
		t.Fatalf("expected task_id, got: %s", start.Content)
	}

	wait, err := ts.shellWait(context.Background(), tc("shell_wait", map[string]any{
		"task_id":    envelope.Data.Payload.TaskID,
		"timeout_ms": 5000,
	}))
	if err != nil || wait.IsError {
		t.Fatalf("shell_wait failed: err=%v res=%+v", err, wait)
	}
	if !strings.Contains(wait.Content, marker) {
		t.Fatalf("background result missing marker %q: %s", marker, wait.Content)
	}
}
