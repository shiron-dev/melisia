locals {
  cloudflare_tunnels = {
    "arm-srv-grafana" = {
      domain          = "grafana.shiron.dev"
      zone_name       = "shiron.dev"
      service         = "http://grafana:3000"
      secret_yaml_dir = "${path.module}/../../compose/hosts/arm-srv/grafana"
      policies        = local.cloudflare_access_policy_refs.shiron
      extra_ingress = [
        # influxdb は退役済み (メトリクス永続化は Mimir に一本化)。
        # コンテナ削除に伴い公開 tunnel ingress と e2e probe 対象も撤去した。
        # home-ep の vmagent が remote_write (push) する書き込みエンドポイント。
        # vmauth が /api/v1/push を Mimir へ、/loki/api/v1/push を Loki へ転送し、
        # それ以外のパスは拒否する。Access は専用 service token
        # (vm_write) のみ許可し、共通 e2e ポリシーや人間ログインは通さない
        # (skip_e2e_policy)。
        {
          hostname        = "vm-write.shiron.dev"
          zone_name       = "shiron.dev"
          service         = "http://vmauth:8427"
          policies        = [local.cloudflare_access_vm_write_policy_ref]
          skip_e2e_policy = true
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
    # photoframe.melisia.net は arm-srv 上の軽量スライドショー (Go) を公開する。
    # 画像ソースは TrueNAS Nextcloud の WebDAV で、photoframe コンテナが
    # 専用 CF Access service token (photoframe) を使って取得する。
    # UI 自体は shiron ポリシーで Access 保護する (私的写真の公開を避ける)。
    "arm-srv-photoframe" = {
      domain          = "photoframe.melisia.net"
      zone_name       = "melisia.net"
      service         = "http://photoframe:8080"
      secret_yaml_dir = "${path.module}/../../compose/hosts/arm-srv/photoframe"
      policies        = local.cloudflare_access_policy_refs.photoframe
    }
    "home-ep-homeassistant" = {
      domain                                         = "home.melisia.net"
      zone_name                                      = "melisia.net"
      service                                        = "http://homeassistant:8123"
      secret_yaml_dir                                = "${path.module}/../../compose/hosts/home-ep/home-assistant"
      policies                                       = []
      dangerously_allow_public_without_access_policy = true
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
        # home-ep の exporter (node / blackbox / cloudflare-speedtest) は
        # vmagent によるローカルスクレイプ + arm-srv VM への push に移行したため、
        # 外部公開する tunnel ingress は廃止した。
      ]
    }
  }

  extra_tunnel_ingress_map = {
    for item in flatten([
      for tunnel_key, tunnel in local.cloudflare_tunnels : [
        for ingress in lookup(tunnel, "extra_ingress", []) : {
          tunnel_key                = tunnel_key
          hostname                  = ingress.hostname
          zone_name                 = ingress.zone_name
          service                   = ingress.service
          policies                  = lookup(ingress, "policies", [])
          skip_e2e_policy           = lookup(ingress, "skip_e2e_policy", false)
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
