terraform {
  required_providers {
    grafana = {
      source  = "grafana/grafana"
      version = "4.37.0"
    }
  }

  required_version = ">= 1.15.6"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state/grafana_irm"
  }
}

provider "grafana" {
  alias = "oncall"

  url        = var.grafana_cloud_stack_url
  auth       = var.grafana_cloud_auth
  oncall_url = var.grafana_cloud_irm_api_url
}
