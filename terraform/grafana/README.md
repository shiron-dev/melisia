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

`grafana_notification_policy.root` manages the whole notification policy tree. Keep the variable default at `null`; once the current Grafana policy has been reviewed and imported, put the managed root policy in `terraform.secrets.tfvars.sops` or another explicit tfvars file used for that environment.

Rule groups are driven by `var.rule_groups` so alert rules can be added in `terraform.secrets.tfvars` or a normal tfvars file without changing the resource shape.

Slack notifications can be enabled by adding a Slack contact point and routing the root notification policy to it. Put webhook URLs, Slack API tokens, and channel IDs in `terraform.secrets.tfvars` and encrypt them with SOPS:

```hcl
slack_contact_points = {
  slack-alerts = {
    url      = "https://hooks.slack.com/services/..."
    username = "Grafana"
    title    = "{{ template \"default.title\" . }}"
    text     = "{{ template \"default.message\" . }}"
  }
}

notification_policy = {
  contact_point   = "slack-alerts"
  group_by        = ["alertname"]
  group_wait      = "30s"
  group_interval  = "5m"
  repeat_interval = "4h"
}
```

If using a Slack bot token instead of an incoming webhook, set `token` and `recipient` instead of `url`.

Grafana Cloud IRM notifications are sent from self-hosted Grafana Alerting through a webhook contact point. Manage the IRM integration endpoint in `terraform/grafana_irm`, then put its sensitive output in this root's `terraform.secrets.tfvars`:

```hcl
webhook_contact_points = {
  grafana-cloud-irm = {
    url = "<terraform/grafana_irm selfhost_grafana_irm_webhook_url>"
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

## Metrics Persistence

**メトリクスの永続化バックエンドは必ず用意すること。**

Grafana 自体はメトリクスを保存しない。Prometheus や類似のスクレイパーが収集したデータを永続化する専用ストレージ（VictoriaMetrics など）を別途立ち上げ、Grafana のデータソースはそこを向けること。

- ✅ 正しい構成: Prometheus → **VictoriaMetrics** → Grafana データソース
- ❌ 避けるべき構成: Prometheus（インメモリのみ）→ Grafana データソース

Prometheus のデフォルト保持期間は短期間（15 日）であり、コンテナ再起動でデータが失われる。VictoriaMetrics のような長期ストレージを挟むことで、再起動・障害・移行をまたいでメトリクスを保持できる。

このリポジトリでは `victoriametrics` コンテナ（`compose/projects/grafana/compose.yml`）が永続ストレージの役割を担い、Grafana データソース UID `P95B22FBE6FE890D0` がそこを参照している。新しいデータソースを追加する場合も、必ず永続化バックエンドを経由させること。

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
