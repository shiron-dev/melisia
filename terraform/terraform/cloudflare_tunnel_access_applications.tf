resource "cloudflare_zero_trust_access_application" "extra_tunnel_ingress" {
  for_each = {
    for k, v in local.extra_tunnel_ingress_map : k => v
    if length(v.policies) > 0 && lookup(v, "manage_access_application", true)
  }

  account_id                = local.cloudflare_account_id
  name                      = "${each.key}${local.cloudflare_resource_name_suffix}"
  domain                    = each.value.hostname
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = concat([local.cloudflare_access_e2e_policy_ref], each.value.policies)
}

resource "cloudflare_zero_trust_access_application" "this" {
  for_each = {
    for k, v in local.cloudflare_tunnels : k => v
    if length(v.policies) > 0 && lookup(v, "manage_access_application", true)
  }

  account_id                = local.cloudflare_account_id
  name                      = "${each.key}${local.cloudflare_resource_name_suffix}"
  domain                    = each.value.domain
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = concat([local.cloudflare_access_e2e_policy_ref], each.value.policies)
}

resource "cloudflare_zero_trust_access_application" "n8n" {
  account_id                 = local.cloudflare_account_id
  name                       = "n8n"
  domain                     = "n8n.shiron.dev"
  type                       = "self_hosted"
  session_duration           = "24h"
  service_auth_401_redirect  = false
  auto_redirect_to_identity  = false
  app_launcher_visible       = true
  enable_binding_cookie      = false
  http_only_cookie_attribute = false
  options_preflight_bypass   = false

  policies = concat([local.cloudflare_access_e2e_policy_ref], local.cloudflare_access_policy_refs.n8n)
}

resource "cloudflare_zero_trust_access_application" "n8n_bypass" {
  account_id                 = local.cloudflare_account_id
  name                       = "n8n bypass"
  domain                     = "n8n.shiron.dev/webhook/*"
  type                       = "self_hosted"
  session_duration           = "24h"
  service_auth_401_redirect  = false
  auto_redirect_to_identity  = false
  app_launcher_visible       = true
  enable_binding_cookie      = false
  http_only_cookie_attribute = false
  options_preflight_bypass   = false

  policies = [
    {
      id         = local.cloudflare_access_policies.bypass
      precedence = 1
    }
  ]
}

resource "cloudflare_zero_trust_access_application" "home_ep_homeassistant_alexa_bypass" {
  account_id                = local.cloudflare_account_id
  name                      = "home-ep-homeassistant alexa bypass${local.cloudflare_resource_name_suffix}"
  domain                    = "home.melisia.net/api/alexa/smart_home"
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = [
    {
      id         = local.cloudflare_access_policies.bypass
      precedence = 1
    }
  ]
}

resource "cloudflare_zero_trust_access_application" "home_ep_homeassistant_auth_token_bypass" {
  account_id                = local.cloudflare_account_id
  name                      = "home-ep-homeassistant auth token bypass${local.cloudflare_resource_name_suffix}"
  domain                    = "home.melisia.net/auth/token"
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = [
    {
      id         = local.cloudflare_access_policies.bypass
      precedence = 1
    }
  ]
}

resource "cloudflare_zero_trust_access_application" "home_ep_homeassistant" {
  account_id                = local.cloudflare_account_id
  name                      = "home-ep-homeassistant${local.cloudflare_resource_name_suffix}"
  domain                    = "home.melisia.net/auth/authorize"
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = [
    local.cloudflare_access_e2e_policy_ref,
    local.cloudflare_home_login_policy_ref,
  ]
}
