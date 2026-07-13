#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${OUT_DIR:-$ROOT/release}"
CHECKSUM_FILE="${1:-$OUT_DIR/SHA256SUMS}"
SIGNATURE_FILE="${SIGNATURE_FILE:-$CHECKSUM_FILE.sig}"

if [[ ! -f "$CHECKSUM_FILE" ]]; then
  echo "Checksum file not found: $CHECKSUM_FILE" >&2
  exit 1
fi

if [[ -z "${RELEASE_SIGN_KEY:-}" && -z "${RELEASE_SIGN_KEY_B64:-}" ]]; then
  echo "RELEASE_SIGN_KEY/RELEASE_SIGN_KEY_B64 not set; skip release signature"
  exit 0
fi

command -v openssl >/dev/null 2>&1 || { echo "openssl is required for signing" >&2; exit 1; }

tmp_key=""
cleanup() {
  if [[ -n "$tmp_key" && -f "$tmp_key" ]]; then
    rm -f "$tmp_key"
  fi
}
trap cleanup EXIT

key_path="${RELEASE_SIGN_KEY:-}"
if [[ -n "${RELEASE_SIGN_KEY_B64:-}" ]]; then
  tmp_key="$(mktemp)"
  printf '%s' "$RELEASE_SIGN_KEY_B64" | base64 -d > "$tmp_key"
  chmod 600 "$tmp_key"
  key_path="$tmp_key"
fi

[[ -f "$key_path" ]] || { echo "Signing key not found: $key_path" >&2; exit 1; }
openssl dgst -sha256 -sign "$key_path" -out "$SIGNATURE_FILE" "$CHECKSUM_FILE"
echo "Release checksum signature written: $SIGNATURE_FILE"
