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
