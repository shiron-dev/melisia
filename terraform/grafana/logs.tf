resource "grafana_dashboard" "logs_overview" {
  folder      = grafana_folder.infrastructure.uid
  config_json = file("${path.module}/logs-overview-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}
