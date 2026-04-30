package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHomeDirDefaultToCWD(t *testing.T) {
	t.Parallel()

	got, err := resolveHomeDir("")
	if err != nil {
		t.Fatalf("resolveHomeDir returned error: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}

	want, err := filepath.Abs(cwd)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}

	if got != want {
		t.Fatalf("resolveHomeDir() = %q, want %q", got, want)
	}
}

func TestResolveHomeDirExplicitDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	got, err := resolveHomeDir(tmpDir)
	if err != nil {
		t.Fatalf("resolveHomeDir returned error: %v", err)
	}

	want, err := filepath.Abs(tmpDir)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}

	if got != want {
		t.Fatalf("resolveHomeDir() = %q, want %q", got, want)
	}
}

func TestResolveHomeDirMissingDir(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := resolveHomeDir(missing)
	if err == nil {
		t.Fatal("expected error for missing directory, got nil")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing directory error, got: %v", err)
	}
}

func TestResolveHomeDirFilePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	_, err := resolveHomeDir(filePath)
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}

	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected not-a-directory error, got: %v", err)
	}
}

func TestRawMarkdownBrowserUserAgentReturnsHTML(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.WriteFile(filepath.Join(rawRoot, "doc.md"), []byte("# Hello\n\nWorld"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)
	req := httptest.NewRequest(http.MethodGet, "/raw/doc.md", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/126.0")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q, want %q", got, "text/html; charset=utf-8")
	}
	if !strings.Contains(rr.Body.String(), "<h1>Hello</h1>") {
		t.Fatalf("expected rendered HTML heading, got body: %q", rr.Body.String())
	}
}

func TestRawMarkdownNonBrowserUserAgentReturnsRawMarkdown(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	contents := "# Hello\n\nWorld"
	if err := os.WriteFile(filepath.Join(rawRoot, "doc.md"), []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)
	req := httptest.NewRequest(http.MethodGet, "/raw/doc.md", nil)
	req.Header.Set("User-Agent", "curl/8.7.1")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("content-type = %q, want %q", got, "text/markdown; charset=utf-8")
	}
	if rr.Body.String() != contents {
		t.Fatalf("body = %q, want %q", rr.Body.String(), contents)
	}
}

func TestWikiMarkdownUsesSameRenderingRules(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	contents := "# Wiki\n\nPage"
	if err := os.WriteFile(filepath.Join(wikiRoot, "page.md"), []byte(contents), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)

	browserReq := httptest.NewRequest(http.MethodGet, "/wiki/page.md", nil)
	browserReq.Header.Set("User-Agent", "Mozilla/5.0 Firefox/125.0")
	browserRR := httptest.NewRecorder()
	handler.ServeHTTP(browserRR, browserReq)

	if browserRR.Code != http.StatusOK {
		t.Fatalf("browser status = %d, want %d", browserRR.Code, http.StatusOK)
	}
	if got := browserRR.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("browser content-type = %q, want %q", got, "text/html; charset=utf-8")
	}

	agentReq := httptest.NewRequest(http.MethodGet, "/wiki/page.md", nil)
	agentReq.Header.Set("User-Agent", "curl/8.7.1")
	agentRR := httptest.NewRecorder()
	handler.ServeHTTP(agentRR, agentReq)

	if agentRR.Code != http.StatusOK {
		t.Fatalf("agent status = %d, want %d", agentRR.Code, http.StatusOK)
	}
	if got := agentRR.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Fatalf("agent content-type = %q, want %q", got, "text/markdown; charset=utf-8")
	}
	if agentRR.Body.String() != contents {
		t.Fatalf("agent body = %q, want %q", agentRR.Body.String(), contents)
	}
}

func TestRawWikiSubtreeBlocked(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(filepath.Join(rawRoot, wikiDirName), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rawRoot, wikiDirName, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)
	req := httptest.NewRequest(http.MethodGet, "/raw/__wiki/secret.md", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestRawDirectoryListingSkipsWikiRoot(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rawRoot, "public.txt"), []byte("public"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wikiRoot, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)
	for _, path := range []string{"/raw", "/raw/"} {
		path := path
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}
			if !strings.Contains(rr.Body.String(), `href="public.txt"`) {
				t.Fatalf("expected public file in raw listing, got %q", rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), `href="__wiki/"`) {
				t.Fatalf("expected raw listing to skip %s, got %q", wikiDirName, rr.Body.String())
			}
		})
	}
}

