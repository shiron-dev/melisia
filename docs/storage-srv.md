# storage-srv

TrueNAS storage server.

## 準備

`storage-srv.network.melisia.net` is managed by Terraform as an unproxied public
DNS record and currently resolves to `192.168.1.64`.

### ホスト名の変更

TrueNAS の UI から `storage-srv` に変更する。

### melisia_ops ユーザーの作成

※CI/CD や初期構築の利便性のために NOPASSWD を指定しているので、注入後は
必要に応じて sudo 権限を絞るか、パスワードを変更すること

ローカルで SSH 鍵を作成する。

```sh
mkdir -p .local/ssh
ssh-keygen -t ed25519 -f .local/ssh/storage_srv_key -C "melisia_ops@storage-srv"
chmod 600 .local/ssh/storage_srv_key
```

TrueNAS UI で `melisia_ops` を作成する。

```text
Credentials > Local Users > Add
```

設定の目安:

```text
Username: melisia_ops
Full Name: Melisia Operations
Primary Group: Create New Primary Group
Home Directory: Create under /mnt/tank
Shell: bash
SMB Access: off
SSH Access: on
Authorized Keys: .local/ssh/storage_srv_key.pub の中身
Sudo: Allow all sudo commands with no password
```

TrueNAS は Shell/SSH access を持つユーザーの home directory に data pool 配下の
writable path を要求するため、`melisia_ops` の home directory は
`/mnt/tank/melisia_ops` として作成する。

SSH service を有効にする。

```text
System > Services > SSH
Start Automatically: on
Allow Password Authentication: off
Root Login: off
```

疎通確認:

```sh
cd ansible
ssh -F ssh_config storage-srv
```
