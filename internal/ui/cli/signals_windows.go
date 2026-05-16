//go:build windows

package cli

import "os"

func cliInterruptSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
