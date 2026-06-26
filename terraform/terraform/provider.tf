terraform {
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "5.21.1"
    }
    google = {
      source  = "hashicorp/google"
      version = "7.38.0"
    }
    github = {
      source  = "integrations/github"
      version = "6.12.1"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.9.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.9.0"
    }
  }

  required_version = ">= 1.15.6"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state"
  }
}

provider "google" {
  project = "shiron-dev"
  region  = "asia-northeast1"
}

provider "github" {
  owner = local.github_owner
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}
