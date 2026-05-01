package search

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

const (
	StateReady      = "ready"
	StateIndexing   = "indexing"
	StateNotIndexed = "not_indexed"
	StateError      = "error"

	ScopeAll  = "all"
	ScopeRaw  = "raw"
	ScopeWiki = "wiki"

	alienshardDir = ".alienshard"
	dbName        = "search.sqlite"
	rebuildDBName = "search.rebuild.sqlite"
	lockName      = "search.lock"
	wikiDirName   = "__wiki"
	maxFileBytes  = 5 * 1024 * 1024
)

var (
	errLocked    = errors.New("search index rebuild already in progress")
	markdownLink = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
)

type RebuildResult struct {
	FilesSeen    int           `json:"files_seen"`
	FilesIndexed int           `json:"files_indexed"`
	RawIndexed   int           `json:"raw_indexed"`
	WikiIndexed  int           `json:"wiki_indexed"`
	FilesSkipped int           `json:"files_skipped"`
	Duration     time.Duration `json:"-"`
}

type QueryOptions struct {
	Query string
	Scope string
	Limit int
}

type QueryResult struct {
	Query      string         `json:"query"`
	Scope      string         `json:"scope"`
	IndexState string         `json:"index_state"`
	Results    []SearchResult `json:"results"`
}

type SearchResult struct {
	Mount   string  `json:"mount"`
	Path    string  `json:"path"`
	Title   string  `json:"title"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

type Status struct {
	State        string     `json:"state"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	FilesSeen    int        `json:"files_seen"`
	FilesIndexed int        `json:"files_indexed"`
	FilesSkipped int        `json:"files_skipped"`
	LastError    *string    `json:"last_error"`
}

type Service struct {
	rawRoot string
	mu      sync.RWMutex
	status  Status
}

type document struct {
	mount      string
	root       string
	relPath    string
	publicPath string
	info       fs.FileInfo
}

type parsedDocument struct {
	document
	hash     string
	title    string
	headings []string
	body     string
	links    []string
}

func NewService(rawRoot string) *Service {
	state := StateNotIndexed
	if _, err := os.Stat(activeDBPath(rawRoot)); err == nil {
		state = StateReady
	}
	return &Service{rawRoot: rawRoot, status: Status{State: state}}
}

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := s.status
	if status.State != StateIndexing && status.LastError == nil {
		if _, err := os.Stat(activeDBPath(s.rawRoot)); err == nil {
			status.State = StateReady
		} else if errors.Is(err, os.ErrNotExist) {
			status.State = StateNotIndexed
		}
	}
	return status
}

func (s *Service) Query(ctx context.Context, opts QueryOptions) (QueryResult, error) {
	result, err := Query(ctx, s.rawRoot, opts)
	if err != nil {
		return result, err
	}

	status := s.Status()
	if status.State == StateIndexing && result.IndexState == StateReady {
		result.IndexState = StateIndexing
	}
	return result, nil
}

func (s *Service) StartReindex() error {
	s.mu.Lock()
	if s.status.State == StateIndexing {
		s.mu.Unlock()
		return errLocked
	}
	now := time.Now().UTC()
	s.status = Status{State: StateIndexing, StartedAt: &now}
	s.mu.Unlock()

	go func() {
		result, err := Rebuild(context.Background(), s.rawRoot)
		finished := time.Now().UTC()

		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			msg := err.Error()
			s.status.State = StateError
			s.status.FinishedAt = &finished
			s.status.LastError = &msg
			return
		}

		s.status.State = StateReady
		s.status.FinishedAt = &finished
		s.status.FilesSeen = result.FilesSeen
		s.status.FilesIndexed = result.FilesIndexed
		s.status.FilesSkipped = result.FilesSkipped
		s.status.LastError = nil
	}()

	return nil
}

