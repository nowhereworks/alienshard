# Alien Shard

[![Release](https://github.com/nowhereworks/alienshard/actions/workflows/release.yaml/badge.svg)](https://github.com/nowhereworks/alienshard/actions/workflows/release.yaml)
[![Latest Testing Release](https://img.shields.io/github/v/release/nowhereworks/alienshard/latest?label=Latest%20Testing%20Release)](https://github.com/nowhereworks/alienshard/releases/tag/latest)
[![Latest Stable Release](https://img.shields.io/github/v/release/nowhereworks/alienshard?display_name=tag&label=Latest%20Stable%20Release)](https://github.com/nowhereworks/alienshard/releases)

<p align="center">
  <img src="docs/alien-shard.png" alt="Alien Shard logo" width="520" />
</p>

Alien Shard is a lightweight Go server for mixed human + machine Markdown workflows.

- Humans (browser user-agents) get rendered HTML for `.md` files.
- Machines (agents, curl, scripts) get raw Markdown for `.md` files.
- One process serves immutable raw sources and a writable wiki layer.

## Why

Most tooling makes you pick one mode: static site for humans, API output for machines.
Alien Shard keeps both in the same place so an LLM can write and maintain wiki pages while
you browse them directly in a browser.

## Features

- Explicit dual mounts:
  - `/raw/*` -> raw source tree (`--home-dir` or current directory)
  - `/wiki/*` -> wiki tree at `rawRoot/__wiki`
- Markdown rendering policy on both mounts:
  - `User-Agent` contains `chrome` or `firefox` -> rendered HTML
  - otherwise -> raw Markdown
- Wiki write API:
  - `PUT /wiki/<path>.md` creates/updates markdown files
  - parent directories are created automatically
  - returns `201` on create, `200` on update
- Auto-managed wiki index:
  - generated marker: `<!-- alienshard:autoindex v1 -->`
  - `index.md` is auto-created when missing
  - `/wiki`, `/wiki/`, and `/wiki/index.md` serve the managed wiki index
  - refreshed on read and after every successful wiki write when marker is present
  - manual `index.md` (no marker) is never overwritten
- Safety:
  - traversal-like wiki write paths are rejected
  - `/raw/__wiki` is blocked with `404` (wiki is only reachable via `/wiki/*`)
  - `/raw` directory listings skip root-level `__wiki`

## Quick Start

Requirements:

- Go `1.26.1`
- Docker, for container usage

Run:

```bash
go run . serve
```

Serve a specific directory:

```bash
go run . serve --home-dir /tmp
```

By default the server binds to `127.0.0.1:8000`.

Run the wiki root smoke test against a fresh local build on port `8001`:

```bash
make smoke-wiki
```

Run the same smoke test against an already running server:

```bash
ALIENSHARD_URL=http://127.0.0.1:8001 make smoke-wiki
```

## Container

Build the image:

```bash
docker build -t alienshard .
```

Run the latest published `main` branch image from Docker Hub with the current directory mounted as the served data root:

```bash
docker run --rm \
  -p 8000:8000 \
  -v "$PWD:/data" \
  nowhereworks/alienshard:edge
```

The container serves `/data`, binds to `0.0.0.0:8000`, and writes wiki pages under
the mounted `/data/__wiki` directory.

The image runs as UID/GID `1000` so files written to common Linux bind mounts are
owned by the host user instead of root. If your mounted directory is owned by a
different user, pass `--user "$(id -u):$(id -g)"` to `docker run`.

Override container options with environment variables:

```bash
docker run --rm \
  -p 9000:9000 \
  -e PORT=9000 \
  -v "$PWD:/data" \
  nowhereworks/alienshard:edge
```

Use `nowhereworks/alienshard:latest` after a stable `v*` release has published.

Published image tags:

- `latest`: latest stable `v*` release
- `vX.Y.Z`: exact stable release tag
- `X.Y.Z`, `X.Y`, `X`: semver aliases for stable releases
- `edge`: latest successful `main` branch release
- `main`: latest successful `main` branch release
- `sha-<shortsha>`: exact `main` branch build

## Command Options

```text
--home-dir string   Directory to serve (env HOME_DIR, default current directory)
--bind string       IP address to bind (env BIND, default "127.0.0.1")
--port int          TCP port to bind (env PORT, default 8000)
```

## API Examples

Assume server is running on `http://127.0.0.1:8000`.

Read raw markdown as a machine client:

```bash
curl -sS http://127.0.0.1:8000/raw/notes.md
```

Write a wiki page:

```bash
curl -i -X PUT \
  -H "Content-Type: text/markdown" \
  --data-binary "# Project Notes\n\nInitial draft." \
  http://127.0.0.1:8000/wiki/project/notes.md
```

Read wiki index (auto-created if missing):

```bash
curl -sS http://127.0.0.1:8000/wiki
```

Open these URLs in a browser to view rendered HTML:

- `http://127.0.0.1:8000/raw/.../*.md`
- `http://127.0.0.1:8000/wiki/.../*.md`

## LLM Wiki Pattern

This project is designed to support persistent, LLM-maintained wiki workflows.

Attribution: the LLM Wiki concept is from Andrej Karpathy:
`https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f`

- Concept document: `docs/llm-wiki.md`
- Raw layer: `/raw/*`
- Writable wiki layer: `/wiki/*`

## Development

Run tests:

```bash
go test ./...
```

Show CLI help:

```bash
go run . serve --help
```

## Credits

- LLM Wiki concept: Andrej Karpathy, `https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f`
