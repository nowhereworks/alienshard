package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alienshard/internal/search"
)

func TestEndToEndDefaultAndSampleNamespace(t *testing.T) {
	homeDir := t.TempDir()
	defaultWikiRoot := filepath.Join(homeDir, wikiDirName)
	sampleRoot := namespaceRawRoot(homeDir, "sample")
	sampleWikiRoot := filepath.Join(sampleRoot, wikiDirName)

	const (
		defaultRawNeedle     = "dracoalpha"
		defaultWikiNeedle    = "dracowiki"
		sampleRawNeedle      = "orionsampleraw"
		sampleWikiNeedle     = "orionsamplewiki"
		hiddenNeedle         = "hiddennebula"
		defaultCreateNeedle  = "defaultcreatedquasar"
		defaultUpdateNeedle  = "defaultupdatedquasar"
		defaultDeleteNeedle  = "defaultdeletedquasar"
		sampleCreateNeedle   = "samplecreatedquasar"
		sampleUpdateNeedle   = "sampleupdatedquasar"
		sampleDeleteNeedle   = "sampledeletedquasar"
		defaultReindexNeedle = "defaultlatereindex"
		sampleReindexNeedle  = "samplelatereindex"
	)

	mustWriteE2EFile(t, filepath.Join(homeDir, "source.md"), "# Default Source\n\n"+defaultRawNeedle+" source body.\n")
	mustWriteE2EFile(t, filepath.Join(homeDir, "notes", "project.txt"), "Default project notes mention "+defaultRawNeedle+" again.\n")
	mustWriteE2EFile(t, filepath.Join(homeDir, "skip.bin"), "\x00"+defaultRawNeedle)
	mustWriteE2EFile(t, filepath.Join(homeDir, searchDirName, "secret.md"), "# Secret\n\n"+hiddenNeedle+"\n")
	mustWriteE2EFile(t, filepath.Join(defaultWikiRoot, "existing.md"), "# Default Wiki\n\n"+defaultWikiNeedle+" links to [sample](/n/sample/wiki/existing.md).\n")
	mustWriteE2EFile(t, filepath.Join(defaultWikiRoot, "manual-link.md"), "# Manual Link\n\n[Relative](existing.md) and [raw](/raw/source.md).\n")

	mustWriteE2EFile(t, filepath.Join(sampleRoot, "source.md"), "# Sample Source\n\n"+sampleRawNeedle+" source body.\n")
	mustWriteE2EFile(t, filepath.Join(sampleRoot, "data.json"), "{\"token\":\""+sampleRawNeedle+"\"}\n")
	mustWriteE2EFile(t, filepath.Join(sampleRoot, searchDirName, "secret.md"), "# Sample Secret\n\n"+hiddenNeedle+"\n")
	mustWriteE2EFile(t, filepath.Join(sampleWikiRoot, "existing.md"), "# Sample Wiki\n\n"+sampleWikiNeedle+" links to [default](/wiki/existing.md).\n")

	server := httptest.NewServer(newMountedHandler(homeDir, defaultWikiRoot))
	t.Cleanup(server.Close)

	assertSearchStateE2E(t, querySearchE2E(t, server, "/search?q="+defaultRawNeedle), search.StateNotIndexed)
	assertSearchStateE2E(t, querySearchE2E(t, server, "/n/sample/search?q="+sampleRawNeedle), search.StateNotIndexed)

	var out bytes.Buffer
	if err := runIndexRebuild(homeDir, defaultNamespace, &out); err != nil {
		t.Fatalf("default runIndexRebuild returned error: %v", err)
	}
	assertContainsE2E(t, out.String(), "namespace: default")
	assertContainsE2E(t, out.String(), "skipped: 1")
	out.Reset()
	if err := runIndexRebuild(homeDir, "sample", &out); err != nil {
		t.Fatalf("sample runIndexRebuild returned error: %v", err)
	}
	assertContainsE2E(t, out.String(), "namespace: sample")

	assertFileExistsE2E(t, filepath.Join(homeDir, searchDirName, "search.sqlite"))
	assertFileExistsE2E(t, filepath.Join(sampleRoot, searchDirName, "search.sqlite"))

	assertResponseE2E(t, server, http.MethodGet, "/raw/source.md", "", "curl/8.7.1", http.StatusOK, "text/markdown; charset=utf-8", "# Default Source\n\n"+defaultRawNeedle+" source body.\n")
	assertResponseE2E(t, server, http.MethodGet, "/n/default/raw/source.md", "", "curl/8.7.1", http.StatusOK, "text/markdown; charset=utf-8", "# Default Source\n\n"+defaultRawNeedle+" source body.\n")
	assertResponseE2E(t, server, http.MethodGet, "/n/sample/raw/source.md", "", "curl/8.7.1", http.StatusOK, "text/markdown; charset=utf-8", "# Sample Source\n\n"+sampleRawNeedle+" source body.\n")

	markdownHTML := doE2ERequest(t, server, http.MethodGet, "/raw/source.md", "", "Mozilla/5.0 Chrome/126.0")
	assertStatusE2E(t, markdownHTML, http.StatusOK)
	assertHeaderE2E(t, markdownHTML, "Content-Type", "text/html; charset=utf-8")
	assertContainsE2E(t, markdownHTML.body, "<h1>Default Source</h1>")

	for _, path := range []string{
		"/raw/__wiki/existing.md",
		"/raw/__namespaces/sample/source.md",
		"/raw/.alienshard/search.sqlite",
		"/n/sample/raw/.alienshard/search.sqlite",
	} {
		res := doE2ERequest(t, server, http.MethodGet, path, "", "curl/8.7.1")
		assertStatusE2E(t, res, http.StatusNotFound)
	}

	assertGeneratedIndexE2E(t, server, "/wiki", "/n/default/wiki/existing.md")
	assertGeneratedIndexE2E(t, server, "/wiki/", "/n/default/wiki/existing.md")
	assertGeneratedIndexE2E(t, server, "/wiki/index.md", "/n/default/wiki/existing.md")
	assertGeneratedIndexE2E(t, server, "/n/sample/wiki", "/n/sample/wiki/existing.md")
	assertGeneratedIndexE2E(t, server, "/n/sample/wiki/", "/n/sample/wiki/existing.md")
	assertGeneratedIndexE2E(t, server, "/n/sample/wiki/index.md", "/n/sample/wiki/existing.md")

	wikiHTML := doE2ERequest(t, server, http.MethodGet, "/n/sample/wiki/existing.md", "", "Mozilla/5.0 Firefox/125.0")
	assertStatusE2E(t, wikiHTML, http.StatusOK)
	assertHeaderE2E(t, wikiHTML, "Content-Type", "text/html; charset=utf-8")
	assertContainsE2E(t, wikiHTML.body, "<h1>Sample Wiki</h1>")

	assertSearchStateE2E(t, querySearchE2E(t, server, "/search?q="+defaultRawNeedle), search.StateReady)
	assertSearchHasPathE2E(t, querySearchE2E(t, server, "/search?q="+defaultRawNeedle+"&scope=raw"), "/n/default/raw/source.md")
	assertSearchNoPathE2E(t, querySearchE2E(t, server, "/search?q="+defaultRawNeedle+"&scope=wiki"), "/n/default/raw/source.md")
	assertSearchHasPathE2E(t, querySearchE2E(t, server, "/search?q="+defaultWikiNeedle+"&scope=wiki"), "/n/default/wiki/existing.md")
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, "/search?q="+sampleRawNeedle))
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, "/search?q="+sampleWikiNeedle))
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, "/search?q="+hiddenNeedle))

	assertSearchHasPathE2E(t, querySearchE2E(t, server, "/n/sample/search?q="+sampleRawNeedle+"&scope=raw"), "/n/sample/raw/source.md")
	assertSearchHasPathE2E(t, querySearchE2E(t, server, "/n/sample/search?q="+sampleWikiNeedle+"&scope=wiki"), "/n/sample/wiki/existing.md")
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, "/n/sample/search?q="+defaultRawNeedle))
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, "/n/sample/search?q="+defaultWikiNeedle))
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, "/n/sample/search?q="+hiddenNeedle))

	limited := querySearchE2E(t, server, "/search?q="+defaultRawNeedle+"&scope=raw&limit=1")
	if len(limited.Results) != 1 {
		t.Fatalf("limited search returned %d results, want 1: %#v", len(limited.Results), limited.Results)
	}

	for _, path := range []string{
		"/search?scope=raw",
		"/search?q=" + defaultRawNeedle + "&scope=bad",
		"/search?q=" + defaultRawNeedle + "&limit=0",
		"/search?q=" + defaultRawNeedle + "&limit=101",
		"/search?q=" + defaultRawNeedle + "&limit=abc",
	} {
		res := doE2ERequest(t, server, http.MethodGet, path, "", "curl/8.7.1")
		assertStatusE2E(t, res, http.StatusBadRequest)
	}
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodPost, "/search?q="+defaultRawNeedle, "", "curl/8.7.1"), http.StatusMethodNotAllowed)

	assertNamespaceWikiCRUDE2E(t, server, homeDir, defaultNamespace, "/wiki", "/search", defaultCreateNeedle, defaultUpdateNeedle, defaultDeleteNeedle)
	assertNamespaceWikiCRUDE2E(t, server, homeDir, "sample", "/n/sample/wiki", "/n/sample/search", sampleCreateNeedle, sampleUpdateNeedle, sampleDeleteNeedle)

	manualIndex := "# Manual Index\n\nManual body should survive generated updates.\n"
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodPut, "/wiki/index.md", manualIndex, "curl/8.7.1"), http.StatusOK)
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodPut, "/wiki/manual-new.md", "# Manual New\n", "curl/8.7.1"), http.StatusCreated)
	manualRead := doE2ERequest(t, server, http.MethodGet, "/wiki", "", "curl/8.7.1")
	assertStatusE2E(t, manualRead, http.StatusOK)
	if manualRead.body != manualIndex {
		t.Fatalf("manual index changed unexpectedly: got %q want %q", manualRead.body, manualIndex)
	}

	managedIndex := autoIndexMarker + "\n\n# Placeholder\n"
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodPut, "/wiki/index.md", managedIndex, "curl/8.7.1"), http.StatusOK)
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodPut, "/wiki/managed-new.md", "# Managed New\n", "curl/8.7.1"), http.StatusCreated)
	managedRead := doE2ERequest(t, server, http.MethodGet, "/wiki", "", "curl/8.7.1")
	assertStatusE2E(t, managedRead, http.StatusOK)
	assertContainsE2E(t, managedRead.body, autoIndexMarker)
	assertContainsE2E(t, managedRead.body, "/n/default/wiki/managed-new.md")
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodDelete, "/wiki/index.md", "", "curl/8.7.1"), http.StatusNoContent)
	recreatedRead := doE2ERequest(t, server, http.MethodGet, "/wiki", "", "curl/8.7.1")
	assertStatusE2E(t, recreatedRead, http.StatusOK)
	assertContainsE2E(t, recreatedRead.body, autoIndexMarker)

	for _, tt := range []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodPut, "/wiki", http.StatusBadRequest},
		{http.MethodPut, "/wiki/", http.StatusBadRequest},
		{http.MethodPut, "/wiki/folder/", http.StatusBadRequest},
		{http.MethodPut, "/wiki/page.txt", http.StatusBadRequest},
		{http.MethodPut, "/wiki/../escape.md", http.StatusForbidden},
		{http.MethodPut, "/wiki/a//page.md", http.StatusForbidden},
		{http.MethodPut, "/wiki/a\\b.md", http.StatusForbidden},
		{http.MethodGet, "/n/Bad/wiki/page.md", http.StatusBadRequest},
		{http.MethodGet, "/n/-bad/wiki/page.md", http.StatusBadRequest},
		{http.MethodGet, "/n/__wiki/wiki/page.md", http.StatusBadRequest},
		{http.MethodGet, "/n/.alienshard/wiki/page.md", http.StatusBadRequest},
	} {
		res := doE2ERequest(t, server, tt.method, tt.path, "# Invalid\n", "curl/8.7.1")
		assertStatusE2E(t, res, tt.want)
	}

	mustWriteE2EFile(t, filepath.Join(homeDir, "late.md"), "# Late Default\n\n"+defaultReindexNeedle+"\n")
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, "/search?q="+defaultReindexNeedle))
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodPost, "/search/reindex", "", "curl/8.7.1"), http.StatusAccepted)
	waitForSearchReadyE2E(t, server, "/search/status")
	assertSearchHasPathE2E(t, querySearchE2E(t, server, "/search?q="+defaultReindexNeedle+"&scope=raw"), "/n/default/raw/late.md")

	mustWriteE2EFile(t, filepath.Join(sampleRoot, "late.md"), "# Late Sample\n\n"+sampleReindexNeedle+"\n")
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, "/n/sample/search?q="+sampleReindexNeedle))
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodPost, "/n/sample/search/reindex", "", "curl/8.7.1"), http.StatusAccepted)
	waitForSearchReadyE2E(t, server, "/n/sample/search/status")
	assertSearchHasPathE2E(t, querySearchE2E(t, server, "/n/sample/search?q="+sampleReindexNeedle+"&scope=raw"), "/n/sample/raw/late.md")
}

