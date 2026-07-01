# arm-srv compose host

## Retired projects

The following projects were removed from the `arm-srv` cmt host configuration
after they were stopped on the remote host:

- `wakapi`

`freshrss` (freshrss.melisia.net) and `rsshub` (rsshub.melisia.net) were later
brought back onto this host with their Cloudflare Tunnel / Access provisioned via
Terraform (`local.cloudflare_tunnels`).

## `rss` project (FreshRSS + RSSHub, merged)

`freshrss` and `rsshub` were merged into a single cmt project, `rss`
(`compose/projects/rss/compose.yml`), running all of FreshRSS, its MariaDB,
RSSHub and its Redis in one compose stack. Because every service now shares the
project's `default` docker network, the previous external `rss-internal` network
(and the `docker network create rss-internal` `postSyncCommand`) is gone:
FreshRSS still reaches RSSHub directly at `http://rsshub:1200`.

Both public hostnames are served by a **single** Cloudflare Tunnel,
`arm-srv-rss`, whose ingress routes `freshrss.melisia.net` → `http://freshrss:80`
and `rsshub.melisia.net` → `http://rsshub:1200` (Terraform
`local.cloudflare_tunnels["arm-srv-rss"]`, with `rsshub.melisia.net` configured as
`extra_ingress`). The old `arm-srv-rsshub` / `arm-srv-freshrss` tunnels were
removed.

### Migration from the split `freshrss` / `rsshub` projects

This was a rename, so two manual steps are required on top of `terraform apply`
and `make cmt-apply`:

1. **New tunnel token.** `arm-srv-rss` is a brand-new tunnel, so its token must be
   bootstrapped into `compose/hosts/arm-srv/rss/cloudflare-tunnel-arm-srv-rss.secrets.yml.sops`
   (temporarily enable the `local_sensitive_file.cloudflare_tunnel_secret` writer
   in `cloudflare_tunnel_secrets.tf`, `-target` apply it, then `make sops-encrypt`).
   Until that file exists, `make cmt-plan`/`cmt-apply` for `rss` fails on the
   missing `cf_tunnel_token` template variable.

2. **Bind-mount data move.** The remote path changes from
   `/opt/compose/{freshrss,rsshub}` to `/opt/compose/rss`, and cmt does **not**
   migrate bind-mount data. Stop the old stacks, move their data, then apply:

   ```sh
   # on arm-srv (ansible_user)
   cd /opt/compose && docker compose -f freshrss/compose.yml down && docker compose -f rsshub/compose.yml down
   # rss/ must stay ansible_user-owned so cmt can SCP compose.yml into it,
   # so create it WITHOUT sudo (/opt/compose is ansible_user-owned).
   mkdir -p rss
   # data/extensions are ansible_user-owned → plain mv works.
   mv freshrss/data freshrss/extensions rss/
   # db_data (uid 999) and redis_data (uid 1000) are owned by a foreign uid;
   # moving a dir rewrites its `..` entry, which needs write on the dir itself,
   # so these two need sudo. Their inode ownership is preserved (host.yml
   # re-chowns them anyway).
   sudo mv freshrss/db_data rss/
   sudo mv rsshub/redis_data rss/            # cache only; safe to drop and let it rebuild
   ```

   RSSHub's `redis_data` is just a cache (`CACHE_TYPE=redis`) and may be dropped
   instead of moved; FreshRSS's `db_data`/`data`/`extensions` hold real state and
   must be preserved.

### RSSHub feeds in FreshRSS (internal path)

`rsshub.melisia.net` is behind the interactive `shiron` Cloudflare Access policy,
so RSS clients cannot fetch through it. FreshRSS instead reaches RSSHub directly
over the shared `default` docker network of the `rss` project. Subscribe in
FreshRSS using the internal URL, not the public hostname:

```text
http://rsshub:1200/<route>
```

The public `rsshub.melisia.net` hostname is for human browsing only. If any feed
was ever subscribed via `https://rsshub.melisia.net/...`, re-point it to the
internal URL (none existed at restore time):

```sh
# one-time migration, run inside freshrss-db
UPDATE shiron_feed
SET url = REPLACE(url, 'https://rsshub.melisia.net', 'http://rsshub:1200')
WHERE url LIKE 'https://rsshub.melisia.net%';
```

Before removing a project from this host configuration, stop it while it is
still listed in `host.yml`, for example:

```sh
GOTOOLCHAIN=auto make cmt-apply CMT_OPT="--host=arm-srv --target=<project> --auto-approve"
```

For projects that should be stopped as part of retirement, temporarily set
`composeAction: down`, apply that targeted change, and only then remove the
project entry and any host-specific secrets.

`wakapi` had no running compose state reported by cmt at retirement time.
