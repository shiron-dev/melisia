variable "cloudflare_api_token" {
  description = "Cloudflare API Token"
  type        = string
  sensitive   = true
}

variable "gh_actions_ssh_private_key" {
  description = "SSH private key for GitHub Actions to connect to arm-srv"
  type        = string
  sensitive   = true
}

variable "slack_bot_token" {
  description = "Slack Bot Token for GitHub Actions notifications"
  type        = string
  sensitive   = true
}

variable "slack_channel_id" {
  description = "Slack Channel ID for GitHub Actions notifications"
  type        = string
}

variable "infracost_api_key" {
  description = "Infracost API Key for cost estimation"
  type        = string
  sensitive   = true
}
