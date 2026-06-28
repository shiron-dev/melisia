resource "google_project_service" "secret_manager" {
  project = "shiron-dev"
  service = "secretmanager.googleapis.com"

  disable_on_destroy = false
}

resource "google_secret_manager_secret" "github_actions_home_ep_ssh_private_key" {
  secret_id = "github-actions-home-ep-ssh-private-key"

  depends_on = [google_project_service.secret_manager]

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_iam_member" "github_actions_home_ep_ssh_private_key_accessor" {
  secret_id = google_secret_manager_secret.github_actions_home_ep_ssh_private_key.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}

resource "google_secret_manager_secret" "github_actions_home_kiosk_ssh_private_key" {
  secret_id = "github-actions-home-kiosk-ssh-private-key"

  depends_on = [google_project_service.secret_manager]

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_iam_member" "github_actions_home_kiosk_ssh_private_key_accessor" {
  secret_id = google_secret_manager_secret.github_actions_home_kiosk_ssh_private_key.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.github_actions_melisia.email}"
}