type e2eHTTPResult struct {
	status int
	header http.Header
	body   string
}

func doE2ERequest(t *testing.T, server *httptest.Server, method, path, body, userAgent string) e2eHTTPResult {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("http.NewRequest(%s %s) returned error: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "text/markdown; charset=utf-8")
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s returned error: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading %s %s body returned error: %v", method, path, err)
	}
	return e2eHTTPResult{status: resp.StatusCode, header: resp.Header, body: string(data)}
}

func assertResponseE2E(t *testing.T, server *httptest.Server, method, path, body, userAgent string, status int, contentType, wantBody string) {
	t.Helper()
	res := doE2ERequest(t, server, method, path, body, userAgent)
	assertStatusE2E(t, res, status)
	assertHeaderE2E(t, res, "Content-Type", contentType)
	if res.body != wantBody {
		t.Fatalf("%s %s body = %q, want %q", method, path, res.body, wantBody)
	}
}

func querySearchE2E(t *testing.T, server *httptest.Server, path string) search.QueryResult {
	t.Helper()
	res := doE2ERequest(t, server, http.MethodGet, path, "", "curl/8.7.1")
	assertStatusE2E(t, res, http.StatusOK)
	assertHeaderE2E(t, res, "Content-Type", "application/json; charset=utf-8")
	var result search.QueryResult
	if err := json.Unmarshal([]byte(res.body), &result); err != nil {
		t.Fatalf("json.Unmarshal search response %s returned error: %v; body=%q", path, err, res.body)
	}
	return result
}

