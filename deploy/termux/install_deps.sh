#!/data/data/com.termux/files/usr/bin/bash
# 安装 caiyun 依赖（Termux 原生，无需 root）
export DEBIAN_FRONTEND=noninteractive
echo "[install] start $(date)"
yes | pkg install -y redis mariadb nginx 2>&1 | tail -30
echo "[install] versions:"
redis-server --version 2>&1 | head -1
mariadbd --version 2>&1 | head -1 || mysqld --version 2>&1 | head -1
nginx -v 2>&1
echo "[install] DONE $(date)"
