# ESPHome デバイス (音声サテライト + コントローラー + 環境センサー)

## 構成

```text
 esphome_config/
├── secrets.yaml.example            # シークレットのサンプル
├── atoms3r-echo-base.yaml          # 音声サテライト (M5Stack AtomS3R + Atomic Echo Base)
├── m5dial.yaml                     # コントローラー (M5Dial v1.1)
└── m5stack-atom-s3-lite.yaml       # 環境センサーノード (CO2 / BME688) + BLE プロキシ
```

> 実体の `secrets.yaml` はリポジトリに含めず、`host.yml` の `preserveRemoteFiles` で
> cmt の同期・削除対象から除外している。WiFi 資格情報・各デバイスの API 暗号化キー /
> OTA パスワードはすべて `!secret` 参照とし、値はインラインに置かない
> (サンプルは `secrets.yaml.example`)。

## 役割分担

| デバイス | 役割 | 備考 |
| --- | --- | --- |
| **M5Stack AtomS3R Echo Base** | 音声サテライト | ES8311 codec + MEMS mic + NS4150B amp。M5Stack 公式 ESPHome パッケージを使用。 |
| **M5Dial v1.1** | コントローラー | ロータリーエンコーダ + タッチ + RFID。HA の操作端末。 |
| **M5Stack AtomS3-Lite** | 環境センサー | Grove PaHub 経由で SCD4x(CO2) + BME688。Bluetooth プロキシも兼ねる。 |

## 音声パイプライン (compose.yml 内のコンテナ)

| 役割 | コンテナ | ポート | 備考 |
| --- | --- | --- | --- |
| ウェイクワード | `openwakeword` | 10400 | `ok_nabu` |
| 音声認識 (STT) | `whisper` | 10300 | faster-whisper `small`, 日本語+英語 auto |
| 音声合成 (TTS) | `piper` | 10200 | `en_US-lessac-medium` (日本語は後述) |
| 会話エンジン | (HA 設定) | - | OpenAI / Gemini 等の LLM を HA UI で選択 |

HA の内部 URL は `configuration.yaml` で `internal_url: http://192.168.1.61:8123`
を設定済み(デバイスが TTS 音声を取得するのに必須)。

## セットアップ

### 1. デプロイ

```sh
make cmt-apply CMT_OPT="--host=home-ep --project=home-assistant"
```

### 2. フラッシュ

ESPHome ダッシュボード (`http://192.168.1.61:6052`) で各デバイスを **Install**
(初回は USB / Chrome 系の Web Serial、以降 OTA)。`!secret` の値は事前に
リモートの `secrets.yaml`(またはダッシュボード右上「Secrets」エディタ)へ。

### 3. Home Assistant (音声サテライト)

1. **Wyoming 統合** 3つ:`127.0.0.1` の `10300`(Whisper)/`10200`(Piper)/`10400`(openWakeWord)。
2. **Assist パイプライン**:会話=LLM、STT=Whisper、TTS=Piper、ウェイクワード=ok_nabu。
3. デバイス `atoms3r-echo-base` にパイプラインを割り当て。

## シークレットのローテーション

API 暗号化キー / OTA パスワードを更新したら、`secrets.yaml` の該当値を新しい値へ
更新し、対象デバイスを再フラッシュ(OTA パスワードを変える場合は初回 USB)、
HA 側でデバイスの API 暗号化キーを再入力(再ペアリング)する。

## 注意: 日本語 TTS

Piper に高品質な日本語ボイスが無いため、日本語の読み上げ品質は限定的。日本語重視なら
パイプラインの TTS だけ HA Cloud / Google Translate TTS に分けるのが現実的。
