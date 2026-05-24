resource "grafana_rule_group" "managed" {
  for_each = var.rule_groups

  name             = each.key
  folder_uid       = grafana_folder.managed[each.value.folder_key].uid
  interval_seconds = each.value.interval_seconds

  dynamic "rule" {
    for_each = each.value.rules

    content {
      name           = rule.value.name
      condition      = rule.value.condition
      for            = rule.value.for
      no_data_state  = rule.value.no_data_state
      exec_err_state = rule.value.exec_err_state
      annotations    = rule.value.annotations
      labels         = rule.value.labels
      is_paused      = rule.value.is_paused

      dynamic "data" {
        for_each = rule.value.data

        content {
          ref_id         = data.value.ref_id
          query_type     = data.value.query_type
          datasource_uid = data.value.datasource_uid
          model          = data.value.model

          relative_time_range {
            from = data.value.from
            to   = data.value.to
          }
        }
      }
    }
  }
}
