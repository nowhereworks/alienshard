# Search Design

This document is the canonical plan and handoff record for Alien Shard search. Update it whenever search implementation work starts, stops, or changes direction so future sessions can resume from the first unchecked item instead of rediscovering prior decisions.

## Goals

- Search both first-class content layers: `/raw/*` and `/wiki/*`.
- Provide better-than-substring results with lexical ranking first, then graph and optional vector improvements.
- Keep startup fast as raw collections and wikis grow.
- Support offline administration through `alienshard index rebuild --home-dir /data`.
- Support server-side search and reindexing after the offline index path is stable.
- Expose only content that is reachable through the public mounts.
- Keep the default deployment lightweight and local-first.

## Non-Goals

- Do not require an external search service for baseline search.
- Do not require vectors, embeddings, or an LLM provider for baseline search.
- Do not make startup synchronously scan the full content tree.
- Do not expose local implementation files, generated indexes, secrets, or private artifacts through search.
- Do not introduce a graph database unless the markdown link graph outgrows a simple persisted edge table.

## Searchable Content

Search should index two public scopes.

| Scope | Filesystem root | Public path prefix | Notes |
| --- | --- | --- | --- |
| `raw` | `rawRoot` | `/raw/` | Raw source tree. Must exclude the root-level `__wiki` implementation directory. |
| `wiki` | `rawRoot/__wiki` | `/wiki/` | Writable wiki layer. |
| `all` | Both roots | Both prefixes | Default scope. |

Raw should be searchable by default because it is one of Alien Shard's first-class content layers and is often the source of truth behind wiki synthesis.

Initial file types should favor text formats:

- `.md`, `.markdown`
- `.txt`, `.text`
- `.rst`
- `.csv`, `.tsv`
- `.json`, `.yaml`, `.yml`, `.toml`
- `.go`, `.js`, `.jsx`, `.ts`, `.tsx`, `.py`, `.html`, `.css`

Markdown should get richer parsing than plain text:

- title from first level-one heading when present
- headings as boosted fields
- body chunks
- markdown links
- frontmatter later if/when wiki conventions include it

Other text files can start as plain text records with path, size, mtime, content hash, and extracted snippets.

## Exclusions

Search must not expose anything that cannot be reached through the public server mounts.

Required exclusions:

- Exclude `rawRoot/__wiki` while scanning the `raw` scope.
- Exclude `rawRoot/.alienshard` from all scans.
- Exclude the active search DB, temporary rebuild DBs, locks, and other index implementation files.
- Exclude directories and files that are configured as local-only in future ignore settings.
- Skip binary files.
- Skip very large files by default once a size limit is introduced.

The wiki scope should scan `rawRoot/__wiki` directly and expose results as `/wiki/...`, never as `/raw/__wiki/...`.

## Index Storage

Use a persistent SQLite index under the served home directory:

```text
<home-dir>/.alienshard/search.sqlite
```

Full rebuilds should write a replacement database first:

```text
<home-dir>/.alienshard/search.rebuild.sqlite
```

After a successful rebuild, atomically replace the active database. Keep search available on the old index while a server-side rebuild runs.

Use a lock file to prevent concurrent rebuild corruption:

```text
<home-dir>/.alienshard/search.lock
```

For the first implementation, prefer opening the SQLite database per search operation or using a short-lived connection. If the server later keeps a long-lived connection, it must detect atomic DB replacement and reopen.

## CLI

Support offline rebuilding through Cobra:

```bash
alienshard index rebuild --home-dir /data
```

Command tree:

```text
alienshard index
alienshard index rebuild
```

Initial `rebuild` flags:

```text
--home-dir string   Directory to index, same meaning as serve --home-dir
```

Possible later flags:

```text
--format text|json       Output format, default text
--include-hidden         Include hidden user files, still excluding .alienshard and __wiki-as-raw
--max-file-bytes int     Maximum file size to index
```

Expected behavior:

```text
/data/.alienshard/search.rebuild.sqlite
-> scan /data as raw
-> scan /data/__wiki as wiki
-> exclude /data/__wiki from raw scope
-> exclude /data/.alienshard
-> populate the temporary index
-> validate the temporary index
-> atomically replace /data/.alienshard/search.sqlite
```

Expected text output:

```text
Indexed 1284 files:
  raw: 912
  wiki: 372
  skipped: 18
  duration: 4.2s
```

Exit non-zero on:

