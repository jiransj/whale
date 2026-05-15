//go:build windows

package shell

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// ConfigureCommand applies platform process settings for shell commands.
func ConfigureCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return killCommandTree(cmd)
	}
	cmd.WaitDelay = 2 * time.Second
}

func killCommandTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	pid := cmd.Process.Pid
	if pid <= 0 {
		return os.ErrProcessDone
	}
	taskkill := exec.Command("taskkill", "/pid", strconv.Itoa(pid), "/T", "/F")
	taskkill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = taskkill.Run()
	return nil
}
