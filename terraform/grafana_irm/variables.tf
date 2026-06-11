variable "grafana_cloud_stack_url" {
  description = "Grafana Cloud stack URL used for the service account token."
  type        = string
}

variable "grafana_cloud_auth" {
  description = "Grafana Cloud service account token with permissions to manage IRM/OnCall resources."
  type        = string
  sensitive   = true
}

variable "grafana_cloud_irm_api_url" {
  description = "Grafana IRM API URL from Alerts & IRM > IRM > Settings > Admin & API."
  type        = string
}

variable "primary_escalation_chain_name" {
  description = "Name of the primary escalation chain in Grafana IRM."
  type        = string
  default     = "Primary"
}

variable "selfhost_grafana_integration_name" {
  description = "Name of the Grafana IRM integration that receives alerts from the self-hosted Grafana instance."
  type        = string
  default     = "Self-host Grafana Alerting"
}

variable "selfhost_grafana_integration_type" {
  description = "IRM integration type for self-hosted Grafana Alerting."
  type        = string
  default     = "alertmanager"

  validation {
    condition     = contains(["alertmanager", "webhook", "formatted_webhook"], var.selfhost_grafana_integration_type)
    error_message = "selfhost_grafana_integration_type must be one of alertmanager, webhook, or formatted_webhook."
  }
}

variable "routes" {
  description = "Optional regex routes from the self-host Grafana integration to the primary escalation chain."
  type = list(object({
    routing_regex = string
    position      = number
  }))
  default = []
}
