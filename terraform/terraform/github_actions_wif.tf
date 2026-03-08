resource "google_iam_workload_identity_pool" "github_actions" {
  workload_identity_pool_id = "github-actions-pool"
  display_name              = "GitHub Actions Pool"
  description               = "OIDC identities from GitHub Actions"
}

resource "google_iam_workload_identity_pool_provider" "github_actions" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github_actions.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-provider"
  display_name                       = "GitHub Actions Provider"
  description                        = "Trust token.actions.githubusercontent.com"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
  }

  attribute_condition = "assertion.repository == \"shiron-dev/melisia\""

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

resource "google_service_account" "github_actions_melisia" {
  account_id   = "github-actions-melisia"
  display_name = "GitHub Actions (shiron-dev/melisia)"
}

resource "google_service_account_iam_member" "github_actions_melisia_wif_user" {
  service_account_id = google_service_account.github_actions_melisia.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github_actions.name}/attribute.repository/shiron-dev/melisia"
}

resource "google_kms_crypto_key_iam_member" "github_actions_melisia_kms_decrypter" {
  crypto_key_id = google_kms_crypto_key.sops_key.id
  role          = "roles/cloudkms.cryptoKeyDecrypter"
  member        = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}

resource "google_storage_bucket_iam_member" "github_actions_melisia_terraform_state" {
  bucket = "shiron-dev-terraform"
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}

resource "google_project_iam_member" "github_actions_melisia_service_usage_admin" {
  project = "shiron-dev"
  role    = "roles/serviceusage.serviceUsageAdmin"
  member  = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}

resource "google_project_iam_member" "github_actions_melisia_service_usage_consumer" {
  project = "shiron-dev"
  role    = "roles/serviceusage.serviceUsageConsumer"
  member  = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}

resource "google_project_iam_member" "github_actions_melisia_iam_security_reviewer" {
  project = "shiron-dev"
  role    = "roles/iam.securityReviewer"
  member  = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}

resource "google_project_iam_member" "github_actions_melisia_wif_pool_viewer" {
  project = "shiron-dev"
  role    = "roles/iam.workloadIdentityPoolViewer"
  member  = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}

resource "google_project_iam_member" "github_actions_melisia_cloudkms_viewer" {
  project = "shiron-dev"
  role    = "roles/cloudkms.viewer"
  member  = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}
