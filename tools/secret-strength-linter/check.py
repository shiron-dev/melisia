#!/usr/bin/env python3
"""Check strength of secrets stored in SOPS-encrypted files."""

import io
import json
import re
import subprocess
import sys

import hcl2
from zxcvbn import zxcvbn

# Matches key=value in double-quoted, single-quoted, or bare formats
# (.env, dotenv). Stops at # to ignore inline comments.
_KV_RE = re.compile(
    r"""^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:"([^"]*?)"|'([^']*?)'|([^"'#\n\s][^#\n]*?))\s*$""",
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
    "auth",
)

# zxcvbn score 0–4; values below this threshold fail
_MIN_SCORE = 3


def _is_secret_key(key: str) -> bool:
    k = key.lower()
    return any(w in k for w in _SECRET_WORDS)


def _walk(obj, path: list[str], failures: list[dict], source: str) -> None:
    if isinstance(obj, dict):
        for key, value in obj.items():
            if key == "sops":
                continue
            _walk(value, path + [key], failures, source)
    elif isinstance(obj, list):
        for item in obj:
            _walk(item, path, failures, source)
    elif isinstance(obj, str) and obj and path and _is_secret_key(path[-1]):
        result = zxcvbn(obj)
        score = result["score"]
        if score < _MIN_SCORE:
            failures.append(
                {
                    "file": source,
                    "key": ".".join(path),
                    "score": score,
                    "feedback": result["feedback"],
                }
            )


def _parse_hcl_data(content: str, source: str) -> dict:
    try:
        return hcl2.load(io.StringIO(content))
    except Exception as e:
        print(f"ERROR: {source}: HCL parse failed: {e}", file=sys.stderr)
        sys.exit(1)


def _parse_env_data(content: str, source: str) -> dict:
    result = {}
    for m in _KV_RE.finditer(content):
        key = m.group(1)
        value = next((g for g in m.groups()[1:] if g is not None), "")
        result[key] = value
    unparseable_lines = [
        i + 1
        for i, line in enumerate(content.splitlines())
        if line.strip() and not line.strip().startswith("#") and not _KV_RE.match(line)
    ]
    if unparseable_lines:
        print(
            f"ERROR: {source}: binary SOPS content has {len(unparseable_lines)} "
            f"unparseable line(s) at line(s) {unparseable_lines[:5]}; "
            "any secrets on those lines are unchecked",
            file=sys.stderr,
        )
        sys.exit(1)
    return result


def _parse_binary_data(content: str, source: str) -> dict:
    if "tfvars" in source:
        return _parse_hcl_data(content, source)
    return _parse_env_data(content, source)


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
    # Parse it as key=value pairs so .env-style secrets are checked, or as
    # HCL for .tfvars files.
    non_sops = [k for k in data if k != "sops"]
    if non_sops == ["data"] and isinstance(data.get("data"), str):
        data = _parse_binary_data(data["data"], path)

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
