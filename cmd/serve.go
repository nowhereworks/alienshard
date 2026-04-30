package cmd

import (
	"bytes"
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
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yuin/goldmark"
)

const (
	defaultBind     = "127.0.0.1"
	defaultPort     = 8000
	wikiDirName     = "__wiki"
	autoIndexMarker = "<!-- alienshard:autoindex v1 -->"
)

var serveViper = viper.New()

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve static files over HTTP",
	RunE: func(cmd *cobra.Command, args []string) error {
		bind := strings.TrimSpace(serveViper.GetString("bind"))
		if net.ParseIP(bind) == nil {
			return fmt.Errorf("invalid --bind value %q: must be a valid IP address", bind)
		}

		port := serveViper.GetInt("port")
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid --port value %d: must be in range 1-65535", port)
		}

		homeDir, err := resolveHomeDir(serveViper.GetString("home_dir"))
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
		return srv.ListenAndServe()
	},
}

func newMountedHandler(rawRoot, wikiRoot string) http.Handler {
	rawFileSystem := http.Dir(rawRoot)
	rawFileServer := http.FileServer(rawFileSystem)
	wikiFileSystem := http.Dir(wikiRoot)
	wikiFileServer := http.FileServer(wikiFileSystem)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRawPath(r.URL.Path) {
			if isBlockedRawPath(r.URL.Path) {
				http.NotFound(w, r)
				return
			}

			strippedPath := stripMountPath(r.URL.Path, "/raw")
			serveMountedPath(w, r, rawFileSystem, rawFileServer, strippedPath)
			return
		}

		if isWikiPath(r.URL.Path) {
			strippedPath := stripMountPath(r.URL.Path, "/wiki")

			if r.Method == http.MethodPut {
				handleWikiPut(w, r, wikiRoot)
				return
			}

			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			if strippedPath == "/index.md" {
				if err := ensureGeneratedIndexIfMissing(wikiRoot); err != nil {
					http.Error(w, "failed to generate index", http.StatusInternalServerError)
					return
				}
			}

			serveMountedPath(w, r, wikiFileSystem, wikiFileServer, strippedPath)
			return
		}

		http.NotFound(w, r)
	})
}

func serveMountedPath(
	w http.ResponseWriter,
	r *http.Request,
	fileSystem http.Dir,
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

func isBlockedRawPath(requestPath string) bool {
	strippedPath := stripMountPath(requestPath, "/raw")
	return strippedPath == "/__wiki" || strings.HasPrefix(strippedPath, "/__wiki/")
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

func handleWikiPut(w http.ResponseWriter, r *http.Request, wikiRoot string) {
	relPath, statusCode, msg := wikiRelativeMarkdownPath(r.URL.Path)
	if statusCode != 0 {
		http.Error(w, msg, statusCode)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		http.Error(w, "failed to prepare wiki directory", http.StatusInternalServerError)
		return
	}

	filePath := filepath.Join(wikiRoot, filepath.FromSlash(relPath))
	if !isPathWithinRoot(wikiRoot, filePath) {
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

	if err := refreshGeneratedIndex(wikiRoot); err != nil {
		http.Error(w, "failed to refresh index", http.StatusInternalServerError)
		return
	}

	if existed {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func wikiRelativeMarkdownPath(requestPath string) (string, int, string) {
	if requestPath == "/wiki" || requestPath == "/wiki/" {
		return "", http.StatusBadRequest, "wiki path must include a markdown file"
	}
	if !strings.HasPrefix(requestPath, "/wiki/") {
		return "", http.StatusForbidden, "writes are only allowed under /wiki/"
	}

	relPath := strings.TrimPrefix(requestPath, "/wiki/")
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
		return "", http.StatusBadRequest, "only .md files can be written under /wiki/"
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

func ensureGeneratedIndexIfMissing(wikiRoot string) error {
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

	return writeGeneratedIndex(wikiRoot, indexPath)
}

func refreshGeneratedIndex(wikiRoot string) error {
	if err := os.MkdirAll(wikiRoot, 0o755); err != nil {
		return err
	}

	indexPath := filepath.Join(wikiRoot, "index.md")
	body, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeGeneratedIndex(wikiRoot, indexPath)
		}
		return err
	}

	if !hasAutoIndexMarker(body) {
		return nil
	}

	return writeGeneratedIndex(wikiRoot, indexPath)
}

func hasAutoIndexMarker(content []byte) bool {
	firstLine := string(content)
	if idx := strings.Index(firstLine, "\n"); idx >= 0 {
		firstLine = firstLine[:idx]
	}

	return strings.TrimSpace(firstLine) == autoIndexMarker
}

func writeGeneratedIndex(wikiRoot, indexPath string) error {
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
			builder.WriteString("](/wiki/")
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
		if entry.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			return nil
		}

		relPath, err := filepath.Rel(wikiRoot, currentPath)
		if err != nil {
			return err
		}

		relPath = filepath.ToSlash(relPath)
		if relPath == "index.md" {
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

func isMarkdownPath(requestPath string) bool {
	return strings.HasSuffix(strings.ToLower(requestPath), ".md")
}

func isBrowserUserAgent(ua string) bool {
	lower := strings.ToLower(ua)
	return strings.Contains(lower, "chrome") || strings.Contains(lower, "firefox")
}

func readMarkdownFile(fileSystem http.Dir, requestPath string) ([]byte, bool, error) {
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

	mustBindFlag("home_dir", "home-dir")
	mustBindFlag("bind", "bind")
	mustBindFlag("port", "port")

	serveViper.SetEnvPrefix("alienshard")
	serveViper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	serveViper.AutomaticEnv()

	serveViper.SetDefault("bind", defaultBind)
	serveViper.SetDefault("port", defaultPort)

	rootCmd.AddCommand(serveCmd)
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
