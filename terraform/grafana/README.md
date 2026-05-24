# Grafana IaC

This Terraform root manages Grafana content that should be reproducible from code:

- folders
- data sources
- dashboards under `dashboards/*.json` for General, or `dashboards/<folder-key>/*.json` for managed folders
- alerting contact points
- alerting notification policy
- alerting rule groups

Grafana itself, its container, local provisioning files, and runtime storage stay under `compose/projects/grafana` and are deployed by `cmt`.

## Auth

Use a Grafana service account token with enough permissions to manage folders, data sources, dashboards, and alerting resources.

Prefer environment variables for local runs:

```sh
export GRAFANA_URL="https://grafana.shiron.dev/"
export GRAFANA_AUTH="glsa_..."
make terraform-plan TERRAFORM_TARGET=grafana
```

For CI or shared encrypted config, put `grafana_auth` in `terraform.secrets.tfvars`, then encrypt it with SOPS.

## Alerting

`grafana_notification_policy.root` manages the whole notification policy tree. Keep the variable default at `null`; once the current Grafana policy has been reviewed and imported, put the managed root policy in `notification_policy.auto.tfvars.json`.

Rule groups are driven by `var.rule_groups` so alert rules can be added in `terraform.secrets.tfvars` or a normal tfvars file without changing the resource shape.

## Existing Resources

The existing VictoriaMetrics-backed Prometheus data source has a deterministic UID in cmt provisioning:

```text
P95B22FBE6FE890D0
```

It is owned by Terraform. Keep Grafana datasource provisioning disabled in `compose/projects/grafana` to avoid double management.

If a dashboard is first drafted in the Grafana UI, export its JSON and place it under the folder key that should own it, for example:

```text
terraform/grafana/dashboards/observability/my-dashboard.json
```
