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
			red.Fprint(os.Stderr, "Error: ")
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
