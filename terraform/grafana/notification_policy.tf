resource "grafana_notification_policy" "root" {
  count = var.notification_policy == null ? 0 : 1

  contact_point      = var.notification_policy.contact_point
  group_by           = var.notification_policy.group_by
  group_wait         = var.notification_policy.group_wait
  group_interval     = var.notification_policy.group_interval
  repeat_interval    = var.notification_policy.repeat_interval
  org_id             = "0"
  disable_provenance = true

  depends_on = [
    grafana_contact_point.email,
    grafana_contact_point.slack,
    grafana_contact_point.webhook,
  ]
}
