# PVE Verification Guide

Use this guide before claiming that the in-repository TrueNAS provider can
replace the previous provider for `terraform/truenas`.

## Scope

This verification must use a real TrueNAS SCALE VM running on PVE. Do not use a
mock TrueNAS API for replacement validation.

The provider currently covers only the Terraform surface used by this
repository:

- `data.truenas_pool`
- `truenas_dataset`

PVE verification proves that this provider can plan, apply, refresh, and reach a
no-op plan against the real TrueNAS SCALE REST API while avoiding the production
TrueNAS host and production Terraform state.

This recreates the Terraform-managed TrueNAS shape only. It does not clone
production data, apps, ACLs, SMB/NFS shares, snapshots, replication tasks, users,
or other TrueNAS settings that are outside `terraform/truenas`.

## Safety Rules

- Do not point verification at `https://storage-srv.network.melisia.net`.
- Do not use the GCS backend during PVE verification.
- Do not run verification against the real `terraform/truenas` state.
- Use a temporary Terraform root under `/private/tmp`.
- Replace `imports.tf` in the temporary root with imports for only the `apps`
  and `tank` pool root datasets created by TrueNAS.
- Pass `truenas_url`, `truenas_api_key`, and
  `truenas_tls_insecure_skip_verify=true` explicitly.
- Only create, change, or delete resources inside the PVE verification VM and
  its attached PVE disks.

## Verified Run

The last verified run used this shape:

- Date: 2026-06-08 JST
- PVE VM ID: `900`
- PVE VM name: `truenas-provider-verify`
- TrueNAS version: TrueNAS SCALE 25.10.4
- TrueNAS VM IP: `192.168.1.154`
- Pools: `apps` and `tank`, each backed by one dedicated 8 GiB PVE disk

The run completed with:

```text
Apply complete! Resources: 6 added, 0 changed, 0 destroyed.
```

That apply was a resumed apply after an earlier provider build had already
imported the two pool root datasets, updated their properties, and created two
child datasets. With a fresh VM and the current provider build, expect the
initial plan to be:

```text
Plan: 2 to import, 8 to add, 2 to change, 0 to destroy.
```

The follow-up `terraform plan -detailed-exitcode` must exit with `0` and report
`No changes`.

## PVE VM

Use a dedicated PVE VM for provider verification.

Recommended VM shape:

- VM ID: `900`
- Name: `truenas-provider-verify`
- ISO: `TrueNAS-SCALE-25.10.4.iso`
- CPU: 2 cores
- Memory: 8192 MiB
- Network: VirtIO on `vmbr0`
- OS disk: 20 GiB on `local-lvm`
- Pool disks: two additional 8 GiB disks on `local-lvm`

Set unique serials on all QEMU disks before creating pools. TrueNAS may reject
pool creation when the disks report duplicate serial `None`.

Example for VM `900`:

```sh
qm set 900 --scsi0 local-lvm:vm-900-disk-0,iothread=1,size=20G,serial=tnverifyos900
qm set 900 --scsi1 local-lvm:vm-900-disk-1,iothread=1,size=8G,serial=tnverifyapps900
qm set 900 --scsi2 local-lvm:vm-900-disk-2,iothread=1,size=8G,serial=tnverifytank900
```

Install TrueNAS SCALE to the 20 GiB disk only. Leave the two 8 GiB disks unused
during installation so they can become test storage pools.

After installation, remove or deprioritize the ISO boot entry so the VM boots
from the installed disk.

## TrueNAS Setup

1. Boot the installed system and open the TrueNAS Web UI.
2. Create a temporary admin password or use the installer-created admin account.
3. Create a temporary API key for Terraform verification.
4. Confirm TrueNAS sees the two pool disks.

   ```sh
   curl -k -u '<admin-user>:<temporary-password>' \
     https://<truenas-vm-ip>/api/v2.0/disk
   ```

5. Create the `apps` and `tank` single-disk pools from the two 8 GiB data disks.
   In the verified run the data disks appeared as `sdb` and `sdc`.

   ```sh
   curl -k -H "Authorization: Bearer <truenas-vm-api-key>" \
     -H "Content-Type: application/json" \
     -X POST \
     -d '{"name":"apps","topology":{"data":[{"type":"STRIPE","disks":["sdb"]}]}}' \
     https://<truenas-vm-ip>/api/v2.0/pool

   curl -k -H "Authorization: Bearer <truenas-vm-api-key>" \
     -H "Content-Type: application/json" \
     -X POST \
     -d '{"name":"tank","topology":{"data":[{"type":"STRIPE","disks":["sdc"]}]}}' \
     https://<truenas-vm-ip>/api/v2.0/pool
   ```

