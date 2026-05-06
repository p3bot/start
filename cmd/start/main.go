package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/start-cli/start/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		if !cli.IsSilentError(err) {
			red := color.New(color.FgRed)
			_, _ = red.Fprint(os.Stderr, "Error: ")
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
