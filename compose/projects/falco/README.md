# falco

[Falco](https://falco.org/) によるランタイムセキュリティ監視 (**arm-srv** 用)。
不審な syscall/プロセス挙動を検知する。

home-ep は Pi カーネルが modern eBPF 非対応のため、別プロジェクト
[`falco-kmod`](../falco-kmod/README.md) (kmod ドライバ) を使う。

## 仕組み

- ドライバは **modern eBPF** (`engine.kind=modern_ebpf`) を使用する。
  `falco-no-driver` イメージに静的同梱されており、ドライバの追加インストール
  (カーネルヘッダ / kmod ビルド) は不要。
- 検知イベントは **JSON で stdout に出力** する。arm-srv の promtail が全
  コンテナの docker ログをスクレイプして Loki へ集約しているため、Falco の
  イベントも自動的に Loki に入る。Falco 専用の forwarder は不要。
- Grafana 側のダッシュボード/アラートは `terraform/grafana/falco.tf`
  (+ `falco-security-dashboard.json`) で管理する
  (Loki データソースを `container="falco"` でクエリ)。

## カーネル要件

modern eBPF はカーネルの **BTF (CO-RE)** 対応が前提
(`/sys/kernel/btf/vmlinux` が存在すること)。arm-srv (OCI ARM / Ubuntu 系) は
BTF 有効で問題なく動作する。

## 配置先 (arm-srv)

arm-srv の他プロジェクトは `/var` 配下だが、falco は**デフォルトの
`/opt/compose` に置く** (`remotePath` を指定しない)。`/var` は SSH ユーザーが
sudo 無しで `mkdir` できず初回 apply が失敗するのに対し、`/opt/compose` は
`ansible_user` 所有のため cmt がそのまま作成でき、事前 bootstrap 不要で
`make cmt-apply` だけで完結する。falco は永続データを持たないので `/var` に
置く必要はない。

## デプロイ

```sh
make cmt-apply CMT_OPT="--host=arm-srv --project=falco"
```

新規プロジェクトは初回 apply で compose.yml が配置されるのみでコンテナが
起動しないことがあるため、apply は 2 回実行する。

## 起動確認

```sh
ssh arm-srv.shiron.dev "docker logs falco"

# 期待されるログ例:
#   Falco initialized with configuration files: ...
#   Loading rules from: /etc/falco/falco_rules.yaml ...
#   Opening 'syscall' source with modern BPF probe.
```

Loki に届いているかは Grafana の **Falco Security** ダッシュボード、または
Explore で `{container="falco"}` をクエリして確認する。

## チューニング

ノイズの多いルールの無効化や独自ルールの追加は `files/falco_rules.local.yaml`
に記述する。`falco_rules.yaml` 本体は編集しない。
