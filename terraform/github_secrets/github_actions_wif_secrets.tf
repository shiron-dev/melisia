resource "github_actions_environment_secret" "workload_identity_provider" {
  for_each = local.github_environments

  repository      = local.github_repository
  environment     = each.value
  secret_name     = "WORKLOAD_IDENTITY_PROVIDER"
  plaintext_value = data.terraform_remote_state.main.outputs.workload_identity_provider
}

resource "github_actions_environment_secret" "service_account" {
  for_each = local.github_environments

  repository      = local.github_repository
  environment     = each.value
  secret_name     = "SERVICE_ACCOUNT"
  plaintext_value = data.terraform_remote_state.main.outputs.service_account
}

resource "github_actions_secret" "cleanup_workload_identity_provider" {
  repository      = local.github_repository
  secret_name     = "CLEANUP_WORKLOAD_IDENTITY_PROVIDER"
  plaintext_value = data.terraform_remote_state.main.outputs.workload_identity_provider
}

resource "github_actions_secret" "cleanup_service_account" {
  repository      = local.github_repository
  secret_name     = "CLEANUP_SERVICE_ACCOUNT"
  plaintext_value = data.terraform_remote_state.main.outputs.service_account
}
