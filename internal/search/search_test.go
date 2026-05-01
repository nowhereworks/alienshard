package search

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRebuildIndexesRawAndWikiWithExclusions(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	mustWriteFile(t, filepath.Join(homeDir, "raw.md"), "# Raw Needle\n\nraw body")
	mustWriteFile(t, filepath.Join(homeDir, wikiDirName, "wiki.md"), "# Wiki Needle\n\nwiki body")
	mustWriteFile(t, filepath.Join(homeDir, wikiDirName, "hidden.md"), "# Hidden Needle\n\nwiki only")
	mustWriteFile(t, filepath.Join(homeDir, alienshardDir, "secret.md"), "# Secret Needle")
	mustWriteFile(t, filepath.Join(homeDir, "image.bin"), "\x00needle")

	result, err := Rebuild(context.Background(), homeDir)
	if err != nil {
		t.Fatalf("Rebuild returned error: %v", err)
	}
	if result.RawIndexed != 1 {
		t.Fatalf("RawIndexed = %d, want 1", result.RawIndexed)
	}
	if result.WikiIndexed != 2 {
		t.Fatalf("WikiIndexed = %d, want 2", result.WikiIndexed)
	}
	if result.FilesSkipped != 1 {
		t.Fatalf("FilesSkipped = %d, want 1", result.FilesSkipped)
	}

	all, err := Query(context.Background(), homeDir, QueryOptions{Query: "needle", Scope: ScopeAll})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if all.IndexState != StateReady {
		t.Fatalf("IndexState = %q, want %q", all.IndexState, StateReady)
	}
	assertHasPath(t, all.Results, "/n/default/raw/raw.md")
	assertHasPath(t, all.Results, "/n/default/wiki/wiki.md")
	assertHasPath(t, all.Results, "/n/default/wiki/hidden.md")
	assertNoPath(t, all.Results, "/raw/__wiki/wiki.md")
	assertNoPath(t, all.Results, "/raw/.alienshard/secret.md")

	rawOnly, err := Query(context.Background(), homeDir, QueryOptions{Query: "needle", Scope: ScopeRaw})
	if err != nil {
		t.Fatalf("raw Query returned error: %v", err)
	}
	if len(rawOnly.Results) != 1 || rawOnly.Results[0].Path != "/n/default/raw/raw.md" {
		t.Fatalf("raw results = %#v, want only /n/default/raw/raw.md", rawOnly.Results)
	}

	wikiOnly, err := Query(context.Background(), homeDir, QueryOptions{Query: "needle", Scope: ScopeWiki})
	if err != nil {
		t.Fatalf("wiki Query returned error: %v", err)
	}
	assertHasPath(t, wikiOnly.Results, "/n/default/wiki/wiki.md")
	assertNoPath(t, wikiOnly.Results, "/n/default/raw/raw.md")
}

func TestQueryReportsNotIndexed(t *testing.T) {
	t.Parallel()

	result, err := Query(context.Background(), t.TempDir(), QueryOptions{Query: "needle"})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if result.IndexState != StateNotIndexed {
		t.Fatalf("IndexState = %q, want %q", result.IndexState, StateNotIndexed)
	}
	if len(result.Results) != 0 {
		t.Fatalf("results = %#v, want none", result.Results)
	}
}

func TestIncrementalWikiUpsertAndDelete(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	mustWriteFile(t, filepath.Join(homeDir, "seed.md"), "# Seed")
	if _, err := Rebuild(context.Background(), homeDir); err != nil {
		t.Fatalf("Rebuild returned error: %v", err)
	}

	mustWriteFile(t, filepath.Join(homeDir, wikiDirName, "new.md"), "# New Page\n\nunique incremental needle")
	if err := UpsertWikiDocument(context.Background(), homeDir, "new.md"); err != nil {
		t.Fatalf("UpsertWikiDocument returned error: %v", err)
	}

	result, err := Query(context.Background(), homeDir, QueryOptions{Query: "incremental", Scope: ScopeWiki})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Path != "/n/default/wiki/new.md" {
		t.Fatalf("results = %#v, want /n/default/wiki/new.md", result.Results)
	}

	if err := os.Remove(filepath.Join(homeDir, wikiDirName, "new.md")); err != nil {
		t.Fatalf("os.Remove returned error: %v", err)
	}
	if err := DeleteWikiDocument(context.Background(), homeDir, "new.md"); err != nil {
		t.Fatalf("DeleteWikiDocument returned error: %v", err)
	}

	result, err = Query(context.Background(), homeDir, QueryOptions{Query: "incremental", Scope: ScopeWiki})
	if err != nil {
		t.Fatalf("Query after delete returned error: %v", err)
	}
	if len(result.Results) != 0 {
		t.Fatalf("results after delete = %#v, want none", result.Results)
	}
}

func TestBacklinksNormalizeRelativeAndPublicLinks(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	mustWriteFile(t, filepath.Join(homeDir, wikiDirName, "target.md"), "# Target\n\nneedle")
	mustWriteFile(t, filepath.Join(homeDir, wikiDirName, "folder", "source.md"), "# Source\n\n[relative](../target.md) and [public](/wiki/target.md)")
	mustWriteFile(t, filepath.Join(homeDir, "raw.md"), "# Raw\n\n[raw link](/wiki/target.md)")

	if _, err := Rebuild(context.Background(), homeDir); err != nil {
		t.Fatalf("Rebuild returned error: %v", err)
	}

	backlinks, err := Backlinks(context.Background(), homeDir, "/wiki/target.md")
	if err != nil {
		t.Fatalf("Backlinks returned error: %v", err)
	}
	if len(backlinks) != 2 {
		t.Fatalf("backlinks = %#v, want two unique source documents", backlinks)
	}
	assertHasPath(t, backlinks, "/n/default/raw/raw.md")
	assertHasPath(t, backlinks, "/n/default/wiki/folder/source.md")
}

func TestNamespaceRebuildIsIsolated(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	namespaceRoot := filepath.Join(homeDir, "__namespaces", "research")
	mustWriteFile(t, filepath.Join(homeDir, "default.md"), "# Default Needle")
	mustWriteFile(t, filepath.Join(namespaceRoot, "research.md"), "# Research Needle")
	mustWriteFile(t, filepath.Join(namespaceRoot, wikiDirName, "note.md"), "# Research Wiki Needle")

	if _, err := RebuildNamespace(context.Background(), namespaceRoot, "research"); err != nil {
		t.Fatalf("RebuildNamespace returned error: %v", err)
	}

	result, err := QueryNamespace(context.Background(), namespaceRoot, "research", QueryOptions{Query: "needle"})
	if err != nil {
		t.Fatalf("QueryNamespace returned error: %v", err)
	}
	assertHasPath(t, result.Results, "/n/research/raw/research.md")
	assertHasPath(t, result.Results, "/n/research/wiki/note.md")
	assertNoPath(t, result.Results, "/n/default/raw/default.md")
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
}

func assertHasPath(t *testing.T, results []SearchResult, path string) {
	t.Helper()
	for _, result := range results {
		if result.Path == path {
			return
		}
	}
	t.Fatalf("results = %#v, missing %s", results, path)
}

func assertNoPath(t *testing.T, results []SearchResult, path string) {
	t.Helper()
	for _, result := range results {
		if result.Path == path {
			t.Fatalf("results = %#v, unexpectedly contained %s", results, path)
		}
	}
}
