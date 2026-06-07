resource "grafana_folder" "infrastructure" {
  title = "Infrastructure"
  uid   = "infrastructure"
}

resource "grafana_folder" "managed" {
  for_each = var.folders

  title = each.value.title
  uid   = coalesce(each.value.uid, each.key)
}
