#!/usr/bin/env python3
"""Keep persistent models from becoming accidental credential response DTOs.

The rule deliberately targets transport-visible sensitive fields rather than
requiring a disruptive one-shot removal of every historical JSON tag. Handler
responses use internal/dto mappers; a future model field such as Token or
PasswordHash must never acquire a public JSON name.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODELS = ROOT / "backend" / "internal" / "models"
HANDLERS = ROOT / "backend" / "internal" / "handlers"
SENSITIVE = ("token", "cookie", "password", "secret", "credential", "authorization")
JSON_FIELD = re.compile(r"^\s*(?P<name>[A-Za-z][A-Za-z0-9_]*)\b.*?`[^`]*json:\"(?P<json>[^\"]+)\"")
DIRECT_MODEL_RESPONSE = re.compile(
    r"(?:response\.(?:Success|Error)|\.JSON)\([^\n]*(?:models\.|\*models\.|&models\.)"
)


def main() -> int:
    failures: list[str] = []
    for path in sorted(MODELS.glob("*.go")):
        for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            match = JSON_FIELD.match(line)
            if not match:
                continue
            field_name = match.group("name").lower()
            json_name = match.group("json").split(",", 1)[0].lower()
            if json_name != "-" and any(term in field_name or term in json_name for term in SENSITIVE):
                failures.append(
                    f"{path.relative_to(ROOT)}:{line_number}: persistent sensitive field "
                    f"{match.group('name')} exposes JSON name {json_name!r}; use an internal DTO"
                )

    for path in sorted(HANDLERS.glob("*.go")):
        source = path.read_text(encoding="utf-8")
        if DIRECT_MODEL_RESPONSE.search(source):
            failures.append(f"{path.relative_to(ROOT)}: direct models.* response detected; map through internal/dto")

    if failures:
        print("DTO boundary validation failed:", file=sys.stderr)
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("DTO boundary validation passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
