# TrueNAS

This Terraform root manages TrueNAS SCALE resources on `storage-srv`.

Existing datasets are imported into state first, then reconciled before making
intentional changes to storage configuration.

The provider uses the TrueNAS REST API at `https://storage-srv.network.melisia.net`.
The pinned `baladithyab/truenas` v0.2.25 provider does not accept a TLS
verification override in Terraform configuration, so `storage-srv` must serve a
certificate trusted by the machine running manual refresh, plan, or apply.
