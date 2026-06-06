# Grafana Cloud IRM IaC

This Terraform root manages only Grafana Cloud IRM resources used for OnCall:

- primary escalation chain
- integration endpoint for self-hosted Grafana Alerting
- optional integration routes

Dashboards, data sources, alert rules, and notification policies for the self-hosted Grafana instance stay under `terraform/grafana`.

## Auth

Use a Grafana Cloud service account token. Legacy OnCall API tokens are deprecated by Grafana, so prefer the stack URL plus service account token provider configuration.

Find `grafana_cloud_irm_api_url` in Grafana Cloud:

```text
Alerts & IRM > IRM > Settings > Admin & API
```

Put secrets in `terraform.secrets.tfvars` and encrypt them with SOPS:

```hcl
grafana_cloud_stack_url   = "https://<stack>.grafana.net/"
grafana_cloud_auth        = "glsa_..."
grafana_cloud_irm_api_url = "https://<irm-api-url>/"
```

## Wiring Self-Hosted Grafana

After applying this root, copy the sensitive output into the self-hosted Grafana root as a webhook contact point:

```sh
terraform output -raw selfhost_grafana_irm_webhook_url
```

Then set it in `terraform/grafana/terraform.secrets.tfvars`:

```hcl
webhook_contact_points = {
  grafana-cloud-irm = {
    url = "<selfhost_grafana_irm_webhook_url>"
  }
}

notification_policy = {
  contact_point = "grafana-cloud-irm"
  group_by = [
    "grafana_folder",
    "alertname",
  ]
}
```

Use `routes` to send selected payloads to the primary escalation chain before the default route:

```hcl
routes = [
  {
    routing_regex = "\"severity\" *: *\"critical\""
    position      = 0
  },
]
```
