package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"alienshard/internal/search"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yuin/goldmark"
)

const (
	defaultBind      = "127.0.0.1"
	defaultPort      = 8000
	wikiDirName      = "__wiki"
	namespaceDirName = "__namespaces"
	searchDirName    = ".alienshard"
	defaultNamespace = "default"
	autoIndexMarker  = "<!-- alienshard:autoindex v1 -->"
	homeDirKey       = "home_dir"
	bindKey          = "bind"
	portKey          = "port"
	homeDirEnv       = "ALIEN_HOME_DIR"
	bindEnv          = "ALIEN_BIND"
	portEnv          = "ALIEN_PORT"
)

var serveViper = viper.New()

var listenAndServe = (*http.Server).ListenAndServe

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve static files over HTTP",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServe(serveViper)
	},
}

func runServe(config *viper.Viper) error {
	bind := strings.TrimSpace(config.GetString(bindKey))
	if net.ParseIP(bind) == nil {
		return fmt.Errorf("invalid --bind value %q: must be a valid IP address", bind)
	}

	port := config.GetInt(portKey)
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid --port value %d: must be in range 1-65535", port)
	}

	homeDir, err := resolveHomeDir(config.GetString(homeDirKey))
	if err != nil {
		return err
	}
	wikiDir := filepath.Join(homeDir, wikiDirName)

	addr := net.JoinHostPort(bind, strconv.Itoa(port))
	handler := newMountedHandler(homeDir, wikiDir)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Serving %s at http://%s", homeDir, addr)
	return listenAndServe(srv)
}

func newMountedHandler(rawRoot, wikiRoot string) http.Handler {
	return newMountedHandlerWithSearch(rawRoot, wikiRoot)
}

func newMountedHandlerWithSearch(rawRoot, wikiRoot string) http.Handler {
	searchServices := newNamespaceSearchServices(rawRoot)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, ok, err := resolveNamespaceRoute(rawRoot, r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}

		if route.mount == "search" {
			handleSearch(w, r, route.strippedPath, searchServices.get(route.namespace, route.rawRoot))
			return
		}

		if route.mount == search.ScopeRaw {
			if isBlockedRawPath(route.strippedPath) {
				http.NotFound(w, r)
				return
			}

			rawFileSystem := rootFilteredFileSystem{fileSystem: http.Dir(route.rawRoot), hiddenNames: []string{wikiDirName, namespaceDirName, searchDirName}}
			rawFileServer := http.FileServer(rawFileSystem)
			serveMountedPath(w, r, rawFileSystem, rawFileServer, route.strippedPath)
			return
		}

		if route.mount == search.ScopeWiki {
			if r.Method == http.MethodPut {
				handleWikiPut(w, r, route)
				return
			}
			if r.Method == http.MethodDelete {
				handleWikiDelete(w, r, route)
				return
			}

			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			strippedPath := route.strippedPath
			if strippedPath == "/" {
				strippedPath = "/index.md"
			}

			if strippedPath == "/index.md" {
				if err := refreshGeneratedIndex(route.wikiRoot, route.wikiMount); err != nil {
					http.Error(w, "failed to generate index", http.StatusInternalServerError)
					return
				}
			}

			wikiFileSystem := http.Dir(route.wikiRoot)
			wikiFileServer := http.FileServer(wikiFileSystem)
			serveMountedPath(w, r, wikiFileSystem, wikiFileServer, strippedPath)
			return
		}

		http.NotFound(w, r)
	})
}

type namespaceRoute struct {
	namespace    string
	mount        string
	strippedPath string
	rawRoot      string
	wikiRoot     string
	wikiMount    string
}

type namespaceSearchServices struct {
	homeRoot string
	mu       sync.Mutex
	services map[string]*search.Service
}

func newNamespaceSearchServices(homeRoot string) *namespaceSearchServices {
	return &namespaceSearchServices{homeRoot: homeRoot, services: map[string]*search.Service{}}
}

func (services *namespaceSearchServices) get(namespace, rawRoot string) *search.Service {
	services.mu.Lock()
	defer services.mu.Unlock()
	service, ok := services.services[namespace]
	if !ok {
		service = search.NewNamespaceService(rawRoot, namespace)
		services.services[namespace] = service
	}
	return service
}

