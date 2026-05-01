# Alienshard

Use this skill when the user asks to use Alienshard, shared wiki notes, agent handoff notes, or workspace files over HTTP.

Alienshard exposes a local HTTP interface for reading raw workspace files and reading, writing, or deleting Markdown wiki pages. Do not guess a server URL. Resolve it with the workflow below before using Alienshard. Use `ALIEN_NAMESPACE` when present; otherwise use `default`.

## Resolve The URL

Resolve the Alienshard base URL in this order:

1. Use the `ALIEN_URL` environment variable when present.
2. Search the workspace `AGENTS.md` for an explicit Alienshard URL.
3. Fall back to `http://127.0.0.1:8000`.

Only accept explicit `AGENTS.md` forms such as:

```md
ALIEN_URL=http://127.0.0.1:8000
Alienshard URL: http://127.0.0.1:8000
alienshard: http://127.0.0.1:8000
```

If multiple entries are present, prefer the first explicit `ALIEN_URL=` entry. Otherwise use the first explicit Alienshard URL.

Normalize the selected URL:

- Add `http://` if no scheme is present.
- Remove trailing slashes.
- Reject empty or malformed values.
- Do not invent hostnames beyond the fallback.

Resolve the namespace:

- Use `ALIEN_NAMESPACE` when present.
- Otherwise use `default`.
- Namespace names are flat slugs; do not include slashes.

You can use the bundled helper:

```bash
./skill/scripts/resolve-alienshard-url.sh
```

## Verify Reachability

Probe Alienshard before relying on it:

```bash
ALIEN_URL="$(./skill/scripts/resolve-alienshard-url.sh)"
ALIEN_NAMESPACE="${ALIEN_NAMESPACE:-default}"
curl -fsS "$ALIEN_URL/n/$ALIEN_NAMESPACE/wiki/index.md"
```

If that fails, optionally probe the raw mount:

```bash
ALIEN_URL="$(./skill/scripts/resolve-alienshard-url.sh)"
ALIEN_NAMESPACE="${ALIEN_NAMESPACE:-default}"
curl -fsS "$ALIEN_URL/n/$ALIEN_NAMESPACE/raw/"
```

If both fail, report that Alienshard was not reachable. Include the resolved URL and where it came from if known.

## Endpoints

- `GET /n/<namespace>/raw/<path>` reads files from the namespace raw root.
- `GET /n/<namespace>/wiki/<path>.md` reads wiki Markdown.
- `PUT /n/<namespace>/wiki/<path>.md` creates or updates wiki Markdown.
- `DELETE /n/<namespace>/wiki/<path>.md` deletes wiki Markdown.
- `/raw/*`, `/wiki/*`, and `/search*` are default namespace aliases.
- Raw implementation directories such as `__wiki`, `__namespaces`, and `.alienshard` are intentionally blocked.
- Wiki files are stored under the server's `__wiki` directories, but clients should access them via wiki mounts.

## Read Examples

Read a workspace file:

```bash
ALIEN_URL="$(./skill/scripts/resolve-alienshard-url.sh)"
ALIEN_NAMESPACE="${ALIEN_NAMESPACE:-default}"
curl -fsS "$ALIEN_URL/n/$ALIEN_NAMESPACE/raw/README.md"
```

Read a wiki page:

```bash
ALIEN_URL="$(./skill/scripts/resolve-alienshard-url.sh)"
ALIEN_NAMESPACE="${ALIEN_NAMESPACE:-default}"
curl -fsS "$ALIEN_URL/n/$ALIEN_NAMESPACE/wiki/index.md"
```

## Write Examples

Create or update a wiki Markdown page:

```bash
ALIEN_URL="$(./skill/scripts/resolve-alienshard-url.sh)"
ALIEN_NAMESPACE="${ALIEN_NAMESPACE:-default}"
curl -fsS -X PUT \
  -H 'Content-Type: text/markdown; charset=utf-8' \
  --data-binary @note.md \
  "$ALIEN_URL/n/$ALIEN_NAMESPACE/wiki/path/to/note.md"
```

Rules for writes:

- Only write `.md` paths.
- Do not use traversal-like paths such as `../secret.md`.
- Treat HTTP `201` as created and HTTP `200` as updated.
- Prefer writing durable notes, investigation findings, and agent handoff context to `/n/<namespace>/wiki/*`.
- Do not write secrets, credentials, tokens, or private environment values.

## Delete Examples

Delete a wiki Markdown page:

```bash
ALIEN_URL="$(./skill/scripts/resolve-alienshard-url.sh)"
ALIEN_NAMESPACE="${ALIEN_NAMESPACE:-default}"
curl -fsS -X DELETE \
  "$ALIEN_URL/n/$ALIEN_NAMESPACE/wiki/path/to/note.md"
```

Rules for deletes:

- Only delete `.md` paths.
- Do not use traversal-like paths such as `../secret.md`.
- Treat HTTP `204` as deleted and HTTP `404` as already missing.
- Do not delete durable wiki notes unless the user asked for removal or the page is clearly obsolete/generated test content.

## Failure Handling

When Alienshard cannot be reached:

- State the resolved URL.
- State whether it came from `ALIEN_URL`, `AGENTS.md`, or fallback if known.
- Include the failed probe endpoint.
- Do not silently keep trying alternate guessed hosts.