func waitForSearchReadyE2E(t *testing.T, server *httptest.Server, statusPath string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		res := doE2ERequest(t, server, http.MethodGet, statusPath, "", "curl/8.7.1")
		assertStatusE2E(t, res, http.StatusOK)
		var status search.Status
		if err := json.Unmarshal([]byte(res.body), &status); err != nil {
			t.Fatalf("json.Unmarshal status %s returned error: %v; body=%q", statusPath, err, res.body)
		}
		if status.State == search.StateReady {
			return
		}
		if status.State == search.StateError {
			t.Fatalf("search status %s entered error: %#v", statusPath, status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to become ready", statusPath)
}

func assertNamespaceWikiCRUDE2E(t *testing.T, server *httptest.Server, homeDir, namespace, wikiMount, searchMount, createNeedle, updateNeedle, deleteNeedle string) {
	t.Helper()
	wikiRoot := filepath.Join(namespaceRawRoot(homeDir, namespace), wikiDirName)
	publicWikiMount := namespacePublicMount(namespace, search.ScopeWiki)

	createPath := wikiMount + "/e2e/created.md"
	createResult := doE2ERequest(t, server, http.MethodPut, createPath, "# Created\n\n"+createNeedle+"\n", "curl/8.7.1")
	assertStatusE2E(t, createResult, http.StatusCreated)
	assertFileContainsE2E(t, filepath.Join(wikiRoot, "e2e", "created.md"), createNeedle)
	assertGeneratedIndexE2E(t, server, wikiMount, publicWikiMount+"/e2e/created.md")
	assertSearchHasPathE2E(t, querySearchE2E(t, server, searchMount+"?q="+createNeedle+"&scope=wiki"), publicWikiMount+"/e2e/created.md")

	updateResult := doE2ERequest(t, server, http.MethodPut, createPath, "# Updated\n\n"+updateNeedle+"\n", "curl/8.7.1")
	assertStatusE2E(t, updateResult, http.StatusOK)
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, searchMount+"?q="+createNeedle+"&scope=wiki"))
	assertSearchHasPathE2E(t, querySearchE2E(t, server, searchMount+"?q="+updateNeedle+"&scope=wiki"), publicWikiMount+"/e2e/created.md")

	deletePath := wikiMount + "/e2e/delete-me.md"
	deleteCreate := doE2ERequest(t, server, http.MethodPut, deletePath, "# Delete Me\n\n"+deleteNeedle+"\n", "curl/8.7.1")
	assertStatusE2E(t, deleteCreate, http.StatusCreated)
	assertSearchHasPathE2E(t, querySearchE2E(t, server, searchMount+"?q="+deleteNeedle+"&scope=wiki"), publicWikiMount+"/e2e/delete-me.md")
	deleteResult := doE2ERequest(t, server, http.MethodDelete, deletePath, "", "curl/8.7.1")
	assertStatusE2E(t, deleteResult, http.StatusNoContent)
	assertStatusE2E(t, doE2ERequest(t, server, http.MethodGet, deletePath, "", "curl/8.7.1"), http.StatusNotFound)
	assertSearchNoResultsE2E(t, querySearchE2E(t, server, searchMount+"?q="+deleteNeedle+"&scope=wiki"))
	indexRead := doE2ERequest(t, server, http.MethodGet, wikiMount, "", "curl/8.7.1")
	assertStatusE2E(t, indexRead, http.StatusOK)
	assertNotContainsE2E(t, indexRead.body, publicWikiMount+"/e2e/delete-me.md")
}

