resource "grafana_contact_point" "email" {
  for_each = var.email_contact_points

  name               = each.key
  org_id             = "0"
  disable_provenance = true

  email {
    addresses               = each.value.addresses
    subject                 = each.value.subject
    message                 = each.value.message
    single_email            = each.value.single_email
    disable_resolve_message = each.value.disable_resolve_message
  }
}

resource "grafana_contact_point" "slack" {
  for_each = var.slack_contact_points

  name               = each.key
  org_id             = "0"
  disable_provenance = true

  slack {
    color                   = each.value.color
    disable_resolve_message = each.value.disable_resolve_message
    endpoint_url            = each.value.endpoint_url
    icon_emoji              = each.value.icon_emoji
    icon_url                = each.value.icon_url
    mention_channel         = each.value.mention_channel
    mention_groups          = each.value.mention_groups
    mention_users           = each.value.mention_users
    recipient               = each.value.recipient
    settings                = each.value.settings
    text                    = each.value.text
    title                   = each.value.title
    token                   = each.value.token
    url                     = each.value.url
    username                = each.value.username
  }
}

resource "grafana_contact_point" "webhook" {
  for_each = var.webhook_contact_points

  name               = each.key
  org_id             = "0"
  disable_provenance = true

  webhook {
    url                     = each.value.url
    disable_resolve_message = each.value.disable_resolve_message
    settings                = each.value.settings
  }
}
