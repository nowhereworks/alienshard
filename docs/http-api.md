# HTTP API

Alien Shard exposes canonical namespaced mounts and default-namespace aliases.

| Mount | Filesystem root | Purpose |
| --- | --- | --- |
| `/n/<namespace>/raw/*` | namespace raw root | Read files from a namespace source tree. |
| `/n/<namespace>/wiki/*` | namespace raw root `__wiki` | Read, create, update, and delete namespace wiki Markdown pages. |
| `/n/<namespace>/search*` | namespace search index | Query or rebuild a namespace search index. |

The default namespace is `default`. Compatibility aliases map to it:

| Alias | Canonical path |
| --- | --- |
| `/raw/*` | `/n/default/raw/*` |
| `/wiki/*` | `/n/default/wiki/*` |
| `/search*` | `/n/default/search*` |

`rawRoot` is `--home-dir` when provided, otherwise the process current working directory. Non-default namespaces use `rawRoot/__namespaces/<namespace>`.

## Markdown Responses

Markdown rendering applies to `.md` paths on both raw and wiki mounts.

| Client `User-Agent` | Response |
| --- | --- |
| Contains `chrome` or `firefox`, case-insensitive | Goldmark-rendered HTML fragment with `Content-Type: text/html; charset=utf-8`. |
| Anything else | Raw Markdown with `Content-Type: text/markdown; charset=utf-8`. |

Missing Markdown files fall back to the standard file-server response, usually `404 Not Found`.

## Raw Reads

Read files through `/n/<namespace>/raw/*`:

```bash
curl -sS http://127.0.0.1:8000/n/default/raw/README.md
```

Raw mounts use Go's `http.FileServer` behavior for files and directory listings.

Safety rules:

- Raw implementation paths such as `__wiki`, `__namespaces`, and `.alienshard` return `404 Not Found`.
- Root raw directory listings omit implementation directories.
- Wiki content is publicly addressed through wiki mounts, never raw implementation paths.

## Wiki Reads

Read wiki pages through `/n/<namespace>/wiki/*`:

```bash
curl -sS http://127.0.0.1:8000/n/default/wiki/index.md
```

The wiki root paths serve the wiki index:

```text
/n/default/wiki
/n/default/wiki/
/n/default/wiki/index.md
```

The `/wiki`, `/wiki/`, and `/wiki/index.md` aliases serve the same default namespace index.

If `index.md` is missing, Alien Shard creates a managed index before serving it. See `docs/wiki.md` for index ownership and refresh rules.

## Wiki Create And Update

Create or update a Markdown page:

```bash
curl -i -X PUT \
  -H 'Content-Type: text/markdown; charset=utf-8' \
  --data-binary @note.md \
  http://127.0.0.1:8000/n/default/wiki/path/to/note.md
```

Behavior:

- `PUT /n/<namespace>/wiki/<path>.md` creates or replaces the target file under the namespace wiki root.
- Parent directories are created automatically.
- The request body is written as the Markdown file content.
- `201 Created` means the page did not previously exist.
- `200 OK` means an existing page was updated.
- `PUT /n/<namespace>/wiki/index.md` is allowed.

## Wiki Delete

Delete a Markdown page:

```bash
curl -i -X DELETE \
  http://127.0.0.1:8000/n/default/wiki/path/to/note.md
```

Behavior:

- `DELETE /n/<namespace>/wiki/<path>.md` removes the target file from the namespace wiki root.
- `204 No Content` means the page was deleted.
- `404 Not Found` means the page did not exist.
- Directory targets are rejected.
- `DELETE /n/<namespace>/wiki/index.md` removes the index file without immediately regenerating it.
- After deleting `index.md`, the next read of the namespace wiki root recreates it if missing.

## Wiki Path Validation

Wiki mutations are intentionally restricted.

Accepted mutation paths must:

- Start with a wiki mount, such as `/n/default/wiki/` or `/wiki/`.
- Point to a file, not the wiki root or a trailing slash directory path.
- End in `.md`, case-insensitive.
- Avoid empty path parts.
- Avoid `.` and `..` path parts.
- Avoid backslashes.
- Resolve inside the namespace wiki root.

Examples rejected by `PUT` and `DELETE`:

```text
/wiki
/wiki/
/wiki/folder/
/wiki/page.txt
/wiki/../secret.md
/wiki/a//page.md
/wiki/a\b.md
```

## Status Codes

| Operation | Status | Meaning |
| --- | --- | --- |
| `GET` or `HEAD` existing file | `200` | File served. |
| `GET /wiki`, `/wiki/`, or `/wiki/index.md` | `200` | Managed or manual index served. |
| `GET /n/<namespace>/wiki`, `/n/<namespace>/wiki/`, or `/n/<namespace>/wiki/index.md` | `200` | Managed or manual namespace index served. |
| `PUT /n/<namespace>/wiki/<path>.md` new page | `201` | Page created. |
| `PUT /n/<namespace>/wiki/<path>.md` existing page | `200` | Page updated. |
| `DELETE /n/<namespace>/wiki/<path>.md` existing page | `204` | Page deleted. |
| Missing files | `404` | File not found. |
| Invalid wiki mutation target | `400` | Bad request, such as a non-Markdown or directory target. |
| Forbidden wiki mutation path | `403` | Traversal-like or otherwise unsafe path. |
| Unsupported wiki method | `405` | Method not allowed. |
| Filesystem or render failure | `500` | Server-side error. |
| `GET /n/<namespace>/search` with invalid query parameters | `400` | Missing `q`, invalid `scope`, or invalid `limit`. |
| `POST /n/<namespace>/search/reindex` accepted | `202` | Background reindex started. |
| `POST /n/<namespace>/search/reindex` while indexing | `409` | Reindex already in progress. |

## Search Endpoints

Search uses a persistent SQLite FTS5 index under each namespace raw root. Build the default namespace offline with:

```bash
alienshard index rebuild --home-dir /data
```

Build another namespace with `--namespace` or `ALIEN_NAMESPACE`:

```bash
ALIEN_NAMESPACE=research alienshard index rebuild --home-dir /data
```

Search both public content layers:

```text
GET /n/<namespace>/search?q=<query>&scope=all|raw|wiki&limit=20
```

`GET /search?...` is the default namespace alias.

Behavior:

- `q` is required.
- `scope` defaults to `all`.
- `limit` defaults to `20` and must be `1-100`.
- Responses are JSON for all user agents.
- If no index exists, the response is `200 OK` with `index_state: "not_indexed"` and an empty result list.
- If a background rebuild is running and an old index exists, searches continue using the old index with `index_state: "indexing"`.

Example response:

```json
{
  "query": "persistent knowledge",
  "scope": "all",
  "index_state": "ready",
  "results": [
    {
      "mount": "raw",
      "path": "/n/default/raw/docs/llm-wiki.md",
      "title": "LLM Wiki",
      "score": 9.8,
      "snippet": "...persistent knowledge..."
    }
  ]
}
```

Check indexing state:

```text
GET /n/<namespace>/search/status
```

Start a server-side background rebuild:

```text
POST /n/<namespace>/search/reindex
```

`POST /n/<namespace>/search/reindex` returns `202 Accepted` when the rebuild starts. The active index is atomically replaced only after a successful rebuild.
