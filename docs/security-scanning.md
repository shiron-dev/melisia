# Security Scanning

本リポジトリではセキュリティを強化するため、複数のセキュリティスキャンツールを導入しています。

## 導入ツール

### Infrastructure as Code (IaC) Security

#### 1. Checkov
- **用途**: IaC（Terraform、CloudFormation等）の設定ミスやセキュリティ違反を検出
- **リンク**: https://www.checkov.io/
- **CI**: `terraform.yml` - `terraform-checkov` ジョブ
- **ローカル実行**:
  ```bash
  make terraform-checkov
  # または
  checkov --directory terraform/ --framework terraform --compact
  ```

#### 2. tfsec
- **用途**: Terraform固有のセキュリティ問題を検出
- **リンク**: https://github.com/aquasecurity/tfsec
- **CI**: `terraform.yml` - `terraform-tfsec` ジョブ
- **ローカル実行**:
  ```bash
  make terraform-tfsec
  # または
  tfsec terraform/
  ```

#### 3. Trivy
- **用途**: IaC設定やコンテナイメージの脆弱性とミスコンフィギュレーションを検出
- **リンク**: https://github.com/aquasecurity/trivy
- **CI**: `terraform.yml` - `terraform-trivy` ジョブ
- **ローカル実行**:
  ```bash
  make terraform-trivy
  # または
  trivy config terraform/
  ```

### Secret Detection

#### 4. gitleaks
- **用途**: リポジトリ履歴からAPIキー、パスワード、その他のシークレットを検出
- **リンク**: https://github.com/gitleaks/gitleaks
- **CI**: `security-scanning.yml` - `gitleaks` ジョブ
- **ローカル実行**:
  ```bash
  make gitleaks
  # または
  gitleaks detect --source . --verbose
  ```

#### 5. TruffleHog
- **用途**: ファイルシステム、Git履歴、その他のソースからシークレットを検出
- **リンク**: https://github.com/trufflesecurity/trufflehog
- **CI**: `security-scanning.yml` - `trufflehog` ジョブ
- **ローカル実行**:
  ```bash
  make trufflehog
  # または
  trufflehog filesystem . --json
  ```

### Cost Analysis

#### 6. Pluralith
- **用途**: Terraform設定の視覚化とコスト分析
- **リンク**: https://www.pluralith.com/
- **CI**: `terraform.yml` - `pluralith` ジョブ
- **ローカル実行**:
  ```bash
  make pluralith
  # または
  cd terraform && pluralith graph
  ```

## ローカルセットアップ

各ツールをローカル環境で実行するには、事前にインストールが必要です。

### インストール（macOS/Linux）

```bash
# Checkov
pip install checkov

# tfsec
brew install tfsec

# Trivy
brew install trivy

# gitleaks
brew install gitleaks

# TruffleHog
pip install trufflehog

# Pluralith
brew install pluralith
```

### インストール（その他のプラットフォーム）
各ツールの公式ドキュメントを参照してください。

## CI/CD パイプライン

### Terraform CI (`terraform.yml`)
Terraformファイルが変更されたときに自動実行：
- `terraform-lint`: TFLintでのリント
- `terraform-checkov`: Checkovでのセキュリティスキャン
- `terraform-tfsec`: tfsecでのセキュリティスキャン
- `terraform-trivy`: Trivyでのセキュリティスキャン
- `pluralith`: Pluralithでの可視化分析
- `infracost`: コスト推定

### Security Scanning (`security-scanning.yml`)
すべてのプッシュとPRで自動実行：
- `gitleaks`: シークレット検出
- `trufflehog`: シークレット検出

## 日常的な使用

### 開発前のセキュリティチェック
```bash
# 全体的なセキュリティチェック
make ci

# または個別実行
make terraform-ci
make gitleaks
make trufflehog
```

### Terraform変更前
```bash
make terraform-ci
```

## トラブルシューティング

### False positives への対処
各ツールで誤検出が発生した場合：

- **Checkov**: `.checkov.yaml` で除外ルールを設定
- **tfsec**: `.tfsec/config.json` で設定を調整
- **gitleaks**: `.gitleaksignore` ファイルで除外設定
- **TruffleHog**: `.trufflehog.json` で除外設定

### ツールが見つからないエラー
```bash
# 各ツールがインストールされているか確認
which checkov tfsec trivy gitleaks trufflehog pluralith

# 必要に応じて再インストール
```

## 参考リンク

- [Checkov Documentation](https://docs.bridgecrewio.com/)
- [tfsec Documentation](https://aquasecurity.github.io/tfsec/)
- [Trivy Documentation](https://aquasecurity.github.io/trivy/)
- [gitleaks Documentation](https://gitleaks.io/)
- [TruffleHog Documentation](https://github.com/trufflesecurity/trufflehog)
- [Pluralith Documentation](https://www.pluralith.com/docs)
