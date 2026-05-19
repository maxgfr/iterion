#!/usr/bin/env python3
"""example-secrets — a minimal programmatic matcher for sec-audit-source.

Demonstrates the contract documented in skills/programmatic-matchers.md:
read stdin JSON, emit stdout JSON. This matcher flags literal API
keys / secrets / tokens / passwords assigned to string literals
>= 16 chars. It is intentionally narrower than gitleaks (which
sec-audit-source already runs always-on) — the goal here is to
serve as a copy-paste-ready skeleton, not to add coverage.

Re-purpose this file to bootstrap your own custom matchers.
"""

import json
import os
import re
import sys


KEY_RE = re.compile(
    r"(?i)\b(api[_-]?key|secret|token|password|passwd|client_secret|access_key)\s*[:=]\s*[\"']([^\"']{16,})[\"']"
)
ALLOWLIST_VALUES = {
    "REPLACE_ME",
    "your-key-here",
    "xxxxxxxxxxxxxxxx",
    "0000000000000000",
    "1111111111111111",
}


def main() -> None:
    try:
        req = json.load(sys.stdin)
    except json.JSONDecodeError as e:
        json.dump(
            {
                "matches": [],
                "errors": ["stdin not valid JSON: %s" % e],
                "matcher_name": "example-secrets",
                "matcher_version": "1.0",
            },
            sys.stdout,
        )
        return

    workspace_dir = req.get("workspace_dir", "")
    files = req.get("files") or []
    matches = []
    errors = []

    for rel in files:
        abs_path = os.path.join(workspace_dir, rel) if not os.path.isabs(rel) else rel
        try:
            with open(abs_path, "r", encoding="utf-8", errors="ignore") as f:
                for line_no, line in enumerate(f, start=1):
                    m = KEY_RE.search(line)
                    if not m:
                        continue
                    key_name, value = m.group(1), m.group(2)
                    if value in ALLOWLIST_VALUES:
                        continue
                    matches.append(
                        {
                            "rule_id": "literal-secret-assignment",
                            "file": rel,
                            "line_range": [line_no, line_no],
                            "snippet": line.rstrip("\n"),
                            "severity": "high",
                            "finding_type": "secrets",
                            "message": (
                                "Literal '%s' = '...' (%d chars) assigned in source. "
                                "Move to environment / secret store; rotate the leaked value."
                                % (key_name, len(value))
                            ),
                            "metadata": {"key_name": key_name},
                        }
                    )
        except (OSError, IOError) as e:
            errors.append("open %s: %s" % (rel, e))
            continue

    json.dump(
        {
            "matches": matches,
            "errors": errors,
            "matcher_name": "example-secrets",
            "matcher_version": "1.0",
        },
        sys.stdout,
    )


if __name__ == "__main__":
    main()
