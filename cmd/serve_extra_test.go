package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestServeCommandRejectsInvalidConfiguration(t *testing.T) {
	originalBind := serveViper.GetString("bind")
	originalPort := serveViper.GetInt("port")
	originalHomeDir := serveViper.GetString("home_dir")
	t.Cleanup(func() {
		serveViper.Set("bind", originalBind)
		serveViper.Set("port", originalPort)
		serveViper.Set("home_dir", originalHomeDir)
	})

	homeDir := t.TempDir()
	serveViper.Set("home_dir", homeDir)
	serveViper.Set("bind", "not-an-ip")
	serveViper.Set("port", defaultPort)
	if err := serveCmd.RunE(serveCmd, nil); err == nil || !strings.Contains(err.Error(), "invalid --bind") {
		t.Fatalf("expected invalid bind error, got %v", err)
	}

	serveViper.Set("bind", defaultBind)
	serveViper.Set("port", 0)
	if err := serveCmd.RunE(serveCmd, nil); err == nil || !strings.Contains(err.Error(), "invalid --port") {
		t.Fatalf("expected invalid port error, got %v", err)
	}

	serveViper.Set("port", defaultPort)
	serveViper.Set("home_dir", filepath.Join(homeDir, "missing"))
	if err := serveCmd.RunE(serveCmd, nil); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing home directory error, got %v", err)
	}
}

func TestServeCommandStartsConfiguredServer(t *testing.T) {
	originalBind := serveViper.GetString("bind")
	originalPort := serveViper.GetInt("port")
	originalHomeDir := serveViper.GetString("home_dir")
	originalListenAndServe := listenAndServe
	t.Cleanup(func() {
		serveViper.Set("bind", originalBind)
		serveViper.Set("port", originalPort)
		serveViper.Set("home_dir", originalHomeDir)
		listenAndServe = originalListenAndServe
	})

	homeDir := t.TempDir()
	serveViper.Set("home_dir", homeDir)
	serveViper.Set("bind", "127.0.0.1")
	serveViper.Set("port", 12345)

	wantErr := errors.New("stop before binding")
	listenAndServe = func(srv *http.Server) error {
		if srv.Addr != "127.0.0.1:12345" {
			t.Fatalf("server address = %q, want %q", srv.Addr, "127.0.0.1:12345")
		}
		if srv.Handler == nil {
			t.Fatal("server handler is nil")
		}
		return wantErr
	}

	if err := serveCmd.RunE(serveCmd, nil); !errors.Is(err, wantErr) {
		t.Fatalf("RunE error = %v, want %v", err, wantErr)
	}
}

func TestMountedHandlerServesStaticFilesAndForwardsUnsupportedMethods(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.WriteFile(filepath.Join(rawRoot, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	handler := newMountedHandler(rawRoot, wikiRoot)

	getReq := httptest.NewRequest(http.MethodGet, "/raw/hello.txt", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRR.Code, http.StatusOK)
	}
	if strings.TrimSpace(getRR.Body.String()) != "hello" {
		t.Fatalf("body = %q, want %q", getRR.Body.String(), "hello")
	}

	postReq := httptest.NewRequest(http.MethodPost, "/raw/hello.txt", nil)
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("post status = %d, want %d", postRR.Code, http.StatusOK)
	}
}

