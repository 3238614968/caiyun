#!/bin/sh
set -eu
set -f

# /tmp is writable in the unprivileged/read-only Kubernetes container.
output="${CAIYUN_NGINX_TRUST_FILE:-/tmp/caiyun-trusted-proxies.conf}"
: > "$output"
for proxy in $(printf '%s' "${NGINX_TRUSTED_PROXIES:-}" | tr ',;' '  '); do
  normalized=$(printf '%s' "$proxy" | tr 'A-F' 'a-f')
  case "$normalized" in
    none) continue ;;
    */0*|*:ffff:*/96|*[!0-9a-f:./]*)
      echo "NGINX_TRUSTED_PROXIES must contain explicit IPs/CIDRs, never trust-all" >&2
      exit 1
      ;;
  esac
  # nginx validates address syntax itself; the character restriction above
  # prevents a value from becoming another directive or an include.
  printf 'set_real_ip_from %s;\n' "$proxy" >> "$output"
done