func TestUnknownPathReturnsNotFound(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)
	req := httptest.NewRequest(http.MethodGet, "/doc.md", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestWikiPutCreatesAndUpdatesMarkdownFile(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	createReq := httptest.NewRequest(http.MethodPut, "/wiki/filex.md", strings.NewReader("# First"))
	createRR := httptest.NewRecorder()
	handler.ServeHTTP(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRR.Code, http.StatusCreated)
	}

	data, err := os.ReadFile(filepath.Join(wikiRoot, "filex.md"))
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if string(data) != "# First" {
		t.Fatalf("wiki file content = %q, want %q", string(data), "# First")
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/wiki/filex.md", strings.NewReader("# Second"))
	updateRR := httptest.NewRecorder()
	handler.ServeHTTP(updateRR, updateReq)

	if updateRR.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRR.Code, http.StatusOK)
	}
}

func TestWikiPutRejectsNonMarkdown(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/file.txt", strings.NewReader("x"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestWikiPutRejectsTraversalPath(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/../escape.md", strings.NewReader("x"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestWikiIndexGeneratedWhenMissingOnGet(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wikiRoot, "page.md"), []byte("# Page"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodGet, "/wiki/index.md", nil)
	req.Header.Set("User-Agent", "curl/8.7.1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	indexData, err := os.ReadFile(filepath.Join(wikiRoot, "index.md"))
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if !strings.HasPrefix(string(indexData), autoIndexMarker) {
		t.Fatalf("expected generated index marker, got %q", string(indexData))
	}
	if !strings.Contains(string(indexData), "- [page](/wiki/page.md)") {
		t.Fatalf("expected generated index to link through /wiki, got %q", string(indexData))
	}
	if rr.Body.String() != string(indexData) {
		t.Fatalf("response body = %q, want generated index %q", rr.Body.String(), string(indexData))
	}
}

func TestWikiRootServesGeneratedIndex(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/wiki", "/wiki/"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			rawRoot := t.TempDir()
			wikiRoot := filepath.Join(rawRoot, wikiDirName)
			if err := os.MkdirAll(filepath.Join(wikiRoot, "nested"), 0o755); err != nil {
				t.Fatalf("os.MkdirAll returned error: %v", err)
			}
			if err := os.WriteFile(filepath.Join(wikiRoot, "hello-world.md"), []byte("# Hello World"), 0o644); err != nil {
				t.Fatalf("os.WriteFile returned error: %v", err)
			}
			if err := os.WriteFile(filepath.Join(wikiRoot, "nested", "index.md"), []byte("# Nested Index"), 0o644); err != nil {
				t.Fatalf("os.WriteFile returned error: %v", err)
			}

			handler := newMountedHandler(rawRoot, wikiRoot)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/126.0")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}
			if got := rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Fatalf("content-type = %q, want %q", got, "text/html; charset=utf-8")
			}
			if !strings.Contains(rr.Body.String(), `href="/wiki/hello-world.md"`) {
				t.Fatalf("expected generated /wiki link, got %q", rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), `href="hello-world.md"`) {
				t.Fatalf("expected no file server relative link, got %q", rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), `href="/wiki/index.md"`) || strings.Contains(rr.Body.String(), `href="/wiki/nested/index.md"`) {
				t.Fatalf("expected no autoindex self-links, got %q", rr.Body.String())
			}

			pageReq := httptest.NewRequest(http.MethodGet, "/wiki/hello-world.md", nil)
			pageReq.Header.Set("User-Agent", "Mozilla/5.0 Chrome/126.0")
			pageRR := httptest.NewRecorder()
			handler.ServeHTTP(pageRR, pageReq)
			if pageRR.Code != http.StatusOK {
				t.Fatalf("linked page status = %d, want %d", pageRR.Code, http.StatusOK)
			}
		})
	}
}

