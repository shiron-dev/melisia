locals {
  cloudflare_access_policies = {
    bypass     = cloudflare_zero_trust_access_policy.bypass_any.id
    ca_teamj   = cloudflare_zero_trust_access_policy.ca_teamj.id
    shiron     = cloudflare_zero_trust_access_policy.shiron.id
    snct_email = cloudflare_zero_trust_access_policy.snct_email.id
  }

  cloudflare_home_login_policy_ref = {
    name       = "home login"
    decision   = "allow"
    precedence = 3
    include = [
      {
        group = {
          id = cloudflare_zero_trust_access_group.shiron.id
        }
      }
    ]
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
}

resource "cloudflare_zero_trust_access_service_token" "e2e" {
  account_id = local.cloudflare_account_id
  name       = "e2e${local.cloudflare_resource_name_suffix}"
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
