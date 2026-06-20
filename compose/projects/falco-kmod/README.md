# falco-kmod

home-ep (Raspberry Pi 5 / kernel `6.12.x +rpt-rpi-2712`) 専用の Falco デプロイ。

`arm-srv` は modern eBPF (`compose/projects/falco`) で動くが、**この Pi カーネルは
modern eBPF プローブ (`BPF_TRACE_RAW_TP`) に非対応**で Falco が起動できないことを
実機で確認した。そのため home-ep だけ **カーネルモジュール (kmod) ドライバ** を使う。

## 仕組み

カーネルモジュールは **ホスト側で dkms によりビルド/ロード** し、コンテナは
ロード済みモジュールを使うだけ (ドライバローダは実行しない)。

- 当初 falcosecurity/falco イメージのエントリポイントでコンテナ内 dkms ビルドを
  試みたが、この Pi のヘッダ構成では **どのマウント構成でも成立しなかった**。
  dkms は `/lib/modules/<ver>/build` (相対 symlink `../../../src/...`) から
  カーネルソースを探すが、これがコンテナのバインドマウント越しに解決しない
  (native / HOST_ROOT / nested / `framework.conf` の `kernel_source_dir` すべて不可)。
  ホスト上ではこの symlink がネイティブ解決するため、ホストビルドは問題なく通る。
- そこで ansible (`roles/raspberrypi/tasks/falco_driver.yml`) で Pi に
  `dkms` とドライバソースを入れ、ホストでモジュールをビルド/インストールし、
  `/etc/modules-load.d/falco.conf` で起動時ロードする。kernel 更新時は dkms の
  autoinstall が自動再ビルドする。
- コンテナ (`compose.yml`) は `SKIP_DRIVER_LOADER=yes` でドライバ取得/ビルドを
  一切行わず、`engine.kind=kmod` でホストの `/dev/falcoN` を開く。kmod エンジンは
  デバイスの mmap 等に特権が必要なため `privileged: true` で動かす
  (モジュールのロード自体はホスト側で完了済み)。
- 検知イベントは JSON で stdout 出力し、既存の promtail が Loki へ集約する。
  Grafana 側は arm-srv と共通の「Falco Security」ダッシュボードで参照できる
  (`container="falco"` でクエリ)。

## バージョン整合 (重要)

ドライバのバージョン (`7.3.0+driver`) は Falco 本体 (0.39.2) に対応する。
**Falco を更新する際は以下を必ず揃えて更新する**:

- `compose/projects/falco-kmod/compose.yml` の `image`
- `ansible/roles/raspberrypi/defaults/main.yml` の `falco_kmod_image` /
  `falco_kmod_driver_version`

## セットアップ / デプロイ

```sh
# 1. ホスト側モジュール (ansible)。raspberrypi ロールに含まれる。
#    dkms 導入・ソース取得・ビルド/インストール・modules-load を行う。
make ansible-run            # site.yml (home_servers 経由で raspberrypi ロール)

# 2. コンテナ (cmt)。新規プロジェクトは初回 apply で起動しないことがあるため 2 回。
make cmt-apply CMT_OPT="--host=home-ep --project=falco-kmod"
```

## 起動確認

```sh
# ホストにモジュールがロードされているか
ssh home-ep "lsmod | grep falco; ls /dev/falco*"

# コンテナが検知イベントを出しているか (JSON)
ssh home-ep "docker logs --tail 20 falco"
```

## チューニング

ノイズの多いルール (例: sshd による `/etc/shadow` 読み取りの "Read sensitive
file untrusted") の無効化や独自ルールの追加は `files/falco_rules.local.yaml`
に記述する。`falco_rules.yaml` 本体は編集しない。
