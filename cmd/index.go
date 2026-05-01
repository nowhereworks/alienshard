package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"alienshard/internal/search"
	"github.com/spf13/cobra"
)

var indexRebuildHomeDir string
var indexRebuildNamespace string

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage the search index",
}

var indexRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild the search index",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIndexRebuild(indexRebuildHomeDir, indexRebuildNamespace, cmd.OutOrStdout())
	},
}

func runIndexRebuild(homeDir, namespace string, out io.Writer) error {
	resolvedHomeDir, err := resolveHomeDir(homeDir)
	if err != nil {
		return err
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = strings.TrimSpace(os.Getenv("ALIEN_NAMESPACE"))
	}
	if namespace == "" {
		namespace = defaultNamespace
	}
	if err := validateNamespaceName(namespace); err != nil {
		return err
	}

	result, err := search.RebuildNamespace(context.Background(), namespaceRawRoot(resolvedHomeDir, namespace), namespace)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(out, "Indexed %d files:\n  namespace: %s\n  raw: %d\n  wiki: %d\n  skipped: %d\n  duration: %.1fs\n",
		result.FilesIndexed,
		namespace,
		result.RawIndexed,
		result.WikiIndexed,
		result.FilesSkipped,
		result.Duration.Seconds(),
	)
	return err
}

func init() {
	indexRebuildCmd.Flags().StringVar(&indexRebuildHomeDir, "home-dir", "", "Directory to index (defaults to current directory)")
	indexRebuildCmd.Flags().StringVar(&indexRebuildNamespace, "namespace", "", "Namespace to index (env ALIEN_NAMESPACE, default default)")
	indexCmd.AddCommand(indexRebuildCmd)
	rootCmd.AddCommand(indexCmd)
}
