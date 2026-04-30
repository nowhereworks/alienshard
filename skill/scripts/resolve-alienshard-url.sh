#!/usr/bin/env bash
set -euo pipefail

normalize_url() {
  local url="$1"

  url="${url#ALIENSHARD_URL=}"
  url="${url%%[[:space:]]*}"
  url="${url%/}"

  if [ -z "$url" ]; then
    return 1
  fi

  case "$url" in
    http://*|https://*) ;;
    *) url="http://$url" ;;
  esac

  case "$url" in
    http://.|https://.|http://..|https://..|http://|https://) return 1 ;;
  esac

  printf '%s\n' "$url"
}

extract_from_agents() {
  local file="AGENTS.md"
  local line=""

  if [ ! -f "$file" ]; then
    return 1
  fi

  line="$(grep -Eim1 '^ALIENSHARD_URL=' "$file" || true)"
  if [ -n "$line" ]; then
    normalize_url "$line"
    return
  fi

  line="$(grep -Eim1 'alienshard.*(https?://[^ )`]+|[0-9]{1,3}(\.[0-9]{1,3}){3}:[0-9]+|localhost:[0-9]+)' "$file" || true)"
  if [ -z "$line" ]; then
    return 1
  fi

  line="$(printf '%s\n' "$line" | grep -Eio 'https?://[^ )`]+' || printf '%s\n' "$line" | grep -Eio '([0-9]{1,3}\.){3}[0-9]{1,3}:[0-9]+|localhost:[0-9]+' || true)"

  if [ -z "$line" ]; then
    return 1
  fi

  normalize_url "$line"
}

if [ -n "${ALIENSHARD_URL:-}" ]; then
  normalize_url "$ALIENSHARD_URL"
  exit
fi

if url="$(extract_from_agents)"; then
  printf '%s\n' "$url"
  exit
fi

printf '%s\n' 'http://127.0.0.1:8000'
