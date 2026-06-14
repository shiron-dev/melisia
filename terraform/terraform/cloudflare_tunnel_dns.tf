resource "cloudflare_dns_record" "tunnel" {
  for_each = local.cloudflare_tunnels

  zone_id = local.cloudflare_zone_ids[each.value.zone_name]
  name    = trimsuffix(each.value.domain, ".${each.value.zone_name}")
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.this[each.key].id}.cfargotunnel.com"
  ttl     = 1
  proxied = true
  comment = "Cloudflare Tunnel ${each.key}${local.cloudflare_resource_name_suffix}"
}

resource "cloudflare_dns_record" "extra_tunnel_ingress" {
  for_each = local.extra_tunnel_ingress_map

  zone_id = local.cloudflare_zone_ids[each.value.zone_name]
  name    = trimsuffix(each.value.hostname, ".${each.value.zone_name}")
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.this[each.value.tunnel_key].id}.cfargotunnel.com"
  ttl     = 1
  proxied = true
  comment = "Cloudflare Tunnel ${each.value.tunnel_key} extra ingress${local.cloudflare_resource_name_suffix}"
}
