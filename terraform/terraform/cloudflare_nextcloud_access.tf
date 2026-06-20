# TrueNAS Nextcloud (Access アプリ "NextCloud Services", 保護ホスト
# cloud.shiron.org) の Cloudflare Access アプリを Terraform 管理下に取り込み、
# photoframe (arm-srv) が WebDAV へ到達するための専用 service token ポリシー
# (auth token) をコード化する。
#
# resource の値は `terraform plan -generate-config-out` で取得した live state に
# 一致させてあり、policies は既存 3 件を維持したまま photoframe を precedence 4 で
# 追加するのみ。apply 後は import ブロックを削除してよい。

import {
  to = cloudflare_zero_trust_access_application.nextcloud
  id = "accounts/edc628145468437b85dc0e6d48bff3e3/ba70e666-46bd-4391-8b0a-3c7a5f15a23b"
}

# 構成値は terraform plan -generate-config-out で取得した live state に一致させてある
# (アプリ名は "NextCloud Services"、保護ホストは cloud.shiron.org)。
resource "cloudflare_zero_trust_access_application" "nextcloud" {
  account_id                 = local.cloudflare_account_id
  name                       = "NextCloud Services"
  domain                     = "cloud.shiron.org"
  type                       = "self_hosted"
  session_duration           = "24h"
  auto_redirect_to_identity  = false
  app_launcher_visible       = true
  enable_binding_cookie      = false
  http_only_cookie_attribute = false
  options_preflight_bypass   = false

  destinations = [
    {
      type = "public"
      uri  = "cloud.shiron.org"
    },
  ]

  policies = [
    # --- 既存ポリシー (import 時の live state を維持) ---
    # 本リポジトリ管理外のポリシー (precedence 1)。
    {
      id         = "9c94d1e5-8172-4284-874f-ff5806da24cd"
      precedence = 1
    },
    {
      id         = local.cloudflare_access_policies.home_ip_bypass
      precedence = 2
    },
    {
      id         = local.cloudflare_access_policies.shiron
      precedence = 3
    },
    # --- 追加: photoframe service token (auth token) ---
    # arm-srv の photoframe が WebDAV へ non-identity (CF-Access-Client-Id/Secret)
    # で到達するための専用トークン。既存ポリシーと衝突しない precedence 4 を付与。
    merge(local.cloudflare_access_photoframe_policy_ref, { precedence = 4 }),
  ]
}
