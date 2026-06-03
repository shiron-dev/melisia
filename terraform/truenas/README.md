# TrueNAS

This Terraform root manages TrueNAS SCALE resources on `storage-srv`.

Existing datasets are imported into state first, then reconciled before making
intentional changes to storage configuration.

The existing `apps` and `tank` pools are tracked with the `truenas_pool` data
source and checked for health and expected mount paths before dataset changes
are applied. The pinned provider exposes pools as a data source only, not as a
managed resource, so ZFS pool creation, disk assignment, and VDEV topology
cannot currently be imported into Terraform state with this provider.

The provider uses the TrueNAS REST API at `https://storage-srv.network.melisia.net`.
The pinned `baladithyab/truenas` v0.2.25 provider does not accept a TLS
verification override in Terraform configuration, so `storage-srv` must serve a
certificate trusted by the machine running manual refresh, plan, or apply.

## Import coverage

Imported storage resources:

- ZFS datasets under `apps/apps` and `tank/{users,apps}` are declared in
  `datasets.tf` and imported through `imports.tf`.

Tracked but not imported:

- `apps` and `tank` pools are queried with `data.truenas_pool.pools`.
- Disk inventory, disk-to-pool assignment, and VDEV topology remain outside
  Terraform state because `baladithyab/truenas` v0.2.25 does not provide pool or
  disk resources.
