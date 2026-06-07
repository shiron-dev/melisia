# Terraform Provider for TrueNAS

Before changing this provider, always read:

- [PVE verification guide](docs/pve-verification.md)

This provider is intentionally scoped to the TrueNAS resources currently used by
this repository:

- `data.truenas_pool`
- `truenas_dataset`

The provider talks to the TrueNAS SCALE REST API under `/api/v2.0` using a
Bearer API key.

## Local Development

Build the provider:

```sh
go build -o terraform-provider-truenas
```

Use a Terraform CLI config with a filesystem mirror while testing the
`terraform/truenas` root:

```hcl
provider_installation {
  filesystem_mirror {
    path = "/absolute/path/to/melisia/.local/terraform-provider-mirror"
    include = ["registry.terraform.io/shiron-dev/truenas"]
  }

  direct {
    exclude = ["registry.terraform.io/shiron-dev/truenas"]
  }
}
```

The provider accepts configuration from Terraform or environment variables:

- `base_url` or `TRUENAS_BASE_URL`
- `api_key` or `TRUENAS_API_KEY`
- `tls_insecure_skip_verify` or `TRUENAS_TLS_INSECURE_SKIP_VERIFY`
