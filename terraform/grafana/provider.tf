terraform {
  required_providers {
    grafana = {
      source  = "grafana/grafana"
      version = "4.36.2"
    }
  }

  required_version = ">= 1.15.5"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state/grafana"
  }
}

provider "grafana" {
  url  = var.grafana_url
  auth = var.grafana_auth
}
