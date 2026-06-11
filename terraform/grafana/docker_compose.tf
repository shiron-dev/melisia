resource "grafana_dashboard" "docker_compose_containers" {
  folder      = grafana_folder.infrastructure.uid
  config_json = file("${path.module}/docker-compose-containers-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}
