#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"
BACKEND_DIR="$ROOT_DIR/backend"
BUILD_DIR="$ROOT_DIR/build/release"
FRONTEND_OUT="$BUILD_DIR/frontend"
BACKEND_OUT="$BUILD_DIR/backend"
PACKAGE_NAME="caiyun-linux-amd64"

rm -rf "$BUILD_DIR"
mkdir -p "$FRONTEND_OUT" "$BACKEND_OUT"

echo "[1/3] Building frontend..."
pushd "$FRONTEND_DIR" >/dev/null
if [[ ! -d node_modules ]]; then
  npm ci
fi
npm run build
cp -r dist/* "$FRONTEND_OUT"/
popd >/dev/null

echo "[2/3] Building backend (linux/amd64)..."
pushd "$BACKEND_DIR" >/dev/null
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$BACKEND_OUT/caiyun-api" ./cmd/api
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$BACKEND_OUT/caiyun-worker" ./cmd/worker
popd >/dev/null

echo "[3/3] Packaging release artifact..."
cp "$ROOT_DIR/backend/configs/.env.example" "$BUILD_DIR/.env.example"
cp "$ROOT_DIR/README.md" "$BUILD_DIR/README.md"

pushd "$ROOT_DIR/build" >/dev/null
tar -czf "$PACKAGE_NAME.tar.gz" -C release .
popd >/dev/null

echo "Build completed: $ROOT_DIR/build/$PACKAGE_NAME.tar.gz"
