resource "grafana_oncall_escalation_chain" "primary" {
  provider = grafana.oncall

  name = var.primary_escalation_chain_name
}

resource "grafana_oncall_integration" "selfhost_grafana" {
  provider = grafana.oncall

  name = var.selfhost_grafana_integration_name
  type = var.selfhost_grafana_integration_type

  default_route {
    escalation_chain_id = grafana_oncall_escalation_chain.primary.id
  }
}

resource "grafana_oncall_route" "selfhost_grafana" {
  for_each = {
    for route in var.routes : route.position => route
  }

  provider = grafana.oncall

  integration_id      = grafana_oncall_integration.selfhost_grafana.id
  escalation_chain_id = grafana_oncall_escalation_chain.primary.id
  routing_regex       = each.value.routing_regex
  position            = each.value.position
}
