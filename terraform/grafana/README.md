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

Grafana 自体はメトリクスを保存しない。Prometheus や類似のスクレイパーが収集したデータを永続化する専用ストレージ（Grafana Mimir など）を別途立ち上げ、Grafana のデータソースはそこを向けること。

- ✅ 正しい構成: vmagent → **Mimir** → Grafana データソース
- ❌ 避けるべき構成: Prometheus（永続 volume なし）→ Grafana データソース

Prometheus はローカル TSDB にデータを書き込むが、永続 volume をマウントしていない場合はコンテナの再作成・削除でデータが失われる。またデフォルトの保持期間は 15 日と短い。Mimir のような長期ストレージを挟むことで、コンテナのライフサイクルや障害・移行をまたいでメトリクスを保持できる。

このリポジトリでは `mimir` コンテナ（`compose/projects/grafana/compose.yml`）が永続ストレージの役割を担い、Grafana データソース UID `P95B22FBE6FE890D0` がそこを参照している。新しいデータソースを追加する場合も、必ず永続化バックエンドを経由させること。

### 収集トポロジ (vmagent push)

メトリクス収集は **vmagent** に統一し、各ホストでローカルスクレイプして
arm-srv の Mimir へ remote_write (push) する。永続化は arm-srv の
`mimir` 一箇所に集約される。

```text
arm-srv:  local exporters / e2e blackbox ──► vmagent ──► mimir ◄── Grafana
home-ep:  node / icmp-ping / speedtest ────► vmagent ──(HTTPS push)────────┘
                                                       vm-write.shiron.dev
                                                       (Tunnel + Access: vm_write token)
```

- arm-srv: `vmagent` (`compose/projects/grafana`) がローカル exporter と
  e2e blackbox プローブをスクレイプし、同居の `mimir` へ remote_write。
  書き込みエンドポイント: `http://mimir:9009/api/v1/push`
- home-ep: `vmagent` (`compose/projects/network-monitor`) がローカルの node /
  icmp-ping (8.8.8.8 へ 10 分ごとに 5 発 ping) / cloudflare-speedtest exporter を
  スクレイプし、`vm-write.shiron.dev` 経由で arm-srv へ push。
  書き込みエンドポイント: `https://vm-write.shiron.dev/api/v1/push`
- node exporter は全ホストで `job="node"` に統一し、対象ホストの識別には
  `host` ラベル (`home-ep`, `arm-srv` など) を使う。`instance` は
  `host.docker.internal:9100` のような scrape 接続先を表すため、dashboard の
  ホスト選択には使わない。
- これにより各 exporter を外部公開してスクレイプさせる必要がなくなり、CF-Access
  認証は「スクレイプ経路」から「remote_write 経路」へ移動した。
- 旧 InfluxDB は停止し、永続化バックエンドは Mimir に一本化した。

#### 書き込み経路の認証

`vm-write.shiron.dev` は Cloudflare Tunnel 経由で `vmauth:8427` に転送する。
`vmauth` は `/api/v1/push` を `mimir:9009` へ、`/loki/api/v1/push` を `loki:3100` へ
ルーティングし、それ以外のパス (query / admin 系) はルート無しで拒否する。
認証は書き込み専用の Cloudflare Access service token (`vm_write`) のみで、
arm-srv 内部の blackbox e2e 用 `e2e` token とは分離している。vm-write の Access
application には共通 e2e ポリシーを付与しない (`skip_e2e_policy = true`)。

## Existing Resources

The existing Mimir-backed Prometheus data source has a deterministic UID in cmt provisioning:

```text
P95B22FBE6FE890D0
```

It is owned by Terraform. Keep Grafana datasource provisioning disabled in `compose/projects/grafana` to avoid double management.

If a dashboard is first drafted in the Grafana UI, export its JSON and place it under the folder key that should own it, for example:

```text
terraform/grafana/dashboards/observability/my-dashboard.json
```
