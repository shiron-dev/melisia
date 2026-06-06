terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.48.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "2.8.0"
    }
    http = {
      source  = "hashicorp/http"
      version = "3.6.0"
    }
  }

  required_version = ">= 1.15.5"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "aws/state"
  }
}

provider "aws" {
  region = var.aws_region
}