- invalid `--home-dir`
- lock acquisition failure caused by another rebuild
- index directory creation failure
- temporary DB creation or write failure
- atomic replacement failure
- scan/read failure that should not be ignored

## HTTP API

Planned endpoints after the CLI rebuild path is stable:

```text
GET /search?q=...&scope=all|raw|wiki&limit=20
GET /search/status
POST /search/reindex
```

`GET /search` should default to `scope=all` and return JSON for machine clients:

```json
{
  "query": "persistent knowledge",
  "scope": "all",
  "index_state": "ready",
  "results": [
    {
      "mount": "raw",
      "path": "/raw/docs/llm-wiki.md",
      "title": "LLM Wiki",
      "score": 9.8,
      "snippet": "...persistent, compounding artifact..."
    }
  ]
}
```

`GET /search/status` should expose reindex state:

```json
{
  "state": "indexing",
  "started_at": "2026-04-30T12:00:00Z",
  "finished_at": null,
  "files_seen": 1234,
  "files_indexed": 1180,
  "files_skipped": 54,
  "last_error": null
}
```

`POST /search/reindex` should return `202 Accepted`, start a background rebuild, keep serving from the old index, and atomically swap in the new index only after success.

## Reindexing

There are three reindex paths.

| Path | Trigger | Behavior |
| --- | --- | --- |
| Full CLI rebuild | `alienshard index rebuild --home-dir /data` | Build a new SQLite DB by scanning raw and wiki, then atomically replace the active index. |
| Full HTTP rebuild | `POST /search/reindex` | Same core rebuild function, but runs in the server background and reports status. |
| Single-document update | successful `PUT /wiki/<path>.md` | Update or delete one wiki document's index rows after the file write succeeds. |

The CLI and HTTP rebuild implementations should share one core function, for example:

```go
func rebuildSearchIndex(rawRoot string) (SearchRebuildResult, error)
```

The actual function name can change, but there should be one source of truth for scan rules, exclusions, schema setup, and atomic replacement.

## Startup Behavior

Startup must not synchronously scan all files.

On `alienshard serve`:

- resolve `rawRoot` and `wikiRoot`
- start HTTP serving immediately
- load compact index metadata when needed
- optionally start a lightweight background freshness scan later
- if no index exists, report `not_indexed` from search endpoints or auto-start background indexing once that behavior is explicitly chosen

If search requests arrive while a rebuild is running, return old-index results with an index state such as `indexing`. If no usable index exists, return a clear `not_indexed` response rather than blocking on a full scan.

## Query Behavior

Baseline search should be lexical BM25-style ranking, not simple string matching.

Fields to index and boost:

| Field | Relative weight | Notes |
| --- | --- | --- |
| title | high | First heading or filename fallback. |
| headings | high | Markdown section headings. |
| path | medium | Useful for known filenames and topic paths. |
| body | normal | Main text. |
| links | medium | Useful once graph extraction exists. |

Search should return snippets from the highest-ranked matching chunks. Do not read every document body at query time; read only top candidates if snippets are not fully stored in the index.

## SQLite Schema Draft

Prefer SQLite FTS5 for the first lexical implementation if the selected Go SQLite driver supports it cleanly in the target build environment. Otherwise, use explicit inverted-index tables.

Draft core tables:

```sql
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
```

Possible FTS table:

```sql
CREATE VIRTUAL TABLE search_fts USING fts5(
  title,
  headings,
  path,
  body,
  content=''
);
```

If using external-content FTS, document the rowid relationship and rebuild procedure here before implementation.

## Graph Index

Graph support should parse markdown links and store outgoing edges. Backlinks can be queried from the same table.

Initial graph features:

- parse standard markdown links that target `/wiki/...`, `/raw/...`, or relative markdown paths
- store outgoing links per document
- expose related pages later
- boost pages with relevant inbound links
- identify orphan wiki pages in future lint commands

Do not add a graph database for the initial graph feature. A persisted edge table is enough.

## Vector Search Roadmap

Vectors are optional future functionality. They should not be required for baseline search, startup, or indexing.

Vector requirements before implementation:

- persistent chunk embeddings
- provider/model/dimension metadata
- content hash to avoid recomputing unchanged chunks
- background embedding generation
- query embedding path
- hybrid ranking with lexical search
- clear privacy model for external embedding providers

Possible embedding table:

```sql
CREATE TABLE chunk_embeddings (
  chunk_id INTEGER NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  dims INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  embedding BLOB NOT NULL,
  PRIMARY KEY(chunk_id, provider, model)
);
```

