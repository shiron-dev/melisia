locals {
  cloudflare_account_id = "edc628145468437b85dc0e6d48bff3e3"
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "arm_srv" {
  account_id = local.cloudflare_account_id
  name       = "oci-arm"
  config_src = "cloudflare"
}

resource "cloudflare_zero_trust_access_service_token" "github_actions_arm_srv" {
  account_id = local.cloudflare_account_id
  name       = "github-actions-arm-srv-ssh"
  duration   = "8760h"
}

resource "cloudflare_zero_trust_access_application" "arm_srv" {
  account_id                = local.cloudflare_account_id
  name                      = "oci-arm"
  domain                    = "arm-srv.shiron.dev"
  type                      = "ssh"
  session_duration          = "24h"
  service_auth_401_redirect = false
  auto_redirect_to_identity = false
  app_launcher_visible      = true

  policies = [
    {
      name       = "Allow GitHub Actions Service Token"
      decision   = "non_identity"
      precedence = 1
      include = [
        {
          service_token = {
            token_id = cloudflare_zero_trust_access_service_token.github_actions_arm_srv.id
          }
        }
      ]
    },
    {
      name       = "SSH Policy"
      decision   = "allow"
      precedence = 2
      include = [
        {
          group = {
            id = "09b05356-05d0-4f9e-89db-b163531b01dc"
          }
        }
      ]
    },
  ]
}