func resolveNamespaceRoute(homeRoot, requestPath string) (namespaceRoute, bool, error) {
	if isRawPath(requestPath) {
		return makeNamespaceRoute(homeRoot, defaultNamespace, search.ScopeRaw, stripMountPath(requestPath, "/raw")), true, nil
	}
	if isWikiPath(requestPath) {
		return makeNamespaceRoute(homeRoot, defaultNamespace, search.ScopeWiki, stripMountPath(requestPath, "/wiki")), true, nil
	}
	if isSearchPath(requestPath) {
		return makeNamespaceRoute(homeRoot, defaultNamespace, "search", stripMountPath(requestPath, "/search")), true, nil
	}

	if requestPath != "/n" && !strings.HasPrefix(requestPath, "/n/") {
		return namespaceRoute{}, false, nil
	}
	parts := strings.Split(strings.TrimPrefix(requestPath, "/n/"), "/")
	if requestPath == "/n" || len(parts) < 2 {
		return namespaceRoute{}, false, fmt.Errorf("namespace path must be /n/<namespace>/<raw|wiki|search>")
	}
	namespace := parts[0]
	if err := validateNamespaceName(namespace); err != nil {
		return namespaceRoute{}, false, err
	}
	mount := parts[1]
	if mount != search.ScopeRaw && mount != search.ScopeWiki && mount != "search" {
		return namespaceRoute{}, false, nil
	}
	strippedPath := "/"
	if len(parts) > 2 {
		strippedPath = "/" + strings.Join(parts[2:], "/")
	}
	return makeNamespaceRoute(homeRoot, namespace, mount, strippedPath), true, nil
}

func makeNamespaceRoute(homeRoot, namespace, mount, strippedPath string) namespaceRoute {
	rawRoot := namespaceRawRoot(homeRoot, namespace)
	return namespaceRoute{
		namespace:    namespace,
		mount:        mount,
		strippedPath: strippedPath,
		rawRoot:      rawRoot,
		wikiRoot:     filepath.Join(rawRoot, wikiDirName),
		wikiMount:    namespacePublicMount(namespace, search.ScopeWiki),
	}
}

func namespaceRawRoot(homeRoot, namespace string) string {
	if namespace == defaultNamespace {
		return homeRoot
	}
	return filepath.Join(homeRoot, namespaceDirName, namespace)
}

func namespacePublicMount(namespace, mount string) string {
	return "/n/" + namespace + "/" + mount
}

func validateNamespaceName(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace must not be empty")
	}
	if len(namespace) > 63 {
		return fmt.Errorf("invalid namespace %q: must be 63 characters or less", namespace)
	}
	if namespace == "." || namespace == ".." || strings.ContainsAny(namespace, `/\\`) {
		return fmt.Errorf("invalid namespace %q", namespace)
	}
	switch namespace {
	case wikiDirName, namespaceDirName, searchDirName:
		return fmt.Errorf("invalid namespace %q: reserved name", namespace)
	}
	for i, r := range namespace {
		valid := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
		if !valid {
			return fmt.Errorf("invalid namespace %q: use lowercase letters, digits, dots, dashes, or underscores", namespace)
		}
		if i == 0 && !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return fmt.Errorf("invalid namespace %q: must start with a lowercase letter or digit", namespace)
		}
	}
	return nil
}

func isSearchPath(requestPath string) bool {
	return requestPath == "/search" || requestPath == "/search/status" || requestPath == "/search/reindex"
}

func handleSearch(w http.ResponseWriter, r *http.Request, strippedPath string, searchService *search.Service) {
	switch strippedPath {
	case "/":
		handleSearchQuery(w, r, searchService)
	case "/status":
		handleSearchStatus(w, r, searchService)
	case "/reindex":
		handleSearchReindex(w, r, searchService)
	default:
		http.NotFound(w, r)
	}
}

