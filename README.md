# Alien Shard

[![Release](https://github.com/nowhereworks/alienshard/actions/workflows/release.yaml/badge.svg)](https://github.com/nowhereworks/alienshard/actions/workflows/release.yaml)
[![Latest Testing Release](https://img.shields.io/github/v/release/nowhereworks/alienshard/latest?label=Latest%20Testing%20Release)](https://github.com/nowhereworks/alienshard/releases/tag/latest)
[![Latest Stable Release](https://img.shields.io/github/v/release/nowhereworks/alienshard?display_name=tag&label=Latest%20Stable%20Release)](https://github.com/nowhereworks/alienshard/releases)

<p align="center">
  <img src="docs/alien-shard.png" alt="Alien Shard logo" width="520" />
</p>

Alien Shard is a lightweight Go server for mixed human + machine Markdown workflows.

- Humans using Chrome or Firefox get rendered HTML for `.md` files.
- Machines, agents, curl, and scripts get raw Markdown for `.md` files.
- One process serves raw source files and a writable wiki layer.

## Why

Most tooling makes you pick one mode: static site for humans, API output for machines.
Alien Shard keeps both in the same place so an LLM can write and maintain wiki pages while you browse them directly in a browser.

## Quick Start

Requirements:

- Go `1.26.1`
- Docker, for container usage

Run locally:

```bash
go run . serve
```

Serve a specific directory:

```bash
go run . serve --home-dir /tmp
```

By default the server binds to `127.0.0.1:8000`.

Open in a browser:

- `http://127.0.0.1:8000/raw/`
- `http://127.0.0.1:8000/wiki`

Read Markdown as a machine client:

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

Delete a wiki page:

```bash
curl -i -X DELETE \
  http://127.0.0.1:8000/wiki/project/notes.md
```

Build the local search index:

```bash
go run . index rebuild --home-dir /tmp
```

Search indexed raw and wiki content:

```bash
curl -sS 'http://127.0.0.1:8000/search?q=project&scope=all'
```

## Container

Run the latest published `main` branch image from Docker Hub with the current directory mounted as the served data root:

```bash
docker run --rm \
  -p 8000:8000 \
  -v "$PWD:/data" \
  nowhereworks/alienshard:edge
```

The container serves `/data`, binds to `0.0.0.0:8000`, and writes wiki pages under `/data/__wiki`.

Build locally:

```bash
docker build -t alienshard .
```

Use `nowhereworks/alienshard:latest` after a stable `v*` release has published.

## Command Options

Serve options:

```text
--home-dir string   Directory to serve (env ALIEN_HOME_DIR, default current directory)
--bind string       IP address to bind (env ALIEN_BIND, default "127.0.0.1")
--port int          TCP port to bind (env ALIEN_PORT, default 8000)
```

Index rebuild options:

```text
--home-dir string   Directory to index (default current directory)
```

## Documentation

- Published docs: `https://nowhereworks.github.io/alienshard/`
- HTTP API reference: `docs/http-api.md`
- Configuration and containers: `docs/configuration.md`
- Wiki behavior and autoindex: `docs/wiki.md`
- Development and release workflow: `docs/development.md`
- LLM Wiki pattern: `docs/llm-wiki.md`
- Search design and status: `docs/search.md`

## Development

Run tests:

```bash
go test ./...
```

Run the wiki smoke test:

```bash
make smoke-wiki
```

Show CLI help:

```bash
go run . serve --help
go run . index --help
```

## Credits

- LLM Wiki concept: Andrej Karpathy, `https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f`
