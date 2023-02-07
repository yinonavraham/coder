package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli"
)

func main() {
	cmd := &cobra.Command{
		Use:  "aistart",
		Long: "A tool for experimenting with our AIstart implementation",
	}

	cmd.AddCommand(loadTrainingCSV())

	cmd, err := cmd.ExecuteC()
	if err != nil {
		cobraErr := cli.FormatCobraError(err, cmd)
		_, _ = fmt.Fprintln(os.Stderr, cobraErr)
		os.Exit(1)
	}
}
