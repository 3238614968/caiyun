#!/data/data/com.termux/files/usr/bin/bash
# 安装 caiyun 依赖（Termux 原生，无需 root）
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
echo "[install] start $(date)"
if ! pkg install -y redis mariadb nginx; then
  echo "[install] pkg install FAILED" >&2
  exit 1
fi
echo "[install] versions:"
for cmd in redis-server mariadbd nginx; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "[install] missing required binary: $cmd" >&2
    exit 1
  fi
done
redis-server --version | head -1
mariadbd --version | head -1
nginx -v 2>&1
echo "[install] DONE $(date)"
