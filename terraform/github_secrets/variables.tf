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

variable "workload_identity_provider" {
  description = "GCP Workload Identity Pool Provider name (output from main terraform)"
  type        = string
}

variable "service_account" {
  description = "GCP Service Account email for GitHub Actions (output from main terraform)"
  type        = string
}

variable "cf_access_client_id" {
  description = "Cloudflare Zero Trust Access Service Token client ID (output from main terraform)"
  type        = string
  sensitive   = true
}

variable "cf_access_client_secret" {
  description = "Cloudflare Zero Trust Access Service Token client secret (output from main terraform)"
  type        = string
  sensitive   = true
}
