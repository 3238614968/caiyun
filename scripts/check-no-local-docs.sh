#!/usr/bin/env bash
set -euo pipefail

is_document_extension() {
  case "$1" in
    *.md|*.mdx|*.txt|*.rst|*.adoc|*.pdf|*.doc|*.docx|*.xls|*.xlsx|*.ppt|*.pptx)
      return 0
      ;;
  esac
  return 1
}

is_local_only_document() {
  local file="$1"
  local name="${file##*/}"

  case "$name" in
    [Pp][Rr][Dd]*|[Ss][Dd][Dd]*|[Ss][Pp][Ee][Cc]*|*审计*|*[Aa][Uu][Dd][Ii][Tt]*) ;;
    *) return 1 ;;
  esac

  # Preserve protection for root/docs notes. Elsewhere, only document-like
  # files are rejected so source files such as audit.go remain valid.
  if [[ "$file" != */* ]]; then
    return 0
  fi
  case "$file" in
    docs/*|backend/docs/*|frontend/docs/*)
      return 0
      ;;
  esac
  is_document_extension "$name"
}

is_local_openapi_artifact() {
  local file="$1"
  [[ "$file" == .local/openapi/* ]] ||
    [[ "$file" =~ (^|/)([Oo][Pp][Ee][Nn][Aa][Pp][Ii]|[Ss][Ww][Aa][Gg][Gg][Ee][Rr])[^/]*\.(json|ya?ml)$ ]]
}

status=0
while IFS= read -r -d '' file; do
  if is_local_only_document "$file"; then
    echo "Local-only PRD/SDD/SPEC/audit document must not be tracked: $file" >&2
    status=1
  fi
  if is_local_openapi_artifact "$file"; then
    echo "Local OpenAPI/Swagger artifact must not be tracked: $file" >&2
    status=1
  fi
done < <(git ls-files -z)

exit "$status"
