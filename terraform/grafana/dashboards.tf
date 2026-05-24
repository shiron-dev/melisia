resource "grafana_dashboard" "managed" {
  for_each = local.dashboard_files

  folder      = grafana_folder.managed[split("/", each.key)[0]].uid
  config_json = file("${path.module}/dashboards/${each.key}")
  overwrite   = true

  depends_on = [
    grafana_data_source.managed,
  ]
}
