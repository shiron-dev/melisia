resource "grafana_folder" "e2e" {
  title = "E2E"
  uid   = "e2e"
}

resource "grafana_rule_group" "cloudflare_tunnel_e2e" {
  name             = "cloudflare-tunnel-e2e"
  folder_uid       = grafana_folder.e2e.uid
  interval_seconds = 30

  rule {
    name           = "Cloudflare tunnel E2E probe failed"
    condition      = "C"
    for            = "2m"
    no_data_state  = "Alerting"
    exec_err_state = "Alerting"
    annotations = {
      summary     = "Cloudflare tunnel E2E probe failed"
      description = "{{ $labels.instance }} is not reachable from the public tunnel endpoint."
    }
    labels = {
      category = "e2e"
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
        expr          = "probe_success{job=\"cloudflare_tunnel_e2e\",edge_auth=~\".+\"}"
        instant       = true
        intervalMs    = 1000
        legendFormat  = "{{ instance }}"
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

resource "grafana_rule_group" "cloudflare_access_block_e2e" {
  name             = "cloudflare-access-block-e2e"
  folder_uid       = grafana_folder.e2e.uid
  interval_seconds = 3600

  rule {
    name           = "Cloudflare Access block E2E probe failed"
    condition      = "C"
    for            = "5m"
    no_data_state  = "Alerting"
    exec_err_state = "Alerting"
    annotations = {
      summary     = "Cloudflare Access block E2E probe failed"
      description = "{{ $labels.instance }} did not return the expected Cloudflare Access login redirect."
    }
    labels = {
      category = "e2e"
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
        expr          = "last_over_time(probe_success{job=\"cloudflare_access_block_e2e\",edge_auth=\"cloudflare_access_block\"}[2h])"
        instant       = true
        intervalMs    = 1000
        legendFormat  = "{{ instance }}"
        maxDataPoints = 43200
        range         = false
        refId         = "A"
      })

      relative_time_range {
        from = 7200
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

resource "grafana_dashboard" "cloudflare_tunnel_e2e" {
  folder      = grafana_folder.e2e.uid
  config_json = file("${path.module}/e2e-cloudflare-tunnel-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}

resource "grafana_dashboard" "cloudflare_access_block_e2e" {
  folder      = grafana_folder.e2e.uid
  config_json = file("${path.module}/cloudflare-access-block-e2e-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}
