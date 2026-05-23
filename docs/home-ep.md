# home-ep

お家環境と外とをつなぐ入り口

## 準備

### hostname の変更

```sh
sudo hostnamectl set-hostname home-ep
```

### Cloudflare Mesh のインストール

Terraform で Cloudflare Mesh node と `192.168.1.0/24` route を作成し、
生成された connector token を Ansible で `home-ep` に投入する。

`cloudflare_api_token` には、対象 account に対する以下の権限が必要。

- `Cloudflare One Connectors: Write` または `Cloudflare One Connector: WARP: Write`
- `Cloudflare One Networks: Write` または `Cloudflare Tunnel: Write`
- `Zero Trust: Write`

接続元の WARP client が Include mode の device profile を使っている場合、
Split Tunnel include に `100.96.0.0/12` と `192.168.1.0/24` が入っている
必要がある。この設定も Terraform で管理する。

```sh
make terraform-plan TERRAFORM_TARGET=terraform
make terraform-apply TERRAFORM_TARGET=terraform
make sops-encrypt FILE=ansible/group_vars/home_ep/cloudflare-mesh.secrets.yml
cd ansible
ansible-playbook -i hosts.yml home_servers.yml --limit home-ep
```

Cloudflare One Client は `warp-cli` の headless connector として登録される。
Cloudflare Mesh 上の client からは、`home-ep` 自身へは node の Mesh IP
(`100.96.0.0/12`) で接続する。`192.168.1.0/24` route は `home-ep` の
背後にある LAN device へ接続するためのもの。

`home-ep` の Mesh IP が分かったら、`terraform.secrets.tfvars` に以下を追加すると DNS 名でも接続できる。

```hcl
home_ep_mesh_ip = "100.96.x.y"
```

これにより `home-ep.network.melisia.net` の unproxied A record が作られる。
到達できるのは Cloudflare Mesh / WARP に接続している端末だけ。

SSH 認証は Access for Infrastructure に寄せる。Cloudflare が発行する短命
SSH certificate を使い、`home-ep` 側では `/etc/ssh/ca.pub` に Cloudflare の
Infrastructure SSH CA public key を配置し、`TrustedUserCAKeys` で信頼する。
UNIX user は自動作成されないため、`ansible_user` は引き続き `home-ep` 上に
存在している必要がある。

### Ansible ユーザーの作成

※CI/CD の利便性のために NOPASSWD を指定しているので、注入後は直ちにユーザーを削除するか、パスワードを変更すること

```sh
sudo su -
adduser ansible_user
usermod -aG sudo ansible_user
echo "ansible_user ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/ansible_user
```

## Home Assistant

`compose/hosts/home-ep/host.yml` に `home-assistant` project の同期先ディレクトリを
定義している。`home-ep` を `compose/config.yml` に登録した後、
対象 host を指定して同期する。

```sh
make cmt-plan CMT_OPT=--host=home-ep
make cmt-apply CMT_OPT=--host=home-ep
```

初回起動後に HACS を入れる場合は、`homeassistant` container が起動してから実行する。

```sh
docker exec homeassistant bash -c 'wget -O - https://get.hacs.xyz | bash -'
```
