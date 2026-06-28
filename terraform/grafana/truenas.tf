resource "grafana_dashboard" "truenas" {
  folder      = grafana_folder.infrastructure.uid
  config_json = file("${path.module}/truenas-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}
