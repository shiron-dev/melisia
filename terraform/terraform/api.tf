resource "google_project_service" "cloudkms" {
  service = "cloudkms.googleapis.com"
}

resource "google_project_service" "iamcredentials" {
  service = "iamcredentials.googleapis.com"
}

resource "google_project_service" "sts" {
  service = "sts.googleapis.com"
}
