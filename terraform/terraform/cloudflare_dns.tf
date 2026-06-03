locals {
  cloudflare_public_lan_dns_records = {
    storage_srv = {
      hostname = "storage-srv.network.melisia.net"
      zone     = "melisia.net"
      content  = "192.168.1.64"
      comment  = "storage-srv SSH endpoint via home LAN"
    }
  }
}

resource "cloudflare_dns_record" "public_lan" {
  for_each = local.cloudflare_public_lan_dns_records

  zone_id = local.cloudflare_zone_ids[each.value.zone]
  name    = trimsuffix(each.value.hostname, ".${each.value.zone}")
  type    = "A"
  content = each.value.content
  ttl     = 1
  proxied = false
  comment = each.value.comment
}
