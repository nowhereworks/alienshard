package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIndexRebuildWritesSummary(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(homeDir, "doc.md"), []byte("# Doc\n\nneedle"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	var out bytes.Buffer
	if err := runIndexRebuild(homeDir, defaultNamespace, &out); err != nil {
		t.Fatalf("runIndexRebuild returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"Indexed 1 files:", "namespace: default", "raw: 1", "wiki: 0", "skipped: 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want to contain %q", got, want)
		}
	}
}

func TestRunIndexRebuildUsesNamespace(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	namespaceRoot := filepath.Join(homeDir, namespaceDirName, "research")
	if err := os.MkdirAll(namespaceRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(namespaceRoot, "doc.md"), []byte("# Doc\n\nneedle"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	var out bytes.Buffer
	if err := runIndexRebuild(homeDir, "research", &out); err != nil {
		t.Fatalf("runIndexRebuild returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"Indexed 1 files:", "namespace: research", "raw: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want to contain %q", got, want)
		}
	}
}

func TestIndexCommandIsRegistered(t *testing.T) {
	t.Parallel()

	index, _, err := rootCmd.Find([]string{"index"})
	if err != nil {
		t.Fatalf("rootCmd.Find index returned error: %v", err)
	}
	if index == nil || index.Use != "index" {
		t.Fatalf("index command = %#v, want registered index command", index)
	}

	rebuild, _, err := rootCmd.Find([]string{"index", "rebuild"})
	if err != nil {
		t.Fatalf("rootCmd.Find index rebuild returned error: %v", err)
	}
	if rebuild == nil || rebuild.Use != "rebuild" {
		t.Fatalf("rebuild command = %#v, want registered rebuild command", rebuild)
	}
}
