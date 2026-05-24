variable "grafana_url" {
  description = "Root URL of the Grafana instance managed by this Terraform root."
  type        = string
  default     = "https://grafana.shiron.dev/"
}

variable "grafana_auth" {
  description = "Grafana service account token, API key, or basic auth string. Prefer GRAFANA_AUTH or terraform.secrets.tfvars.sops."
  type        = string
  sensitive   = true
  default     = null
}

variable "folders" {
  description = "Grafana folders managed as code. The map key is also used by dashboard subdirectories."
  type = map(object({
    title = string
    uid   = optional(string)
  }))
  default = {
    observability = {
      title = "Observability"
      uid   = "observability"
    }
  }
}

variable "datasources" {
  description = "Grafana data sources managed as code."
  type = map(object({
    name                     = string
    type                     = string
    uid                      = optional(string)
    url                      = optional(string)
    access_mode              = optional(string, "proxy")
    is_default               = optional(bool, false)
    json_data_encoded        = optional(string)
    secure_json_data_encoded = optional(string)
  }))
  default = {
    prometheus_vm_localhost = {
      name              = "Prometheus-vm_localhost"
      type              = "prometheus"
      uid               = "prometheus-vm-localhost"
      url               = "http://victoriametrics:8428"
      access_mode       = "proxy"
      is_default        = true
      json_data_encoded = <<-JSON
        {
          "httpMethod": "POST",
          "manageAlerts": true,
          "prometheusType": "Prometheus",
          "prometheusVersion": "2.24.0"
        }
      JSON
    }
  }
}

variable "email_contact_points" {
  description = "Email contact points for Grafana Alerting."
  type = map(object({
    addresses               = list(string)
    subject                 = optional(string, "{{ template \"default.title\" . }}")
    message                 = optional(string, "{{ template \"default.message\" . }}")
    single_email            = optional(bool, true)
    disable_resolve_message = optional(bool, false)
  }))
  default = {}
}

variable "notification_policy" {
  description = "Root Grafana Alerting notification policy. This manages the entire policy tree, so set it only after importing or intentionally replacing the current policy."
  type = object({
    contact_point   = string
    group_by        = list(string)
    group_wait      = optional(string, "30s")
    group_interval  = optional(string, "5m")
    repeat_interval = optional(string, "4h")
  })
  default = null
}

variable "rule_groups" {
  description = "Grafana Alerting rule groups managed as code."
  type = map(object({
    folder_key       = string
    interval_seconds = number
    rules = list(object({
      name           = string
      condition      = string
      for            = optional(string, "0s")
      no_data_state  = optional(string, "NoData")
      exec_err_state = optional(string, "Alerting")
      annotations    = optional(map(string), {})
      labels         = optional(map(string), {})
      is_paused      = optional(bool, false)
      data = list(object({
        ref_id         = string
        datasource_uid = string
        model          = string
        query_type     = optional(string, "")
        from           = number
        to             = number
      }))
    }))
  }))
  default = {}
}
