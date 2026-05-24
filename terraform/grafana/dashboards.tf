resource "grafana_dashboard" "root" {
  for_each = local.root_dashboard_files

  config_json = file("${path.module}/dashboards/${each.key}")

  depends_on = [
    grafana_data_source.managed,
  ]
}

resource "grafana_dashboard" "folder" {
  for_each = local.folder_dashboard_files

  folder      = grafana_folder.managed[split("/", each.key)[0]].uid
  config_json = file("${path.module}/dashboards/${each.key}")

  depends_on = [
    grafana_data_source.managed,
  ]
}
