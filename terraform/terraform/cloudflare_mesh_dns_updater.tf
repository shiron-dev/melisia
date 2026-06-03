/*
locals {
  cloudflare_access_team_domain = "shiron-dev.cloudflareaccess.com"

  cloudflare_mesh_dns_updater = {
    worker_name = "mesh-dns-updater"
    hostname    = "mesh-dns-updater.melisia.net"
    zone_name   = "melisia.net"

    records = {
      home_ep = {
        hostname        = "home-ep.network.melisia.net"
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

  # The permission group API requires API Tokens Read, so keep the stable ID
  # inline to avoid widening the bootstrap Terraform token.
  cloudflare_api_token_permission_groups = {
    dns_write = "4755a26eedb94da69e1066d98aa820be"
  }
}

resource "cloudflare_api_token" "mesh_dns_updater_worker" {
  name = "mesh-dns-updater-worker${local.cloudflare_resource_name_suffix}"

  policies = [
    {
      effect = "allow"
      permission_groups = [
        {
          id = local.cloudflare_api_token_permission_groups.dns_write
        }
      ]
      # Provider 5.19.1 expects this policy field as a JSON object string.
      resources = jsonencode({
        for zone_name, zone_id in local.cloudflare_zone_ids :
        "com.cloudflare.api.account.zone.${zone_id}" => "*"
        if contains(distinct([for record in local.cloudflare_mesh_dns_updater_records : record.zone_name]), zone_name)
      })
    }
  ]
}

resource "cloudflare_dns_record" "mesh_dynamic" {
  for_each = local.cloudflare_mesh_dns_updater_records

  zone_id = each.value.zone_id
  name    = each.value.record_name
  type    = "A"
  content = "100.96.0.1"
  ttl     = 1
  proxied = false
  comment = "Cloudflare Mesh IP managed by mesh-dns-updater"

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
      text = cloudflare_api_token.mesh_dns_updater_worker.value
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

resource "local_sensitive_file" "mesh_dns_updater_secret" {
  for_each = local.cloudflare_mesh_dns_updater_records

  filename = "${trimsuffix(each.value.secret_yaml_dir, "/")}/cloudflare-mesh-dns-updater.secrets.yml"
  content = yamlencode({
    cloudflare_mesh_dns_updater_endpoint             = "https://${local.cloudflare_mesh_dns_updater.hostname}/update"
    cloudflare_mesh_dns_updater_access_client_id     = cloudflare_zero_trust_access_service_token.mesh_dns_updater[each.key].client_id
    cloudflare_mesh_dns_updater_access_client_secret = cloudflare_zero_trust_access_service_token.mesh_dns_updater[each.key].client_secret
  })
}
*/
