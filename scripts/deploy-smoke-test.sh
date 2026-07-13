#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

SOURCE_DIR="$TMP_DIR/source"
TARGET_DIR="$TMP_DIR/target"
RELEASE_DIR="$TMP_DIR/release"
RELEASE_TARGET_DIR="$TMP_DIR/release-target"
mkdir -p "$SOURCE_DIR/backend" "$SOURCE_DIR/frontend/dist/assets" "$TARGET_DIR/assets" "$TARGET_DIR/backups" "$TARGET_DIR/logs"
mkdir -p "$RELEASE_DIR" "$RELEASE_TARGET_DIR/assets" "$RELEASE_TARGET_DIR/backups" "$RELEASE_TARGET_DIR/logs"

cat > "$SOURCE_DIR/backend/caiyun-linux" <<'BIN'
#!/bin/sh
if [ "${1:-}" = "migrate" ]; then echo migration-ok; exit 0; fi
echo new-caiyun
BIN
chmod +x "$SOURCE_DIR/backend/caiyun-linux"
printf 'new-index' > "$SOURCE_DIR/frontend/dist/index.html"
printf 'new-asset' > "$SOURCE_DIR/frontend/dist/assets/app.js"
(
  cd "$SOURCE_DIR/backend"
  sha256sum caiyun-linux > SHA256SUMS
)

printf '#!/bin/sh\necho old-caiyun\n' > "$TARGET_DIR/caiyun-linux"
printf 'legacy-api' > "$TARGET_DIR/api-linux"
printf 'legacy-worker' > "$TARGET_DIR/worker-linux"
printf 'old-index' > "$TARGET_DIR/index.html"
printf 'old-asset' > "$TARGET_DIR/assets/app.js"
chmod +x "$TARGET_DIR/caiyun-linux"

bash "$ROOT/scripts/deploy-linux.sh" --source "$SOURCE_DIR" --target "$TARGET_DIR" --skip-services --skip-nginx

grep -q 'new-caiyun' "$TARGET_DIR/caiyun-linux"
[[ ! -e "$TARGET_DIR/api-linux" && ! -e "$TARGET_DIR/worker-linux" ]]
grep -q 'new-index' "$TARGET_DIR/index.html"
grep -q 'new-asset' "$TARGET_DIR/assets/app.js"

BACKUP_DIR="$(ls -dt "$TARGET_DIR"/backups/deploy-* | head -n 1)"
[[ -d "$BACKUP_DIR" ]] || { echo "backup directory was not created" >&2; exit 1; }
grep -q 'old-caiyun' "$BACKUP_DIR/caiyun-linux"
grep -q 'legacy-api' "$BACKUP_DIR/api-linux"
grep -q 'old-index' "$BACKUP_DIR/index.html"

bash "$ROOT/scripts/rollback-linux.sh" --target "$TARGET_DIR" --backup "$BACKUP_DIR" --skip-services --skip-nginx

grep -q 'old-caiyun' "$TARGET_DIR/caiyun-linux"
grep -q 'legacy-api' "$TARGET_DIR/api-linux"
grep -q 'legacy-worker' "$TARGET_DIR/worker-linux"
grep -q 'old-index' "$TARGET_DIR/index.html"
grep -q 'old-asset' "$TARGET_DIR/assets/app.js"

cp "$SOURCE_DIR/backend/caiyun-linux" "$RELEASE_DIR/caiyun-linux"
tar -czf "$RELEASE_DIR/caiyun-frontend-test.tar.gz" -C "$SOURCE_DIR/frontend/dist" .
(
  cd "$RELEASE_DIR"
  sha256sum caiyun-linux caiyun-frontend-test.tar.gz > SHA256SUMS
)

printf '#!/bin/sh\necho release-old-caiyun\n' > "$RELEASE_TARGET_DIR/caiyun-linux"
printf 'release-old-index' > "$RELEASE_TARGET_DIR/index.html"
printf 'release-old-asset' > "$RELEASE_TARGET_DIR/assets/app.js"
chmod +x "$RELEASE_TARGET_DIR/caiyun-linux"

bash "$ROOT/scripts/deploy-linux.sh" --release-dir "$RELEASE_DIR" --target "$RELEASE_TARGET_DIR" --skip-services --skip-nginx

grep -q 'new-caiyun' "$RELEASE_TARGET_DIR/caiyun-linux"
grep -q 'new-index' "$RELEASE_TARGET_DIR/index.html"
grep -q 'new-asset' "$RELEASE_TARGET_DIR/assets/app.js"

RELEASE_BACKUP_DIR="$(ls -dt "$RELEASE_TARGET_DIR"/backups/deploy-* | head -n 1)"
[[ -d "$RELEASE_BACKUP_DIR" ]] || { echo "release backup directory was not created" >&2; exit 1; }
grep -q 'release-old-caiyun' "$RELEASE_BACKUP_DIR/caiyun-linux"
grep -q 'release-old-index' "$RELEASE_BACKUP_DIR/index.html"

bash "$ROOT/scripts/rollback-linux.sh" --target "$RELEASE_TARGET_DIR" --backup "$RELEASE_BACKUP_DIR" --skip-services --skip-nginx

grep -q 'release-old-caiyun' "$RELEASE_TARGET_DIR/caiyun-linux"
grep -q 'release-old-index' "$RELEASE_TARGET_DIR/index.html"
grep -q 'release-old-asset' "$RELEASE_TARGET_DIR/assets/app.js"

echo "deploy smoke test passed"
