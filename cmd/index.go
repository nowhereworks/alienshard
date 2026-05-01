package cmd

import (
	"context"
	"fmt"
	"io"

	"alienshard/internal/search"
	"github.com/spf13/cobra"
)

var indexRebuildHomeDir string

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage the search index",
}

var indexRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild the search index",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIndexRebuild(indexRebuildHomeDir, cmd.OutOrStdout())
	},
}

func runIndexRebuild(homeDir string, out io.Writer) error {
	resolvedHomeDir, err := resolveHomeDir(homeDir)
	if err != nil {
		return err
	}

	result, err := search.Rebuild(context.Background(), resolvedHomeDir)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(out, "Indexed %d files:\n  raw: %d\n  wiki: %d\n  skipped: %d\n  duration: %.1fs\n",
		result.FilesIndexed,
		result.RawIndexed,
		result.WikiIndexed,
		result.FilesSkipped,
		result.Duration.Seconds(),
	)
	return err
}

func init() {
	indexRebuildCmd.Flags().StringVar(&indexRebuildHomeDir, "home-dir", "", "Directory to index (defaults to current directory)")
	indexCmd.AddCommand(indexRebuildCmd)
	rootCmd.AddCommand(indexCmd)
}