For small and medium indexes, brute-force vector comparison over stored vectors may be acceptable. For larger indexes, add a vector-specific index later.

## Implementation Phases

Phase 1: durable plan.

- Add this document.
- Link it from README.
- Record the existence of the plan in AGENTS.md.

Phase 2: offline index rebuild.

- Add `alienshard index` and `alienshard index rebuild` command wiring.
- Add home-dir resolution shared with `serve`.
- Add scanner with raw/wiki inclusion and exclusion rules.
- Add SQLite schema and atomic rebuild.
- Add tests for command wiring, invalid home dir, raw inclusion, wiki inclusion, `__wiki` raw exclusion, and `.alienshard` exclusion.

Phase 3: lexical query support.

- Add query function against SQLite.
- Add BM25/FTS ranking.
- Add result snippets.
- Add scope filtering.
- Add tests for ranking and scope behavior.

Phase 4: server search endpoints.

- Add `GET /search`.
- Add `GET /search/status`.
- Add `POST /search/reindex`.
- Ensure search does not block startup.
- Add tests for HTTP behavior and status reporting.

Phase 5: incremental wiki indexing.

- Hook successful `PUT /wiki/<path>.md` into single-document index updates.
- Remove document rows if a future delete API is added.
- Add tests proving wiki writes update searchable content.

Phase 6: graph metadata.

- Parse markdown links.
- Store outgoing edges.
- Add backlink queries and ranking boosts.
- Add tests for link extraction and public path normalization.

Phase 7: optional vectors.

- Choose provider interface.
- Add optional configuration.
- Persist embeddings by chunk hash and model.
- Add hybrid ranking.
- Add tests using deterministic fake embeddings.

## Current Status

- [x] Design documented
- [x] README linked to search roadmap
- [x] AGENTS.md records canonical search plan location
- [ ] Search package scaffolded
- [ ] SQLite driver selected
- [ ] SQLite schema implemented
- [ ] Raw scanner implemented
- [ ] Wiki scanner implemented
- [ ] Exclusion rules implemented
- [ ] CLI command group implemented
- [ ] CLI rebuild implemented
- [ ] CLI rebuild tests added
- [ ] Lexical query implemented
- [ ] HTTP search endpoint implemented
- [ ] HTTP status endpoint implemented
- [ ] HTTP reindex endpoint implemented
- [ ] Wiki PUT incremental indexing implemented
- [ ] Graph/link metadata implemented
- [ ] Vector roadmap revisited with provider decision
- [ ] README updated with implemented user-facing behavior
- [ ] AGENTS.md updated with verified implemented facts

## Resume Protocol

When resuming search work:

1. Read this document.
2. Check `git status` for local work.
3. Find the first unchecked item in Current Status.
4. Confirm the implementation phase still matches the codebase.
5. Make the smallest coherent change for that item.
6. Add or update tests for code changes.
7. Run the relevant verification commands.
8. Update Current Status and Session Log before stopping.

## Verification Commands

Current general commands:

```bash
rtk go test ./...
go run . serve --help
```

Future search commands:

```bash
go run . index --help
go run . index rebuild --home-dir /tmp/alienshard-data
```

After HTTP search exists:

```bash
curl -sS 'http://127.0.0.1:8000/search?q=wiki&scope=all'
curl -sS 'http://127.0.0.1:8000/search/status'
curl -i -X POST 'http://127.0.0.1:8000/search/reindex'
```

## Open Questions

- Should a missing index auto-start a background rebuild during `serve`, or should users explicitly run `alienshard index rebuild` or `POST /search/reindex`?
- Which SQLite driver should be used for Go 1.26.1 and the existing Docker target?
- Is FTS5 available and acceptable in the selected driver/build configuration?
- What default maximum file size should be indexed?
- Should hidden user files be skipped by default beyond `.alienshard`?
- Should search output eventually support Markdown rendering for browser user agents, or remain JSON-only?
- Should HTTP `POST /search/reindex` require any local-only guard or future authentication story?

## Session Log

### 2026-04-30

- Captured the search design before implementation.
- Linked the roadmap from README and recorded the canonical plan location in AGENTS.md.
- Decided that raw and wiki are both searchable by default.
- Decided that startup must not synchronously scan the full tree.
- Decided that `alienshard index rebuild --home-dir /data` is a required offline administration command.
- Decided to pursue persistent SQLite lexical search before graph ranking and optional vectors.
