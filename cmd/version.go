package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version   = "dev" // default value
	commit    = "none"
	buildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:              "version",
	Short:            "Print the Axel version",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("axel %s (commit: %s, built: %s)\n", version, commit, buildDate)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
