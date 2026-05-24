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
      tunnel_id       = "23c35bdd-2d9d-4540-9639-b25af6338433"
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

resource "cloudflare_zero_trust_device_default_profile" "mesh" {
  account_id            = local.cloudflare_account_id
  lan_allow_minutes     = 0
  lan_allow_subnet_size = 24
  service_mode_v2       = { mode = "warp" }
  include               = local.cloudflare_mesh_split_tunnel_include_routes
}

removed {
  from = cloudflare_zero_trust_tunnel_warp_connector.this

  lifecycle {
    destroy = false
  }
}

data "cloudflare_zero_trust_tunnel_warp_connector" "this" {
  for_each = local.cloudflare_mesh_nodes

  # The former cloudflare_zero_trust_tunnel_warp_connector.this resource was
  # removed from state after apply. If applying from an older state snapshot,
  # run: terraform state rm 'cloudflare_zero_trust_tunnel_warp_connector.this["home_ep"]'
  # before planning this change so Terraform does not destroy the connector.
  account_id = local.cloudflare_account_id
  tunnel_id  = each.value.tunnel_id
}

resource "cloudflare_zero_trust_tunnel_cloudflared_route" "mesh" {
  for_each = local.cloudflare_mesh_routes

  account_id = local.cloudflare_account_id
  tunnel_id  = data.cloudflare_zero_trust_tunnel_warp_connector.this[each.value.node_key].id
  network    = each.value.network
  comment    = each.value.comment
}

data "cloudflare_zero_trust_tunnel_warp_connector_token" "this" {
  for_each = local.cloudflare_mesh_nodes

  account_id = local.cloudflare_account_id
  tunnel_id  = data.cloudflare_zero_trust_tunnel_warp_connector.this[each.key].id
}

resource "local_sensitive_file" "cloudflare_mesh_secret" {
  for_each = local.cloudflare_mesh_nodes

  filename = "${trimsuffix(lookup(each.value, "secret_yaml_dir", path.module), "/")}/cloudflare-mesh.secrets.yml"
  content = yamlencode({
    cloudflare_mesh_connector_token = data.cloudflare_zero_trust_tunnel_warp_connector_token.this[each.key].token
  })
}
