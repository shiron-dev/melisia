# ESPHome デバイス (音声サテライト + 環境センサー)

## 構成

```text
 esphome_config/
├── secrets.yaml.example            # WiFi シークレットのサンプル
├── m5stick-s3-voice.yaml           # 音声サテライト (M5Stack StickS3)
└── m5stack-atom-s3-lite.yaml       # 環境センサーノード (CO2 / BME688)
```

> 実体の `secrets.yaml`(`wifi_ssid` / `wifi_password`)はリポジトリに含めず、
> `host.yml` の `preserveRemoteFiles` で cmt の同期・削除対象から除外している。
> API 暗号化キー / OTA パスワードは各デバイス YAML に直書き。

## 役割分担

| デバイス | 役割 | 備考 |
| --- | --- | --- |
| **M5Stack StickS3** | 音声サテライト | **8MB PSRAM** 搭載。ES8311+マイク+AW8737+スピーカー内蔵。音声向き。 |
| **M5Stack AtomS3-Lite** | 環境センサー | Grove PaHub 経由で SCD4x(CO2)+ BME688。PSRAM 非搭載で音声には不向きだったため役割を分離。 |

AtomS3-Lite は PSRAM 非搭載で、音声バッファ確保が `ESP_ERR_NO_MEM` で破綻したため、
音声機能は PSRAM 搭載の StickS3 に移設した。

## 音声パイプライン (compose.yml 内のコンテナ)

| 役割 | コンテナ | ポート | 備考 |
| --- | --- | --- | --- |
| ウェイクワード | `openwakeword` | 10400 | `ok_nabu` |
| 音声認識 (STT) | `whisper` | 10300 | faster-whisper `small`, 日本語+英語 auto |
| 音声合成 (TTS) | `piper` | 10200 | `en_US-lessac-medium` (日本語は後述) |
| 会話エンジン | (HA 設定) | - | OpenAI / Gemini 等の LLM を HA UI で選択 |

HA の内部 URL は `configuration.yaml` で `internal_url: http://192.168.1.61:8123`
を設定済み(デバイスが TTS 音声を取得するのに必須)。

## StickS3 セットアップ

### 1. デプロイ

```sh
cmt apply --host home-ep --project home-assistant
```

### 2. フラッシュ

ESPHome ダッシュボード (`http://192.168.1.61:6052`) で `m5stick-s3-voice` を
**Install**(初回は USB / Chrome 系の Web Serial、以降 OTA)。

### 3. Home Assistant

1. **Wyoming 統合** 3つ:`127.0.0.1` の `10300`(Whisper)/`10200`(Piper)/`10400`(openWakeWord)。
2. **Assist パイプライン**:会話=LLM、STT=Whisper、TTS=Piper、ウェイクワード=ok_nabu。
3. デバイス `m5stick-s3-voice` にパイプラインを割り当て。

### ⚠️ StickS3 は実機未検証の推定を含む

M5 公式の ESPHome パッケージが StickS3 用にまだ無いため、以下は実機で要確認:

1. **PSRAM mode**: `octal` を指定(N8R8 の定説)。boot 失敗/クラッシュ時は `quad` に。
2. **M5PM1 電源**: LCD/MIC/SPK 電源(L3B)は M5PM1(0x6E)の PYG2=GPIO2 で給電。
   **アクティブ LOW**(M5 公式)なので on_boot で GPIO2 を出力 **LOW** にして給電する。
   HIGH だと給電されず音声回路が動かない(「ジジ」になる)。
3. **ES8311 マイク経路**: `use_microphone: true`(ADC 経由)と推定。マイクが拾わなければ
   `use_microphone` / `adc_type` を見直す。
4. **ウェイクワード**: 現状サーバ側 openWakeWord(`start_continuous`)。PSRAM があるので
   on-device `micro_wake_word` も可能(その場合 HA のデバイス設定で wake word を選べる)。

起動ログ(`ESP_ERR_NO_MEM` が無いか、音声が出るか、マイク RMS など)を見ながら
上記を詰める。

## 注意: 日本語 TTS

Piper に高品質な日本語ボイスが無いため、日本語の読み上げ品質は限定的。日本語重視なら
パイプラインの TTS だけ HA Cloud / Google Translate TTS に分けるのが現実的。
