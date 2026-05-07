terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.94.1"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "2.7.0"
    }
    http = {
      source  = "hashicorp/http"
      version = "3.5.0"
    }
  }

  required_version = ">= 1.14.7"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "aws/state"
  }
}

provider "aws" {
  region  = var.aws_region
  profile = "shiron-dev"
}
