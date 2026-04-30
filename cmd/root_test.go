package cmd

import (
	"io"
	"testing"
)

func TestExecuteShowsHelp(t *testing.T) {
	rootCmd.SetArgs([]string{"--help"})
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
}
