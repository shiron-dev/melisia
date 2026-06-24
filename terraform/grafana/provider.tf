terraform {
  required_providers {
    grafana = {
      source  = "grafana/grafana"
      version = "4.39.0"
    }
  }

  required_version = ">= 1.15.6"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state/grafana"
  }
}

data "terraform_remote_state" "terraform" {
  backend = "gcs"
  config = {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state"
  }
}

provider "grafana" {
  url  = var.grafana_url
  auth = var.grafana_auth
  http_headers = {
    CF-Access-Client-Id     = data.terraform_remote_state.terraform.outputs.cf_access_e2e_client_id
    CF-Access-Client-Secret = data.terraform_remote_state.terraform.outputs.cf_access_e2e_client_secret
  }
}
