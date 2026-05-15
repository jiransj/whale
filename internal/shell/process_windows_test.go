//go:build windows

package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsConfigureCommandKillsProcessTree(t *testing.T) {
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "ready")
	markerPath := filepath.Join(dir, "marker")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestWindowsProcessTreeHelper")
	cmd.Env = append(os.Environ(),
		"WHALE_PROCESS_TREE_HELPER=parent",
		"WHALE_PROCESS_TREE_READY="+readyPath,
		"WHALE_PROCESS_TREE_MARKER="+markerPath,
	)
	ConfigureCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitForFile(t, readyPath, 2*time.Second)

	cancel()
	_ = cmd.Wait()
	time.Sleep(1500 * time.Millisecond)

	if _, err := os.Stat(markerPath); err == nil {
		t.Fatalf("descendant process survived cancellation and wrote %s", markerPath)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat marker: %v", err)
	}
}

func TestWindowsProcessTreeHelper(t *testing.T) {
	switch os.Getenv("WHALE_PROCESS_TREE_HELPER") {
	case "parent":
		markerPath := os.Getenv("WHALE_PROCESS_TREE_MARKER")
		readyPath := os.Getenv("WHALE_PROCESS_TREE_READY")
		cmd := exec.Command(os.Args[0], "-test.run=TestWindowsProcessTreeHelper")
		cmd.Env = append(os.Environ(),
			"WHALE_PROCESS_TREE_HELPER=child",
			"WHALE_PROCESS_TREE_MARKER="+markerPath,
		)
		if err := cmd.Start(); err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(readyPath, []byte("ready"), 0o644); err != nil {
			os.Exit(3)
		}
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "child":
		time.Sleep(time.Second)
		if err := os.WriteFile(os.Getenv("WHALE_PROCESS_TREE_MARKER"), []byte("alive"), 0o644); err != nil {
			os.Exit(4)
		}
		os.Exit(0)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
