package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alienshard/internal/search"
)

func TestSearchHTTPQueryAndStatus(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.WriteFile(filepath.Join(rawRoot, "doc.md"), []byte("# Searchable\n\nhttpquery needle"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	if _, err := search.Rebuild(context.Background(), rawRoot); err != nil {
		t.Fatalf("search.Rebuild returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)

	statusRR := httptest.NewRecorder()
	handler.ServeHTTP(statusRR, httptest.NewRequest(http.MethodGet, "/search/status", nil))
	if statusRR.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", statusRR.Code, http.StatusOK)
	}
	var status search.Status
	if err := json.Unmarshal(statusRR.Body.Bytes(), &status); err != nil {
		t.Fatalf("json.Unmarshal status returned error: %v", err)
	}
	if status.State != search.StateReady {
		t.Fatalf("status.State = %q, want %q", status.State, search.StateReady)
	}

	queryRR := httptest.NewRecorder()
	handler.ServeHTTP(queryRR, httptest.NewRequest(http.MethodGet, "/search?q=httpquery&scope=raw&limit=5", nil))
	if queryRR.Code != http.StatusOK {
		t.Fatalf("query code = %d, want %d; body=%q", queryRR.Code, http.StatusOK, queryRR.Body.String())
	}
	if got := queryRR.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type = %q, want JSON", got)
	}
	var result search.QueryResult
	if err := json.Unmarshal(queryRR.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal query returned error: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Path != "/n/default/raw/doc.md" {
		t.Fatalf("results = %#v, want /n/default/raw/doc.md", result.Results)
	}
}

func TestSearchHTTPReportsNotIndexed(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	handler := newMountedHandler(rawRoot, filepath.Join(rawRoot, wikiDirName))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/search?q=missing", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d, want %d", rr.Code, http.StatusOK)
	}
	var result search.QueryResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if result.IndexState != search.StateNotIndexed {
		t.Fatalf("IndexState = %q, want %q", result.IndexState, search.StateNotIndexed)
	}
}

func TestSearchHTTPReindex(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.WriteFile(filepath.Join(rawRoot, "doc.md"), []byte("# Reindex\n\nreindexneedle"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, wikiRoot)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/search/reindex", nil))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("code = %d, want %d; body=%q", rr.Code, http.StatusAccepted, rr.Body.String())
	}

	waitForSearchState(t, handler, search.StateReady)

	queryRR := httptest.NewRecorder()
	handler.ServeHTTP(queryRR, httptest.NewRequest(http.MethodGet, "/search?q=reindexneedle", nil))
	if queryRR.Code != http.StatusOK {
		t.Fatalf("query code = %d, want %d", queryRR.Code, http.StatusOK)
	}
	var result search.QueryResult
	if err := json.Unmarshal(queryRR.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Path != "/n/default/raw/doc.md" {
		t.Fatalf("results = %#v, want /n/default/raw/doc.md", result.Results)
	}
}

func TestWikiMutationUpdatesSearchIndex(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if _, err := search.Rebuild(context.Background(), rawRoot); err != nil {
		t.Fatalf("search.Rebuild returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, wikiRoot)

	putReq := httptest.NewRequest(http.MethodPut, "/wiki/live.md", strings.NewReader("# Live\n\nliveunique needle"))
	putRR := httptest.NewRecorder()
	handler.ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusCreated {
		t.Fatalf("PUT code = %d, want %d; body=%q", putRR.Code, http.StatusCreated, putRR.Body.String())
	}

	queryRR := httptest.NewRecorder()
	handler.ServeHTTP(queryRR, httptest.NewRequest(http.MethodGet, "/search?q=liveunique&scope=wiki", nil))
	var result search.QueryResult
	if err := json.Unmarshal(queryRR.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal query returned error: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Path != "/n/default/wiki/live.md" {
		t.Fatalf("results after PUT = %#v, want /n/default/wiki/live.md", result.Results)
	}

	deleteRR := httptest.NewRecorder()
	handler.ServeHTTP(deleteRR, httptest.NewRequest(http.MethodDelete, "/wiki/live.md", nil))
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("DELETE code = %d, want %d; body=%q", deleteRR.Code, http.StatusNoContent, deleteRR.Body.String())
	}

	queryRR = httptest.NewRecorder()
	handler.ServeHTTP(queryRR, httptest.NewRequest(http.MethodGet, "/search?q=liveunique&scope=wiki", nil))
	if err := json.Unmarshal(queryRR.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal query after delete returned error: %v", err)
	}
	if len(result.Results) != 0 {
		t.Fatalf("results after DELETE = %#v, want none", result.Results)
	}
}

func TestNamespacedSearchHTTPIsIsolated(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	namespaceRoot := namespaceRawRoot(rawRoot, "research")
	if err := os.MkdirAll(namespaceRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rawRoot, "default.md"), []byte("# Default\n\nsharedneedle"), 0o644); err != nil {
		t.Fatalf("os.WriteFile default returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(namespaceRoot, "research.md"), []byte("# Research\n\nsharedneedle"), 0o644); err != nil {
		t.Fatalf("os.WriteFile research returned error: %v", err)
	}
	if _, err := search.Rebuild(context.Background(), rawRoot); err != nil {
		t.Fatalf("default Rebuild returned error: %v", err)
	}
	if _, err := search.RebuildNamespace(context.Background(), namespaceRoot, "research"); err != nil {
		t.Fatalf("research RebuildNamespace returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, filepath.Join(rawRoot, wikiDirName))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/n/research/search?q=sharedneedle&scope=raw", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d, want %d; body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
	var result search.QueryResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if len(result.Results) != 1 || result.Results[0].Path != "/n/research/raw/research.md" {
		t.Fatalf("results = %#v, want only /n/research/raw/research.md", result.Results)
	}
}

func waitForSearchState(t *testing.T, handler http.Handler, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/search/status", nil))
		var status search.Status
		if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
			t.Fatalf("json.Unmarshal status returned error: %v", err)
		}
		if status.State == want {
			return
		}
		if status.State == search.StateError {
			t.Fatalf("search status entered error: %#v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for search state %q", want)
}
