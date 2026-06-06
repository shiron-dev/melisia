locals {
  storage_pools = {
    apps = {
      path        = "/mnt/apps"
      description = "Application data pool."
    }
    tank = {
      path        = "/mnt/tank"
      description = "Primary user and service data pool."
    }
  }
}

data "truenas_pool" "pools" {
  for_each = local.storage_pools

  id = each.key
}
