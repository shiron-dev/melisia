# TrueNAS

This Terraform root manages TrueNAS SCALE resources on `storage-srv`.

Existing datasets are now tracked as normal Terraform resources. The remote
state was aligned from the running TrueNAS instance first, and the resource
configuration keeps those observed values explicit so routine plans stay clean.

The existing `apps` and `tank` pools are tracked with the `truenas_pool` data
source and enforced with dataset lifecycle preconditions for health and expected
mount paths before dataset changes are applied. The pinned provider exposes
pools as a data source only, not as a managed resource, so ZFS pool creation,
disk assignment, and VDEV topology cannot currently be imported into Terraform
state with this provider.

The provider uses the TrueNAS REST API at `https://storage-srv.network.melisia.net`.
The pinned `baladithyab/truenas` v0.2.25 provider does not accept a TLS
verification override in Terraform configuration, so `storage-srv` must serve a
certificate trusted by the machine running manual refresh, plan, or apply.

## Resource coverage

Managed storage resources:

- ZFS datasets for the `apps` and `tank` pool roots, plus datasets under
  `apps/apps` and `tank/users`, are declared in `datasets.tf`.

Removed storage resources:

- `tank/apps` and its former Nextcloud child datasets were deleted from TrueNAS
  and removed from Terraform state on 2026-06-07.

Tracked but not imported as pool resources:

- `apps` and `tank` pools are queried with `data.truenas_pool.pools`.
- Disk inventory, disk-to-pool assignment, and VDEV topology remain outside
  Terraform state because `baladithyab/truenas` v0.2.25 does not provide pool or
  disk resources.
