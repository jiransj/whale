//go:build !unix

package tools

import (
	"os/exec"
	"time"
)

func configureShellCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}