func assertGeneratedIndexE2E(t *testing.T, server *httptest.Server, path, publicLink string) {
	t.Helper()
	res := doE2ERequest(t, server, http.MethodGet, path, "", "curl/8.7.1")
	assertStatusE2E(t, res, http.StatusOK)
	assertHeaderE2E(t, res, "Content-Type", "text/markdown; charset=utf-8")
	assertContainsE2E(t, res.body, autoIndexMarker)
	assertContainsE2E(t, res.body, publicLink)
	assertNotContainsE2E(t, res.body, "__wiki")
}

func mustWriteE2EFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q) returned error: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q) returned error: %v", path, err)
	}
}

func assertStatusE2E(t *testing.T, res e2eHTTPResult, want int) {
	t.Helper()
	if res.status != want {
		t.Fatalf("status = %d, want %d; body=%q", res.status, want, res.body)
	}
}

func assertHeaderE2E(t *testing.T, res e2eHTTPResult, name, want string) {
	t.Helper()
	if got := res.header.Get(name); got != want {
		t.Fatalf("header %s = %q, want %q; body=%q", name, got, want, res.body)
	}
}

func assertContainsE2E(t *testing.T, got, wantSubstring string) {
	t.Helper()
	if !strings.Contains(got, wantSubstring) {
		t.Fatalf("expected %q to contain %q", got, wantSubstring)
	}
}

