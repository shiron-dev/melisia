removed {
  from = truenas_dataset.datasets["tank_apps"]

  lifecycle {
    destroy = false
  }
}

removed {
  from = truenas_dataset.datasets["tank_apps_nextcloud"]

  lifecycle {
    destroy = false
  }
}

removed {
  from = truenas_dataset.datasets["tank_apps_nextcloud_appdata"]

  lifecycle {
    destroy = false
  }
}

removed {
  from = truenas_dataset.datasets["tank_apps_nextcloud_config"]

  lifecycle {
    destroy = false
  }
}

removed {
  from = truenas_dataset.datasets["tank_apps_nextcloud_pgdata"]

  lifecycle {
    destroy = false
  }
}

removed {
  from = truenas_dataset.datasets["tank_apps_nextcloud_userdata"]

  lifecycle {
    destroy = false
  }
}
