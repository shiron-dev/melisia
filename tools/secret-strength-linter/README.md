# Secret strength linter

UV-managed tool that decrypts SOPS-encrypted files and checks the strength of
any secret values they contain using [zxcvbn](https://github.com/dropbox/zxcvbn).
Intended to block weak secrets from being committed; the reusable workflow at
`.github/workflows/secret-strength-linter.yml` is not yet wired into CI (its
unit tests are). Run it manually as shown below until the CI wiring lands.

## How it works

For each file passed on the command line, `check.py`:

1. Decrypts it with `sops --decrypt --output-type json`.
2. Walks the decrypted structure (nested dicts/lists supported), skipping the
   `sops` metadata key.
3. For every string value whose key name looks like a secret (see
   `_SECRET_WORDS` — `password`, `token`, `secret`, `api_key`, …), scores it
   with zxcvbn (0–4).
4. Fails (exit code 1) if any secret scores below `_MIN_SCORE` (default `3`),
   printing the file, key path, score, and zxcvbn feedback.

SOPS binary-format files wrap their plaintext in a single `data` key. Such
content is parsed as HCL when the filename contains `tfvars`, otherwise as
`.env`-style `key=value` pairs. If a `.env` line can't be parsed the linter
errors out rather than silently skipping a potentially weak secret.

## Usage

From repo root, against one or more SOPS files:

```bash
uv run --project tools/secret-strength-linter \
  python tools/secret-strength-linter/check.py path/to/file.secrets.sops.env
```

Requires `sops` on `PATH` and access to the decryption key. With no file
arguments it prints a notice and exits `0`.

## Development

```bash
cd tools/secret-strength-linter
uv sync
uv run pytest
```

The tests cover the pure parsing/scoring helpers (`sops` decryption is mocked),
so no decryption key or `sops` binary is needed to run them.
