data "terraform_remote_state" "main" {
  backend = "gcs"

  config = {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state"
  }
}
