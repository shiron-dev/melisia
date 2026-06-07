variable "truenas_url" {
  description = "Base URL of the TrueNAS SCALE instance managed by this Terraform root."
  type        = string
  default     = "https://storage-srv.network.melisia.net"
}

variable "truenas_api_key" {
  description = "TrueNAS API key. Prefer terraform.secrets.tfvars.sops or TRUENAS_API_KEY."
  type        = string
  sensitive   = true
  default     = null
}

variable "truenas_tls_insecure_skip_verify" {
  description = "Whether to skip TLS certificate verification for the TrueNAS API."
  type        = bool
  default     = true
}
