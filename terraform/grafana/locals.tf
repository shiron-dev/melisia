locals {
  root_dashboard_files   = fileset("${path.module}/dashboards", "*.json")
  folder_dashboard_files = fileset("${path.module}/dashboards", "*/*.json")
}
