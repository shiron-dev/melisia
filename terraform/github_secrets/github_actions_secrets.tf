resource "github_actions_secret" "gh_actions_ssh_private_key" {
  repository  = local.github_repository
  secret_name = "GH_ACTIONS_SSH_PRIVATE_KEY"
  value       = var.gh_actions_ssh_private_key
}

resource "github_actions_secret" "slack_bot_token" {
  repository  = local.github_repository
  secret_name = "SLACK_BOT_TOKEN"
  value       = var.slack_bot_token
}

resource "github_actions_secret" "infracost_api_key" {
  repository  = local.github_repository
  secret_name = "INFRACOST_API_KEY"
  value       = var.infracost_api_key
}
