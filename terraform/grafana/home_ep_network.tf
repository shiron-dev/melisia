resource "grafana_dashboard" "home_ep_network" {
  folder      = grafana_folder.infrastructure.uid
  config_json = file("${path.module}/home-ep-network-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}
