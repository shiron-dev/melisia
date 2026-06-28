# ディスク使用率が 90% を超えたホスト/マウントポイントを検出してアラートする。
# node_exporter は全ホストで job="node" に統一され、ホストの識別は host ラベル
# (arm-srv, home-ep など) で行う (README.md「収集トポロジ」参照)。
# tmpfs / overlay / squashfs などの擬似・コンテナ・読み取り専用 FS は使用率が
# 100% で張り付くため除外する。ルートの通知ポリシー (slack-notify) により発火時は
# Slack へ通知される。
resource "grafana_rule_group" "disk_usage" {
  name             = "disk-usage"
  folder_uid       = grafana_folder.infrastructure.uid
  interval_seconds = 60

  rule {
    name      = "High disk usage (>90%)"
    condition = "C"
    # 一時的なスパイクで発火させないため 5 分継続したら発火する。
    for = "5m"
    # メトリクス欠落 (NoData) はノード/スクレイプ障害であり、ディスク逼迫とは
    # 別事象なので発火させない。クエリ実行エラーは監視自体の故障なので通知する。
    no_data_state  = "OK"
    exec_err_state = "Alerting"
    annotations = {
      summary     = "[{{ $labels.host }}] {{ $labels.mountpoint }} のディスク使用率が 90% を超過"
      description = "{{ $labels.host }} の {{ $labels.mountpoint }} ({{ $labels.device }}) のディスク使用率が {{ $values.B.Value }}% です (閾値 90%)。不要なファイル・ログ・イメージを削除して空き容量を確保すること。"
    }
    labels = {
      category = "infrastructure"
      severity = "warning"
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
        expr          = "100 - (node_filesystem_avail_bytes{job=\"node\",fstype!~\"tmpfs|overlay|squashfs|ramfs|fuse.*|nsfs\"} * 100 / node_filesystem_size_bytes{job=\"node\",fstype!~\"tmpfs|overlay|squashfs|ramfs|fuse.*|nsfs\"})"
        instant       = true
        intervalMs    = 1000
        legendFormat  = "{{ host }} {{ mountpoint }}"
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
              params = [90]
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
