output "workload_identity_provider" {
  description = "GCP Workload Identity Pool Provider name"
  value       = google_iam_workload_identity_pool_provider.github_actions.name
}

output "service_account" {
  description = "GCP Service Account email for GitHub Actions"
  value       = google_service_account.github_actions_melisia.email
}

output "cf_access_client_id" {
  description = "Cloudflare Zero Trust Access Service Token client ID"
  value       = cloudflare_zero_trust_access_service_token.github_actions_arm_srv.client_id
  sensitive   = true
}

output "cf_access_client_secret" {
  description = "Cloudflare Zero Trust Access Service Token client secret"
  value       = cloudflare_zero_trust_access_service_token.github_actions_arm_srv.client_secret
  sensitive   = true
}
