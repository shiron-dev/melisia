resource "local_sensitive_file" "cloudflare_access_e2e_secret" {
  filename = "${path.module}/../../compose/hosts/arm-srv/grafana/cloudflare-access-e2e.secrets.yml"
  content = yamlencode({
    cloudflare_access_e2e_client_id = cloudflare_zero_trust_access_service_token.e2e.client_id
    # kics-scan ignore-line
    cloudflare_access_e2e_client_secret = cloudflare_zero_trust_access_service_token.e2e.client_secret
  })
}

removed {
  from = local_sensitive_file.cloudflare_tunnel_secret

  lifecycle {
    destroy = false
  }
}

/*
data "cloudflare_zero_trust_tunnel_cloudflared_token" "this" {
  for_each = local.cloudflare_tunnels

  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.this[each.key].id
}

resource "local_sensitive_file" "cloudflare_tunnel_secret" {
  for_each = local.cloudflare_tunnels

  filename = "${trimsuffix(lookup(each.value, "secret_yaml_dir", path.module), "/")}/cloudflare-tunnel-${each.key}.secrets.yml"
  content = yamlencode({
    cf_tunnel_token = data.cloudflare_zero_trust_tunnel_cloudflared_token.this[each.key].token
  })
}
*/
