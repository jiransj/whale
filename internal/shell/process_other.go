//go:build !unix

package shell

import (
	"os/exec"
	"time"
)

// ConfigureCommand applies platform process settings for shell commands.
func ConfigureCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}
