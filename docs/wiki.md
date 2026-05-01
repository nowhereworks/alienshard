# Wiki

Alien Shard serves writable Markdown wikis beside raw source trees. Each namespace has its own raw, wiki, and search storage.

## Layers

| Layer | Public mount | Filesystem location | Role |
| --- | --- | --- | --- |
| Raw | `/n/<namespace>/raw/*` | namespace raw root | Source files and documents. |
| Wiki | `/n/<namespace>/wiki/*` | namespace raw root `__wiki` | LLM-maintained Markdown pages. |

The raw layer is intended as source material. The wiki layer is intended as the maintained synthesis over those sources.

The default namespace is `default`. `/raw/*` and `/wiki/*` are compatibility aliases for `/n/default/raw/*` and `/n/default/wiki/*`.

## Storage

If the server starts with `--home-dir /data`, default namespace wiki pages are stored under:

```text
/data/__wiki
```

Non-default namespace wiki pages are stored under:

```text
/data/__namespaces/<namespace>/__wiki
```

Examples:

| Public path | Disk path |
| --- | --- |
| `/n/default/wiki/index.md` | `/data/__wiki/index.md` |
| `/n/default/wiki/project/notes.md` | `/data/__wiki/project/notes.md` |
| `/n/research/wiki/index.md` | `/data/__namespaces/research/__wiki/index.md` |
| `/n/research/wiki/project/notes.md` | `/data/__namespaces/research/__wiki/project/notes.md` |

Implementation directories are not served through raw mounts; clients should use wiki mounts.

## Mutations

Wiki Markdown pages can be created, updated, and deleted over HTTP.

```bash
curl -i -X PUT \
  -H 'Content-Type: text/markdown' \
  --data-binary '# Notes' \
  http://127.0.0.1:8000/n/default/wiki/notes.md
```

```bash
curl -i -X DELETE \
  http://127.0.0.1:8000/n/default/wiki/notes.md
```

Only `.md` targets can be mutated. See `docs/http-api.md` for the full API and path validation rules.

## Auto-Managed Index

Alien Shard can manage each namespace root wiki index at `<namespace-raw-root>/__wiki/index.md`.

Managed indexes start with this marker as the first line:

```md
<!-- alienshard:autoindex v1 -->
```

Managed index behavior:

- If `index.md` is missing, it is generated automatically.
- `GET /n/<namespace>/wiki`, `GET /n/<namespace>/wiki/`, and `GET /n/<namespace>/wiki/index.md` serve the index.
- Index reads regenerate `index.md` when the marker is present.
- Successful namespace wiki `PUT` refreshes the index when the marker is present.
- Successful non-index namespace wiki `DELETE` refreshes the index when the marker is present.
- `DELETE /n/<namespace>/wiki/index.md` removes the index without immediately regenerating it.
- The next index read recreates `index.md` when it is missing.

Generated entries:

- Exclude `index.md` pages.
- Exclude a root-level `__wiki` subtree inside the wiki directory.
- Use canonical public `/n/<namespace>/wiki/...` links.
- Sort paths lexically.

Example generated index:

```md
<!-- alienshard:autoindex v1 -->

# Index

- [notes](/n/default/wiki/notes.md)
- [summary](/n/default/wiki/project/summary.md)
```

## Manual Index

If `index.md` exists without the autoindex marker, Alien Shard treats it as manually owned.

Manual index behavior:

- Reads serve the manual file as-is.
- Wiki `PUT` and non-index `DELETE` operations do not modify it.
- `PUT /n/<namespace>/wiki/index.md` can switch between manual and managed ownership depending on marker presence.

## LLM Wiki Pattern

Alien Shard is designed for persistent, LLM-maintained wiki workflows:

- Raw files are the curated source material.
- Wiki pages are maintained summaries, entities, concepts, comparisons, and handoff notes.
- The wiki compounds over time instead of being regenerated from scratch for each query.

The concept document is `docs/llm-wiki.md`.
