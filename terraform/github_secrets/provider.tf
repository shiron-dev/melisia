terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "6.12.1"
    }
  }

  required_version = ">= 1.15.5"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state/github_secrets"
  }
}

provider "github" {
  owner = local.github_owner
}
