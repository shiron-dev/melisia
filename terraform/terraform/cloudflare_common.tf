locals {
  cloudflare_resource_name_suffix                                   = " - Melisia Terraform"
  cloudflare_zone_name                                              = "shiron.dev"
  cloudflare_default_dangerously_allow_public_without_access_policy = false

  cloudflare_zone_ids = {
    "shiron.dev"  = data.cloudflare_zone.this.zone_id
    "melisia.net" = data.cloudflare_zone.melisia_net.zone_id
  }
}

data "cloudflare_zone" "this" {
  filter = {
    name = local.cloudflare_zone_name
  }
}

data "cloudflare_zone" "melisia_net" {
  filter = {
    name = "melisia.net"
  }
}
