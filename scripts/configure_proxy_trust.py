#!/usr/bin/env python3
"""Validate and render deployment-specific proxy sources before kubectl apply."""
from __future__ import annotations

import argparse
import ipaddress
import json
from pathlib import Path
import re


def validate_proxies(raw: str) -> str:
    proxies = []
    for value in re.split(r"[,;\s]+", raw.strip()):
        if not value:
            continue
        # Exclude scope identifiers and every YAML/shell metacharacter.
        if not re.fullmatch(r"[0-9a-fA-F:./]+", value):
            raise ValueError("TRUSTED_PROXIES must contain explicit IP addresses or CIDRs")
        if "/" in value:
            network = ipaddress.ip_network(value, strict=False)
            mapped_all_ipv4 = isinstance(network, ipaddress.IPv6Network) and network.prefixlen == 96 and network.network_address.ipv4_mapped is not None
            if network.prefixlen == 0 or mapped_all_ipv4:
                raise ValueError("TRUSTED_PROXIES must not trust all sources")
            normalized = str(network)
        else:
            normalized = str(ipaddress.ip_address(value))
        if normalized not in proxies:
            proxies.append(normalized)
    if not proxies:
        raise ValueError("reverse-proxy deployments require TRUSTED_PROXIES")
    return ",".join(proxies)


def render_manifest(manifest: str, raw: str | None) -> str:
    pattern = re.compile(r"^(?P<indent>\s*)TRUSTED_PROXIES:[^\r\n]*$", re.M)
    matches = list(pattern.finditer(manifest))
    if len(matches) != 1:
        raise ValueError("manifest must contain exactly one TRUSTED_PROXIES ConfigMap entry")
    if raw is None:
        raw = matches[0].group().split(":", 1)[1].strip().strip('"\'')
    value = validate_proxies(raw)
    return pattern.sub(lambda match: match["indent"] + "TRUSTED_PROXIES: " + json.dumps(value), manifest)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--trusted-proxies")
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    try:
        rendered = render_manifest(args.manifest.read_text(encoding="utf-8"), args.trusted_proxies)
    except ValueError as error:
        parser.error(str(error))
    args.out.write_text(rendered, encoding="utf-8", newline="\n")


if __name__ == "__main__":
    main()
