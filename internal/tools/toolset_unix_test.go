//go:build unix

package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecShellCancelKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type execResult struct {
		res string
		err error
	}
	done := make(chan execResult, 1)
	go func() {
		res, err := ts.execShell(ctx, tc("exec_shell", map[string]any{
			"command":    "sleep 30 & echo $! > child.pid; wait",
			"timeout_ms": 120000,
		}))
		done <- execResult{res: res.Content, err: err}
	}()

	pid := waitForPIDFile(t, filepath.Join(dir, "child.pid"))
	cancel()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("exec shell returned error: %v", got.err)
		}
		if !strings.Contains(got.res, `"code":"cancelled"`) {
			t.Fatalf("expected cancelled result, got: %s", got.res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exec shell did not return promptly after cancel")
	}

	waitForProcessExit(t, pid)
}

func TestExecShellBackgroundTimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewToolset(dir)
	if err != nil {
		t.Fatalf("new toolset: %v", err)
	}

	startRes, err := ts.execShell(context.Background(), tc("exec_shell", map[string]any{
		"command":    "sleep 30 & echo $! > child.pid; wait",
		"background": true,
		"timeout_ms": 100,
	}))
	if err != nil || startRes.IsError {
		t.Fatalf("exec_shell background failed: err=%v res=%+v", err, startRes)
	}
	taskID := backgroundTaskID(t, startRes.Content)
	pid := waitForPIDFile(t, filepath.Join(dir, "child.pid"))

	waitRes, err := ts.execShellWait(context.Background(), tc("exec_shell_wait", map[string]any{
		"task_id":    taskID,
		"timeout_ms": 3000,
	}))
	if err != nil || waitRes.IsError {
		t.Fatalf("exec_shell_wait failed: err=%v res=%+v", err, waitRes)
	}
	if !strings.Contains(waitRes.Content, `"status":"timeout"`) {
		t.Fatalf("expected timeout status, got: %s", waitRes.Content)
	}
	waitForProcessExit(t, pid)
}

func backgroundTaskID(t *testing.T, content string) string {
	t.Helper()
	var envelope struct {
		Data struct {
			Payload struct {
				TaskID string `json:"task_id"`
			} `json:"payload"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		t.Fatalf("unmarshal background result: %v", err)
	}
	if envelope.Data.Payload.TaskID == "" {
		t.Fatalf("expected task_id, got: %s", content)
	}
	return envelope.Data.Payload.TaskID
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(b)))
			if convErr != nil {
				t.Fatalf("parse pid %q: %v", strings.TrimSpace(string(b)), convErr)
			}
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pid file was not written: %s", path)
	return 0
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("child process %d still exists after cancel", pid)
}