6. Confirm both pools are healthy and mounted at the paths expected by
   `terraform/truenas/storage.tf`.

   ```sh
   curl -k -H "Authorization: Bearer <truenas-vm-api-key>" \
     https://<truenas-vm-ip>/api/v2.0/pool
   ```

Expected pool facts:

- `apps` is healthy and mounted at `/mnt/apps`.
- `tank` is healthy and mounted at `/mnt/tank`.

## Terraform Procedure

1. Build the local provider mirror and CLI config from the repository root.

   ```sh
   make terraform-provider-devrc
   ```

2. Copy the TrueNAS Terraform root to `/private/tmp`.

   ```sh
   tmpdir="$(mktemp -d /private/tmp/melisia-truenas-apply.XXXXXX)"
   cp terraform/truenas/*.tf "$tmpdir"
   mv "$tmpdir/imports.tf" "$tmpdir/imports.tf.disabled"
   ```

3. In the temporary root only, remove the `backend "gcs"` block from
   `provider.tf`. The PVE verification must use local temporary state.

4. In the temporary root only, create a VM-specific `imports.tf` for the pool
   root datasets that TrueNAS creates with the pools.

   ```hcl
   import {
     to = truenas_dataset.datasets["apps"]
     id = "apps"
   }

   import {
     to = truenas_dataset.datasets["tank"]
     id = "tank"
   }
   ```

5. Initialize the temporary root with the local provider mirror.

   ```sh
   TF_CLI_CONFIG_FILE="$PWD/.local/terraform-provider-truenas.tfrc" \
     terraform -chdir="$tmpdir" init -reconfigure
   ```

6. Plan against the TrueNAS VM.

   ```sh
   TF_CLI_CONFIG_FILE="$PWD/.local/terraform-provider-truenas.tfrc" \
     terraform -chdir="$tmpdir" plan \
       -var=truenas_url=https://<truenas-vm-ip> \
       -var=truenas_api_key=<truenas-vm-api-key> \
       -var=truenas_tls_insecure_skip_verify=true \
       -out="$tmpdir/tfplan"
   ```

   On a fresh verification VM, the expected summary is:

   ```text
   Plan: 2 to import, 8 to add, 2 to change, 0 to destroy.
   ```

7. Apply the saved plan.

   ```sh
   TF_CLI_CONFIG_FILE="$PWD/.local/terraform-provider-truenas.tfrc" \
     terraform -chdir="$tmpdir" apply -auto-approve "$tmpdir/tfplan"
   ```

8. Verify the follow-up plan is a no-op.

   ```sh
   TF_CLI_CONFIG_FILE="$PWD/.local/terraform-provider-truenas.tfrc" \
     terraform -chdir="$tmpdir" plan \
       -var=truenas_url=https://<truenas-vm-ip> \
       -var=truenas_api_key=<truenas-vm-api-key> \
       -var=truenas_tls_insecure_skip_verify=true \
       -detailed-exitcode
   ```

   Success means exit code `0` with:

   ```text
   No changes. Your infrastructure matches the configuration.
   ```

## Troubleshooting

- If pool creation fails because disks have duplicate serial `None`, stop the VM,
  set unique `serial=` values on the PVE disks with `qm set`, boot TrueNAS again,
  and retry pool creation.
- If `terraform apply` reports that a parent dataset does not exist while
  creating child datasets, rebuild the provider from current source with
  `make terraform-provider-devrc`. The provider includes retry handling for the
  short TrueNAS consistency window after parent dataset creation.
- If Terraform reports provider checksum mismatch after rebuilding the local
  provider, rerun `terraform init -reconfigure` in the temporary root. If the
  temporary lock file still references the previous local build, delete only the
  temporary root's `.terraform.lock.hcl` and rerun init.

## Cleanup

After verification, delete only the temporary Terraform root and the dedicated
PVE verification resources that were created for this guide. Do not touch the
production TrueNAS host or production Terraform state.

## Production Migration Is Separate

After PVE verification, production replacement still requires:

- Running against the real production TrueNAS SCALE API.
- Migrating remote state provider addresses from
  `registry.terraform.io/baladithyab/truenas` to
  `registry.terraform.io/shiron-dev/truenas`.
- Confirming that the production API response shape matches this provider's read
  and update behavior.