func TestWikiIndexRefreshedWhenManagedOnGet(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wikiRoot, "page.md"), []byte("# Page"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	staleIndex := autoIndexMarker + "\n\n# Index\n\n- [page](page.md)\n"
	if err := os.WriteFile(filepath.Join(wikiRoot, "index.md"), []byte(staleIndex), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)
	req := httptest.NewRequest(http.MethodGet, "/wiki/index.md", nil)
	req.Header.Set("User-Agent", "curl/8.7.1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if strings.Contains(rr.Body.String(), "- [page](page.md)") {
		t.Fatalf("expected stale relative link to be removed, got %q", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "- [page](/wiki/page.md)") {
		t.Fatalf("expected refreshed /wiki link, got %q", rr.Body.String())
	}
}

func TestWikiIndexRegeneratedWhenManaged(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	firstReq := httptest.NewRequest(http.MethodPut, "/wiki/beta.md", strings.NewReader("# Beta"))
	firstRR := httptest.NewRecorder()
	handler.ServeHTTP(firstRR, firstReq)
	if firstRR.Code != http.StatusCreated {
		t.Fatalf("first put status = %d, want %d", firstRR.Code, http.StatusCreated)
	}

	secondReq := httptest.NewRequest(http.MethodPut, "/wiki/alpha.md", strings.NewReader("# Alpha"))
	secondRR := httptest.NewRecorder()
	handler.ServeHTTP(secondRR, secondReq)
	if secondRR.Code != http.StatusCreated {
		t.Fatalf("second put status = %d, want %d", secondRR.Code, http.StatusCreated)
	}

	indexData, err := os.ReadFile(filepath.Join(wikiRoot, "index.md"))
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	index := string(indexData)
	if !strings.HasPrefix(index, autoIndexMarker) {
		t.Fatalf("expected generated index marker, got %q", index)
	}

	alphaPos := strings.Index(index, "- [alpha](/wiki/alpha.md)")
	betaPos := strings.Index(index, "- [beta](/wiki/beta.md)")
	if alphaPos < 0 || betaPos < 0 {
		t.Fatalf("expected alpha and beta entries in index, got %q", index)
	}
	if alphaPos > betaPos {
		t.Fatalf("expected lexical ordering in index, got %q", index)
	}
}

func TestWikiIndexNotModifiedWhenManual(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}

	manualIndex := "# Index\n\n- custom entry\n"
	if err := os.WriteFile(filepath.Join(wikiRoot, "index.md"), []byte(manualIndex), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)
	req := httptest.NewRequest(http.MethodPut, "/wiki/new.md", strings.NewReader("# New"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	indexData, err := os.ReadFile(filepath.Join(wikiRoot, "index.md"))
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if string(indexData) != manualIndex {
		t.Fatalf("index.md changed unexpectedly: got %q want %q", string(indexData), manualIndex)
	}
}

func TestWikiPutIndexAllowedAndOwnershipInferredByMarker(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	manualReq := httptest.NewRequest(http.MethodPut, "/wiki/index.md", strings.NewReader("# Manual\n"))
	manualRR := httptest.NewRecorder()
	handler.ServeHTTP(manualRR, manualReq)
	if manualRR.Code != http.StatusCreated {
		t.Fatalf("manual put status = %d, want %d", manualRR.Code, http.StatusCreated)
	}

	manualIndex, err := os.ReadFile(filepath.Join(wikiRoot, "index.md"))
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if strings.Contains(string(manualIndex), autoIndexMarker) {
		t.Fatalf("expected manual index without marker, got %q", string(manualIndex))
	}

	managedReq := httptest.NewRequest(http.MethodPut, "/wiki/index.md", strings.NewReader(autoIndexMarker+"\n\n# Placeholder\n"))
	managedRR := httptest.NewRecorder()
	handler.ServeHTTP(managedRR, managedReq)
	if managedRR.Code != http.StatusOK {
		t.Fatalf("managed put status = %d, want %d", managedRR.Code, http.StatusOK)
	}

	managedIndex, err := os.ReadFile(filepath.Join(wikiRoot, "index.md"))
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if !strings.HasPrefix(string(managedIndex), autoIndexMarker) {
		t.Fatalf("expected managed index marker, got %q", string(managedIndex))
	}
}
