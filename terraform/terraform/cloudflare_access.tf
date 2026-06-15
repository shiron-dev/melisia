locals {
  cloudflare_account_id = "edc628145468437b85dc0e6d48bff3e3"

  cloudflare_arm_srv_tunnel_ingress = [
    {
      hostname       = "arm-srv.shiron.dev"
      service        = "ssh://localhost"
      origin_request = {}
    },
    {
      hostname       = "netbox.shiron.dev"
      service        = "http://127.0.0.1:8000"
      origin_request = {}
    },
    {
      hostname       = "gr-slide.shiron.dev"
      service        = "http://127.0.0.1:3030"
      origin_request = {}
    },
    {
      hostname       = "portainer.shiron.dev"
      service        = "http://127.0.0.1:9000"
      origin_request = {}
    },
    {
      service = "http_status:404"
    },
  ]

  cloudflare_arm_srv_tunnel_dns_records = {
    arm_srv = "arm-srv.shiron.dev"
    netbox  = "netbox.shiron.dev"
    # gr-slide.shiron.dev exists in the tunnel config, but has no DNS record today.
    portainer = "portainer.shiron.dev"
  }
}

import {
  to = cloudflare_zero_trust_access_group.shiron
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/09b05356-05d0-4f9e-89db-b163531b01dc"
}

import {
  to = cloudflare_zero_trust_access_group.snct_email
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/4bf6790b-1710-49e8-bfec-4287af381a7b"
}

import {
  to = cloudflare_zero_trust_tunnel_cloudflared.arm_srv
  id = "edc628145468437b85dc0e6d48bff3e3/6b6ae57d-ce74-4db1-b122-63f23053ec1f"
}

import {
  to = cloudflare_zero_trust_access_service_token.github_actions_arm_srv
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/4350fda9-d790-494c-aca2-259fe39edc55"
}

import {
  to = cloudflare_zero_trust_access_application.arm_srv
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/4156d5c6-ec06-4ec4-89b0-998066f0f175"
}

import {
  to = cloudflare_zero_trust_tunnel_cloudflared_config.arm_srv
  id = "edc628145468437b85dc0e6d48bff3e3/6b6ae57d-ce74-4db1-b122-63f23053ec1f"
}

import {
  to = cloudflare_dns_record.arm_srv_tunnel["arm_srv"]
  id = format("%s/%s%s", local.cloudflare_zone_ids["shiron.dev"], "a8956b61433c2f97", "150757d3a3eacf12")
}

import {
  to = cloudflare_dns_record.arm_srv_tunnel["netbox"]
  id = format("%s/%s%s", local.cloudflare_zone_ids["shiron.dev"], "b738d5425d056188", "5c1cf4eba88a51eb")
}

import {
  to = cloudflare_dns_record.arm_srv_tunnel["portainer"]
  id = format("%s/%s%s", local.cloudflare_zone_ids["shiron.dev"], "ddef5c5fbc2635b8", "600fac3d5950474d")
}

import {
  to = cloudflare_zero_trust_access_application.arm_services
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/55492262-7a50-4dd2-8afb-4265faa5d4f1"
}

import {
  to = cloudflare_zero_trust_access_application.nas_services
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/db5ddb8e-b457-4212-83fd-4ca0129f4b24"
}

resource "cloudflare_zero_trust_access_group" "shiron" {
  account_id = local.cloudflare_account_id
  name       = "shiron"

  include = [
    {
      email = {
        email = "shiron4710@gmail.com"
      }
    }
  ]
}

resource "cloudflare_zero_trust_access_group" "snct_email" {
  account_id = local.cloudflare_account_id
  name       = "snct email"

  include = [
    {
      email_domain = {
        domain = "ed.cc.suzuka-ct.ac.jp"
      }
    }
  ]
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "arm_srv" {
  account_id = local.cloudflare_account_id
  name       = "oci-arm"
  config_src = "cloudflare"
}

resource "cloudflare_zero_trust_tunnel_cloudflared_config" "arm_srv" {
  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.arm_srv.id
  source     = "cloudflare"

  config = {
    ingress = local.cloudflare_arm_srv_tunnel_ingress
    warp_routing = {
      enabled = true
    }
  }
}

resource "cloudflare_dns_record" "arm_srv_tunnel" {
  for_each = local.cloudflare_arm_srv_tunnel_dns_records

  zone_id = local.cloudflare_zone_ids["shiron.dev"]
  name    = trimsuffix(each.value, ".shiron.dev")
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.arm_srv.id}.cfargotunnel.com"
  ttl     = 1
  proxied = true
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
            id = cloudflare_zero_trust_access_group.shiron.id
          }
        }
      ]
    },
  ]
}

resource "cloudflare_zero_trust_access_application" "arm_services" {
  account_id                 = local.cloudflare_account_id
  name                       = "Arm Services"
  domain                     = "netbox.shiron.dev"
  type                       = "self_hosted"
  session_duration           = "24h"
  auto_redirect_to_identity  = false
  app_launcher_visible       = true
  enable_binding_cookie      = false
  http_only_cookie_attribute = true
  options_preflight_bypass   = false

  destinations = [
    {
      type = "public"
      uri  = "netbox.shiron.dev"
    },
    {
      type = "public"
      uri  = "portainer.shiron.dev"
    },
  ]

  policies = [
    {
      id         = local.cloudflare_access_policies.shiron
      precedence = 1
    }
  ]
}

resource "cloudflare_zero_trust_access_application" "nas_services" {
  account_id                 = local.cloudflare_account_id
  name                       = "NAS Services"
  domain                     = "nas.shiron.dev"
  type                       = "self_hosted"
  session_duration           = "24h"
  auto_redirect_to_identity  = false
  app_launcher_visible       = true
  enable_binding_cookie      = false
  http_only_cookie_attribute = false
  options_preflight_bypass   = false

  destinations = [
    {
      type = "public"
      uri  = "nas.shiron.dev"
    },
    {
      type = "public"
      uri  = "nas-files.shiron.dev"
    },
    {
      type = "public"
      uri  = "calibre.melisia.net"
    },
  ]

  policies = local.cloudflare_access_policy_refs.nas_services
}
