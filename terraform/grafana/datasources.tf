resource "grafana_data_source" "managed" {
  for_each = var.datasources

  name        = each.value.name
  type        = each.value.type
  uid         = coalesce(each.value.uid, each.key)
  url         = each.value.url
  access_mode = each.value.access_mode
  is_default  = each.value.is_default

  json_data_encoded        = each.value.json_data_encoded
  secure_json_data_encoded = each.value.secure_json_data_encoded
}
