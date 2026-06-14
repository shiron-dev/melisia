locals {
  cloudflare_access_team_domain = "shiron-dev.cloudflareaccess.com"

  cloudflare_mesh_dns_updater = {
    worker_name = "mesh-dns-updater"
    hostname    = "mesh-dns-updater.melisia.net"
    zone_name   = "melisia.net"

    records = {
      home_ep = {
        hostname        = "home-ep.mesh.network.melisia.net"
        zone_name       = "melisia.net"
        secret_yaml_dir = "${path.module}/../../ansible/group_vars/home_ep"
      }
    }
  }

  cloudflare_mesh_dns_updater_records = {
    for key, record in local.cloudflare_mesh_dns_updater.records : key => merge(record, {
      record_name = trimsuffix(record.hostname, ".${record.zone_name}")
      zone_id     = local.cloudflare_zone_ids[record.zone_name]
    })
  }

  cloudflare_mesh_dns_updater_worker_records = {
    for key, record in local.cloudflare_mesh_dns_updater_records :
    cloudflare_zero_trust_access_service_token.mesh_dns_updater[key].client_id => {
      hostname  = record.hostname
      zone_id   = record.zone_id
      record_id = cloudflare_dns_record.mesh_dynamic[key].id
    }
  }

}

resource "cloudflare_dns_record" "mesh_dynamic" {
  for_each = local.cloudflare_mesh_dns_updater_records

  zone_id = each.value.zone_id
  name    = each.value.record_name
  type    = "A"
  content = "100.96.0.1"
  ttl     = 1
  proxied = false
  comment = "Cloudflare Mesh IP managed by mesh-dns-updater${local.cloudflare_resource_name_suffix}"

  lifecycle {
    ignore_changes = [content]
  }
}

resource "cloudflare_zero_trust_access_service_token" "mesh_dns_updater" {
  for_each = local.cloudflare_mesh_dns_updater_records

  account_id = local.cloudflare_account_id
  name       = "${each.key}-mesh-dns-updater${local.cloudflare_resource_name_suffix}"
  duration   = "8760h"
}

resource "cloudflare_workers_script" "mesh_dns_updater" {
  account_id         = local.cloudflare_account_id
  script_name        = local.cloudflare_mesh_dns_updater.worker_name
  main_module        = "mesh_dns_updater.js"
  content_file       = "${path.module}/workers/mesh_dns_updater.js"
  content_sha256     = filesha256("${path.module}/workers/mesh_dns_updater.js")
  compatibility_date = "2026-05-24"

  bindings = [
    {
      name = "ACCESS_TEAM_DOMAIN"
      type = "plain_text"
      text = local.cloudflare_access_team_domain
    },
    {
      name = "ACCESS_AUD"
      type = "plain_text"
      text = cloudflare_zero_trust_access_application.mesh_dns_updater.aud
    },
    {
      name = "CLOUDFLARE_API_TOKEN"
      type = "secret_text"
      text = var.cloudflare_mesh_dns_updater_api_token
    },
    {
      name = "RECORDS"
      type = "plain_text"
      text = jsonencode(local.cloudflare_mesh_dns_updater_worker_records)
    },
  ]
}

resource "cloudflare_workers_custom_domain" "mesh_dns_updater" {
  account_id = local.cloudflare_account_id
  zone_id    = local.cloudflare_zone_ids[local.cloudflare_mesh_dns_updater.zone_name]
  hostname   = local.cloudflare_mesh_dns_updater.hostname
  service    = cloudflare_workers_script.mesh_dns_updater.script_name
}

resource "cloudflare_zero_trust_access_application" "mesh_dns_updater" {
  account_id                = local.cloudflare_account_id
  name                      = "${local.cloudflare_mesh_dns_updater.worker_name}${local.cloudflare_resource_name_suffix}"
  domain                    = local.cloudflare_mesh_dns_updater.hostname
  type                      = "self_hosted"
  session_duration          = "24h"
  service_auth_401_redirect = false
  auto_redirect_to_identity = false

  policies = [
    {
      name       = "Allow Mesh DNS Updater Service Tokens"
      decision   = "non_identity"
      precedence = 1
      include = [
        for token in cloudflare_zero_trust_access_service_token.mesh_dns_updater : {
          service_token = {
            token_id = token.id
          }
        }
      ]
    }
  ]
}

removed {
  from = local_sensitive_file.mesh_dns_updater_secret

  lifecycle {
    destroy = false
  }
}