func handleSearchQuery(w http.ResponseWriter, r *http.Request, searchService *search.Service) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "missing search query", http.StatusBadRequest)
		return
	}

	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = search.ScopeAll
	}
	if scope != search.ScopeAll && scope != search.ScopeRaw && scope != search.ScopeWiki {
		http.Error(w, "invalid search scope", http.StatusBadRequest)
		return
	}

	limit := 20
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			http.Error(w, "invalid search limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	result, err := searchService.Query(r.Context(), search.QueryOptions{Query: query, Scope: scope, Limit: limit})
	if err != nil {
		http.Error(w, "failed to search", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleSearchStatus(w http.ResponseWriter, r *http.Request, searchService *search.Service) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, searchService.Status())
}

func handleSearchReindex(w http.ResponseWriter, r *http.Request, searchService *search.Service) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := searchService.StartReindex(); err != nil {
		if search.IsLocked(err) {
			http.Error(w, "search index rebuild already in progress", http.StatusConflict)
			return
		}
		http.Error(w, "failed to start search reindex", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, searchService.Status())
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}

type rootFilteredFileSystem struct {
	fileSystem  http.FileSystem
	hiddenNames []string
}

func (fileSystem rootFilteredFileSystem) Open(name string) (http.File, error) {
	cleanName := filepath.ToSlash(filepath.Clean("/" + name))
	for _, hiddenName := range fileSystem.hiddenNames {
		hiddenRoot := "/" + hiddenName
		if cleanName == hiddenRoot || strings.HasPrefix(cleanName, hiddenRoot+"/") {
			return nil, fs.ErrNotExist
		}
	}

	file, err := fileSystem.fileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	if cleanName == "/" {
		return rootFilteredFile{File: file, hiddenNames: fileSystem.hiddenNames}, nil
	}

	return file, nil
}

type rootFilteredFile struct {
	http.File
	hiddenNames []string
}

func (file rootFilteredFile) Readdir(count int) ([]fs.FileInfo, error) {
	entries, err := file.File.Readdir(count)
	if len(entries) == 0 {
		return entries, err
	}

	filtered := entries[:0]
	for _, entry := range entries {
		if file.isHidden(entry.Name()) {
			continue
		}
		filtered = append(filtered, entry)
	}

	return filtered, err
}

func (file rootFilteredFile) isHidden(name string) bool {
	for _, hiddenName := range file.hiddenNames {
		if name == hiddenName {
			return true
		}
	}
	return false
}

func serveMountedPath(
	w http.ResponseWriter,
	r *http.Request,
	fileSystem http.FileSystem,
	fileServer http.Handler,
	strippedPath string,
) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		serveWithPath(fileServer, w, r, strippedPath)
		return
	}

	if !isMarkdownPath(strippedPath) {
		serveWithPath(fileServer, w, r, strippedPath)
		return
	}

	body, ok, err := readMarkdownFile(fileSystem, strippedPath)
	if err != nil {
		http.Error(w, "failed to read markdown file", http.StatusInternalServerError)
		return
	}
	if !ok {
		serveWithPath(fileServer, w, r, strippedPath)
		return
	}

	if isBrowserUserAgent(r.UserAgent()) {
		var rendered bytes.Buffer
		if err := goldmark.Convert(body, &rendered); err != nil {
			http.Error(w, "failed to render markdown", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(rendered.Bytes())
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write(body)
}

func serveWithPath(fileServer http.Handler, w http.ResponseWriter, r *http.Request, requestPath string) {
	req := r.Clone(r.Context())
	urlCopy := *r.URL
	urlCopy.Path = requestPath
	req.URL = &urlCopy
	fileServer.ServeHTTP(w, req)
}

func isRawPath(requestPath string) bool {
	return requestPath == "/raw" || strings.HasPrefix(requestPath, "/raw/")
}

func isWikiPath(requestPath string) bool {
	return requestPath == "/wiki" || strings.HasPrefix(requestPath, "/wiki/")
}

func isBlockedRawPath(strippedPath string) bool {
	if isRawPath(strippedPath) {
		strippedPath = stripMountPath(strippedPath, "/raw")
	}
	for _, hiddenName := range []string{wikiDirName, namespaceDirName, searchDirName} {
		hiddenPath := "/" + hiddenName
		if strippedPath == hiddenPath || strings.HasPrefix(strippedPath, hiddenPath+"/") {
			return true
		}
	}
	return false
}

func stripMountPath(requestPath, mount string) string {
	if requestPath == mount {
		return "/"
	}

	strippedPath := strings.TrimPrefix(requestPath, mount)
	if strippedPath == "" {
		return "/"
	}

	return strippedPath
}

func handleWikiPut(w http.ResponseWriter, r *http.Request, route namespaceRoute) {
	relPath, statusCode, msg := wikiRelativeMarkdownPath(route.strippedPath)
	if statusCode != 0 {
		http.Error(w, msg, statusCode)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(route.wikiRoot, 0o755); err != nil {
		http.Error(w, "failed to prepare wiki directory", http.StatusInternalServerError)
		return
	}

	filePath := filepath.Join(route.wikiRoot, filepath.FromSlash(relPath))
	if !isPathWithinRoot(route.wikiRoot, filePath) {
		http.Error(w, "invalid wiki path", http.StatusForbidden)
		return
	}

	stat, err := os.Stat(filePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, "failed to inspect wiki file", http.StatusInternalServerError)
		return
	}
	existed := err == nil
	if existed && stat.IsDir() {
		http.Error(w, "cannot write to a directory", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		http.Error(w, "failed to prepare wiki path", http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(filePath, body, 0o644); err != nil {
		http.Error(w, "failed to write wiki file", http.StatusInternalServerError)
		return
	}

	if err := refreshGeneratedIndex(route.wikiRoot, route.wikiMount); err != nil {
		http.Error(w, "failed to refresh index", http.StatusInternalServerError)
		return
	}
	if err := search.UpsertWikiDocumentNamespace(r.Context(), route.rawRoot, route.namespace, relPath); err != nil {
		http.Error(w, "failed to update search index", http.StatusInternalServerError)
		return
	}
	if relPath != "index.md" {
		if err := search.UpsertWikiDocumentNamespace(r.Context(), route.rawRoot, route.namespace, "index.md"); err != nil {
			http.Error(w, "failed to update search index", http.StatusInternalServerError)
			return
		}
	}

	if existed {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func handleWikiDelete(w http.ResponseWriter, r *http.Request, route namespaceRoute) {
	relPath, statusCode, msg := wikiRelativeMarkdownPath(route.strippedPath)
	if statusCode != 0 {
		http.Error(w, msg, statusCode)
		return
	}

	filePath := filepath.Join(route.wikiRoot, filepath.FromSlash(relPath))
	if !isPathWithinRoot(route.wikiRoot, filePath) {
		http.Error(w, "invalid wiki path", http.StatusForbidden)
		return
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to inspect wiki file", http.StatusInternalServerError)
		return
	}
	if stat.IsDir() {
		http.Error(w, "cannot delete a directory", http.StatusBadRequest)
		return
	}

	if err := os.Remove(filePath); err != nil {
		http.Error(w, "failed to delete wiki file", http.StatusInternalServerError)
		return
	}

	if relPath != "index.md" {
		if err := refreshGeneratedIndex(route.wikiRoot, route.wikiMount); err != nil {
			http.Error(w, "failed to refresh index", http.StatusInternalServerError)
			return
		}
	}
	if err := search.DeleteWikiDocumentNamespace(r.Context(), route.rawRoot, route.namespace, relPath); err != nil {
		http.Error(w, "failed to update search index", http.StatusInternalServerError)
		return
	}
	if relPath != "index.md" {
		if err := search.UpsertWikiDocumentNamespace(r.Context(), route.rawRoot, route.namespace, "index.md"); err != nil {
			http.Error(w, "failed to update search index", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

func wikiRelativeMarkdownPath(requestPath string) (string, int, string) {
	if isWikiPath(requestPath) {
		requestPath = stripMountPath(requestPath, "/wiki")
	} else if strings.HasPrefix(requestPath, "/raw") {
		return "", http.StatusForbidden, "wiki mutations are only allowed under a wiki mount"
	}
	if requestPath == "/" {
		return "", http.StatusBadRequest, "wiki path must include a markdown file"
	}

	relPath := strings.TrimPrefix(requestPath, "/")
	if relPath == "" || strings.HasSuffix(relPath, "/") {
		return "", http.StatusBadRequest, "wiki path must point to a file"
	}
	if strings.Contains(relPath, "\\") {
		return "", http.StatusForbidden, "invalid wiki path"
	}

	parts := strings.Split(relPath, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", http.StatusForbidden, "invalid wiki path"
		}
	}

	if !strings.HasSuffix(strings.ToLower(relPath), ".md") {
		return "", http.StatusBadRequest, "only .md files can be mutated under /wiki/"
	}

	return relPath, 0, ""
}

func isPathWithinRoot(root, target string) bool {
	rootPath := filepath.Clean(root)
	targetPath := filepath.Clean(target)

	relPath, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return false
	}

	if relPath == "." {
		return true
	}

	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return false
	}

	return true
}

func ensureGeneratedIndexIfMissing(wikiRoot string, wikiMounts ...string) error {
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		return err
	}

	indexPath := filepath.Join(wikiRoot, "index.md")
	_, err := os.Stat(indexPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return writeGeneratedIndex(wikiRoot, indexPath, defaultWikiMount(wikiMounts...))
}

func refreshGeneratedIndex(wikiRoot string, wikiMounts ...string) error {
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		return err
	}

	indexPath := filepath.Join(wikiRoot, "index.md")
	body, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeGeneratedIndex(wikiRoot, indexPath, defaultWikiMount(wikiMounts...))
		}
		return err
	}

	if !hasAutoIndexMarker(body) {
		return nil
	}

	return writeGeneratedIndex(wikiRoot, indexPath, defaultWikiMount(wikiMounts...))
}

func defaultWikiMount(wikiMounts ...string) string {
	if len(wikiMounts) > 0 && strings.TrimSpace(wikiMounts[0]) != "" {
		return wikiMounts[0]
	}
	return namespacePublicMount(defaultNamespace, search.ScopeWiki)
}

func hasAutoIndexMarker(content []byte) bool {
	firstLine := string(content)
	if idx := strings.Index(firstLine, "\n"); idx >= 0 {
		firstLine = firstLine[:idx]
	}

	return strings.TrimSpace(firstLine) == autoIndexMarker
}

func writeGeneratedIndex(wikiRoot, indexPath string, wikiMounts ...string) error {
	wikiMount := defaultWikiMount(wikiMounts...)
	pages, err := collectWikiMarkdownPaths(wikiRoot)
	if err != nil {
		return err
	}

	var builder strings.Builder
	builder.WriteString(autoIndexMarker)
	builder.WriteString("\n\n# Index\n\n")

	if len(pages) == 0 {
		builder.WriteString("_No wiki pages yet._\n")
	} else {
		for _, page := range pages {
			title := strings.TrimSuffix(filepath.Base(page), ".md")
			if title == "" {
				title = page
			}
			builder.WriteString("- [")
			builder.WriteString(title)
			builder.WriteString("](")
			builder.WriteString(wikiMount)
			builder.WriteString("/")
			builder.WriteString(page)
			builder.WriteString(")\n")
		}
	}

	return os.WriteFile(indexPath, []byte(builder.String()), 0o644)
}

func collectWikiMarkdownPaths(wikiRoot string) ([]string, error) {
	pages := make([]string, 0)
	err := filepath.WalkDir(wikiRoot, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(wikiRoot, currentPath)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		if entry.IsDir() {
			if isRootWikiDirPath(relPath) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			return nil
		}
		if isRootWikiDirPath(relPath) {
			return nil
		}
		if strings.EqualFold(filepath.Base(relPath), "index.md") {
			return nil
		}

		pages = append(pages, relPath)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(pages)
	return pages, nil
}

func isRootWikiDirPath(relPath string) bool {
	return relPath == wikiDirName || strings.HasPrefix(relPath, wikiDirName+"/")
}

func isMarkdownPath(requestPath string) bool {
	return strings.HasSuffix(strings.ToLower(requestPath), ".md")
}

func isBrowserUserAgent(ua string) bool {
	lower := strings.ToLower(ua)
	return strings.Contains(lower, "chrome") || strings.Contains(lower, "firefox")
}

func readMarkdownFile(fileSystem http.FileSystem, requestPath string) ([]byte, bool, error) {
	file, err := fileSystem.Open(requestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if info.IsDir() {
		return nil, false, nil
	}

	body, err := io.ReadAll(file)
	if err != nil {
		return nil, false, err
	}

	return body, true, nil
}

func init() {
	serveCmd.Flags().String("home-dir", "", "Directory to serve (defaults to current directory)")
	serveCmd.Flags().String("bind", defaultBind, "IP address to bind")
	serveCmd.Flags().Int("port", defaultPort, "TCP port to bind")

	mustBindFlag(homeDirKey, "home-dir")
	mustBindFlag(bindKey, "bind")
	mustBindFlag(portKey, "port")
	configureServeViper(serveViper)

	rootCmd.AddCommand(serveCmd)
}

func configureServeViper(config *viper.Viper) {
	config.SetEnvPrefix("alien")
	config.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	mustBindEnv(config, homeDirKey, homeDirEnv)
	mustBindEnv(config, bindKey, bindEnv)
	mustBindEnv(config, portKey, portEnv)
	config.AutomaticEnv()

	config.SetDefault(bindKey, defaultBind)
	config.SetDefault(portKey, defaultPort)
}

func mustBindEnv(config *viper.Viper, key string, envVars ...string) {
	if err := config.BindEnv(append([]string{key}, envVars...)...); err != nil {
		panic(err)
	}
}

func mustBindFlag(key, flagName string) {
	flag := serveCmd.Flags().Lookup(flagName)
	if flag == nil {
		panic(fmt.Sprintf("flag %s not found", flagName))
	}
	if err := serveViper.BindPFlag(key, flag); err != nil {
		panic(err)
	}
}

func resolveHomeDir(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to determine current directory: %w", err)
		}
		trimmed = cwd
	}

	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory %q: %w", trimmed, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("home directory does not exist: %s", abs)
		}
		return "", fmt.Errorf("cannot access home directory %s: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("home directory is not a directory: %s", abs)
	}

	return abs, nil
}
