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
