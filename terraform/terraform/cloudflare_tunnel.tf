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
        precedence = 2
      }
    ]
  }

  cloudflare_tunnels = {
    "arm-srv-snipeit" = {
      domain          = "snipeit.shiron.dev"
      zone_name       = "shiron.dev"
      service         = "http://snipeit-app:80"
      secret_yaml_dir = "${path.module}/../../compose/hosts/arm-srv/snipeit"
      policies        = local.cloudflare_access_policy_refs.shiron
    }
    "home-ep-homeassistant" = {
      domain          = "home.melisia.net"
      zone_name       = "melisia.net"
      service         = "http://homeassistant:8123"
      secret_yaml_dir = "${path.module}/../../compose/hosts/home-ep/home-assistant"
      policies        = []
      extra_ingress = [
        {
          hostname  = "zigbee2mqtt.melisia.net"
          zone_name = "melisia.net"
          service   = "http://zigbee2mqtt:8080"
          policies  = local.cloudflare_access_policy_refs.shiron
        },
        {
          hostname  = "switchbot-mqtt.melisia.net"
          zone_name = "melisia.net"
          service   = "http://switchbot-mqtt:8099"
          policies  = local.cloudflare_access_policy_refs.shiron
        },
        {
          hostname  = "esphome.melisia.net"
          zone_name = "melisia.net"
          service   = "http://esphome:6052"
          policies  = local.cloudflare_access_policy_refs.shiron
        }
      ]
    }
  }
}

resource "cloudflare_zero_trust_access_service_token" "e2e" {
  account_id = local.cloudflare_account_id
  name       = "e2e${local.cloudflare_resource_name_suffix}"
  duration   = "8760h"
}

locals {
  cloudflare_access_e2e_policy_ref = {
    name       = "Allow E2E Service Token"
    decision   = "non_identity"
    precedence = 1
    include = [
      {
        service_token = {
          token_id = cloudflare_zero_trust_access_service_token.e2e.id
        }
      }
    ]
  }
}

locals {
  extra_tunnel_ingress_map = {
    for item in flatten([
      for tunnel_key, tunnel in local.cloudflare_tunnels : [
        for ingress in lookup(tunnel, "extra_ingress", []) : {
          tunnel_key = tunnel_key
          hostname   = ingress.hostname
          zone_name  = ingress.zone_name
          service    = ingress.service
          policies   = lookup(ingress, "policies", [])
        }
      ]
    ]) : "${item.tunnel_key}-${item.hostname}" => item
  }
}

locals {
  cloudflare_zone_ids = {
    "shiron.dev"  = data.cloudflare_zone.this.zone_id
    "melisia.net" = data.cloudflare_zone.melisia_net.zone_id
  }
}

data "cloudflare_zone" "this" {
  filter = {
    name = local.cloudflare_zone_name
  }
}

data "cloudflare_zone" "melisia_net" {
  filter = {
    name = "melisia.net"
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
    ingress = concat(
      [
        {
          hostname = each.value.domain
          service  = each.value.service
        }
      ],
      [for i in lookup(each.value, "extra_ingress", []) : {
        hostname = i.hostname
        service  = i.service
      }],
      [
        {
          service = "http_status:404"
        }
      ]
    )
  }
}

resource "cloudflare_dns_record" "tunnel" {
  for_each = local.cloudflare_tunnels

  zone_id = local.cloudflare_zone_ids[each.value.zone_name]
  name    = trimsuffix(each.value.domain, ".${each.value.zone_name}")
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.this[each.key].id}.cfargotunnel.com"
  ttl     = 1
  proxied = true
}

resource "cloudflare_dns_record" "extra_tunnel_ingress" {
  for_each = local.extra_tunnel_ingress_map

  zone_id = local.cloudflare_zone_ids[each.value.zone_name]
  name    = trimsuffix(each.value.hostname, ".${each.value.zone_name}")
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.this[each.value.tunnel_key].id}.cfargotunnel.com"
  ttl     = 1
  proxied = true
}

resource "cloudflare_zero_trust_access_application" "extra_tunnel_ingress" {
  for_each = { for k, v in local.extra_tunnel_ingress_map : k => v if length(v.policies) > 0 }

  account_id                = local.cloudflare_account_id
  name                      = "${each.key}${local.cloudflare_resource_name_suffix}"
  domain                    = each.value.hostname
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = concat([local.cloudflare_access_e2e_policy_ref], each.value.policies)
}

resource "cloudflare_zero_trust_access_application" "this" {
  for_each = { for k, v in local.cloudflare_tunnels : k => v if length(v.policies) > 0 }

  account_id                = local.cloudflare_account_id
  name                      = "${each.key}${local.cloudflare_resource_name_suffix}"
  domain                    = each.value.domain
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false

  policies = concat([local.cloudflare_access_e2e_policy_ref], each.value.policies)
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
    {
      name       = "home login"
      decision   = "allow"
      precedence = 2
      include = [
        {
          group = {
            id = "09b05356-05d0-4f9e-89db-b163531b01dc"
          }
        }
      ]
    }
  ]
}

resource "local_sensitive_file" "cloudflare_access_e2e_secret" {
  filename = "${path.module}/../../compose/hosts/arm-srv/grafana/cloudflare-access-e2e.secrets.yml"
  content = yamlencode({
    cloudflare_access_e2e_client_id = cloudflare_zero_trust_access_service_token.e2e.client_id
    # kics-scan ignore-line
    cloudflare_access_e2e_client_secret = cloudflare_zero_trust_access_service_token.e2e.client_secret
  })
}

removed {
  from = local_sensitive_file.cloudflare_tunnel_secret

  lifecycle {
    destroy = false
  }
}

/*
data "cloudflare_zero_trust_tunnel_cloudflared_token" "this" {
  for_each = local.cloudflare_tunnels

  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.this[each.key].id
}

resource "local_sensitive_file" "cloudflare_tunnel_secret" {
  for_each = local.cloudflare_tunnels

  filename = "${trimsuffix(lookup(each.value, "secret_yaml_dir", path.module), "/")}/cloudflare-tunnel-${each.key}.secrets.yml"
  content = yamlencode({
    cf_tunnel_token = data.cloudflare_zero_trust_tunnel_cloudflared_token.this[each.key].token
  })
}
*/