func Rebuild(ctx context.Context, rawRoot string) (RebuildResult, error) {
	start := time.Now()
	var result RebuildResult

	root, err := filepath.Abs(rawRoot)
	if err != nil {
		return result, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		return result, fmt.Errorf("home directory is not a directory: %s", root)
	}

	indexDir := filepath.Join(root, alienshardDir)
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return result, err
	}

	unlock, err := acquireLock(filepath.Join(indexDir, lockName))
	if err != nil {
		return result, err
	}
	defer unlock()

	rebuildPath := filepath.Join(indexDir, rebuildDBName)
	activePath := filepath.Join(indexDir, dbName)
	_ = os.Remove(rebuildPath)

	db, err := openDB(rebuildPath)
	if err != nil {
		return result, err
	}
	defer db.Close()

	if err := createSchema(ctx, db); err != nil {
		return result, err
	}

	if err := scanScope(ctx, db, root, root, ScopeRaw, &result); err != nil {
		return result, err
	}
	wikiRoot := filepath.Join(root, wikiDirName)
	if wikiInfo, err := os.Stat(wikiRoot); err == nil && wikiInfo.IsDir() {
		if err := scanScope(ctx, db, root, wikiRoot, ScopeWiki, &result); err != nil {
			return result, err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}

	if _, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO meta(key, value) VALUES('rebuilt_at', ?), ('schema_version', '1')`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return result, err
	}
	if err := db.Close(); err != nil {
		return result, err
	}

	if err := os.Rename(rebuildPath, activePath); err != nil {
		return result, err
	}

	result.Duration = time.Since(start)
	return result, nil
}

func Query(ctx context.Context, rawRoot string, opts QueryOptions) (QueryResult, error) {
	scope := normalizeScope(opts.Scope)
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	result := QueryResult{Query: opts.Query, Scope: scope, IndexState: StateReady, Results: []SearchResult{}}
	activePath := activeDBPath(rawRoot)
	if _, err := os.Stat(activePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.IndexState = StateNotIndexed
			return result, nil
		}
		return result, err
	}

	ftsQuery := buildFTSQuery(opts.Query)
	if ftsQuery == "" {
		return result, nil
	}

	db, err := openDB(activePath)
	if err != nil {
		return result, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
SELECT d.mount, d.public_path, d.title,
       -bm25(search_fts, 3.0, 2.5, 1.4, 1.0) + (SELECT COUNT(*) FROM links WHERE links.to_path = d.public_path) * 0.1 AS score,
       snippet(search_fts, 5, '', '', '...', 24) AS snippet
FROM search_fts
JOIN documents d ON d.id = search_fts.doc_id
WHERE search_fts MATCH ? AND (? = 'all' OR d.mount = ?)
ORDER BY score DESC, d.public_path ASC
LIMIT ?`, ftsQuery, scope, scope, limit)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(&item.Mount, &item.Path, &item.Title, &item.Score, &item.Snippet); err != nil {
			return result, err
		}
		result.Results = append(result.Results, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	return result, nil
}

func Backlinks(ctx context.Context, rawRoot, publicPath string) ([]SearchResult, error) {
	activePath := activeDBPath(rawRoot)
	if _, err := os.Stat(activePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []SearchResult{}, nil
		}
		return nil, err
	}

	db, err := openDB(activePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
SELECT d.mount, d.public_path, d.title, 0.0 AS score, '' AS snippet
FROM links l
JOIN documents d ON d.id = l.from_doc_id
WHERE l.to_path = ?
ORDER BY d.public_path ASC`, publicPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(&item.Mount, &item.Path, &item.Title, &item.Score, &item.Snippet); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func UpsertWikiDocument(ctx context.Context, rawRoot, relPath string) error {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." || strings.HasPrefix(relPath, "../") || relPath == ".." {
		return fmt.Errorf("invalid wiki relative path: %s", relPath)
	}
	activePath := activeDBPath(rawRoot)
	if _, err := os.Stat(activePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	db, err := openDB(activePath)
	if err != nil {
		return err
	}
	defer db.Close()

	wikiRoot := filepath.Join(rawRoot, wikiDirName)
	filePath := filepath.Join(wikiRoot, filepath.FromSlash(relPath))
	info, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DeleteWikiDocument(ctx, rawRoot, relPath)
		}
		return err
	}
	if info.IsDir() {
		return nil
	}

	doc := document{mount: ScopeWiki, root: wikiRoot, relPath: relPath, publicPath: makePublicPath("/wiki", relPath), info: info}
	parsed, ok, err := parseDocument(doc)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := deleteDocument(ctx, tx, ScopeWiki, relPath); err != nil {
		return err
	}
	if ok {
		if err := insertDocument(ctx, tx, parsed); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func DeleteWikiDocument(ctx context.Context, rawRoot, relPath string) error {
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." || strings.HasPrefix(relPath, "../") || relPath == ".." {
		return fmt.Errorf("invalid wiki relative path: %s", relPath)
	}
	activePath := activeDBPath(rawRoot)
	if _, err := os.Stat(activePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	db, err := openDB(activePath)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := deleteDocument(ctx, tx, ScopeWiki, relPath); err != nil {
		return err
	}
	return tx.Commit()
}

func IsLocked(err error) bool {
	return errors.Is(err, errLocked)
}

func activeDBPath(rawRoot string) string {
	return filepath.Join(rawRoot, alienshardDir, dbName)
}

func acquireLock(lockPath string) (func(), error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, errLocked
		}
		return nil, err
	}
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	return func() {
		_ = file.Close()
		_ = os.Remove(lockPath)
	}, nil
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func createSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE documents (
  id INTEGER PRIMARY KEY,
  mount TEXT NOT NULL,
  rel_path TEXT NOT NULL,
  public_path TEXT NOT NULL,
  size INTEGER NOT NULL,
  mtime_unix_nano INTEGER NOT NULL,
  hash TEXT NOT NULL,
  title TEXT NOT NULL,
  UNIQUE(mount, rel_path)
);
CREATE TABLE chunks (
  id INTEGER PRIMARY KEY,
  doc_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  heading TEXT NOT NULL,
  text_hash TEXT NOT NULL,
  start_byte INTEGER NOT NULL,
  end_byte INTEGER NOT NULL,
  text TEXT NOT NULL
);
CREATE TABLE links (
  from_doc_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  to_path TEXT NOT NULL
);
CREATE TABLE meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE VIRTUAL TABLE search_fts USING fts5(
  doc_id UNINDEXED,
  mount UNINDEXED,
  public_path UNINDEXED,
  title,
  headings,
  path,
  body
);`)
	return err
}

