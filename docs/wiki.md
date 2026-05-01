# Wiki

Alien Shard serves a writable Markdown wiki beside a read-only raw source tree.

## Layers

| Layer | Public mount | Filesystem location | Role |
| --- | --- | --- | --- |
| Raw | `/raw/*` | `rawRoot` | Source files and documents. |
| Wiki | `/wiki/*` | `rawRoot/__wiki` | LLM-maintained Markdown pages. |

The raw layer is intended as source material. The wiki layer is intended as the maintained synthesis over those sources.

## Storage

If the server starts with `--home-dir /data`, wiki pages are stored under:

```text
/data/__wiki
```

Examples:

| Public path | Disk path |
| --- | --- |
| `/wiki/index.md` | `/data/__wiki/index.md` |
| `/wiki/project/notes.md` | `/data/__wiki/project/notes.md` |

The implementation directory is not served through `/raw/__wiki`; clients should use `/wiki/*`.

## Mutations

Wiki Markdown pages can be created, updated, and deleted over HTTP.

```bash
curl -i -X PUT \
  -H 'Content-Type: text/markdown' \
  --data-binary '# Notes' \
  http://127.0.0.1:8000/wiki/notes.md
```

```bash
curl -i -X DELETE \
  http://127.0.0.1:8000/wiki/notes.md
```

Only `.md` targets can be mutated. See `docs/http-api.md` for the full API and path validation rules.

## Auto-Managed Index

Alien Shard can manage the root wiki index at `rawRoot/__wiki/index.md`.

Managed indexes start with this marker as the first line:

```md
<!-- alienshard:autoindex v1 -->
```

Managed index behavior:

- If `index.md` is missing, it is generated automatically.
- `GET /wiki`, `GET /wiki/`, and `GET /wiki/index.md` serve the index.
- Index reads regenerate `index.md` when the marker is present.
- Successful `PUT /wiki/*.md` refreshes the index when the marker is present.
- Successful non-index `DELETE /wiki/*.md` refreshes the index when the marker is present.
- `DELETE /wiki/index.md` removes the index without immediately regenerating it.
- The next index read recreates `index.md` when it is missing.

Generated entries:

- Exclude `index.md` pages.
- Exclude a root-level `__wiki` subtree inside the wiki directory.
- Use public `/wiki/...` links.
- Sort paths lexically.

Example generated index:

```md
<!-- alienshard:autoindex v1 -->

# Index

- [notes](/wiki/notes.md)
- [summary](/wiki/project/summary.md)
```

## Manual Index

If `index.md` exists without the autoindex marker, Alien Shard treats it as manually owned.

Manual index behavior:

- Reads serve the manual file as-is.
- Wiki `PUT` and non-index `DELETE` operations do not modify it.
- `PUT /wiki/index.md` can switch between manual and managed ownership depending on marker presence.

## LLM Wiki Pattern

Alien Shard is designed for persistent, LLM-maintained wiki workflows:

- Raw files are the curated source material.
- Wiki pages are maintained summaries, entities, concepts, comparisons, and handoff notes.
- The wiki compounds over time instead of being regenerated from scratch for each query.

The concept document is `docs/llm-wiki.md`.
