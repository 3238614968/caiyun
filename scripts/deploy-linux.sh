#!/usr/bin/env bash
set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RELEASE_DIR=""
TARGET_DIR="/www/wwwroot/caiyun"
SERVICE_PREFIX="caiyun"
SKIP_SERVICES=0
SKIP_NGINX=0
RUN_HEALTH_CHECK=0
RUN_MIGRATIONS=1
SKIP_TASK_CONFIG_SYNC=0
HEALTH_SCRIPT=""
ENV_FILE=""
TMP_DIR=""

cleanup() {
  if [[ -n "$TMP_DIR" && -d "$TMP_DIR" ]]; then
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT

usage() {
  cat <<USAGE
Usage: $0 [--source DIR | --release-dir DIR] [--target DIR] [--service-prefix PREFIX]
          [--skip-services] [--skip-nginx] [--health-check] [--health-script FILE]
          [--skip-migrations] [--skip-task-config-sync] [--env-file FILE]

Deploys either:
  1) backend/caiyun-linux plus frontend/dist/* from a source tree
  2) a packaged release containing caiyun-linux, SHA256SUMS and caiyun-frontend-*.tar.gz

A timestamped backup is created under TARGET/backups before replacement. The new
unified binary runs "migrate" before installation by default. Use
--skip-migrations only when an external release pipeline has already completed
and validated the same migration set. --run-migrations remains accepted for
backwards-compatible automation and is equivalent to the default.
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

verify_sigstore_bundle_if_present() {
  local dir="$1"
  local sums_file="$dir/SHA256SUMS"
  local bundle_file="$dir/SHA256SUMS.sigstore.json"
  local require_sigstore="${RELEASE_VERIFY_SIGSTORE:-0}"
  local oidc_issuer="${RELEASE_VERIFY_OIDC_ISSUER:-https://token.actions.githubusercontent.com}"
  local identity_regexp="${RELEASE_VERIFY_IDENTITY_REGEXP:-https://github.com/.+/.+/.github/workflows/ci.yml@refs/tags/(v.*|release-.*)}"

  if [[ ! -f "$bundle_file" ]]; then
    if [[ "$require_sigstore" == "1" ]]; then
      echo "Missing Sigstore bundle: $bundle_file" >&2
      exit 1
    fi
    return 0
  fi

  if ! command -v cosign >/dev/null 2>&1; then
    if [[ "$require_sigstore" == "1" ]]; then
      echo "cosign is required to verify $bundle_file" >&2
      exit 1
    fi
    echo "Skipping Sigstore verification because cosign is not installed"
    return 0
  fi

  echo "Verifying SHA256SUMS Sigstore bundle"
  cosign verify-blob "$sums_file" \
    --bundle "$bundle_file" \
    --certificate-oidc-issuer "$oidc_issuer" \
    --certificate-identity-regexp "$identity_regexp"
}

run_migrator() {
  local caiyun_bin="$1"
  [[ -x "$caiyun_bin" || -f "$caiyun_bin" ]] || { echo "Missing caiyun binary: $caiyun_bin" >&2; exit 1; }

  local args=()
  if [[ "$SKIP_TASK_CONFIG_SYNC" -eq 1 ]]; then
    args+=(--skip-task-config-sync)
  fi

  local env_loaded=0
  if [[ -n "$ENV_FILE" ]]; then
    ENV_FILE="$(cd "$(dirname "$ENV_FILE")" && pwd)/$(basename "$ENV_FILE")"
    load_env_file "$ENV_FILE"
    env_loaded=1
  elif [[ -f "$TARGET_DIR/.env" ]]; then
    ENV_FILE="$TARGET_DIR/.env"
    load_env_file "$ENV_FILE"
    env_loaded=1
  fi

  if [[ "$env_loaded" -eq 1 ]]; then
    echo "Loaded environment for migration from $ENV_FILE"
  else
    echo "No env file provided/found; migration will rely on existing process environment"
  fi

  echo "Running database migration: $caiyun_bin migrate ${args[*]:-}"
  APP_ENV="${APP_ENV:-production}" DB_AUTO_MIGRATE=false "$caiyun_bin" migrate "${args[@]}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source) SOURCE_DIR="$2"; shift 2 ;;
    --release-dir) RELEASE_DIR="$2"; shift 2 ;;
    --target) TARGET_DIR="$2"; shift 2 ;;
    --service-prefix) SERVICE_PREFIX="$2"; shift 2 ;;
    --skip-services) SKIP_SERVICES=1; shift ;;
    --skip-nginx) SKIP_NGINX=1; shift ;;
    --health-check) RUN_HEALTH_CHECK=1; shift ;;
    --health-script) HEALTH_SCRIPT="$2"; shift 2 ;;
    --run-migrations) RUN_MIGRATIONS=1; shift ;;
    --skip-migrations) RUN_MIGRATIONS=0; shift ;;
    --skip-task-config-sync) SKIP_TASK_CONFIG_SYNC=1; shift ;;
    --env-file) ENV_FILE="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ -n "$RELEASE_DIR" ]]; then
  RELEASE_DIR="$(cd "$RELEASE_DIR" && pwd)"
  CAIYUN_BIN="$RELEASE_DIR/caiyun-linux"
  [[ -f "$CAIYUN_BIN" ]] || { echo "Missing $CAIYUN_BIN" >&2; exit 1; }

  if [[ -f "$RELEASE_DIR/SHA256SUMS" ]] && command -v sha256sum >/dev/null 2>&1; then
    echo "Verifying release SHA256SUMS"
    (cd "$RELEASE_DIR" && sha256sum -c SHA256SUMS)
    verify_sigstore_bundle_if_present "$RELEASE_DIR"
  fi

  mapfile -t frontend_archives < <(find "$RELEASE_DIR" -maxdepth 1 -type f -name 'caiyun-frontend-*.tar.gz' | sort)
  [[ "${#frontend_archives[@]}" -gt 0 ]] || { echo "Missing caiyun-frontend-*.tar.gz in $RELEASE_DIR" >&2; exit 1; }
  FRONTEND_ARCHIVE="${frontend_archives[${#frontend_archives[@]}-1]}"
  if [[ "${#frontend_archives[@]}" -gt 1 ]]; then
    echo "Multiple frontend archives found; using newest sorted entry: $(basename "$FRONTEND_ARCHIVE")"
  fi

  TMP_DIR="$(mktemp -d)"
  DIST_DIR="$TMP_DIR/frontend-dist"
  mkdir -p "$DIST_DIR"
  tar -xzf "$FRONTEND_ARCHIVE" -C "$DIST_DIR"
else
  SOURCE_DIR="$(cd "$SOURCE_DIR" && pwd)"
  CAIYUN_BIN="$SOURCE_DIR/backend/caiyun-linux"
  DIST_DIR="$SOURCE_DIR/frontend/dist"

  [[ -f "$CAIYUN_BIN" ]] || { echo "Missing $CAIYUN_BIN" >&2; exit 1; }
  [[ -d "$DIST_DIR" ]] || { echo "Missing $DIST_DIR; run npm run build first" >&2; exit 1; }
  if [[ -f "$SOURCE_DIR/backend/SHA256SUMS" ]] && command -v sha256sum >/dev/null 2>&1; then
    echo "Verifying backend SHA256SUMS"
    (cd "$SOURCE_DIR/backend" && sha256sum -c SHA256SUMS)
  fi
fi

case "$(readlink -f "$TARGET_DIR" 2>/dev/null || echo "$TARGET_DIR")" in
  /|/www|/www/wwwroot) echo "Refusing unsafe target: $TARGET_DIR" >&2; exit 1 ;;
esac

mkdir -p "$TARGET_DIR" "$TARGET_DIR/backups" "$TARGET_DIR/logs"
BACKUP_DIR="$TARGET_DIR/backups/deploy-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"

# Legacy binary names are backed up and then removed during the one-artifact transition.
STATIC_ITEMS=("caiyun-linux" "api-linux" "worker-linux" "migrator-linux" "reencrypt-linux" "index.html" "assets" "images" "bundle-report.json")
while IFS= read -r -d '' path; do
  item="$(basename "$path")"
  exists=0
  for current in "${STATIC_ITEMS[@]}"; do
    if [[ "$current" == "$item" ]]; then
      exists=1
      break
    fi
  done
  if [[ "$exists" -eq 0 ]]; then
    STATIC_ITEMS+=("$item")
  fi
done < <(find "$DIST_DIR" -mindepth 1 -maxdepth 1 -print0)

echo "Backing up current deployment to $BACKUP_DIR"
for item in "${STATIC_ITEMS[@]}"; do
  if [[ -e "$TARGET_DIR/$item" ]]; then
    cp -a "$TARGET_DIR/$item" "$BACKUP_DIR/"
  fi
done

if [[ "$SKIP_SERVICES" -eq 0 ]] && command -v systemctl >/dev/null 2>&1; then
  systemctl stop "${SERVICE_PREFIX}-worker.service" 2>/dev/null || true
  systemctl stop "${SERVICE_PREFIX}-api.service" 2>/dev/null || true
fi

if [[ "$RUN_MIGRATIONS" -eq 1 ]]; then
  if ! run_migrator "$CAIYUN_BIN"; then
    echo "Database migration failed; restoring previously stopped services" >&2
    if [[ "$SKIP_SERVICES" -eq 0 ]] && command -v systemctl >/dev/null 2>&1; then
      systemctl restart "${SERVICE_PREFIX}-api.service" 2>/dev/null || true
      systemctl restart "${SERVICE_PREFIX}-worker.service" 2>/dev/null || true
    fi
    exit 1
  fi
fi

echo "Installing unified backend binary"
install -m 0755 "$CAIYUN_BIN" "$TARGET_DIR/caiyun-linux"
rm -f "$TARGET_DIR/api-linux" "$TARGET_DIR/worker-linux" "$TARGET_DIR/migrator-linux" "$TARGET_DIR/reencrypt-linux"

echo "Installing frontend static files"
for item in "${STATIC_ITEMS[@]}"; do
  case "$item" in
    caiyun-linux|api-linux|worker-linux|migrator-linux|reencrypt-linux) continue ;;
  esac
  rm -rf "$TARGET_DIR/$item"
done
cp -a "$DIST_DIR"/. "$TARGET_DIR"/

if [[ "$SKIP_SERVICES" -eq 0 ]] && command -v systemctl >/dev/null 2>&1; then
  systemctl restart "${SERVICE_PREFIX}-api.service"
  systemctl restart "${SERVICE_PREFIX}-worker.service"
fi

if [[ "$SKIP_NGINX" -eq 0 ]] && command -v nginx >/dev/null 2>&1; then
  nginx -t && nginx -s reload
fi

if [[ "$RUN_HEALTH_CHECK" -eq 1 ]]; then
  if [[ -z "$HEALTH_SCRIPT" ]]; then
    HEALTH_SCRIPT="$SOURCE_DIR/scripts/health-check.sh"
    if [[ -n "$RELEASE_DIR" && ! -f "$HEALTH_SCRIPT" ]]; then
      HEALTH_SCRIPT="$RELEASE_DIR/health-check.sh"
    fi
  fi
  [[ -x "$HEALTH_SCRIPT" || -f "$HEALTH_SCRIPT" ]] || { echo "Missing health script: $HEALTH_SCRIPT" >&2; exit 1; }
  bash "$HEALTH_SCRIPT"
fi

echo "Deployment finished. Backup: $BACKUP_DIR"
echo "Suggested checks:"
echo "  API_URL=http://127.0.0.1:8080/readyz WORKER_URL=http://127.0.0.1:8081/readyz bash scripts/health-check.sh"
