resource "github_actions_environment_secret" "cloudflare_access_client_id" {
  for_each = local.github_environments

  repository      = local.github_repository
  environment     = each.value
  secret_name     = "CF_ACCESS_CLIENT_ID"
  plaintext_value = var.cf_access_client_id
}

resource "github_actions_environment_secret" "cloudflare_access_client_secret" {
  for_each = local.github_environments

  repository      = local.github_repository
  environment     = each.value
  secret_name     = "CF_ACCESS_CLIENT_SECRET"
  plaintext_value = var.cf_access_client_secret
}
