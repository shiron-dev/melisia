# Compose

## Directory structure

```
compose/
├── projects/        # Shared compose definitions
├── hosts/           # Per-host settings
└── README.md
```

### projects

Each subdirectory is a Docker Compose **project**:

```
projects/
└── <project>/
    ├── compose.yml        # Compose service definitions
    └── files/             # Files copied alongside compose.yml
        └── ...
```

### hosts

Each subdirectory matches a host name defined in the cmt config:

```
hosts/
└── <hostname>/
    ├── host.yml                   # Host-level defaults & project overrides
    └── <project>/
        ├── compose.override.yml   # Host-specific compose override
        ├── .env                   # Host-specific environment variables
        ├── env.secrets.yml        # Secret variables (YAML, overrides .env)
        └── files/                 # Host-specific files (override project files)
            └── ...
```

**host.yml** sets host-level defaults and optional per-project overrides:

```yaml
remotePath: /opt/compose
composeAction: up                # up|down|ignore (default: up)

projects:
  grafana:
    composeAction: up            # per-project override
  legacy:
    composeAction: ignore        # ignore up/down runtime drift
```

## Go Template support

All files synced by cmt (compose.yml, compose.override.yml, .env, files/) are
processed as **Go templates** before being compared or uploaded to the remote
host.

### Template variable sources

Template variables are loaded from the host project directory
(`hosts/<hostname>/<project>/`) in the following order:

1. `.env` — KEY=VALUE format (lower priority)
2. `env.secrets.yml` — flat YAML key-value (higher priority, overrides `.env`)

When both files exist, values from `env.secrets.yml` take precedence over `.env`.

> **Note:** `.env` is still synced to the remote host as a regular file (and is
> itself processed as a Go template). `env.secrets.yml` is used **only** as a
> template variable source and is **not** synced.

### env.secrets.yml format

A flat YAML file with string key-value pairs:

```yaml
grafana_github_client_id: Ov23liJlPV8sqCDhQso7
grafana_github_client_secret: cc99787102c3822d256292768aa7345fc4884614
smtp_host: mail.example.com:587
smtp_password: s3cret
```

### Template syntax

Use standard Go template syntax (`{{ .key_name }}`).  
See <https://pkg.go.dev/text/template> for full reference.

**Example — compose.yml:**

```yaml
services:
  grafana:
    environment:
      - GF_AUTH_GITHUB_CLIENT_ID={{ .grafana_github_client_id }}
      - GF_AUTH_GITHUB_CLIENT_SECRET={{ .grafana_github_client_secret }}
```

**Example — config file in files/:**

```ini
[smtp]
host = {{ .smtp_host }}
password = {{ .smtp_password }}
```

### Behaviour details

- **Binary files** (files containing null bytes) are **not** processed as
  templates and are synced as-is.
- If a template variable referenced in a file is missing from both `.env` and
  `env.secrets.yml`, cmt exits with an error.
- If neither `.env` nor `env.secrets.yml` exists, files are synced without
  template processing.

## Sync to hosts

Syncing is handled by the **Compose Manage Tool** (`/tools/cmt`).  
See [`/tools/cmt/README.md`](/tools/cmt/README.md) for usage.
