# falco

[Falco](https://falco.org/) によるランタイムセキュリティ監視。`arm-srv` と
`home-ep` の両ホストにデプロイし、不審な syscall/プロセス挙動を検知する。

## 仕組み

- ドライバは **modern eBPF** (`engine.kind=modern_ebpf`) を使用する。
  `falco-no-driver` イメージに静的同梱されており、ドライバの追加インストール
  (カーネルヘッダ / kmod ビルド) は不要。
- 検知イベントは **JSON で stdout に出力** する。各ホストの promtail が全
  コンテナの docker ログをスクレイプして Loki へ集約しているため、Falco の
  イベントも自動的に Loki に入る。Falco 専用の forwarder は不要。
- Grafana 側のダッシュボードは `terraform/grafana/falco-security-dashboard.json`
  で管理する (Loki データソースを `container="falco"` でクエリ)。

## カーネル要件 (重要)

modern eBPF はカーネルの **BTF (CO-RE)** 対応が前提
(`/sys/kernel/btf/vmlinux` が存在すること)。

- **arm-srv** (OCI ARM / Ubuntu 系): BTF 有効。問題なく動作する。
- **home-ep** (Raspberry Pi): 標準カーネルは BTF が無効な場合がある。
  初回デプロイ後に必ず起動を確認すること (下記)。BTF 非対応で起動しない
  場合は、`falcosecurity/falco` イメージ + `falcoctl` による kmod/legacy
  eBPF ドライバへの切り替えを検討する。

## デプロイ

```sh
# home-ep
make cmt-apply CMT_OPT="--host=home-ep --project=falco"
# arm-srv
make cmt-apply CMT_OPT="--host=arm-srv --project=falco"
```

新規プロジェクトは初回 apply で compose.yml が配置されるのみでコンテナが
起動しないことがあるため、apply は 2 回実行する。

## 起動確認

```sh
# 対象ホストで
docker logs falco

# 期待されるログ例:
#   Falco initialized with configuration files: ...
#   Loading rules from: /etc/falco/falco_rules.yaml ...
#   Starting health webserver ... (本設定では無効)
#   Falco internal: syscall event source. Opening 'syscall' source ...
#   Events detected: ... / または定常稼働ログ

# BTF 非対応で失敗する場合は以下のようなエラーが出る:
#   Unable to load the driver ... / failed to open BPF ...
```

Loki に届いているかは Grafana の **Falco Security** ダッシュボード、または
Explore で `{container="falco"}` をクエリして確認する。

## チューニング

ノイズの多いルールの無効化や独自ルールの追加は `files/falco_rules.local.yaml`
に記述する。`falco_rules.yaml` 本体は編集しない。
