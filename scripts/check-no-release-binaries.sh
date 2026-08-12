#!/usr/bin/env bash
# Reject generated runtime artifacts in the Git index. Release packages are
# published as CI artifacts; source history contains only reproducible inputs.
set -euo pipefail

tracked="$(git ls-files -- backend/caiyun-linux backend/api-linux backend/worker-linux backend/migrator-linux backend/reencrypt-linux release || true)"
if [[ -n "$tracked" ]]; then
  echo "Generated release binaries must not be tracked:" >&2
  echo "$tracked" >&2
  exit 1
fi
