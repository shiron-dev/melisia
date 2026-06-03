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
      compression = "ON"
    }
    tank_users_shiron = {
      pool        = "tank"
      name        = "users/shiron"
      compression = "ON"
    }
    tank_apps = {
      pool        = "tank"
      name        = "apps"
      compression = "ON"
    }
    tank_apps_nextcloud = {
      pool        = "tank"
      name        = "apps/nextcloud"
      compression = "ON"
    }
    tank_apps_nextcloud_config = {
      pool        = "tank"
      name        = "apps/nextcloud/config"
      compression = "ON"
    }
    tank_apps_nextcloud_userdata = {
      pool        = "tank"
      name        = "apps/nextcloud/userdata"
      compression = "ON"
    }
    tank_apps_nextcloud_appdata = {
      pool        = "tank"
      name        = "apps/nextcloud/appdata"
      compression = "ON"
    }
    tank_apps_nextcloud_pgdata = {
      pool        = "tank"
      name        = "apps/nextcloud/pgdata"
      compression = "ON"
    }
  }
}

resource "truenas_dataset" "datasets" {
  for_each = local.datasets

  name = "${each.value.pool}/${each.value.name}"
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
  }
}
