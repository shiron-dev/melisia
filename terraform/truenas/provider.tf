terraform {
  required_providers {
    truenas = {
      source  = "baladithyab/truenas"
      version = "0.2.25"
    }
  }

  required_version = ">= 1.15.5"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state/truenas"
  }
}

provider "truenas" {
  base_url = var.truenas_url
  api_key  = var.truenas_api_key
}
