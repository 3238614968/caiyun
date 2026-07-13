#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="/www/wwwroot/caiyun"
BACKUP_DIR=""
SERVICE_PREFIX="caiyun"
SKIP_SERVICES=0
SKIP_NGINX=0
RUN_HEALTH_CHECK=0
HEALTH_SCRIPT=""

usage() {
  cat <<USAGE
Usage: $0 [--target DIR] [--backup DIR] [--service-prefix PREFIX] [--skip-services] [--skip-nginx] [--health-check] [--health-script FILE]

Restores the unified backend binary and frontend static files from a deployment backup.
If --backup is omitted, the newest TARGET/backups/deploy-* directory is used.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) TARGET_DIR="$2"; shift 2 ;;
    --backup) BACKUP_DIR="$2"; shift 2 ;;
    --service-prefix) SERVICE_PREFIX="$2"; shift 2 ;;
    --skip-services) SKIP_SERVICES=1; shift ;;
    --skip-nginx) SKIP_NGINX=1; shift ;;
    --health-check) RUN_HEALTH_CHECK=1; shift ;;
    --health-script) HEALTH_SCRIPT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -z "$BACKUP_DIR" ]]; then
  BACKUP_DIR="$(ls -dt "$TARGET_DIR"/backups/deploy-* 2>/dev/null | head -n 1 || true)"
fi
[[ -n "$BACKUP_DIR" && -d "$BACKUP_DIR" ]] || { echo "No backup directory found" >&2; exit 1; }

case "$(readlink -f "$TARGET_DIR" 2>/dev/null || echo "$TARGET_DIR")" in
  /|/www|/www/wwwroot) echo "Refusing unsafe target: $TARGET_DIR" >&2; exit 1 ;;
esac

if [[ "$SKIP_SERVICES" -eq 0 ]] && command -v systemctl >/dev/null 2>&1; then
  systemctl stop "${SERVICE_PREFIX}-worker.service" 2>/dev/null || true
  systemctl stop "${SERVICE_PREFIX}-api.service" 2>/dev/null || true
fi

RESTORE_ITEMS=("caiyun-linux" "api-linux" "worker-linux" "migrator-linux" "reencrypt-linux" "index.html" "assets" "images" "bundle-report.json")
while IFS= read -r -d '' path; do
  item="$(basename "$path")"
  exists=0
  for current in "${RESTORE_ITEMS[@]}"; do
    if [[ "$current" == "$item" ]]; then
      exists=1
      break
    fi
  done
  if [[ "$exists" -eq 0 ]]; then
    RESTORE_ITEMS+=("$item")
  fi
done < <(find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -print0)

for item in "${RESTORE_ITEMS[@]}"; do
  rm -rf "$TARGET_DIR/$item"
  if [[ -e "$BACKUP_DIR/$item" ]]; then
    cp -a "$BACKUP_DIR/$item" "$TARGET_DIR/"
  fi
done
chmod +x "$TARGET_DIR/caiyun-linux" "$TARGET_DIR/api-linux" "$TARGET_DIR/worker-linux" "$TARGET_DIR/migrator-linux" "$TARGET_DIR/reencrypt-linux" 2>/dev/null || true

if [[ "$SKIP_SERVICES" -eq 0 ]] && command -v systemctl >/dev/null 2>&1; then
  systemctl restart "${SERVICE_PREFIX}-api.service" 2>/dev/null || true
  systemctl restart "${SERVICE_PREFIX}-worker.service" 2>/dev/null || true
fi

if [[ "$SKIP_NGINX" -eq 0 ]] && command -v nginx >/dev/null 2>&1; then
  nginx -t && nginx -s reload || true
fi

if [[ "$RUN_HEALTH_CHECK" -eq 1 ]]; then
  if [[ -z "$HEALTH_SCRIPT" ]]; then
    HEALTH_SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/health-check.sh"
  fi
  [[ -x "$HEALTH_SCRIPT" || -f "$HEALTH_SCRIPT" ]] || { echo "Missing health script: $HEALTH_SCRIPT" >&2; exit 1; }
  bash "$HEALTH_SCRIPT"
fi

echo "Rollback finished from $BACKUP_DIR"
