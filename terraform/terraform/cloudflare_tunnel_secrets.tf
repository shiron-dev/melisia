resource "local_sensitive_file" "cloudflare_access_e2e_secret" {
  filename = "${path.module}/../../compose/hosts/arm-srv/grafana/cloudflare-access-e2e.secrets.yml"
  content = yamlencode({
    cloudflare_access_e2e_client_id = cloudflare_zero_trust_access_service_token.e2e.client_id
    # kics-scan ignore-line
    cloudflare_access_e2e_client_secret = cloudflare_zero_trust_access_service_token.e2e.client_secret
  })
}

# home-ep の vmagent が vm-write.shiron.dev (Access 保護) へ remote_write する際の
# CF-Access service token。書き込み経路専用の vm_write token を配布し、e2e token は
# 共有しない (漏洩時の影響を書き込みパスのみに限定する)。
resource "local_sensitive_file" "cloudflare_vm_write_secret_home_ep" {
  filename = "${path.module}/../../compose/hosts/home-ep/network-monitor/vm-write.secrets.yml"
  content = yamlencode({
    cloudflare_vm_write_client_id = cloudflare_zero_trust_access_service_token.vm_write.client_id
    # kics-scan ignore-line
    cloudflare_vm_write_client_secret = cloudflare_zero_trust_access_service_token.vm_write.client_secret
  })
}

# photoframe (arm-srv) が Nextcloud WebDAV へ CF-Access service token で
# アクセスするための client_id / client_secret。compose の template 変数として
# 参照される (photoframe_access_client_id / photoframe_access_client_secret)。
resource "local_sensitive_file" "cloudflare_photoframe_access_secret" {
  filename = "${path.module}/../../compose/hosts/arm-srv/photoframe/cloudflare-access-photoframe.secrets.yml"
  content = yamlencode({
    photoframe_access_client_id = cloudflare_zero_trust_access_service_token.photoframe.client_id
    # kics-scan ignore-line
    photoframe_access_client_secret = cloudflare_zero_trust_access_service_token.photoframe.client_secret
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
