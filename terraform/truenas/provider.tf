terraform {
  required_providers {
    truenas = {
      source  = "shiron-dev/truenas"
      version = "0.0.1"
    }
  }

  required_version = ">= 1.15.7"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state/truenas"
  }
}

provider "truenas" {
  base_url                 = var.truenas_url
  api_key                  = var.truenas_api_key
  tls_insecure_skip_verify = var.truenas_tls_insecure_skip_verify
}
