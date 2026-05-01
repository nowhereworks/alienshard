#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"

ALIEN_PORT="${ALIEN_PORT:-8001}"
ALIEN_BIND="${ALIEN_BIND:-127.0.0.1}"
PAGE_PATH="${ALIEN_SMOKE_PAGE:-/wiki/__smoke/wiki-root.md}"
PAGE_NAME="${PAGE_PATH##*/}"
DELETE_PAGE_PATH="/wiki/__smoke/delete-me.md"
BASE_URL="${ALIEN_URL:-}"
TMP_DIR=""
SERVER_PID=""
LOG_FILE=""

fail() {
	printf 'error: %s\n' "$*" >&2
	if [[ -n "$LOG_FILE" && -f "$LOG_FILE" ]]; then
		printf '\nserver log:\n' >&2
		while IFS= read -r line; do
			printf '%s\n' "$line" >&2
		done <"$LOG_FILE"
	fi
	exit 1
}

cleanup() {
	if [[ -n "$SERVER_PID" ]]; then
		kill "$SERVER_PID" >/dev/null 2>&1 || true
		wait "$SERVER_PID" >/dev/null 2>&1 || true
	fi
	if [[ -n "$TMP_DIR" ]]; then
		rm -rf "$TMP_DIR"
	fi
}
trap cleanup EXIT

if [[ -z "$BASE_URL" ]]; then
	TMP_DIR="$(mktemp -d)"
	LOG_FILE="$TMP_DIR/server.log"
	ALIEN_HOME_DIR="$TMP_DIR/data"
	BIN="$TMP_DIR/alienshard"
	mkdir -p "$ALIEN_HOME_DIR"

	(cd "$REPO_ROOT" && go build -o "$BIN" .)
	ALIEN_HOME_DIR="$ALIEN_HOME_DIR" ALIEN_BIND="$ALIEN_BIND" ALIEN_PORT="$ALIEN_PORT" "$BIN" serve >"$LOG_FILE" 2>&1 &
	SERVER_PID="$!"
	BASE_URL="http://$ALIEN_BIND:$ALIEN_PORT"

	for _ in {1..50}; do
		if curl -fsS "$BASE_URL/raw/" >/dev/null 2>&1; then
			break
		fi
		if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
			fail "server exited before becoming ready"
		fi
		sleep 0.1
	done

	if ! curl -fsS "$BASE_URL/raw/" >/dev/null 2>&1; then
		fail "server did not become ready at $BASE_URL"
	fi
else
	BASE_URL="${BASE_URL%/}"
fi

case "$PAGE_PATH" in
	/wiki/*.md) ;;
	*) fail "ALIEN_SMOKE_PAGE must look like /wiki/<path>.md" ;;
esac

curl -fsS \
	-X PUT \
	-H 'Content-Type: text/markdown' \
	--data-binary '# Wiki Smoke Test' \
	"$BASE_URL$PAGE_PATH" >/dev/null

curl -fsS \
	-X PUT \
	-H 'Content-Type: text/markdown' \
	--data-binary '# Delete Smoke Test' \
	"$BASE_URL$DELETE_PAGE_PATH" >/dev/null

curl -fsS -X DELETE "$BASE_URL$DELETE_PAGE_PATH" >/dev/null
if curl -fsS "$BASE_URL$DELETE_PAGE_PATH" >/dev/null 2>&1; then
	fail "expected deleted wiki page to return non-200: $DELETE_PAGE_PATH"
fi

raw_html="$(curl -fsS -A 'Mozilla/5.0 Chrome/126.0' "$BASE_URL/raw")"
if [[ "$raw_html" == *'href="__wiki/"'* ]]; then
	fail 'expected /raw autoindex to skip __wiki/'
fi

index_html="$(curl -fsS -A 'Mozilla/5.0 Chrome/126.0' "$BASE_URL/wiki")"
expected="href=\"$PAGE_PATH\""
relative="href=\"$PAGE_NAME\""

if [[ "$index_html" != *"$expected"* ]]; then
	fail "expected /wiki to contain $expected"
fi

if [[ "$index_html" == *"$relative"* ]]; then
	fail "expected /wiki not to contain file-server relative link $relative"
fi

declare -A seen_links=()
remaining="$index_html"
while [[ "$remaining" =~ href=\"(/wiki/[^\"]+)\" ]]; do
	link="${BASH_REMATCH[1]}"
	seen_links["$link"]=1
	remaining="${remaining#*href=\"$link\"}"
done

if [[ ${#seen_links[@]} -eq 0 ]]; then
	fail "expected /wiki to contain at least one generated /wiki link"
fi

for link in "${!seen_links[@]}"; do
	case "$link" in
		/wiki|/wiki/|/wiki/index.md|*/index.md)
			fail "expected autoindex not to link to index page $link"
			;;
	esac

	if ! curl -fsS -A 'Mozilla/5.0 Chrome/126.0' "$BASE_URL$link" >/dev/null; then
		fail "generated autoindex link returned non-200: $link"
	fi
done

curl -fsS -A 'Mozilla/5.0 Chrome/126.0' "$BASE_URL$PAGE_PATH" >/dev/null
printf 'ok: %s/wiki links to %s\n' "$BASE_URL" "$PAGE_PATH"
