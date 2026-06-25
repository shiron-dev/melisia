# arm-srv compose host

## Retired projects

The following projects were removed from the `arm-srv` cmt host configuration
after they were stopped on the remote host:

- `wakapi`

`freshrss` (freshrss.melisia.net) and `rsshub` (rsshub.melisia.net) were later
brought back onto this host with their Cloudflare Tunnel / Access provisioned via
Terraform (`local.cloudflare_tunnels`).

Before removing a project from this host configuration, stop it while it is
still listed in `host.yml`, for example:

```sh
GOTOOLCHAIN=auto make cmt-apply CMT_OPT="--host=arm-srv --target=<project> --auto-approve"
```

For projects that should be stopped as part of retirement, temporarily set
`composeAction: down`, apply that targeted change, and only then remove the
project entry and any host-specific secrets.

`wakapi` had no running compose state reported by cmt at retirement time.
