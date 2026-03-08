resource "google_kms_key_ring" "sops" {
  name     = "sops"
  location = "global"

  depends_on = [google_project_service.cloudkms]
}

resource "google_kms_crypto_key" "sops_key" {
  name     = "sops-key"
  key_ring = google_kms_key_ring.sops.id
  purpose  = "ENCRYPT_DECRYPT"

  rotation_period = "7776000s"
}

output "kms_keyring_name" {
  description = "Name of the KMS key ring for SOPS encryption"
  value       = google_kms_key_ring.sops.name
}

output "kms_key_name" {
  description = "Name of the KMS crypto key for SOPS encryption"
  value       = google_kms_crypto_key.sops_key.name
}

output "kms_key_id" {
  description = "ID of the KMS crypto key for SOPS encryption"
  value       = google_kms_crypto_key.sops_key.id
}
