locals {
  datasets = {
    apps_apps = {
      pool        = "apps"
      name        = "apps"
      compression = "LZ4"
    }
    apps_apps_nextcloud = {
      pool        = "apps"
      name        = "apps/nextcloud"
      compression = "LZ4"
    }
    apps_apps_nextcloud_pgdata = {
      pool        = "apps"
      name        = "apps/nextcloud/pgdata"
      compression = "LZ4"
    }
    apps_apps_nextcloud_appdata = {
      pool        = "apps"
      name        = "apps/nextcloud/appdata"
      compression = "LZ4"
    }
    apps_apps_nextcloud_config = {
      pool        = "apps"
      name        = "apps/nextcloud/config"
      compression = "LZ4"
    }
    apps_apps_nextcloud_userdata = {
      pool        = "apps"
      name        = "apps/nextcloud/userdata"
      compression = "LZ4"
    }
    tank_users = {
      pool        = "tank"
      name        = "users"
      compression = "LZ4"
    }
    tank_users_shiron = {
      pool        = "tank"
      name        = "users/shiron"
      compression = "LZ4"
    }
  }
}

resource "truenas_dataset" "datasets" {
  for_each = local.datasets

  name = "${data.truenas_pool.pools[each.value.pool].name}/${each.value.name}"
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
