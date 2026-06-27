resource "cloudflare_zero_trust_access_application" "extra_tunnel_ingress" {
  for_each = {
    for k, v in local.extra_tunnel_ingress_map : k => v
    if length(v.policies) > 0 && lookup(v, "manage_access_application", true) && !v.skip_e2e_policy
  }

  account_id                = local.cloudflare_account_id
  name                      = "${each.key}${local.cloudflare_resource_name_suffix}"
  domain                    = each.value.hostname
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = concat([local.cloudflare_access_e2e_policy_ref], each.value.policies)
}

# skip_e2e_policy = true の ingress (vm-write 等) 用。共通の e2e ポリシーを付与せず、
# ingress 側で指定した専用ポリシーだけで許可を絞る。
resource "cloudflare_zero_trust_access_application" "extra_tunnel_ingress_no_e2e" {
  for_each = {
    for k, v in local.extra_tunnel_ingress_map : k => v
    if length(v.policies) > 0 && lookup(v, "manage_access_application", true) && v.skip_e2e_policy
  }

  account_id                = local.cloudflare_account_id
  name                      = "${each.key}${local.cloudflare_resource_name_suffix}"
  domain                    = each.value.hostname
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = each.value.policies
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

import {
  to = cloudflare_zero_trust_access_application.n8n
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/b52adec0-a363-431f-80f7-43e5a5e1b9df"
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

  # n8n のみを対象にする。過去に同じアプリへ紛れ込んだ n8n-pgadmin.shiron.dev
  # (削除済み ingress) を destinations から外し、ドリフトを防ぐ。
  destinations = [
    {
      type = "public"
      uri  = "n8n.shiron.dev"
    }
  ]

  policies = concat([local.cloudflare_access_e2e_policy_ref], local.cloudflare_access_policy_refs.n8n)
}

import {
  to = cloudflare_zero_trust_access_application.n8n_bypass
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/d39f708f-53b7-41e3-b930-ce81174eeeae"
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

# FreshRSS の API パス (/api/greader.php, /api/fever.php 等) は RSS クライアントが
# 直接アクセスするため、対話的な Access ログインを通せない。親アプリ
# (freshrss.melisia.net, shiron ポリシー) より具体的なパスとして /api/* を
# bypass で公開し、API だけ SSO を免除する (UI 本体は引き続き Access 保護)。
resource "cloudflare_zero_trust_access_application" "freshrss_bypass" {
  account_id                 = local.cloudflare_account_id
  name                       = "freshrss bypass"
  domain                     = "freshrss.melisia.net/api/*"
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
