locals {
  cloudflare_resource_name_suffix = " - Melisia Terraform"
  cloudflare_zone_name            = "shiron.dev"

  cloudflare_access_policies = {
    shiron = "7af17427-a95f-44da-ad13-c0e6e74cef90"
  }

  cloudflare_access_policy_refs = {
    shiron = [
      {
        id         = local.cloudflare_access_policies.shiron
        precedence = 1
      }
    ]
  }

  cloudflare_tunnels = {
    "arm-srv-snipeit" = {
      domain          = "snipeit.shiron.dev"
      service         = "http://snipeit-app:80"
      secret_yaml_dir = "${path.module}/../compose/hosts/arm-srv/snipeit"
      policies        = local.cloudflare_access_policy_refs.shiron
    }
  }
}

data "cloudflare_zone" "this" {
  filter = {
    name = local.cloudflare_zone_name
  }
}

resource "random_id" "cloudflare_tunnel_secret" {
  for_each = local.cloudflare_tunnels

  byte_length = 180
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "this" {
  for_each = local.cloudflare_tunnels

  account_id    = local.cloudflare_account_id
  name          = "${each.key}${local.cloudflare_resource_name_suffix}"
  config_src    = "cloudflare"
  tunnel_secret = random_id.cloudflare_tunnel_secret[each.key].b64_std
}

resource "cloudflare_zero_trust_tunnel_cloudflared_config" "this" {
  for_each = local.cloudflare_tunnels

  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.this[each.key].id
  source     = "cloudflare"

  config = {
    ingress = [
      {
        hostname = each.value.domain
        service  = each.value.service
      },
      {
        service = "http_status:404"
      }
    ]
  }
}

resource "cloudflare_dns_record" "tunnel" {
  for_each = local.cloudflare_tunnels

  zone_id = data.cloudflare_zone.this.zone_id
  name    = trimsuffix(each.value.domain, ".${local.cloudflare_zone_name}")
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.this[each.key].id}.cfargotunnel.com"
  ttl     = 1
  proxied = true
}

data "cloudflare_zero_trust_tunnel_cloudflared_token" "this" {
  for_each = local.cloudflare_tunnels

  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.this[each.key].id
}

resource "cloudflare_zero_trust_access_application" "this" {
  for_each = local.cloudflare_tunnels

  account_id                = local.cloudflare_account_id
  name                      = "${each.key}${local.cloudflare_resource_name_suffix}"
  domain                    = each.value.domain
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = each.value.policies
}

resource "local_sensitive_file" "cloudflare_tunnel_secret" {
  for_each = local.cloudflare_tunnels

  filename = "${trimsuffix(lookup(each.value, "secret_yaml_dir", path.module), "/")}/cloudflare-tunnel-${each.key}.secrets.yml"
  content = yamlencode({
    cf_tunnel_token = data.cloudflare_zero_trust_tunnel_cloudflared_token.this[each.key].token
  })
}
