locals {
  github_actions_slack_channel_variables = {
    SLACK_DEPLOY_CHANNEL_ID  = "C0AGSD63GAV"
    SLACK_PR_CHANNEL_ID      = "C0B6NKW59G8"
    SLACK_PR_PLAN_CHANNEL_ID = "C0AK975HA2E"
  }
}

resource "github_actions_variable" "slack_channel_ids" {
  for_each = local.github_actions_slack_channel_variables

  repository    = local.github_repository
  variable_name = each.key
  value         = each.value
}
