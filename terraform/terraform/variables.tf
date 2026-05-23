variable "cloudflare_api_token" {
  description = "Cloudflare API Token. Mesh requires Account: Cloudflare One Connectors Write (or Cloudflare One Connector: WARP Write), Cloudflare One Networks Write / Cloudflare Tunnel Write for routes, and Zero Trust Write for WARP device profile split tunnels."
  type        = string
  sensitive   = true
}

variable "home_ep_mesh_ip" {
  description = "Cloudflare Mesh IP assigned to home-ep, for example 100.96.x.y. Leave null until the node is enrolled."
  type        = string
  default     = null

  validation {
    condition     = var.home_ep_mesh_ip == null || can(cidrhost("${var.home_ep_mesh_ip}/32", 0))
    error_message = "home_ep_mesh_ip must be a valid IPv4 address."
  }
}
