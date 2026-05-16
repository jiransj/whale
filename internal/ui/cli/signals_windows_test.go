//go:build windows

package cli

import (
	"os"
	"testing"
)

func TestCLIInterruptSignalsWindows(t *testing.T) {
	got := cliInterruptSignals()
	if len(got) != 1 || got[0] != os.Interrupt {
		t.Fatalf("signals = %#v, want only os.Interrupt", got)
	}
}
