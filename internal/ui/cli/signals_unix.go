//go:build unix

package cli

import (
	"os"
	"syscall"
)

func cliInterruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
