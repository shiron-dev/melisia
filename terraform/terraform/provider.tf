terraform {
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "5.17.0"
    }
    google = {
      source  = "hashicorp/google"
      version = "7.23.0"
    }
    github = {
      source  = "integrations/github"
      version = "6.11.1"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.7.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.8.1"
    }
  }

  required_version = ">= 1.14.7"

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
