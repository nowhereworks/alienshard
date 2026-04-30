package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "alienshard",
	Short: "Alienshard serves files over HTTP",
}

func Execute() error {
	return rootCmd.Execute()
}
