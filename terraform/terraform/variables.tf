variable "cloudflare_api_token" {
  description = "Cloudflare API Token. Mesh requires Account: Cloudflare One Connectors Write (or Cloudflare One Connector: WARP Write), Cloudflare One Networks Write / Cloudflare Tunnel Write for routes, and Zero Trust Write for WARP device profile split tunnels."
  type        = string
  sensitive   = true
}

variable "cloudflare_mesh_dns_updater_api_token" {
  description = "Cloudflare API Token used by the Mesh DNS updater Worker. Requires DNS Edit for the managed zone."
  type        = string
  sensitive   = true
}
