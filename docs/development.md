# Development

This document collects commands and release conventions for working on Alien Shard.

## Requirements

- Go `1.26.1`
- Docker, for image builds and container checks

## Commands

Run tests:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test ./... -coverprofile=coverage.out -covermode=atomic && go tool cover -func=coverage.out
```

Show serve help:

```bash
go run . serve --help
```

Show index help and rebuild a local index:

```bash
go run . index --help
go run . index rebuild --home-dir /tmp/alienshard-data
```

Build the binary:

```bash
go build .
```

Build the container image:

```bash
docker build -t alienshard .
```

Run the local container image:

```bash
docker run --rm -p 8000:8000 -v "$PWD:/data" alienshard
```

Run the published stable image:

```bash
docker run --rm -p 8000:8000 -v "$PWD:/data" nowhereworks/alienshard:latest
```

## Smoke Test

Run the wiki smoke test against a fresh local build on port `8001`:

```bash
make smoke-wiki
```

Run the same smoke test against an already running server:

```bash
ALIENSHARD_URL=http://127.0.0.1:8001 make smoke-wiki
```

The smoke test verifies:

- The server becomes reachable.
- Wiki pages can be written.
- Wiki pages can be deleted.
- `/raw` listings skip `__wiki`.
- `/wiki` uses generated public `/wiki/...` links.
- Generated wiki links resolve.

## Tests

Tests cover command configuration, mount behavior, Markdown rendering, wiki mutation validation, autoindex behavior, search indexing/querying/reindexing, Docker-sensitive file ownership assumptions, and helper functions.

For code changes:

- Add or improve tests for changed behavior.
- Prefer executable checks over prose-only assertions.
- Run `go test ./...` before considering the change complete.
- Use coverage when touching behavior that has edge cases or release risk.

## Release Workflow

Release automation lives in `.github/workflows/release.yaml`.

The workflow runs on pushes to `main` and `v*` tags when relevant source, Docker, or workflow files change.

Jobs:

| Job | Purpose |
| --- | --- |
| `test` | Runs `go test ./... -coverprofile=coverage.out -covermode=atomic` and reports coverage. |
| `build` | Builds release binaries for Linux, macOS, and Windows. |
| `github-release` | Publishes the replaceable `latest` release for `main` or stable releases for `v*` tags. |
| `docker-image` | Publishes Docker Hub images after the GitHub release job succeeds. |

Branch and tag behavior:

| Ref | GitHub release | Docker tags |
| --- | --- | --- |
| `main` | Replaceable `latest` release | `edge`, `main`, `sha-<shortsha>` |
| `v*` | Stable release for the tag | `latest`, `vX.Y.Z`, `X.Y.Z`, `X.Y`, `X` |

Docker Hub publishing requires GitHub secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.

## Documentation Site

The documentation site is built with MkDocs Material and published to GitHub Pages by `.github/workflows/docs.yaml`.

Create a local documentation virtual environment and install dependencies:

```bash
python -m venv .venv-docs
.venv-docs/bin/python -m pip install -r requirements-docs.txt
```

Build the site locally:

```bash
.venv-docs/bin/python -m mkdocs build --strict
```

Use a virtual environment because some systems block global pip installs for externally managed Python installations.

Automatic publishing runs only for pushes to `main` when documentation-related files change:

- `docs/**`
- `mkdocs.yml`
- `requirements-docs.txt`
- `.github/workflows/docs.yaml`

GitHub Pages must be configured in the repository settings with source set to GitHub Actions.

## Documentation Conventions

`README.md` is the all-around quickstarter. Put deeper reference material in `docs/`.

Current docs:

| File | Purpose |
| --- | --- |
| `docs/http-api.md` | HTTP routes, methods, content negotiation, status codes, and path validation. |
| `docs/configuration.md` | Flags, environment variables, Docker defaults, and image tags. |
| `docs/wiki.md` | Wiki storage, mutations, autoindex, and manual index behavior. |
| `docs/llm-wiki.md` | Product concept and LLM-maintained wiki pattern. |
| `docs/search.md` | Search design, implementation notes, and future roadmap. |
| `docs/development.md` | Build, test, smoke, release, and documentation workflow. |

Keep `CHANGELOG.md` brief and current with user-visible changes and notable project decisions.

## Release Safety

Never commit or publish secrets, credentials, tokens, private environment values, personal data, local wiki contents, generated artifacts, logs, local binaries, or editor-local settings.

Before committing, tagging, releasing, or publishing, inspect staged, unstaged, untracked, and ignored files for non-public or sensitive data.

Do not stage ignored or local artifacts such as:

- `.env*`
- key files
- `__wiki/`
- `coverage.out`
- local binaries
- logs
- personal editor settings
