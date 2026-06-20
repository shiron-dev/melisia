resource "grafana_dashboard" "loki_data_size" {
  folder      = grafana_folder.infrastructure.uid
  config_json = file("${path.module}/loki-data-size-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}
