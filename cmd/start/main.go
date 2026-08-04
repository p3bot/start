package main

import (
	"fmt"
	"os"

	"github.com/p3bot/start/internal/cli"
	"github.com/p3bot/start/internal/tui"
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
