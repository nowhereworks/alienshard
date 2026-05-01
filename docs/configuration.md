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
| `/data` | Raw source tree served through `/raw/*`. |
| `/data/__wiki` | Wiki storage served through `/wiki/*`. |

The wiki implementation directory is blocked through `/raw/__wiki` so clients use `/wiki/*` consistently.

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
