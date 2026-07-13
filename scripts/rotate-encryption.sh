#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="/www/wwwroot/caiyun"
REENCRYPT_BIN=""
ENV_FILE=""
TABLE="all"
BATCH_SIZE="${BATCH_SIZE:-200}"
APPLY=0

usage() {
  cat <<USAGE
Usage: $0 [--target DIR] [--bin FILE] [--env-file FILE] [--table NAME] [--batch-size N] [--apply]

Runs the unified caiyun binary's reencrypt subcommand after loading deployment env vars.
Default target is /www/wwwroot/caiyun and default mode is dry-run.
USAGE
}

load_env_file() {
  local file="$1"
  [[ -f "$file" ]] || return 1
  set -a
  # shellcheck disable=SC1090
  . "$file"
  set +a
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) TARGET_DIR="$2"; shift 2 ;;
    --bin) REENCRYPT_BIN="$2"; shift 2 ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    --table) TABLE="$2"; shift 2 ;;
    --batch-size) BATCH_SIZE="$2"; shift 2 ;;
    --apply) APPLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -z "$REENCRYPT_BIN" ]]; then
  REENCRYPT_BIN="$TARGET_DIR/caiyun-linux"
fi
[[ -x "$REENCRYPT_BIN" || -f "$REENCRYPT_BIN" ]] || { echo "Missing reencrypt binary: $REENCRYPT_BIN" >&2; exit 1; }

if [[ -z "$ENV_FILE" && -f "$TARGET_DIR/.env" ]]; then
  ENV_FILE="$TARGET_DIR/.env"
fi
if [[ -n "$ENV_FILE" ]]; then
  ENV_FILE="$(cd "$(dirname "$ENV_FILE")" && pwd)/$(basename "$ENV_FILE")"
  load_env_file "$ENV_FILE"
  echo "Loaded environment from $ENV_FILE"
fi

ARGS=(--table "$TABLE" --batch-size "$BATCH_SIZE")
if [[ "$APPLY" -eq 1 ]]; then
  ARGS+=(--apply)
else
  echo "Dry-run mode. Add --apply to persist changes."
fi

echo "Running: $REENCRYPT_BIN reencrypt ${ARGS[*]}"
"$REENCRYPT_BIN" reencrypt "${ARGS[@]}"
