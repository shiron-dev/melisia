variable "slack_bot_token" {
  description = "Slack Bot Token for GitHub Actions notifications"
  type        = string
  sensitive   = true
}

variable "infracost_api_key" {
  description = "Infracost API Key for cost estimation"
  type        = string
  sensitive   = true
}
