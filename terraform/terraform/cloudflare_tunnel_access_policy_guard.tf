resource "terraform_data" "cloudflare_tunnel_access_policy_guard" {
  input = local.cloudflare_tunnel_domains_without_access_policy

  lifecycle {
    precondition {
      condition     = length(local.cloudflare_tunnel_domains_without_access_policy) == 0
      error_message = "Cloudflare tunnel ingress without Access policy is dangerous. Add policies or set dangerously_allow_public_without_access_policy = true explicitly for: ${join(", ", local.cloudflare_tunnel_domains_without_access_policy)}"
    }
  }
}
