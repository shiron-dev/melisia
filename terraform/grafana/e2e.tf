resource "grafana_folder" "e2e" {
  title = "E2E"
  uid   = "e2e"
}

resource "grafana_dashboard" "cloudflare_tunnel_e2e" {
  folder      = grafana_folder.e2e.uid
  config_json = file("${path.module}/e2e-cloudflare-tunnel-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}

resource "grafana_dashboard" "cloudflare_access_block_e2e" {
  folder      = grafana_folder.e2e.uid
  config_json = file("${path.module}/cloudflare-access-block-e2e-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}
