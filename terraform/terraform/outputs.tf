output "workload_identity_provider" {
  description = "GCP Workload Identity Pool Provider name"
  value       = google_iam_workload_identity_pool_provider.github_actions.name
}

output "service_account" {
  description = "GCP Service Account email for GitHub Actions"
  value       = google_service_account.github_actions_melisia.email
}

output "home_ep_ssh_private_key_secret_id" {
  description = "Secret Manager secret ID containing the home-ep SSH private key for GitHub Actions"
  value       = google_secret_manager_secret.github_actions_home_ep_ssh_private_key.secret_id
}

output "home_kiosk_ssh_private_key_secret_id" {
  description = "Secret Manager secret ID containing the home-kiosk SSH private key for GitHub Actions"
  value       = google_secret_manager_secret.github_actions_home_kiosk_ssh_private_key.secret_id
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

output "cf_access_e2e_client_id" {
  description = "Cloudflare Zero Trust Access Service Token client ID for Grafana E2E probes"
  value       = cloudflare_zero_trust_access_service_token.e2e.client_id
  sensitive   = true
}

output "cf_access_e2e_client_secret" {
  description = "Cloudflare Zero Trust Access Service Token client secret for Grafana E2E probes"
  value       = cloudflare_zero_trust_access_service_token.e2e.client_secret
  sensitive   = true
}
