locals {
  datasets = {
    apps = {
      pool        = "apps"
      full_name   = "apps"
      compression = "LZ4"
    }
    apps_apps = {
      pool        = "apps"
      full_name   = "apps/apps"
      compression = "LZ4"
    }
    apps_apps_nextcloud = {
      pool        = "apps"
      full_name   = "apps/apps/nextcloud"
      compression = "LZ4"
    }
    apps_apps_nextcloud_pgdata = {
      pool        = "apps"
      full_name   = "apps/apps/nextcloud/pgdata"
      compression = "LZ4"
    }
    apps_apps_nextcloud_appdata = {
      pool        = "apps"
      full_name   = "apps/apps/nextcloud/appdata"
      compression = "LZ4"
    }
    apps_apps_nextcloud_config = {
      pool        = "apps"
      full_name   = "apps/apps/nextcloud/config"
      compression = "LZ4"
    }
    apps_apps_nextcloud_userdata = {
      pool        = "apps"
      full_name   = "apps/apps/nextcloud/userdata"
      compression = "LZ4"
    }
    tank = {
      pool        = "tank"
      full_name   = "tank"
      compression = "LZ4"
    }
    tank_users = {
      pool        = "tank"
      full_name   = "tank/users"
      compression = "LZ4"
    }
    tank_users_shiron = {
      pool        = "tank"
      full_name   = "tank/users/shiron"
      compression = "LZ4"
    }
  }
}

resource "truenas_dataset" "datasets" {
  for_each = local.datasets

  name = each.value.full_name
  type = "FILESYSTEM"

  atime         = "ON"
  compression   = each.value.compression
  deduplication = "OFF"
  exec          = "ON"
  readonly      = "OFF"
  recordsize    = "128K"
  sync          = "STANDARD"

  lifecycle {
    prevent_destroy = true

    precondition {
      condition     = data.truenas_pool.pools[each.value.pool].healthy
      error_message = "The target TrueNAS storage pool must be healthy before applying dataset changes."
    }

    precondition {
      condition     = data.truenas_pool.pools[each.value.pool].path == local.storage_pools[each.value.pool].path
      error_message = "The target TrueNAS storage pool mount path must match the expected /mnt/<pool> path."
    }
  }
}
