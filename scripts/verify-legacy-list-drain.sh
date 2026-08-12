#!/usr/bin/env bash
# Read-only release gate for retiring the Redis List compatibility queue.
# Run this in the target Redis environment after Streams has been the only
# producer backend for the agreed retention window.
set -euo pipefail

REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_DB="${REDIS_DB:-0}"
REDIS_USERNAME="${REDIS_USERNAME:-}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"
REDIS_TLS="${REDIS_TLS:-false}"

command -v redis-cli >/dev/null 2>&1 || { echo "redis-cli is required" >&2; exit 1; }

args=(-h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" --raw)
if [[ -n "$REDIS_USERNAME" ]]; then
  args+=(--user "$REDIS_USERNAME")
fi
if [[ -n "$REDIS_PASSWORD" ]]; then
  args+=(-a "$REDIS_PASSWORD")
fi
if [[ "$REDIS_TLS" == "true" ]]; then
  args+=(--tls)
fi

count_key() {
  local key="$1" kind="$2" count
  if [[ "$kind" == "zset" ]]; then
    count="$(redis-cli "${args[@]}" ZCARD "$key")"
  else
    count="$(redis-cli "${args[@]}" LLEN "$key")"
  fi
  [[ "$count" =~ ^[0-9]+$ ]] || { echo "unexpected Redis count for $key: $count" >&2; exit 1; }
  printf '%s=%s\n' "$key" "$count"
  [[ "$count" == "0" ]]
}

status=0
count_key 'task:queue:pending' list || status=1
count_key 'task:queue:processing' list || status=1
count_key 'task:queue:delayed' zset || status=1
count_key 'task:queue:dead' list || status=1

if [[ "$status" -ne 0 ]]; then
  echo "legacy Redis List queue still has data; retain compatibility code and complete drain/replay first" >&2
  exit 1
fi
echo "legacy Redis List queue drain verified; compatibility implementation is eligible for a separately reviewed removal"
