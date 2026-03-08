locals {
  github_owner      = "shiron-dev"
  github_repository = "melisia"
  github_environments = toset([
    "production",
    "production-plan",
  ])
}
