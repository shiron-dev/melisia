locals {
  cloudflare_access_policies = {
    bypass         = cloudflare_zero_trust_access_policy.bypass_any.id
    ca_teamj       = cloudflare_zero_trust_access_policy.ca_teamj.id
    home_ip_bypass = cloudflare_zero_trust_access_policy.home_ip_bypass.id
    shiron         = cloudflare_zero_trust_access_policy.shiron.id
    snct_email     = cloudflare_zero_trust_access_policy.snct_email.id
  }

  cloudflare_access_policy_refs = {
    n8n = [
      {
        id         = local.cloudflare_access_policies.shiron
        precedence = 2
      },
      {
        id         = local.cloudflare_access_policies.snct_email
        precedence = 3
      },
      {
        id         = local.cloudflare_access_policies.ca_teamj
        precedence = 4
      }
    ]
    shiron = [
      {
        id         = local.cloudflare_access_policies.shiron
        precedence = 2
      }
    ]
    nas_services = [
      {
        id         = local.cloudflare_access_policies.home_ip_bypass
        precedence = 1
      },
      {
        id         = local.cloudflare_access_policies.shiron
        precedence = 2
      }
    ]
    # photoframe.melisia.net 用。tunnel の Access アプリは concat([e2e(prec1)], ...)
    # で生成されるため、precedence は 2 以降を使う (e2e の 1 と衝突回避)。
    # 据置フォトフレーム設置先の home IP は bypass で SSO 不要、それ以外は shiron SSO。
    photoframe = [
      {
        id         = local.cloudflare_access_policies.home_ip_bypass
        precedence = 2
      },
      {
        id         = local.cloudflare_access_policies.shiron
        precedence = 3
      }
    ]
  }

  cloudflare_access_e2e_policy_ref = {
    name       = "Allow E2E Service Token"
    decision   = "non_identity"
    precedence = 1
    include = [
      {
        service_token = {
          token_id = cloudflare_zero_trust_access_service_token.e2e.id
        }
      }
    ]
  }

  # vm-write.shiron.dev 専用ポリシー。home-ep の vmagent が remote_write 時に
  # 使う専用 service token のみを許可する (e2e / 人間ログインは通さない)。
  cloudflare_access_vm_write_policy_ref = {
    name       = "Allow VM Write Service Token"
    decision   = "non_identity"
    precedence = 1
    include = [
      {
        service_token = {
          token_id = cloudflare_zero_trust_access_service_token.vm_write.id
        }
      }
    ]
  }

  # cloud.melisia.net (Nextcloud) の Access アプリに付与する photoframe 専用の
  # non-identity ポリシー。arm-srv の photoframe が WebDAV へ service token で
  # 到達できるようにする。precedence は既存ポリシーと衝突しない値を
  # cloud.melisia.net 取り込み時に調整すること。
  cloudflare_access_photoframe_policy_ref = {
    name       = "Allow Photoframe Service Token"
    decision   = "non_identity"
    precedence = 1
    include = [
      {
        service_token = {
          token_id = cloudflare_zero_trust_access_service_token.photoframe.id
        }
      }
    ]
  }
}

resource "cloudflare_zero_trust_access_service_token" "e2e" {
  account_id = local.cloudflare_account_id
  name       = "e2e${local.cloudflare_resource_name_suffix}"
  duration   = "8760h"
}

resource "cloudflare_zero_trust_access_service_token" "vm_write" {
  account_id = local.cloudflare_account_id
  name       = "vm-write${local.cloudflare_resource_name_suffix}"
  duration   = "8760h"
}

# photoframe (arm-srv) が TrueNAS Nextcloud の WebDAV (Cloudflare Access 保護)
# へアクセスするための専用 service token。他用途のトークンとは分離し、漏洩時の
# 影響を写真取得経路のみに限定する。client_id / client_secret は
# cloudflare_tunnel_secrets.tf で compose の secret として書き出す。
# このトークンを Nextcloud 公開ホスト名の Access アプリケーションのポリシーに
# 追加する必要がある (Nextcloud の Access アプリは本リポジトリ管理外)。
resource "cloudflare_zero_trust_access_service_token" "photoframe" {
  account_id = local.cloudflare_account_id
  name       = "photoframe${local.cloudflare_resource_name_suffix}"
  duration   = "8760h"
}

import {
  to = cloudflare_zero_trust_access_policy.ca_teamj
  id = "edc628145468437b85dc0e6d48bff3e3/421669c1-64c3-424c-b7aa-93bd37462218"
}

resource "cloudflare_zero_trust_access_policy" "ca_teamj" {
  account_id       = local.cloudflare_account_id
  name             = "ca teamj"
  decision         = "allow"
  session_duration = "24h"

  connection_rules = {
    rdp = {}
  }

  include = [
    {
      email = {
        email = "endo_taichi@cyberagent.co.jp"
      }
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
}

import {
  to = cloudflare_zero_trust_access_policy.shiron
  id = "edc628145468437b85dc0e6d48bff3e3/7af17427-a95f-44da-ad13-c0e6e74cef90"
}

resource "cloudflare_zero_trust_access_policy" "shiron" {
  account_id       = local.cloudflare_account_id
  name             = "shiron"
  decision         = "allow"
  session_duration = "24h"

  include = [
    {
      login_method = {
        id = "f74ed3ac-be7e-43db-81a6-c84909c669b3"
      }
    }
  ]

  require = [
    {
      group = {
        id = cloudflare_zero_trust_access_group.shiron.id
      }
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
}

import {
  to = cloudflare_zero_trust_access_policy.home_ip_bypass
  id = "edc628145468437b85dc0e6d48bff3e3/5042db41-17fc-4381-8e87-d6db17439e4a"
}

resource "cloudflare_zero_trust_access_policy" "home_ip_bypass" {
  account_id       = local.cloudflare_account_id
  name             = "home-ip-bypass"
  decision         = "bypass"
  session_duration = "24h"

  connection_rules = {
    rdp = {}
  }

  include = [
    {
      ip = {
        ip = "106.72.56.1/32"
      }
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
}

import {
  to = cloudflare_zero_trust_access_policy.snct_email
  id = "edc628145468437b85dc0e6d48bff3e3/675d41ec-8115-432f-9b87-345beeeb64dc"
}

resource "cloudflare_zero_trust_access_policy" "snct_email" {
  account_id       = local.cloudflare_account_id
  name             = "snct-email"
  decision         = "allow"
  session_duration = "24h"

  include = [
    {
      group = {
        id = cloudflare_zero_trust_access_group.snct_email.id
      }
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
}

import {
  to = cloudflare_zero_trust_access_policy.bypass_any
  id = "edc628145468437b85dc0e6d48bff3e3/f581fde1-d087-4973-ac95-7d7d1cbe8eef"
}

resource "cloudflare_zero_trust_access_policy" "bypass_any" {
  account_id       = local.cloudflare_account_id
  name             = "bypass any"
  decision         = "bypass"
  session_duration = "24h"

  include = [
    {
      everyone = {}
    }
  ]

  require = [
    {
      everyone = {}
    }
  ]

  lifecycle {
    create_before_destroy = true
  }
}
