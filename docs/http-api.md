# HTTP API

Alien Shard exposes two explicit public mounts:

| Mount | Filesystem root | Purpose |
| --- | --- | --- |
| `/raw/*` | `rawRoot` | Read files from the served source tree. |
| `/wiki/*` | `rawRoot/__wiki` | Read, create, update, and delete wiki Markdown pages. |

`rawRoot` is `--home-dir` when provided, otherwise the process current working directory.

## Markdown Responses

Markdown rendering applies to `.md` paths on both `/raw/*` and `/wiki/*`.

| Client `User-Agent` | Response |
| --- | --- |
| Contains `chrome` or `firefox`, case-insensitive | Goldmark-rendered HTML fragment with `Content-Type: text/html; charset=utf-8`. |
| Anything else | Raw Markdown with `Content-Type: text/markdown; charset=utf-8`. |

Missing Markdown files fall back to the standard file-server response, usually `404 Not Found`.

## Raw Reads

Read files through `/raw/*`:

```bash
curl -sS http://127.0.0.1:8000/raw/README.md
```

The `/raw` mount uses Go's `http.FileServer` behavior for files and directory listings.

Safety rules:

- `/raw/__wiki` and `/raw/__wiki/*` return `404 Not Found`.
- Root `/raw` directory listings omit the root-level `__wiki` implementation directory.
- Wiki content is publicly addressed through `/wiki/*`, never `/raw/__wiki/*`.

## Wiki Reads

Read wiki pages through `/wiki/*`:

```bash
curl -sS http://127.0.0.1:8000/wiki/index.md
```

The wiki root paths serve the wiki index:

```text
/wiki
/wiki/
/wiki/index.md
```

If `index.md` is missing, Alien Shard creates a managed index before serving it. See `docs/wiki.md` for index ownership and refresh rules.

## Wiki Create And Update

Create or update a Markdown page:

```bash
curl -i -X PUT \
  -H 'Content-Type: text/markdown; charset=utf-8' \
  --data-binary @note.md \
  http://127.0.0.1:8000/wiki/path/to/note.md
```

Behavior:

- `PUT /wiki/<path>.md` creates or replaces the target file under `rawRoot/__wiki`.
- Parent directories are created automatically.
- The request body is written as the Markdown file content.
- `201 Created` means the page did not previously exist.
- `200 OK` means an existing page was updated.
- `PUT /wiki/index.md` is allowed.

## Wiki Delete

Delete a Markdown page:

```bash
curl -i -X DELETE \
  http://127.0.0.1:8000/wiki/path/to/note.md
```

Behavior:

- `DELETE /wiki/<path>.md` removes the target file from `rawRoot/__wiki`.
- `204 No Content` means the page was deleted.
- `404 Not Found` means the page did not exist.
- Directory targets are rejected.
- `DELETE /wiki/index.md` removes the index file without immediately regenerating it.
- After deleting `index.md`, the next read of `/wiki`, `/wiki/`, or `/wiki/index.md` recreates it if missing.

## Wiki Path Validation

Wiki mutations are intentionally restricted.

Accepted mutation paths must:

- Start with `/wiki/`.
- Point to a file, not `/wiki` or a trailing slash directory path.
- End in `.md`, case-insensitive.
- Avoid empty path parts.
- Avoid `.` and `..` path parts.
- Avoid backslashes.
- Resolve inside `rawRoot/__wiki`.

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
| `PUT /wiki/<path>.md` new page | `201` | Page created. |
| `PUT /wiki/<path>.md` existing page | `200` | Page updated. |
| `DELETE /wiki/<path>.md` existing page | `204` | Page deleted. |
| Missing files | `404` | File not found. |
| Invalid wiki mutation target | `400` | Bad request, such as a non-Markdown or directory target. |
| Forbidden wiki mutation path | `403` | Traversal-like or otherwise unsafe path. |
| Unsupported `/wiki` method | `405` | Method not allowed. |
| Filesystem or render failure | `500` | Server-side error. |
| `GET /search` with invalid query parameters | `400` | Missing `q`, invalid `scope`, or invalid `limit`. |
| `POST /search/reindex` accepted | `202` | Background reindex started. |
| `POST /search/reindex` while indexing | `409` | Reindex already in progress. |

## Search Endpoints

Search uses a persistent SQLite FTS5 index under `rawRoot/.alienshard/search.sqlite`. Build it offline with:

```bash
alienshard index rebuild --home-dir /data
```

Search both public content layers:

```text
GET /search?q=<query>&scope=all|raw|wiki&limit=20
```

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
      "path": "/raw/docs/llm-wiki.md",
      "title": "LLM Wiki",
      "score": 9.8,
      "snippet": "...persistent knowledge..."
    }
  ]
}
```

Check indexing state:

```text
GET /search/status
```

Start a server-side background rebuild:

```text
POST /search/reindex
```

`POST /search/reindex` returns `202 Accepted` when the rebuild starts. The active index is atomically replaced only after a successful rebuild.
