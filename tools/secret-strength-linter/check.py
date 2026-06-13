#!/usr/bin/env python3
"""Check strength of secrets stored in SOPS-encrypted files."""

import json
import re
import subprocess
import sys

from zxcvbn import zxcvbn

# Matches `key = "value"` in plaintext formats like .tfvars or .env
_KV_RE = re.compile(
    r"""^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*"([^"]+)"\s*$""",
    re.MULTILINE,
)

# Key names containing any of these substrings (case-insensitive) are checked
_SECRET_WORDS = (
    "password",
    "passwd",
    "pw",
    "secret",
    "token",
    "credential",
    "private_key",
    "access_key",
    "api_key",
    "auth_key",
)

# zxcvbn score 0–4; values below this threshold fail
_MIN_SCORE = 3


def _is_secret_key(key: str) -> bool:
    k = key.lower()
    return any(w in k for w in _SECRET_WORDS)


def _walk(obj: dict, path: list[str], failures: list[dict], source: str) -> None:
    for key, value in obj.items():
        if key == "sops":
            continue
        current = path + [key]
        if isinstance(value, dict):
            _walk(value, current, failures, source)
        elif isinstance(value, str) and value and _is_secret_key(key):
            result = zxcvbn(value)
            score = result["score"]
            if score < _MIN_SCORE:
                failures.append(
                    {
                        "file": source,
                        "key": ".".join(current),
                        "score": score,
                        "feedback": result["feedback"],
                    }
                )


def _check_file(path: str) -> list[dict]:
    proc = subprocess.run(
        ["sops", "--decrypt", "--output-type", "json", path],
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        print(f"ERROR: sops decrypt failed for {path}:\n{proc.stderr}", file=sys.stderr)
        sys.exit(1)

    data = json.loads(proc.stdout)

    # SOPS binary format wraps the whole plaintext in a single "data" key.
    # Parse it as key=value pairs so .tfvars/.env-style secrets are checked.
    non_sops = [k for k in data if k != "sops"]
    if non_sops == ["data"] and isinstance(data.get("data"), str):
        data = {k: v for k, v in _KV_RE.findall(data["data"])}

    failures: list[dict] = []
    _walk(data, [], failures, path)
    return failures


def main() -> None:
    files = sys.argv[1:]
    if not files:
        print("No SOPS files to check.")
        sys.exit(0)

    all_failures: list[dict] = []
    for path in files:
        all_failures.extend(_check_file(path))

    if not all_failures:
        print(f"All secrets passed strength check (score >= {_MIN_SCORE}/4).")
        return

    print(f"\n{len(all_failures)} weak secret(s) found:\n")
    for f in all_failures:
        print(f"  FAIL  {f['file']} -> {f['key']}  (score {f['score']}/4)")
        if warning := f["feedback"].get("warning"):
            print(f"        Warning: {warning}")
        for suggestion in f["feedback"].get("suggestions", []):
            print(f"        Suggestion: {suggestion}")
    print(f"\nMinimum required score: {_MIN_SCORE}/4")
    sys.exit(1)


if __name__ == "__main__":
    main()