func TestMountedHandlerFallsBackForMissingMarkdown(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodGet, "/raw/missing.md", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestWikiRejectsUnsupportedMethod(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPost, "/wiki/page.md", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestWikiIndexGenerationErrorReturnsServerError(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.WriteFile(wikiRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodGet, "/wiki/index.md", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestWikiPutCreatesNestedMarkdownFile(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/nested/Topic.MD", strings.NewReader("# Nested"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	data, err := os.ReadFile(filepath.Join(wikiRoot, "nested", "Topic.MD"))
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if string(data) != "# Nested" {
		t.Fatalf("body = %q, want %q", string(data), "# Nested")
	}
}

func TestWikiPutRejectsDirectoryTarget(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(filepath.Join(wikiRoot, "folder.md"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/folder.md", strings.NewReader("# Folder"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestWikiPutFailsWhenWikiRootCannotBeCreated(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.WriteFile(wikiRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/page.md", strings.NewReader("# Page"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestWikiPutFailsWhenParentPathCannotBeCreated(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wikiRoot, "blocked"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/blocked/page.md", strings.NewReader("# Page"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestWikiPutFailsWhenManagedIndexCannotBeRead(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	if err := os.MkdirAll(filepath.Join(wikiRoot, "index.md"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/page.md", strings.NewReader("# Page"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestWikiPutFailsWhenFileCannotBeWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write through read-only directory permissions")
	}

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	readOnlyDir := filepath.Join(wikiRoot, "readonly")
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	if err := os.Mkdir(readOnlyDir, 0o500); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0o700) })
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/readonly/page.md", strings.NewReader("# Page"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestWikiRelativeMarkdownPathValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantRel    string
		wantStatus int
	}{
		{name: "missing wiki prefix", path: "/raw/page.md", wantStatus: http.StatusForbidden},
		{name: "wiki root", path: "/wiki", wantStatus: http.StatusBadRequest},
		{name: "wiki root slash", path: "/wiki/", wantStatus: http.StatusBadRequest},
		{name: "directory", path: "/wiki/dir/", wantStatus: http.StatusBadRequest},
		{name: "backslash", path: "/wiki/a\\b.md", wantStatus: http.StatusForbidden},
		{name: "dot part", path: "/wiki/./page.md", wantStatus: http.StatusForbidden},
		{name: "empty part", path: "/wiki/a//page.md", wantStatus: http.StatusForbidden},
		{name: "parent part", path: "/wiki/../page.md", wantStatus: http.StatusForbidden},
		{name: "non markdown", path: "/wiki/page.txt", wantStatus: http.StatusBadRequest},
		{name: "valid", path: "/wiki/dir/page.md", wantRel: "dir/page.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRel, gotStatus, gotMsg := wikiRelativeMarkdownPath(tt.path)
			if gotRel != tt.wantRel || gotStatus != tt.wantStatus {
				t.Fatalf("wikiRelativeMarkdownPath(%q) = (%q, %d, %q), want (%q, %d, _)", tt.path, gotRel, gotStatus, gotMsg, tt.wantRel, tt.wantStatus)
			}
			if tt.wantStatus != 0 && gotMsg == "" {
				t.Fatal("expected validation message")
			}
		})
	}
}

func TestPathWithinRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "root")
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "root", target: root, want: true},
		{name: "child", target: filepath.Join(root, "child.md"), want: true},
		{name: "normalized child", target: filepath.Join(root, "nested", "..", "child.md"), want: true},
		{name: "parent", target: filepath.Dir(root), want: false},
		{name: "sibling", target: root + "-sibling", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPathWithinRoot(root, tt.target); got != tt.want {
				t.Fatalf("isPathWithinRoot(%q, %q) = %v, want %v", root, tt.target, got, tt.want)
			}
		})
	}
}

