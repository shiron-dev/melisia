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

variable "truenas_apps_docker_config" {
  description = "Global TrueNAS Apps Docker settings managed through the TrueNAS REST API. This does not manage installed apps."
  type = object({
    enable_image_updates = bool
    pool                 = string
    nvidia               = bool
    address_pools = list(object({
      base = string
      size = number
    }))
  })
  default = {
    enable_image_updates = true
    pool                 = "apps"
    nvidia               = false
    address_pools = [
      {
        base = "172.17.0.0/12"
        size = 24
      },
      {
        base = "fdd0::/48"
        size = 64
      },
    ]
  }
}

variable "truenas_apps_catalog_config" {
  description = "Global TrueNAS Apps catalog settings managed through the TrueNAS REST API. This does not manage installed apps."
  type = object({
    preferred_trains = list(string)
  })
  default = {
    preferred_trains = ["community", "stable"]
  }
}

variable "truenas_nextcloud_admin_password" {
  description = "Admin password for the installed nextcloud TrueNAS app."
  type        = string
  sensitive   = true
}

variable "truenas_nextcloud_db_password" {
  description = "Database password for the installed nextcloud TrueNAS app."
  type        = string
  sensitive   = true
}

variable "truenas_nextcloud_redis_password" {
  description = "Redis password for the installed nextcloud TrueNAS app."
  type        = string
  sensitive   = true
}

variable "truenas_cloudflared_tunnel_token" {
  description = "Tunnel token for the installed cloudflared TrueNAS app."
  type        = string
  sensitive   = true
}
