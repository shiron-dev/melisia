resource "grafana_folder" "infrastructure" {
  title = "Infrastructure"
  uid   = "infrastructure"
}

resource "grafana_rule_group" "server_reachability" {
  name             = "server-reachability"
  folder_uid       = grafana_folder.infrastructure.uid
  interval_seconds = 30

  rule {
    name           = "Server Cloudflare route probe failed"
    condition      = "C"
    for            = "2m"
    no_data_state  = "Alerting"
    exec_err_state = "Alerting"
    annotations = {
      summary     = "Server Cloudflare route probe failed"
      description = "{{ $labels.target_server }} is not reachable through Cloudflare from {{ $labels.probe_server }}."
    }
    labels = {
      category = "server-reachability"
      severity = "critical"
    }

    data {
      ref_id         = "A"
      datasource_uid = "P95B22FBE6FE890D0"
      model = jsonencode({
        datasource = {
          type = "prometheus"
          uid  = "P95B22FBE6FE890D0"
        }
        editorMode    = "code"
        expr          = "probe_success{job=\"server_cloudflare\"}"
        instant       = true
        intervalMs    = 1000
        legendFormat  = "{{ target_server }}"
        maxDataPoints = 43200
        range         = false
        refId         = "A"
      })

      relative_time_range {
        from = 600
        to   = 0
      }
    }

    data {
      ref_id         = "B"
      datasource_uid = "__expr__"
      model = jsonencode({
        conditions = [
          {
            evaluator = {
              params = []
              type   = "gt"
            }
            operator = {
              type = "and"
            }
            query = {
              params = ["B"]
            }
            reducer = {
              params = []
              type   = "last"
            }
            type = "query"
          }
        ]
        datasource = {
          type = "__expr__"
          uid  = "__expr__"
        }
        expression    = "A"
        intervalMs    = 1000
        maxDataPoints = 43200
        reducer       = "last"
        refId         = "B"
        type          = "reduce"
      })

      relative_time_range {
        from = 0
        to   = 0
      }
    }

    data {
      ref_id         = "C"
      datasource_uid = "__expr__"
      model = jsonencode({
        conditions = [
          {
            evaluator = {
              params = [1]
              type   = "lt"
            }
            operator = {
              type = "and"
            }
            query = {
              params = ["C"]
            }
            reducer = {
              params = []
              type   = "last"
            }
            type = "query"
          }
        ]
        datasource = {
          type = "__expr__"
          uid  = "__expr__"
        }
        expression    = "B"
        intervalMs    = 1000
        maxDataPoints = 43200
        refId         = "C"
        type          = "threshold"
      })

      relative_time_range {
        from = 0
        to   = 0
      }
    }
  }
}

resource "grafana_dashboard" "server_reachability" {
  folder      = grafana_folder.infrastructure.uid
  config_json = file("${path.module}/server-reachability-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}
