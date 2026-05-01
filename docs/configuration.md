# Configuration

Alien Shard is configured with Cobra flags, Viper environment variables, or Docker defaults.

## Serve Command

```bash
alienshard serve
```

Flags:

| Flag | Environment | Default | Description |
| --- | --- | --- | --- |
| `--home-dir` | `ALIEN_HOME_DIR` | Current working directory | Directory served as `rawRoot`. |
| `--bind` | `ALIEN_BIND` | `127.0.0.1` | IP address to bind. |
| `--port` | `ALIEN_PORT` | `8000` | TCP port to bind. |

Examples:

```bash
alienshard serve --home-dir /data --bind 127.0.0.1 --port 8000
```

```bash
ALIEN_HOME_DIR=/data ALIEN_BIND=0.0.0.0 ALIEN_PORT=9000 alienshard serve
```

## Validation

Startup fails before binding when configuration is invalid.

Rules:

- `--home-dir` must exist.
- `--home-dir` must be a directory.
- `--bind` must be a valid IP address.
- `--port` must be in range `1-65535`.

When `--home-dir` is omitted or empty, Alien Shard resolves the process current working directory and uses it as `rawRoot`.

## Filesystem Layout

For a home directory of `/data`:

| Path | Meaning |
| --- | --- |
| `/data` | Default namespace raw source tree served through `/n/default/raw/*` and `/raw/*`. |
| `/data/__wiki` | Default namespace wiki storage served through `/n/default/wiki/*` and `/wiki/*`. |
| `/data/.alienshard/search.sqlite` | Default namespace search index. |
| `/data/__namespaces/<namespace>` | Non-default namespace raw source tree served through `/n/<namespace>/raw/*`. |
| `/data/__namespaces/<namespace>/__wiki` | Non-default namespace wiki storage served through `/n/<namespace>/wiki/*`. |
| `/data/__namespaces/<namespace>/.alienshard/search.sqlite` | Non-default namespace search index. |

Implementation directories are blocked through raw mounts so clients use public raw, wiki, and search URLs consistently.

## Client Namespace

Clients can source `ALIEN_NAMESPACE` to choose an isolated working area. The server still receives an ordinary URL; clients use the variable to build `/n/<namespace>/...` paths.

```bash
ALIEN_NAMESPACE=research
curl -sS "http://127.0.0.1:8000/n/$ALIEN_NAMESPACE/wiki/index.md"
```

Namespace names are flat lowercase slugs. They can contain lowercase letters, digits, dots, dashes, and underscores, must start with a letter or digit, and must not contain slashes.

## Index Command

`alienshard index rebuild` supports namespace selection:

| Flag | Environment | Default | Description |
| --- | --- | --- | --- |
| `--home-dir` | none | Current working directory | Base home directory. |
| `--namespace` | `ALIEN_NAMESPACE` | `default` | Namespace to index. |

Examples:

```bash
alienshard index rebuild --home-dir /data --namespace research
```

```bash
ALIEN_NAMESPACE=research alienshard index rebuild --home-dir /data
```

## Docker

Build locally:

```bash
docker build -t alienshard .
```

Run a local image with the current directory mounted as `/data`:

```bash
docker run --rm \
  -p 8000:8000 \
  -v "$PWD:/data" \
  alienshard
```

Run the latest published `main` branch image:

```bash
docker run --rm \
  -p 8000:8000 \
  -v "$PWD:/data" \
  nowhereworks/alienshard:edge
```

Container defaults:

| Setting | Value |
| --- | --- |
| `ALIEN_HOME_DIR` | `/data` |
| `ALIEN_BIND` | `0.0.0.0` |
| `ALIEN_PORT` | `8000` |
| User | `alienshard` UID/GID `1000` |
| Workdir | `/data` |

The UID/GID `1000` default improves common Linux bind-mount compatibility by avoiding root-owned wiki files. If your mounted directory is owned by another user, run with an explicit user:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -p 8000:8000 \
  -v "$PWD:/data" \
  nowhereworks/alienshard:edge
```

Override container options with environment variables:

```bash
docker run --rm \
  -p 9000:9000 \
  -e ALIEN_PORT=9000 \
  -v "$PWD:/data" \
  nowhereworks/alienshard:edge
```

## Published Image Tags

| Tag | Meaning |
| --- | --- |
| `latest` | Latest stable `v*` release. |
| `vX.Y.Z` | Exact stable release tag. |
| `X.Y.Z`, `X.Y`, `X` | Semver aliases for stable releases. |
| `edge` | Latest successful `main` branch release. |
| `main` | Latest successful `main` branch release. |
| `sha-<shortsha>` | Exact `main` branch build. |

Docker Hub image: `nowhereworks/alienshard`.
