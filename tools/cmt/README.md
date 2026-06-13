# Compose Manage Tool (cmt)

Docker Compose プロジェクトのための、Source of Truth・プッシュ型同期ツールです。  
`docker compose` 環境向けのプッシュモデル ArgoCD のようなものです。

## 仕組み

`cmt` はローカルリポジトリからプロジェクト定義とホスト設定を読み取り、
SSH/SFTP 経由でリモートホストにプッシュします。
Terraform と同様の **plan / apply** ワークフローに従います。

```
cmt plan   — 変更内容を表示（読み取り専用、`docker compose config` で構成検証）
cmt apply  — 変更を適用（確認あり、--auto-approve で省略可）
```

## ディレクトリ構成

cmt 設定の `basePath` で指定する compose ルートディレクトリは、
以下の構成を前提とします:

```
compose/
├── projects/
│   └── <project>/
│       ├── compose.yml          # 共通の compose 定義
│       └── files/               # compose.yml と同じディレクトリにコピーされるファイル
│           └── ...
├── hosts/
│   └── <hostname>/
│       ├── host.yml             # ホストレベルのデフォルト設定・プロジェクト別上書き
│       └── <project>/
│           ├── compose.override.yml   # ホスト固有の compose override
│           ├── *.yml / *.yaml         # テンプレート変数ソース（リモートには同期されない）
│           └── files/                 # ホスト固有のファイル（プロジェクトの files/ を上書き）
│               └── ...
└── README.md
```

### リモート側の配置

各プロジェクトのファイルは `<remotePath>/<project>/` に配置されます:

```
/opt/compose/grafana/
├── compose.yml
├── compose.override.yml
├── grafana.ini          (files/ から)
└── .cmt-manifest.json   (cmt が管理)
```

## 設定ファイル

### cmt 設定 (`--config`)

```yaml
basePath: ../compose            # compose ルートへのパス（設定ファイルからの相対パス）

defaults:                       # 最低優先度のデフォルト値
  remotePath: /opt/compose
  composeAction: up             # up|down|ignore（未指定時は up）
  # templateVarSources:         # テンプレート変数ソースの glob（デフォルト: ["*.yml", "*.yaml"]）
  #   - "*.yml"
  #   - "*.yaml"

hosts:
  - name: server1               # hosts/<hostname>/ ディレクトリ名と一致させる
    host: 192.168.1.10
    user: deploy
    sshAgent: true
    # sshKeyPath: ~/.ssh/id_ed25519.pub
```

### host.yml (`hosts/<hostname>/host.yml`)

```yaml
sshConfig: ../../ssh_config     # SSH config ファイルのパス（host.yml からの相対パス）

remotePath: /srv/compose        # cmt デフォルトをこのホスト用に上書き
composeAction: up               # up|down|ignore（未指定時は up）

projects:                       # プロジェクト別の上書き
  grafana:
    composeAction: up           # プロジェクト単位で上書き可能
    removeOrphans: true         # composeAction=down 時に --remove-orphans を付与
    dirs:                       # Docker ボリューム用ディレクトリの事前作成
      - grafana_storage
      - grafana_conf
```

#### `sshConfig` — SSH config による接続設定の解決

cmt は常に `ssh -G <host>` を実行して SSH 接続パラメータ
（hostname, user, port, identity file, proxy command 等）を解決します。
解決された値は cmt 設定の YAML 値を**上書き**します。

`sshConfig` を指定した場合は `ssh -G -F <path> <host>` が実行され、
指定した SSH config ファイルが使われます。
未指定の場合はデフォルトの SSH config（`~/.ssh/config` 等）が使われます。

`ssh -G` による解決のため、SSH config の `Match`, `ProxyCommand`, `ProxyJump`,
`IdentityFile` などの高度な機能がそのまま利用できます。

- パスは `host.yml` があるディレクトリ (`hosts/<hostname>/`) からの相対パスで指定します
- cmt 設定の `host` フィールドが `ssh -G` の引数（SSH destination）として使われます

#### `dirs` — ボリュームディレクトリの事前作成

`dirs` にはリモートのプロジェクトディレクトリからの相対パスを指定します。
`cmt apply` 時にファイル同期より先にディレクトリを作成します。
Docker Compose の bind mount 用ディレクトリを事前に用意する用途を想定しています。

文字列で指定する場合はパスのみが設定されます（従来の形式）。
属性を指定する場合は `- <path>:` の下に `permission`（8進数文字列）、`owner`、`group`、`recursive`、`become`、`becomeUser` を指定します。