func assertNotContainsE2E(t *testing.T, got, unwantedSubstring string) {
	t.Helper()
	if strings.Contains(got, unwantedSubstring) {
		t.Fatalf("expected %q not to contain %q", got, unwantedSubstring)
	}
}

func assertFileExistsE2E(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%q) returned error: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%q is a directory, want file", path)
	}
}

func assertFileContainsE2E(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) returned error: %v", path, err)
	}
	assertContainsE2E(t, string(data), want)
}

func assertSearchStateE2E(t *testing.T, result search.QueryResult, want string) {
	t.Helper()
	if result.IndexState != want {
		t.Fatalf("IndexState = %q, want %q; result=%#v", result.IndexState, want, result)
	}
}

func assertSearchHasPathE2E(t *testing.T, result search.QueryResult, path string) {
	t.Helper()
	if result.IndexState != search.StateReady && result.IndexState != search.StateIndexing {
		t.Fatalf("search state = %q, want ready or indexing; result=%#v", result.IndexState, result)
	}
	for _, item := range result.Results {
		if item.Path == path {
			return
		}
	}
	t.Fatalf("search results missing %s: %#v", path, result.Results)
}

func assertSearchNoPathE2E(t *testing.T, result search.QueryResult, path string) {
	t.Helper()
	for _, item := range result.Results {
		if item.Path == path {
			t.Fatalf("search results unexpectedly contained %s: %#v", path, result.Results)
		}
	}
}

func assertSearchNoResultsE2E(t *testing.T, result search.QueryResult) {
	t.Helper()
	if len(result.Results) != 0 {
		t.Fatalf("search results = %#v, want none", result.Results)
	}
}
