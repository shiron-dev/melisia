# Home Assistant

home-ep 上で動く Home Assistant 一式の Compose プロジェクト。cmt でリモート
(`home-ep:/opt/compose/home-assistant`) に同期する。デプロイ手順とホスト構成は
[`docs/home-ep.md`](../../../docs/home-ep.md)、ホスト固有設定は
[`compose/hosts/home-ep/host.yml`](../../hosts/home-ep/host.yml) を参照。

## Source of Truth は repo

automation / script / scene / template を含む HA 設定は **この repo が唯一の正**。
リモート (HA の UI エディタや ESPHome ダッシュボード) で編集しても、次の
`cmt apply` で repo の内容に上書きされる。設定変更は必ず repo を編集して反映する。

### repo 管理（cmt が同期する）

| ファイル | 内容 |
| --- | --- |
| `files/config/configuration.yaml` | HA コア設定（認証 / プロキシ / logger 等） |
| `files/config/automations.yaml` | オートメーション |
| `files/config/scripts.yaml` | スクリプト |
| `files/config/scenes.yaml` | シーン |
| `files/config/templates.yaml` | テンプレートエンティティ（ライト / カーテン） |
| `files/config/lights.yaml` | light プラットフォーム定義 |
| `files/config/themes/` | フロントエンドテーマ |
| `files/esphome_config/*.yaml` | ESPHome デバイス設定（音声サテライト / 環境センサー） |
| `files/mosquitto.conf` | MQTT ブローカー設定 |
| `files/switchbot-mqtt/options.json` | SwitchBot ブリッジ設定 |
| `compose.yml` | サービス定義一式 |

### リモート管理（repo に置かない / cmt が触らない）

`host.yml` の `preserveRemoteFiles` で保護、または同期対象外のもの。

| 対象 | 理由 |
| --- | --- |
| `config/.storage/` | エンティティ / デバイス / エリアレジストリ、ヘルパー、ダッシュボード、Alarmo 等の HA 内部状態。UI で管理。 |
| `config/custom_components/` | HACS で導入したカスタム統合。 |
| `mosquitto_passwd` | `mosquitto_passwd` CLI で生成するハッシュ済み資格情報。 |
| `esphome_config/secrets.yaml` | ESPHome の実シークレット（`wifi_ssid` / `wifi_password`）。サンプルは `secrets.yaml.example`。 |
| `esphome_config/.esphome/`, `*.bak-*`, `archive/` | ESPHome ビルドキャッシュ / 手動バックアップ。 |
| `*.secrets.yml.sops` | SOPS 暗号化シークレット（`compose/hosts/home-ep/home-assistant/`）。 |

## Jinja と Go テンプレート

cmt は同期ファイルを Go の `text/template` で処理するため、本文の Jinja2
(`{{ ... }}` / `{% ... %}`) がデリミタ衝突を起こす。`host.yml` の `templateIgnore`
に `config` と `esphome_config` を指定して**変数展開をスキップ**しているので、
automations.yaml 等は**素の Jinja のまま**（`{{"{{"}}` エスケープ不要）で書ける。

`switchbot-mqtt/options.json` だけは `{{ .switchbot_api_key }}` 等の Go テンプレート
変数を使うため `templateIgnore` に**含めない**。

## 変更の反映ワークフロー

```sh
# 1. repo の files/config/*.yaml 等を編集

# 2. 差分を確認（読み取りのみ）
make cmt-plan CMT_OPT="--host=home-ep --project=home-assistant"

# 3. 適用（リモートへ同期 + compose up）
make cmt-apply CMT_OPT="--host=home-ep --project=home-assistant"

# 4. HA に設定を再読み込みさせる
#    開発者ツール → YAML → 「オートメーション」「スクリプト」「シーン」
#    「テンプレートエンティティ」を再読み込み（または HA 再起動）。
```

> automation / script の `id:`（タイムスタンプ）はエンティティ ID とトレース履歴の
> 紐付けに使われるため、編集時も維持すること。

## ESPHome

デバイス設定の詳細・フラッシュ手順は
[`files/esphome_config/README.md`](files/esphome_config/README.md) を参照。
