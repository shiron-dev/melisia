# TrueNAS

This Terraform root manages TrueNAS SCALE resources on `storage-srv`.

Existing datasets are imported into state first, then reconciled before making
intentional changes to storage configuration.

The existing `apps` and `tank` pools are tracked with the `truenas_pool` data
source and enforced with dataset lifecycle preconditions for health and expected
mount paths before dataset changes are applied. The in-repository provider under
`terraform/provider` currently exposes pools as a data source only, not as a
managed resource, so ZFS pool creation, disk assignment, and VDEV topology cannot
currently be imported into Terraform state with this provider.

The provider uses the TrueNAS REST API at `https://storage-srv.network.melisia.net`.
The local `shiron-dev/truenas` provider accepts `tls_insecure_skip_verify`.
This root defaults it to `true` because `storage-srv` currently serves a
certificate that is not trusted by the machine running manual refresh, plan, or
apply.

## Local provider

The provider is developed in `terraform/provider`. For this root, the repository
Makefile builds the provider and generates a local Terraform CLI filesystem
mirror configuration under `.local/terraform-provider-truenas.tfrc`
automatically when `TERRAFORM_TARGET=truenas`.

After switching state from the previous community provider, run the state
provider address migration once:

```sh
make terraform-truenas-replace-provider-state
```

## Import coverage

Imported storage resources:

- ZFS datasets for the `apps` and `tank` pool roots, plus datasets under
  `apps/apps` and `tank/users`, are declared in `datasets.tf` and imported
  through `imports.tf`.

Removed storage resources:

- `tank/apps` and its former Nextcloud child datasets were deleted from TrueNAS
  and removed from Terraform state on 2026-06-07.

Tracked but not imported as pool resources:

- `apps` and `tank` pools are queried with `data.truenas_pool.pools`.
- Disk inventory, disk-to-pool assignment, and VDEV topology remain outside
  Terraform state because the local provider does not provide pool or disk
  resources yet.
