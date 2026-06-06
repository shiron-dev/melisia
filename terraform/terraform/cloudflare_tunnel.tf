locals {
  cloudflare_resource_name_suffix                                   = " - Melisia Terraform"
  cloudflare_zone_name                                              = "shiron.dev"
  cloudflare_default_dangerously_allow_public_without_access_policy = false

  cloudflare_access_policies = {
    bypass     = cloudflare_zero_trust_access_policy.bypass_any.id
    ca_teamj   = cloudflare_zero_trust_access_policy.ca_teamj.id
    shiron     = cloudflare_zero_trust_access_policy.shiron.id
    snct_email = cloudflare_zero_trust_access_policy.snct_email.id
  }

  cloudflare_home_login_policy_ref = {
    name       = "home login"
    decision   = "allow"
    precedence = 3
    include = [
      {
        group = {
          id = cloudflare_zero_trust_access_group.shiron.id
        }
      }
    ]
  }

  cloudflare_access_policy_refs = {
    n8n = [
      {
        id         = local.cloudflare_access_policies.shiron
        precedence = 2
      },
      {
        id         = local.cloudflare_access_policies.snct_email
        precedence = 3
      },
      {
        id         = local.cloudflare_access_policies.ca_teamj
        precedence = 4
      }
    ]
    shiron = [
      {
        id         = local.cloudflare_access_policies.shiron
        precedence = 2
      }
    ]
  }

  cloudflare_tunnels = {
    "arm-srv-grafana" = {
      domain          = "grafana.shiron.dev"
      zone_name       = "shiron.dev"
      service         = "http://grafana:3000"
      secret_yaml_dir = "${path.module}/../../compose/hosts/arm-srv/grafana"
      policies        = local.cloudflare_access_policy_refs.shiron
      extra_ingress = [
        {
          hostname  = "influxdb.shiron.dev"
          zone_name = "shiron.dev"
          service   = "http://influxdb:8086"
          policies  = local.cloudflare_access_policy_refs.shiron
        }
      ]
    }
    "arm-srv-n8n" = {
      domain                    = "n8n.shiron.dev"
      zone_name                 = "shiron.dev"
      service                   = "http://n8n:5678"
      secret_yaml_dir           = "${path.module}/../../compose/hosts/arm-srv/n8n"
      policies                  = local.cloudflare_access_policy_refs.shiron
      manage_access_application = false
    }
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
      policies        = concat(local.cloudflare_access_policy_refs.shiron, [local.cloudflare_home_login_policy_ref])
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

import {
  to = cloudflare_zero_trust_access_policy.ca_teamj
  id = "edc628145468437b85dc0e6d48bff3e3/421669c1-64c3-424c-b7aa-93bd37462218"
}

resource "cloudflare_zero_trust_access_policy" "ca_teamj" {
  account_id       = local.cloudflare_account_id
  name             = "ca teamj"
  decision         = "allow"
  session_duration = "24h"

  connection_rules = {
    rdp = {}
  }

  include = [
    {
      email = {
        email = "endo_taichi@cyberagent.co.jp"
      }
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
}

import {
  to = cloudflare_zero_trust_access_policy.shiron
  id = "edc628145468437b85dc0e6d48bff3e3/7af17427-a95f-44da-ad13-c0e6e74cef90"
}

resource "cloudflare_zero_trust_access_policy" "shiron" {
  account_id       = local.cloudflare_account_id
  name             = "shiron"
  decision         = "allow"
  session_duration = "24h"

  include = [
    {
      login_method = {
        id = "f74ed3ac-be7e-43db-81a6-c84909c669b3"
      }
    }
  ]

  require = [
    {
      group = {
        id = cloudflare_zero_trust_access_group.shiron.id
      }
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
}

import {
  to = cloudflare_zero_trust_access_policy.snct_email
  id = "edc628145468437b85dc0e6d48bff3e3/675d41ec-8115-432f-9b87-345beeeb64dc"
}

resource "cloudflare_zero_trust_access_policy" "snct_email" {
  account_id       = local.cloudflare_account_id
  name             = "snct-email"
  decision         = "allow"
  session_duration = "24h"

  include = [
    {
      group = {
        id = cloudflare_zero_trust_access_group.snct_email.id
      }
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
}

import {
  to = cloudflare_zero_trust_access_policy.bypass_any
  id = "edc628145468437b85dc0e6d48bff3e3/f581fde1-d087-4973-ac95-7d7d1cbe8eef"
}

resource "cloudflare_zero_trust_access_policy" "bypass_any" {
  account_id       = local.cloudflare_account_id
  name             = "bypass any"
  decision         = "bypass"
  session_duration = "24h"

  include = [
    {
      everyone = {}
    }
  ]

  require = [
    {
      everyone = {}
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
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
          tunnel_key                = tunnel_key
          hostname                  = ingress.hostname
          zone_name                 = ingress.zone_name
          service                   = ingress.service
          policies                  = lookup(ingress, "policies", [])
          manage_access_application = lookup(ingress, "manage_access_application", true)
          dangerously_allow_public_without_access_policy = lookup(
            ingress,
            "dangerously_allow_public_without_access_policy",
            local.cloudflare_default_dangerously_allow_public_without_access_policy,
          )
        }
      ]
    ]) : "${item.tunnel_key}-${item.hostname}" => item
  }

  cloudflare_tunnel_domains_without_access_policy = concat(
    [
      for tunnel in local.cloudflare_tunnels : tunnel.domain
      if length(lookup(tunnel, "policies", [])) == 0
      && !lookup(
        tunnel,
        "dangerously_allow_public_without_access_policy",
        local.cloudflare_default_dangerously_allow_public_without_access_policy,
      )
    ],
    [
      for ingress in local.extra_tunnel_ingress_map : ingress.hostname
      if length(ingress.policies) == 0
      && !ingress.dangerously_allow_public_without_access_policy
    ],
  )
}

resource "terraform_data" "cloudflare_tunnel_access_policy_guard" {
  input = local.cloudflare_tunnel_domains_without_access_policy

  lifecycle {
    precondition {
      condition     = length(local.cloudflare_tunnel_domains_without_access_policy) == 0
      error_message = "Cloudflare tunnel ingress without Access policy is dangerous. Add policies or set dangerously_allow_public_without_access_policy = true explicitly for: ${join(", ", local.cloudflare_tunnel_domains_without_access_policy)}"
    }
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

  lifecycle {
    ignore_changes = [tunnel_secret]
  }
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
