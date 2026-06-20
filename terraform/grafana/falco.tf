resource "grafana_dashboard" "falco_security" {
  folder      = grafana_folder.infrastructure.uid
  config_json = file("${path.module}/falco-security-dashboard.json")

  depends_on = [
    grafana_data_source.managed,
  ]
}

# 高プライオリティ (Emergency/Alert/Critical/Error) の Falco 検知イベントを
# Loki から検出してアラートする。ルートの通知ポリシー (slack-notify) により
# 発火時は Slack へ通知される。host ラベルを付けるため sum by (host)。
# 検知が無い場合 (NoData) は正常扱い (no_data_state=OK)。
resource "grafana_rule_group" "falco_alerts" {
  name             = "falco"
  folder_uid       = grafana_folder.infrastructure.uid
  interval_seconds = 60

  rule {
    name           = "Falco high-severity event detected"
    condition      = "C"
    for            = "0s"
    no_data_state  = "OK"
    exec_err_state = "OK"
    annotations = {
      summary     = "[{{ $labels.host }}] Falco high-severity event(s) detected"
      description = "Falco reported {{ $values.B }} high-severity (Error 以上) event(s) on {{ $labels.host }} in the last 5m. Grafana の Falco Security ダッシュボード / Explore ({container=\"falco\"}) で詳細を確認すること。"
    }
    labels = {
      category = "falco"
      severity = "critical"
    }

    data {
      ref_id         = "A"
      datasource_uid = "loki_localhost"
      query_type     = "instant"
      model = jsonencode({
        datasource = {
          type = "loki"
          uid  = "loki_localhost"
        }
        editorMode    = "code"
        expr          = "sum by (host) (count_over_time({container=~\"falco\"} | json | priority=~\"Emergency|Alert|Critical|Error\" [5m]))"
        instant       = true
        intervalMs    = 1000
        legendFormat  = "{{ host }}"
        maxDataPoints = 43200
        queryType     = "instant"
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
              params = [0]
              type   = "gt"
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
