locals {
  cloudflare_tunnels = {
    "arm-srv-grafana" = {
      domain          = "grafana.shiron.dev"
      zone_name       = "shiron.dev"
      service         = "http://grafana:3000"
      secret_yaml_dir = "${path.module}/../../compose/hosts/arm-srv/grafana"
      policies        = local.cloudflare_access_policy_refs.shiron
      extra_ingress = [
        {
          hostname  = "influxdb.shiron.dev"
          zone_name = "shiron.dev"
          service   = "http://influxdb:8086"
          policies  = local.cloudflare_access_policy_refs.shiron
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
    "home-ep-homeassistant" = {
      domain          = "home.melisia.net"
      zone_name       = "melisia.net"
      service         = "http://homeassistant:8123"
      secret_yaml_dir = "${path.module}/../../compose/hosts/home-ep/home-assistant"
      policies        = concat(local.cloudflare_access_policy_refs.shiron, [local.cloudflare_home_login_policy_ref])
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
