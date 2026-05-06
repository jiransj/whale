package main

import (
	"fmt"
	"os"

	"github.com/usewhale/whale/internal/ui/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		if code, ok := cmd.ExitCode(err); ok {
			os.Exit(code)
		}
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
