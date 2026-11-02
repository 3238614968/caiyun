"""Inspect exchange evidence without printing account credentials.

HAR headers are an ordered list: HTTP/2 often splits Cookie into many entries.
Never convert that list to a single-value dictionary before inspecting cookies.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path
from urllib.parse import urlsplit


def header_values(message, name):
    return [h.get("value", "") for h in message.get("headers", [])
            if h.get("name", "").lower() == name.lower()]


def cookie_names(message):
    result = []
    for value in header_values(message, "cookie"):
        for pair in value.split(";"):
            if "=" in pair:
                name = pair.split("=", 1)[0].strip()
                result.append(re.sub(r"1[3-9]\d{9}", "[PHONE]", name))
    return result


def summarize_entry(index, entry):
    request, response = entry["request"], entry["response"]
    url = urlsplit(request["url"])
    counts = {}
    for header in request.get("headers", []):
        name = header["name"].lower()
        counts[name] = counts.get(name, 0) + 1
    return {
        "entry": index, "method": request["method"], "host": url.hostname,
        "path": url.path, "status": response["status"],
        "repeated_headers": {k: v for k, v in counts.items() if v > 1},
        "cookie_names": cookie_names(request),
        "set_cookie_names": [value.split("=", 1)[0] for value in header_values(response, "set-cookie")],
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("har", type=Path)
    parser.add_argument("--entries", default="1352,1792,1793,1794,1798")
    args = parser.parse_args()
    raw = args.har.read_bytes()
    entries = json.loads(raw.decode("utf-8-sig"))["log"]["entries"]
    print(json.dumps({"entries": len(entries), "sha256": hashlib.sha256(raw).hexdigest()}))
    for index in map(int, args.entries.split(",")):
        print(json.dumps(summarize_entry(index, entries[index]), ensure_ascii=False))


if __name__ == "__main__":
    main()