func scanScope(ctx context.Context, db *sql.DB, rawRoot, scopeRoot, mount string, result *RebuildResult) error {
	return filepath.WalkDir(scopeRoot, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		relPath, err := filepath.Rel(scopeRoot, currentPath)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		if entry.IsDir() {
			if relPath == "." {
				return nil
			}
			if entry.Name() == alienshardDir {
				return filepath.SkipDir
			}
			if mount == ScopeRaw && relPath == wikiDirName {
				return filepath.SkipDir
			}
			return nil
		}

		result.FilesSeen++
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !isSearchableExtension(entry.Name()) || info.Size() > maxFileBytes {
			result.FilesSkipped++
			return nil
		}

		doc := document{mount: mount, root: scopeRoot, relPath: relPath, publicPath: makePublicPath("/"+mount, relPath), info: info}
		parsed, ok, err := parseDocument(doc)
		if err != nil {
			return err
		}
		if !ok {
			result.FilesSkipped++
			return nil
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := insertDocument(ctx, tx, parsed); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}

		result.FilesIndexed++
		if mount == ScopeRaw {
			result.RawIndexed++
		} else if mount == ScopeWiki {
			result.WikiIndexed++
		}
		_ = rawRoot
		return nil
	})
}

func parseDocument(doc document) (parsedDocument, bool, error) {
	var parsed parsedDocument
	filePath := filepath.Join(doc.root, filepath.FromSlash(doc.relPath))
	body, err := os.ReadFile(filePath)
	if err != nil {
		return parsed, false, err
	}
	if bytesLookBinary(body) || !utf8.Valid(body) {
		return parsed, false, nil
	}

	text := string(body)
	hashBytes := sha256.Sum256(body)
	parsed = parsedDocument{document: doc, hash: hex.EncodeToString(hashBytes[:]), body: text}
	parsed.title, parsed.headings = extractTitleAndHeadings(doc.relPath, text)
	parsed.links = extractLinks(doc.mount, doc.relPath, text)
	return parsed, true, nil
}

