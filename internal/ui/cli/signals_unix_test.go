//go:build unix

package cli

import (
	"os"
	"syscall"
	"testing"
)

func TestCLIInterruptSignalsUnix(t *testing.T) {
	got := cliInterruptSignals()
	if len(got) != 2 {
		t.Fatalf("signals = %#v, want os.Interrupt and syscall.SIGTERM", got)
	}
	if got[0] != os.Interrupt || got[1] != syscall.SIGTERM {
		t.Fatalf("signals = %#v, want os.Interrupt and syscall.SIGTERM", got)
	}
}