```yaml
projects:
  grafana:
    dirs:
      - grafana_storage                         # 文字列形式（従来どおり）
      - grafana_conf
      - influxdb_data:                          # 属性付き形式（推奨）
        permission: "0755"
        owner: influxdb
        group: influxdb
        become: true                            # 権限変更コマンドを sudo 経由で実行
        # becomeUser: root                      # 未指定時は root
      - redis_data:                             # recursive で既存ファイルも chown
        owner: 1000
        group: 1000
        recursive: true                         # chown -R でディレクトリ内も再帰的に変更
        become: true
      - vmdata:
        permission: "0700"
```

`permission` を指定した場合はディレクトリ作成後に `chmod` を実行します。
`owner` または `group` を指定した場合は `chown` を実行します。
`recursive: true` を指定すると `chown -R` でディレクトリとその中身を再帰的に変更します。
Docker コンテナが root で作成したファイルを非 root ユーザーで動くサービス（例: Redis）が使う場合に有用です。

デフォルトでは（`become` 未指定または `false`）SSH 接続ユーザーの権限で実行されます。
`become: true` を指定すると `sudo` を使って実行され、`becomeUser` 未指定時は `root`、指定時はそのユーザーで実行されます。

`cmt plan` では各ディレクトリの状態（`create` / `exists`）と設定された属性が表示されます。

#### `templateVarSources` — テンプレート変数ソースの指定

`templateVarSources` でテンプレート変数の読み込み元ファイルの glob パターンを指定します。
未指定時のデフォルトは `["*.yml", "*.yaml"]` です。

```yaml
defaults:
  templateVarSources:
    - "*.yml"
    - "*.yaml"
```

ファイルはアルファベット順に読み込まれ、同一キーは後のファイルが上書きします。
`compose.override.yml` と `host.yml` は自動的に除外されます。

テンプレート変数ソースファイルはリモートには**同期されません**。
同期したい場合は `files/` ディレクトリに配置してください。

設定の解決順序は他のフィールドと同じく `defaults` → `host.yml` → `projects.<name>` です。

```yaml
projects:
  grafana:
    templateVarSources:
      - "env.secrets.yml"
```

#### `preserveRemoteFiles` — リモート側で管理するファイルの保護

`preserveRemoteFiles` には、cmt が同期・削除しないリモートファイルを指定します。
Home Assistant の UI が書き換える `automations.yaml` のように、リポジトリではなく
リモート側の内容を正としたいファイルに使います。

```yaml
projects:
  home-assistant:
    preserveRemoteFiles:
      - config/automations.yaml
      - config/scripts.yaml
      - config/scenes.yaml
```

指定したファイルは、`files/` 配下に存在していても同期対象から除外されます。
また、過去の `.cmt-manifest.json` に含まれていても削除対象にならず、次回 apply 後の
manifest からは外れます。

#### `composeAction` — Compose 理想状態の管理

`composeAction` でプロジェクトの理想状態（`up` / `down`）を宣言的に管理します。
未指定時は `up` がデフォルトです。

```yaml
projects:
  grafana:
    composeAction: up          # サービスが起動している状態を理想とする
  legacy-app:
    composeAction: down        # サービスが停止している状態を理想とする
  static-app:
    composeAction: ignore      # up/down の状態差分は無視する
```

`cmt plan` ではリモートの現在の Compose サービス状態と理想状態を比較し、差分を表示します:

- `up` の場合:
  - ファイル/ディレクトリに更新がある場合: 全サービスを「再作成予定」として表示（`docker compose up -d --force-recreate`）
  - ファイル更新なし + 停止サービスがある場合: 停止中のサービスを「起動予定」として表示（`docker compose up -d`）
- `down` の場合: 現在起動中のサービスを「停止予定」として表示
- `ignore` の場合: up/down の状態差分を確認・表示しない

`cmt apply` では差分に基づいて自動的に適切な Compose コマンドを実行します。
`ignore` の場合は Compose の up/down 実行自体をスキップします。
ファイル差分がなくても Compose 状態に差分があれば apply の対象になります。

`projects.<name>.removeOrphans: true` を指定すると、サービス再作成時に
`docker compose up -d --force-recreate --remove-orphans`、`composeAction: down` の実行時に
`docker compose down --remove-orphans` を使います。

### `beforeApplyHooks` — apply 前フック

`cmt apply` の実行中に、任意のコマンドを実行できるフックを設定できます。
フックは cmt 設定（`config.yml`）で定義します。

```yaml
beforeApplyHooks:
  beforePlan:
    command: ./scripts/prepare-context.sh
  beforeApplyPrompt:
    command: ./scripts/check-policy.sh
  beforeApply:
    command: ./scripts/final-gate.sh
```

#### フックの実行タイミング

- **`beforePlan`** — plan 出力の**前**に実行
- **`beforeApplyPrompt`** — plan 出力後、ユーザー確認プロンプトの**前**に実行
- **`beforeApply`** — ユーザーが `y` で承認した後（`--auto-approve` の場合はプロンプト省略後）、実際の apply の**前**に実行

