package main

import (
	"fmt"
	"os"

	"github.com/start-cli/start/internal/cli"
	"github.com/start-cli/start/internal/tui"
)

func main() {
	if err := cli.Execute(); err != nil {
		if !cli.IsSilentError(err) {
			tui.ColorError.Fprint(os.Stderr, "Error: ")
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(cli.ExitCodeFromError(err))
	}
}
