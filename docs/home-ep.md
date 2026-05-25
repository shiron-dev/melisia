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

`home-ep` 自身は同じ `192.168.1.0/24` LAN 上にいるため、WARP の Split
Tunnel include によって LAN 宛の戻り経路まで `CloudflareWARP` に吸われると、
LAN 内からの SSH がタイムアウトする。Ansible の `cloudflare_mesh` role では
`cloudflare-mesh-lan-route-guard.service` を配置し、priority `1` の policy
rule で `192.168.1.0/24` 宛を main routing table に戻す。WARP client が
`inet cloudflare-warp` nftables chain で LAN 宛/送信元を drop する場合もあるため、
同じ guard で `192.168.1.0/24` の input/output allow rule を先頭に追加する。
WARP が起動後に policy rule や nftables rule を作り直すことがあるため、
`cloudflare-mesh-lan-route-guard.timer` で定期的に再適用する。

```sh
ip rule
ip route get 192.168.1.155
sudo nft list chain inet cloudflare-warp input
sudo nft list chain inet cloudflare-warp output
```

`ip route get` の結果が `dev CloudflareWARP` ではなく `dev eth0` で、nftables に
`cloudflare-mesh-lan-route-guard` comment の allow rule があればよい。

`home-ep.network.melisia.net` の unproxied A record は Terraform が初期作成し、
`home-ep` 上の `cloudflare-mesh-dns-updater.timer` が現在の Mesh IP
(`100.96.0.0/12`) を Worker に送って更新する。Worker は Cloudflare Access
service token の `common_name` から更新対象 record を決めるため、`home-ep`
から任意の DNS record は更新できない。

Worker が DNS record を更新するための API token も Terraform が発行する。
この token は updater 対象 zone の `DNS Write` だけを持つ。

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
