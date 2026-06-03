output "storage_pools" {
  description = "Observed TrueNAS storage pools tracked by this Terraform root."
  value = {
    for name, pool in data.truenas_pool.pools : name => {
      name        = pool.name
      path        = pool.path
      status      = pool.status
      healthy     = pool.healthy
      size        = pool.size
      available   = pool.available
      description = local.storage_pools[name].description
    }
  }
}