func insertDocument(ctx context.Context, tx *sql.Tx, doc parsedDocument) error {
	res, err := tx.ExecContext(ctx, `INSERT INTO documents(mount, rel_path, public_path, size, mtime_unix_nano, hash, title) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		doc.mount, doc.relPath, doc.publicPath, doc.info.Size(), doc.info.ModTime().UnixNano(), doc.hash, doc.title)
	if err != nil {
		return err
	}
	docID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	bodyHash := sha256.Sum256([]byte(doc.body))
	if _, err := tx.ExecContext(ctx, `INSERT INTO chunks(doc_id, heading, text_hash, start_byte, end_byte, text) VALUES(?, ?, ?, 0, ?, ?)`,
		docID, strings.Join(doc.headings, "\n"), hex.EncodeToString(bodyHash[:]), len([]byte(doc.body)), doc.body); err != nil {
		return err
	}
	for _, link := range doc.links {
		if _, err := tx.ExecContext(ctx, `INSERT INTO links(from_doc_id, to_path) VALUES(?, ?)`, docID, link); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO search_fts(doc_id, mount, public_path, title, headings, path, body) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		docID, doc.mount, doc.publicPath, doc.title, strings.Join(doc.headings, "\n"), doc.publicPath, doc.body)
	return err
}

func deleteDocument(ctx context.Context, tx *sql.Tx, mount, relPath string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM documents WHERE mount = ? AND rel_path = ?`, mount, relPath)
	if err != nil {
		return err
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM search_fts WHERE doc_id = ?`, id); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM documents WHERE mount = ? AND rel_path = ?`, mount, relPath)
	return err
}

func isSearchableExtension(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".txt", ".text", ".rst", ".csv", ".tsv", ".json", ".yaml", ".yml", ".toml", ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".html", ".css":
		return true
	default:
		return false
	}
}

func bytesLookBinary(body []byte) bool {
	for i, b := range body {
		if i >= 8192 {
			return false
		}
		if b == 0 {
			return true
		}
	}
	return false
}

func extractTitleAndHeadings(relPath, body string) (string, []string) {
	headings := []string{}
	title := ""
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level == 0 || level > 6 || level >= len(trimmed) || trimmed[level] != ' ' {
			continue
		}
		heading := strings.TrimSpace(trimmed[level:])
		if heading == "" {
			continue
		}
		headings = append(headings, heading)
		if level == 1 && title == "" {
			title = heading
		}
	}

	if title != "" {
		return title, headings
	}
	if len(headings) > 0 {
		return headings[0], headings
	}
	base := filepath.Base(relPath)
	return strings.TrimSuffix(base, filepath.Ext(base)), headings
}

func extractLinks(mount, relPath, body string) []string {
	matches := markdownLink.FindAllStringSubmatch(body, -1)
	links := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		link := strings.TrimSpace(match[1])
		if link == "" || strings.HasPrefix(link, "#") {
			continue
		}
		if idx := strings.Index(link, "#"); idx >= 0 {
			link = link[:idx]
		}
		if idx := strings.Index(link, "?"); idx >= 0 {
			link = link[:idx]
		}
		normalized := normalizeLinkTarget(mount, relPath, link)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		links = append(links, normalized)
	}
	sort.Strings(links)
	return links
}

func normalizeLinkTarget(mount, relPath, target string) string {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return ""
	}
	if strings.HasPrefix(target, "/wiki/") || strings.HasPrefix(target, "/raw/") {
		return cleanPublicPath(target)
	}
	if strings.HasPrefix(target, "/") {
		return ""
	}

	cleanRel := filepath.ToSlash(filepath.Clean(filepath.ToSlash(filepath.Join(filepath.Dir(relPath), target))))
	if cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, "../") {
		return ""
	}
	if mount == ScopeRaw {
		return makePublicPath("/raw", cleanRel)
	}
	if mount == ScopeWiki {
		return makePublicPath("/wiki", cleanRel)
	}
	return ""
}

func cleanPublicPath(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	prefix := "/" + parts[0]
	relParts := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			return ""
		}
		relParts = append(relParts, unescaped)
	}
	relPath := filepath.ToSlash(filepath.Clean(strings.Join(relParts, "/")))
	if relPath == "." || relPath == ".." || strings.HasPrefix(relPath, "../") {
		return ""
	}
	return makePublicPath(prefix, relPath)
}

func normalizeScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case ScopeRaw:
		return ScopeRaw
	case ScopeWiki:
		return ScopeWiki
	default:
		return ScopeAll
	}
}

func buildFTSQuery(query string) string {
	tokens := []string{}
	var builder strings.Builder
	flush := func() {
		if builder.Len() == 0 {
			return
		}
		tokens = append(tokens, builder.String()+"*")
		builder.Reset()
	}
	for _, r := range strings.ToLower(query) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			builder.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return strings.Join(tokens, " ")
}

func makePublicPath(prefix, relPath string) string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return prefix + "/" + strings.Join(parts, "/")
}
