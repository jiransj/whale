//go:build !unix

package tools

import (
	"os"
	"os/exec"
	"time"
)

func configureShellCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		return killShellCommandGroup(cmd)
	}
	cmd.WaitDelay = 2 * time.Second
}

func killShellCommandGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
