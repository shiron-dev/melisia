# home-nas は TrueNAS (storage-srv) 上で動く cloudflared アプリのトンネル。
# トークンは terraform/truenas ルートの truenas_cloudflared_tunnel_token (sops) で
# 配布しているため、ここではトンネル本体・ingress 設定・DNS のみを import して管理する
# (トークン/シークレットは再生成しない)。oci-arm (arm_srv) と同じ運用パターン。
locals {
  # TrueNAS cloudflared が接続している既存トンネル ID。
  home_nas_tunnel_id = "6d6847fe-06ba-415d-a4bf-b697b24dc27c"

  # terraform で DNS レコードまで管理するホスト名。cloud.shiron.org は
  # shiron.org ゾーン (cloudflare_zone_ids 未定義) のため ingress には残すが
  # DNS レコードは管理対象外とする。
  home_nas_tunnel_dns_records = {
    nas       = { hostname = "nas.shiron.dev", zone = "shiron.dev" }
    nas_srv   = { hostname = "nas-srv.shiron.dev", zone = "shiron.dev" }
    nas_files = { hostname = "nas-files.shiron.dev", zone = "shiron.dev" }
    calibre   = { hostname = "calibre.melisia.net", zone = "melisia.net" }
    vault     = { hostname = "vault.melisia.net", zone = "melisia.net" }
  }
}

import {
  to = cloudflare_zero_trust_tunnel_cloudflared.home_nas
  id = "${local.cloudflare_account_id}/${local.home_nas_tunnel_id}"
}

import {
  to = cloudflare_zero_trust_tunnel_cloudflared_config.home_nas
  id = "${local.cloudflare_account_id}/${local.home_nas_tunnel_id}"
}

import {
  to = cloudflare_dns_record.home_nas_tunnel["nas"]
  id = "${local.cloudflare_zone_ids["shiron.dev"]}/bde901ebc7f3b00881f5c6a81643787a"
}

import {
  to = cloudflare_dns_record.home_nas_tunnel["nas_srv"]
  id = "${local.cloudflare_zone_ids["shiron.dev"]}/4aa0d6ed0642cad38904dc1d5d3bbf66"
}

import {
  to = cloudflare_dns_record.home_nas_tunnel["nas_files"]
  id = "${local.cloudflare_zone_ids["shiron.dev"]}/0ea74de9c2de24fc5e82dc197158b0ff"
}

import {
  to = cloudflare_dns_record.home_nas_tunnel["calibre"]
  id = "${local.cloudflare_zone_ids["melisia.net"]}/87844ef8e57e00dd526ee56fda0a709c"
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "home_nas" {
  account_id = local.cloudflare_account_id
  name       = "home-nas"
  config_src = "cloudflare"
}

resource "cloudflare_zero_trust_tunnel_cloudflared_config" "home_nas" {
  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.home_nas.id
  source     = "cloudflare"

  config = {
    ingress = [
      {
        hostname = "nas.shiron.dev"
        service  = "http://127.0.0.1"
        origin_request = {
          access = {
            aud_tag   = [cloudflare_zero_trust_access_application.nas_services.aud]
            required  = true
            team_name = "shiron-dev"
          }
        }
      },
      {
        hostname = "nas-srv.shiron.dev"
        service  = "ssh://127.0.0.1:22"
        origin_request = {
          access = {
            aud_tag   = ["311c6affac6866b9e96e7c2c9b8c706104080f89b62c16188a5b371bbb706c1d"]
            required  = true
            team_name = "shiron-dev"
          }
        }
      },
      {
        hostname = "cloud.shiron.org"
        service  = "http://127.0.0.1:30027"
        origin_request = {
          access = {
            aud_tag   = []
            required  = false
            team_name = "shiron-dev"
          }
        }
      },
      {
        hostname = "nas-files.shiron.dev"
        service  = "http://127.0.0.1:30051"
        origin_request = {
          access = {
            aud_tag   = [cloudflare_zero_trust_access_application.nas_services.aud]
            required  = true
            team_name = "shiron-dev"
          }
        }
      },
      {
        hostname       = "calibre.melisia.net"
        service        = "http://127.0.0.1:32015"
        origin_request = {}
      },
      # vault.melisia.net は TrueNAS 上のサービス (port 30032)。Access 保護は
      # nas_services アプリ (home-ip-bypass + shiron) で行うため、ingress 側は
      # calibre と同じく origin_request を空にする。
      {
        hostname       = "vault.melisia.net"
        service        = "http://127.0.0.1:30032"
        origin_request = {}
      },
      {
        service = "http_status:404"
      },
    ]
    warp_routing = {
      enabled = false
    }
  }
}

resource "cloudflare_dns_record" "home_nas_tunnel" {
  for_each = local.home_nas_tunnel_dns_records

  zone_id = local.cloudflare_zone_ids[each.value.zone]
  name    = trimsuffix(each.value.hostname, ".${each.value.zone}")
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.home_nas.id}.cfargotunnel.com"
  ttl     = 1
  proxied = true
  comment = "Cloudflare Tunnel home-nas"
}
