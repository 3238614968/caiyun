#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR_BASE="${OUT_DIR_BASE:-$ROOT/release}"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
BUILD_ID="${BUILD_ID:-}"
RELEASE_SUFFIX="$VERSION"
if [[ -n "$BUILD_ID" ]]; then
  RELEASE_SUFFIX+="-$BUILD_ID"
fi
OUT_DIR="${OUT_DIR:-$OUT_DIR_BASE/${RELEASE_SUFFIX}-linux-amd64}"
STAGE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/caiyun-release.XXXXXX")"
cleanup() {
  if [[ -d "$STAGE_DIR" ]]; then
    rm -rf "$STAGE_DIR"
  fi
}
trap cleanup EXIT

[[ -f "$ROOT/backend/caiyun-linux" ]] || { echo "Missing backend/caiyun-linux; run scripts/build-linux.ps1 or make backend-build first" >&2; exit 1; }
[[ -d "$ROOT/frontend/dist" ]] || { echo "Missing frontend/dist; run npm run build first" >&2; exit 1; }

cp "$ROOT/backend/caiyun-linux" "$STAGE_DIR/caiyun-linux"
chmod 0755 "$STAGE_DIR/caiyun-linux"
tar -czf "$STAGE_DIR/caiyun-frontend-${VERSION}.tar.gz" -C "$ROOT/frontend/dist" .
cp "$ROOT/nginx-server.conf" "$STAGE_DIR/nginx-server.conf"
tar -czf "$STAGE_DIR/caiyun-migrations-${VERSION}.tar.gz" -C "$ROOT/backend" migrations
tar -czf "$STAGE_DIR/caiyun-monitoring-${VERSION}.tar.gz" -C "$ROOT/deploy" monitoring
tar -czf "$STAGE_DIR/caiyun-calendar-${VERSION}.tar.gz" -C "$ROOT/deploy" calendar
tar -czf "$STAGE_DIR/caiyun-systemd-${VERSION}.tar.gz" -C "$ROOT/deploy" systemd
tar -czf "$STAGE_DIR/caiyun-k8s-${VERSION}.tar.gz" -C "$ROOT" k8s
cp "$ROOT/docker-compose.yml" "$STAGE_DIR/docker-compose.yml"
install -m 0755 "$ROOT/scripts/deploy-linux.sh" "$STAGE_DIR/deploy-linux.sh"
install -m 0755 "$ROOT/scripts/rollback-linux.sh" "$STAGE_DIR/rollback-linux.sh"
install -m 0755 "$ROOT/scripts/health-check.sh" "$STAGE_DIR/health-check.sh"
install -m 0755 "$ROOT/scripts/import-calendar.sh" "$STAGE_DIR/import-calendar.sh"
install -m 0755 "$ROOT/scripts/archive-history.sh" "$STAGE_DIR/archive-history.sh"
install -m 0755 "$ROOT/scripts/rotate-encryption.sh" "$STAGE_DIR/rotate-encryption.sh"
install -m 0755 "$ROOT/scripts/collect-slo-snapshot.sh" "$STAGE_DIR/collect-slo-snapshot.sh"
install -m 0755 "$ROOT/scripts/deploy-k8s.sh" "$STAGE_DIR/deploy-k8s.sh"
install -m 0755 "$ROOT/scripts/verify-compose-runtime.sh" "$STAGE_DIR/verify-compose-runtime.sh"
install -m 0755 "$ROOT/scripts/verify-k8s-runtime.sh" "$STAGE_DIR/verify-k8s-runtime.sh"
install -m 0755 "$ROOT/scripts/verify-otel-runtime.sh" "$STAGE_DIR/verify-otel-runtime.sh"
install -m 0755 "$ROOT/scripts/verify-legacy-list-drain.sh" "$STAGE_DIR/verify-legacy-list-drain.sh"
install -m 0755 "$ROOT/scripts/chaos-drill.sh" "$STAGE_DIR/chaos-drill.sh"
install -m 0755 "$ROOT/scripts/capacity-drill.sh" "$STAGE_DIR/capacity-drill.sh"
install -m 0644 "$ROOT/scripts/append-quarterly-slo-report.py" "$STAGE_DIR/append-quarterly-slo-report.py"

SBOM_FILE="$STAGE_DIR/caiyun-sbom-${VERSION}.json" bash "$ROOT/scripts/generate-sbom.sh" "$STAGE_DIR/caiyun-sbom-${VERSION}.json"
(
  cd "$STAGE_DIR"
  find . -maxdepth 1 -type f \
    ! -name 'SHA256SUMS' \
    ! -name 'SHA256SUMS.sig' \
    ! -name '*.sig' \
    -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
)
OUT_DIR="$STAGE_DIR" bash "$ROOT/scripts/sign-release.sh" "$STAGE_DIR/SHA256SUMS"

mkdir -p "$(dirname "$OUT_DIR")"
rm -rf "$OUT_DIR"
mv "$STAGE_DIR" "$OUT_DIR"
STAGE_DIR=""
trap - EXIT

echo "Release package prepared in $OUT_DIR"
ls -lh "$OUT_DIR"
