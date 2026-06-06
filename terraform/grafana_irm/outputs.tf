output "selfhost_grafana_irm_webhook_url" {
  description = "Webhook URL for sending self-hosted Grafana Alerting notifications to Grafana Cloud IRM."
  value       = grafana_oncall_integration.selfhost_grafana.link
  sensitive   = true
}

output "selfhost_grafana_irm_integration_id" {
  description = "Grafana IRM integration ID for the self-hosted Grafana Alerting integration."
  value       = grafana_oncall_integration.selfhost_grafana.id
}
