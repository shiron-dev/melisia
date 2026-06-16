# photoframe

WebDAV を画像ソースにする軽量な Web スライドショー。TrueNAS Nextcloud の
WebDAV フォルダを、必要に応じて Cloudflare Access の service token を付けて取得し、
全画面のスライドショーとしてブラウザに配信する。

WebDAV の認証情報と Cloudflare Access トークンはサーバ側に隠蔽され、ブラウザには
不透明な ID 経由でしか画像を返さない (`/img/{id}`)。

## エンドポイント

| パス          | 説明                                             |
| ------------- | ------------------------------------------------ |
| `GET /`       | スライドショー HTML (埋め込み)                    |
| `GET /api/images` | 画像 URL の一覧と表示間隔 (JSON)             |
| `GET /img/{id}`   | WebDAV から取得した画像をプロキシ配信         |
| `GET /healthz`    | ヘルスチェック (常に 200)                     |

`photoframe healthcheck` サブコマンドで `/healthz` を叩いて 0/1 を返す
(distroless イメージにシェルが無いため、コンテナの HEALTHCHECK に使う)。

## 環境変数

| 変数                     | 必須 | デフォルト | 説明                                                            |
| ------------------------ | ---- | ---------- | --------------------------------------------------------------- |
| `WEBDAV_BASE_URL`        | ○    | -          | WebDAV のベース URL 例 `https://host/remote.php/dav/files/USER` |
| `WEBDAV_PATH`            |      | (ルート)   | 表示対象フォルダ 例 `/Photos/Frame`                             |
| `WEBDAV_USERNAME`        | ○    | -          | WebDAV ユーザー名                                               |
| `WEBDAV_PASSWORD`        | ○    | -          | Nextcloud アプリパスワード推奨                                  |
| `CF_ACCESS_CLIENT_ID`    |      | -          | Cloudflare Access service token client id                       |
| `CF_ACCESS_CLIENT_SECRET`|      | -          | 同 client secret (id と対で指定)                                |
| `SLIDE_INTERVAL`         |      | `10`       | スライド切替間隔 (秒、または `30s`/`1m` 形式)                   |
| `REFRESH_INTERVAL`       |      | `5m`       | フォルダ再走査の間隔                                            |
| `REQUEST_TIMEOUT`        |      | `30s`      | WebDAV への 1 リクエストのタイムアウト                          |
| `LISTEN_ADDR`            |      | `:8080`    | リッスンアドレス                                                |

## ローカル実行

```sh
cd apps/photoframe
WEBDAV_BASE_URL=https://nextcloud.example/remote.php/dav/files/me \
WEBDAV_PATH=/Photos/Frame \
WEBDAV_USERNAME=me WEBDAV_PASSWORD=app-password \
go run .
# http://localhost:8080
```

## イメージ

`apps/photoframe/**` を main にマージするか `photoframe-v*` タグを打つと
`.github/workflows/photoframe-image.yml` が `ghcr.io/shiron-dev/photoframe` を
linux/amd64 + linux/arm64 でビルド・push する。`compose/projects/photoframe`
はこのイメージを参照する。

## デプロイ (arm-srv)

詳細は [`compose/projects/photoframe`](../../compose/projects/photoframe) と
`terraform/terraform/cloudflare_tunnel_locals.tf` を参照。

Cloudflare 側 (apply 済み):

- Tunnel `arm-srv-photoframe` / DNS `photoframe.melisia.net` / Access アプリ
  (shiron ポリシー保護) を作成。
- photoframe 専用 Access service token を発行し、Nextcloud Access アプリ
  ("NextCloud Services", 保護ホスト `cloud.shiron.org`) のポリシーに precedence 4 で付与。
- `cf_tunnel_token` と `photoframe_access_client_id/secret` を SOPS 暗号化済み
  (`cloudflare-tunnel-arm-srv-photoframe.secrets.yml.sops` /
  `cloudflare-access-photoframe.secrets.yml.sops`)。

残り:

1. `photoframe-v0.1.0` タグを push してイメージを publish し、digest を
   `compose/projects/photoframe/compose.yml` に固定する。
2. `compose/hosts/arm-srv/photoframe/env.secrets.yml` に WebDAV 接続情報
   (`https://cloud.shiron.org/remote.php/dav/files/<ユーザー名>` と
   アプリパスワード) を記入し、`make sops-encrypt FILE=...` で暗号化する。
3. `make cmt-apply CMT_OPT="--host arm-srv --target photoframe"` でデプロイ。