#### 終了コード

| 終了コード | 動作 |
|-----------|------|
| `0` | 続行 |
| `1` | apply を中止（正常終了） |
| その他 | エラーとして異常終了 |

#### stdin JSON

各フックにはコマンドの stdin に JSON が渡されます。
スキーマは `cmt schema hook-before-plan` /
`cmt schema hook-before-apply-prompt` /
`cmt schema hook-before-apply` で生成できます。

```json
{
  "hosts": ["server1", "server2"],
  "pwd": "/path/to/working/directory",
  "paths": {
    "configPath": "config.yml",
    "basePath": "/absolute/path/to/compose"
  }
}
```

- `hosts` — 今回の apply 対象ホスト名（`--host` フィルタ適用後）
- `pwd` — cmt 実行時のカレントディレクトリ
- `paths.configPath` — `--config` で指定された設定ファイルパス
- `paths.basePath` — 解決済みの compose ルート絶対パス

フックの stdout / stderr は cmt の出力にそのまま表示されます。

### デフォルト値の解決順序

1. cmt 設定の `defaults`
2. `host.yml` のトップレベルフィールド
3. `host.yml` の `projects.<name>` フィールド

後の設定が前の設定を上書きします。

## CLI リファレンス

```
cmt [--config <path>] <command> [flags]

コマンド:
  plan          変更内容を表示（変更は行わない）
  apply         リモートホストに変更を適用
  force-unlock  スタックしたロックを解除（<host> <project> を指定）
  schema        設定ファイルの JSON Schema を生成

グローバルフラグ:
  --config  cmt 設定ファイルのパス（デフォルト: config.yml）

plan / apply フラグ:
  --host      ホスト名でフィルタ（複数指定可）
  --project   プロジェクト名でフィルタ（複数指定可）
  --target   --project の別名。Terraform 風に -target=NAME も利用可（複数指定可）

plan フラグ:
  --exit-code  終了コードで結果を返す（CI 等向け）:
               0 = 差分なし, 1 = エラー, 2 = 差分あり

apply フラグ:
  --auto-approve  確認プロンプトをスキップ

force-unlock:
  cmt force-unlock <host> <project>  指定 project のリモートロックを解除
  --force                            確認プロンプトをスキップ

schema:
  cmt schema cmt                 cmt 設定の JSON Schema を出力
  cmt schema host                host.yml の JSON Schema を出力
  cmt schema hook-before-plan          beforePlan フックの stdin JSON Schema を出力
  cmt schema hook-before-apply-prompt  beforeApplyPrompt フックの stdin JSON Schema を出力
  cmt schema hook-before-apply         beforeApply フックの stdin JSON Schema を出力
```

## ロック

`plan` / `apply` は対象**リモートホスト上**に project 単位のロックを作成し、
操作の競合を防ぎます。

- ロックファイルは各 project のリモートディレクトリ直下に作られます:
  `<remotePath>/<project>/.cmt.lock`（例: `/opt/compose/grafana/.cmt.lock`）。
- ロックはリモート側に置かれるため、ローカル開発機と CI runner など**別マシンからの
  同時操作も排他**できます。
- ロックは **project 単位**です。`--target` / `--project` で対象を絞った場合、
  別 project への操作とは**競合しません**。
- ロックは操作終了時に自動解放されます。プロセスがクラッシュした等でロックが
  残った場合は `cmt force-unlock <host> <project>` で解除します。
- `apply` はロック取得時に project ディレクトリを作成しますが、`plan` は読み取り
  専用のためディレクトリを作成しません。未デプロイ（ディレクトリ未作成）の project
  に対する `plan` はロックを取得せずにスキップします。
- `apply` がロック取得でディレクトリを作成したものの、確認キャンセル・差分なし・
  hook 中断・plan digest 不一致などで何も書き込まなかった場合、ロック解放時に
  空ディレクトリをロールバック（削除）します。

```bash
cmt force-unlock arm-srv grafana            # 確認プロンプトあり
cmt force-unlock arm-srv grafana --force    # 確認なしで解除
```

## JSON Schema

スキーマは Go の構造体から自動生成されるため、コードとの乖離がありません:

```bash
cmt schema cmt                 > schemas/cmt-config.schema.json
cmt schema host                > schemas/host-config.schema.json
cmt schema hook-before-plan          > schemas/hook-before-plan.schema.json
cmt schema hook-before-apply-prompt  > schemas/hook-before-apply-prompt.schema.json
cmt schema hook-before-apply         > schemas/hook-before-apply.schema.json
```

エディタでのバリデーションや補完に利用できます
（例: VS Code の YAML 拡張で `# yaml-language-server` コメントを指定）。

## ビルド

```bash
cd tools/cmt
go build -o cmt .
```
