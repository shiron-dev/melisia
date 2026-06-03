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

check "storage_pools_healthy" {
  assert {
    condition = alltrue([
      for pool in data.truenas_pool.pools : pool.healthy
    ])
    error_message = "All managed TrueNAS storage pools must be healthy before applying storage changes."
  }
}

check "storage_pool_mount_paths" {
  assert {
    condition = alltrue([
      for name, pool in data.truenas_pool.pools : pool.path == local.storage_pools[name].path
    ])
    error_message = "Managed TrueNAS storage pool mount paths must match the expected /mnt/<pool> paths."
  }
}