func TestReadMarkdownFileSpecialCases(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder.md"), 0o755); err != nil {
		t.Fatalf("os.Mkdir returned error: %v", err)
	}

	_, ok, err := readMarkdownFile(http.Dir(root), "/missing.md")
	if err != nil || ok {
		t.Fatalf("missing file returned ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	_, ok, err = readMarkdownFile(http.Dir(root), "/folder.md")
	if err != nil || ok {
		t.Fatalf("directory returned ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	_, ok, err = readMarkdownFile(http.Dir(string([]byte{0})), "/file.md")
	if err == nil || ok {
		t.Fatalf("invalid root returned ok=%v err=%v, want ok=false err!=nil", ok, err)
	}
}

func TestIndexHelpers(t *testing.T) {
	t.Parallel()

	wikiRoot := t.TempDir()
	manualIndexPath := filepath.Join(wikiRoot, "index.md")
	manualIndex := []byte("# Manual\n")
	if err := os.WriteFile(manualIndexPath, manualIndex, 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
	if err := ensureGeneratedIndexIfMissing(wikiRoot); err != nil {
		t.Fatalf("ensureGeneratedIndexIfMissing returned error: %v", err)
	}
	data, err := os.ReadFile(manualIndexPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if string(data) != string(manualIndex) {
		t.Fatalf("index changed to %q, want %q", string(data), string(manualIndex))
	}

	if err := os.MkdirAll(filepath.Join(wikiRoot, "nested"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}
	for path, body := range map[string]string{
		"nested/b.md": "# B",
		"a.md":        "# A",
		"note.txt":    "ignore",
	} {
		if err := os.WriteFile(filepath.Join(wikiRoot, filepath.FromSlash(path)), []byte(body), 0o644); err != nil {
			t.Fatalf("os.WriteFile(%q) returned error: %v", path, err)
		}
	}

	pages, err := collectWikiMarkdownPaths(wikiRoot)
	if err != nil {
		t.Fatalf("collectWikiMarkdownPaths returned error: %v", err)
	}
	wantPages := []string{"a.md", "nested/b.md"}
	if !reflect.DeepEqual(pages, wantPages) {
		t.Fatalf("pages = %#v, want %#v", pages, wantPages)
	}

	if _, err := collectWikiMarkdownPaths(filepath.Join(wikiRoot, "missing")); err == nil {
		t.Fatal("expected missing root error, got nil")
	}
	if err := writeGeneratedIndex(filepath.Join(wikiRoot, "missing"), filepath.Join(wikiRoot, "missing.md")); err == nil {
		t.Fatal("expected collect error from writeGeneratedIndex, got nil")
	}
	if err := ensureGeneratedIndexIfMissing(filepath.Join(wikiRoot, "note.txt")); err == nil {
		t.Fatal("expected ensureGeneratedIndexIfMissing error for file root, got nil")
	}
	if err := refreshGeneratedIndex(filepath.Join(wikiRoot, "note.txt")); err == nil {
		t.Fatal("expected refreshGeneratedIndex error for file root, got nil")
	}

	if err := os.WriteFile(filepath.Join(wikiRoot, ".md"), []byte("# Dot"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	managedIndexPath := filepath.Join(wikiRoot, "managed.md")
	if err := writeGeneratedIndex(wikiRoot, managedIndexPath); err != nil {
		t.Fatalf("writeGeneratedIndex returned error: %v", err)
	}
	managedIndex, err := os.ReadFile(managedIndexPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(managedIndex), "- [a](/wiki/a.md)") || !strings.Contains(string(managedIndex), "- [.md](/wiki/.md)") || !strings.Contains(string(managedIndex), "- [b](/wiki/nested/b.md)") {
		t.Fatalf("generated index missing expected links: %q", string(managedIndex))
	}
}

func TestResolveHomeDirReportsStatError(t *testing.T) {
	t.Parallel()

	_, err := resolveHomeDir(string([]byte{0}))
	if err == nil || !strings.Contains(err.Error(), "cannot access home directory") {
		t.Fatalf("expected access error, got %v", err)
	}
}

func TestServeMountedPathReportsReadErrors(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/raw/file.md", nil)
	rr := httptest.NewRecorder()
	serveMountedPath(rr, req, http.Dir(string([]byte{0})), http.NotFoundHandler(), "/file.md")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHelperPredicates(t *testing.T) {
	t.Parallel()

	if !isRawPath("/raw") || !isRawPath("/raw/file.txt") || isRawPath("/rawish") {
		t.Fatal("isRawPath returned unexpected result")
	}
	if !isWikiPath("/wiki") || !isWikiPath("/wiki/page.md") || isWikiPath("/wikia") {
		t.Fatal("isWikiPath returned unexpected result")
	}
	if !isBlockedRawPath("/raw/__wiki") || !isBlockedRawPath("/raw/__wiki/page.md") || isBlockedRawPath("/raw/__wikia/page.md") {
		t.Fatal("isBlockedRawPath returned unexpected result")
	}
	if stripMountPath("/raw", "/raw") != "/" || stripMountPath("/raw/page.md", "/raw") != "/page.md" {
		t.Fatal("stripMountPath returned unexpected result")
	}
	if !isMarkdownPath("/PAGE.MD") || isMarkdownPath("/page.txt") {
		t.Fatal("isMarkdownPath returned unexpected result")
	}
	if !isBrowserUserAgent("Mozilla FIREFOX") || !isBrowserUserAgent("Mozilla Chrome") || isBrowserUserAgent("curl") {
		t.Fatal("isBrowserUserAgent returned unexpected result")
	}
	if !hasAutoIndexMarker([]byte(autoIndexMarker+"\n# Index")) || hasAutoIndexMarker([]byte("# Manual\n"+autoIndexMarker)) {
		t.Fatal("hasAutoIndexMarker returned unexpected result")
	}
}

func TestMustBindFlagPanicsForMissingFlag(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic")
		}
	}()

	mustBindFlag("missing", "does-not-exist")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

func TestWikiPutRejectsUnreadableBody(t *testing.T) {
	t.Parallel()

	rawRoot := t.TempDir()
	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	handler := newMountedHandler(rawRoot, wikiRoot)

	req := httptest.NewRequest(http.MethodPut, "/wiki/page.md", failingReader{})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
