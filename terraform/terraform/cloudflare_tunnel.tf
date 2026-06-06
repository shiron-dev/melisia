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
