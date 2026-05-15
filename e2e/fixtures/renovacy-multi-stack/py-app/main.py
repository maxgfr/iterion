"""Tiny consumer of the pinned vulnerable libs.

Not production code — the fixture exists so secured-renovacy's
align_code node has a real call-site to update on breaking changes.
"""

import requests
import yaml


def fetch_title(url: str) -> str:
    res = requests.get(url, timeout=5)
    payload = yaml.safe_load(res.text)
    if isinstance(payload, dict):
        return payload.get("title", "")
    return ""


if __name__ == "__main__":
    print(fetch_title("https://example.com"))
