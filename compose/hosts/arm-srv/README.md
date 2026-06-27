# arm-srv compose host

## Retired projects

The following projects were removed from the `arm-srv` cmt host configuration
after they were stopped on the remote host:

- `wakapi`

`freshrss` (freshrss.melisia.net) and `rsshub` (rsshub.melisia.net) were later
brought back onto this host with their Cloudflare Tunnel / Access provisioned via
Terraform (`local.cloudflare_tunnels`).

### RSSHub feeds in FreshRSS (internal path)

`rsshub.melisia.net` is behind the interactive `shiron` Cloudflare Access policy,
so RSS clients cannot fetch through it. FreshRSS instead reaches RSSHub directly
over the shared `rss-internal` docker network. Subscribe in FreshRSS using the
internal URL, not the public hostname:

```
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
