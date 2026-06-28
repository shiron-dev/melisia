# node_exporter は TrueNAS SCALE 25.04 以降に netdata から間引かれた host メトリクス
# (memory/cpu 等) を durable に取得するための Custom App。TrueNAS Apps は apps プール
# に保存されアップデートを越えて生存するため、netdata.conf の再適用に頼らずに済む。
# home-ep の vmagent が storage-srv:9100 を LAN スクレイプする
# (compose/projects/network-monitor の node ジョブ)。
#
# 既存の手動作成アプリを Terraform 管理下へ取り込むには、初回のみ import が必要:
#   make terraform-provider-devrc
#   cd terraform/truenas && TF_CLI_CONFIG_FILE=../../.local/terraform-provider-truenas.tfrc \
#     terraform import -var-file=terraform.secrets.tfvars \
#     truenas_custom_app.node_exporter node-exporter
# import 後の plan が no-op になることを確認してから apply すること。
locals {
  node_exporter_compose = {
    services = {
      "node-exporter" = {
        command = [
          "--path.procfs=/host/proc",
          "--path.sysfs=/host/sys",
          "--path.rootfs=/host/root",
        ]
        container_name = "node-exporter"
        image          = "quay.io/prometheus/node-exporter:v1.10.2"
        network_mode   = "host"
        pid            = "host"
        restart        = "unless-stopped"
        volumes = [
          "/proc:/host/proc:ro",
          "/sys:/host/sys:ro",
          "/:/host/root:ro,rslave",
        ]
      }
    }
  }
}

data "truenas_app_config_document" "node_exporter" {
  config = local.node_exporter_compose
}

resource "truenas_custom_app" "node_exporter" {
  name           = "node-exporter"
  compose_config = data.truenas_app_config_document.node_exporter.json
}
