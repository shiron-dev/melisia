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
  for_each = toset(nonsensitive(keys(var.slack_contact_points)))

  name               = each.key
  org_id             = "0"
  disable_provenance = true

  slack {
    color                   = var.slack_contact_points[each.key].color
    disable_resolve_message = var.slack_contact_points[each.key].disable_resolve_message
    endpoint_url            = var.slack_contact_points[each.key].endpoint_url
    icon_emoji              = var.slack_contact_points[each.key].icon_emoji
    icon_url                = var.slack_contact_points[each.key].icon_url
    mention_channel         = var.slack_contact_points[each.key].mention_channel
    mention_groups          = var.slack_contact_points[each.key].mention_groups
    mention_users           = var.slack_contact_points[each.key].mention_users
    recipient               = var.slack_contact_points[each.key].recipient
    settings                = var.slack_contact_points[each.key].settings
    text                    = var.slack_contact_points[each.key].text
    title                   = var.slack_contact_points[each.key].title
    token                   = var.slack_contact_points[each.key].token
    url                     = var.slack_contact_points[each.key].url
    username                = var.slack_contact_points[each.key].username
  }
}

resource "grafana_contact_point" "webhook" {
  for_each = toset(nonsensitive(keys(var.webhook_contact_points)))

  name               = each.key
  org_id             = "0"
  disable_provenance = true

  webhook {
    url                     = var.webhook_contact_points[each.key].url
    disable_resolve_message = var.webhook_contact_points[each.key].disable_resolve_message
    settings                = var.webhook_contact_points[each.key].settings
  }
}
