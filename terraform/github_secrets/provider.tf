terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "6.11.1"
    }
  }

  required_version = ">= 1.14.7"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state/github_secrets"
  }
}

provider "github" {
  owner = local.github_owner
}
