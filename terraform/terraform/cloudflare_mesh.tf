locals {
  cloudflare_mesh_split_tunnel_include_routes = [
    {
      address     = "100.96.0.0/12"
      description = "Cloudflare Mesh IP range"
    },
    {
      address     = "192.168.1.0/24"
      description = "home LAN via Cloudflare Mesh"
    }
  ]

  cloudflare_mesh_nodes = {
    home_ep = {
      name            = "home-ep${local.cloudflare_resource_name_suffix}"
      secret_yaml_dir = "${path.module}/../../ansible/group_vars/home_ep"
      routes = {
        home_lan = {
          network = "192.168.1.0/24"
          comment = "home LAN via Cloudflare Mesh"
        }
      }
    }
  }

  cloudflare_mesh_routes = {
    for item in flatten([
      for node_key, node in local.cloudflare_mesh_nodes : [
        for route_key, route in lookup(node, "routes", {}) : {
          key      = "${node_key}-${route_key}"
          node_key = node_key
          network  = route.network
          comment  = route.comment
        }
      ]
    ]) : item.key => item
  }
}

data "cloudflare_zero_trust_tunnel_cloudflared_virtual_networks" "default" {
  account_id         = local.cloudflare_account_id
  is_default_network = true
}

resource "cloudflare_zero_trust_device_default_profile" "mesh" {
  account_id      = local.cloudflare_account_id
  service_mode_v2 = { mode = "warp" }
  include         = local.cloudflare_mesh_split_tunnel_include_routes
}

resource "cloudflare_zero_trust_tunnel_warp_connector" "this" {
  for_each = local.cloudflare_mesh_nodes

  account_id = local.cloudflare_account_id
  name       = each.value.name
}

resource "cloudflare_zero_trust_tunnel_cloudflared_route" "mesh" {
  for_each = local.cloudflare_mesh_routes

  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_warp_connector.this[each.value.node_key].id
  network    = each.value.network
  comment    = each.value.comment
}

resource "cloudflare_dns_record" "home_ep_mesh" {
  count = var.home_ep_mesh_ip == null ? 0 : 1

  zone_id = local.cloudflare_zone_ids["melisia.net"]
  name    = "home-ep.network"
  type    = "A"
  content = var.home_ep_mesh_ip
  ttl     = 1
  proxied = false
  comment = "Cloudflare Mesh IP for home-ep"
}

resource "cloudflare_zero_trust_access_infrastructure_target" "home_ep" {
  count = var.home_ep_mesh_ip == null ? 0 : 1

  account_id = local.cloudflare_account_id
  hostname   = "home-ep.network.melisia.net"
  ip = {
    ipv4 = {
      ip_addr            = var.home_ep_mesh_ip
      virtual_network_id = data.cloudflare_zero_trust_tunnel_cloudflared_virtual_networks.default.result[0].id
    }
  }
}

resource "cloudflare_zero_trust_access_application" "home_ep_infrastructure_ssh" {
  count = var.home_ep_mesh_ip == null ? 0 : 1

  account_id = local.cloudflare_account_id
  name       = "home-ep infrastructure ssh${local.cloudflare_resource_name_suffix}"
  type       = "infrastructure"

  target_criteria = [
    {
      port     = 22
      protocol = "SSH"
      target_attributes = {
        hostname = [cloudflare_zero_trust_access_infrastructure_target.home_ep[0].hostname]
      }
    }
  ]

  policies = [
    {
      name       = "SSH Policy"
      decision   = "allow"
      precedence = 1
      include = [
        {
          group = {
            id = local.cloudflare_access_policies.shiron
          }
        }
      ]
      connection_rules = {
        ssh = {
          usernames = ["ansible_user"]
        }
      }
    }
  ]
}

data "cloudflare_zero_trust_tunnel_warp_connector_token" "this" {
  for_each = local.cloudflare_mesh_nodes

  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_warp_connector.this[each.key].id
}

resource "local_sensitive_file" "cloudflare_mesh_secret" {
  for_each = local.cloudflare_mesh_nodes

  filename = "${trimsuffix(lookup(each.value, "secret_yaml_dir", path.module), "/")}/cloudflare-mesh.secrets.yml"
  content = yamlencode({
    cloudflare_mesh_connector_token = data.cloudflare_zero_trust_tunnel_warp_connector_token.this[each.key].token
  })
}
