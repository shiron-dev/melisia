resource "truenas_apps_config" "apps" {
  enable_image_updates = var.truenas_apps_docker_config.enable_image_updates
  pool                 = var.truenas_apps_docker_config.pool
  nvidia               = var.truenas_apps_docker_config.nvidia
  address_pools        = var.truenas_apps_docker_config.address_pools
  preferred_trains     = var.truenas_apps_catalog_config.preferred_trains
}

moved {
  from = truenas_app_config.apps["nextcloud"]
  to   = truenas_app_config.nextcloud
}

moved {
  from = truenas_app_config.apps["cloudflared"]
  to   = truenas_app_config.cloudflared
}

locals {
  nextcloud_app_config = {
    TZ                         = "Asia/Tokyo"
    ix_certificate_authorities = {}
    ix_certificates            = {}
    ix_context = {
      app_metadata = {
        annotations = {
          min_scale_version = "24.10.2.2"
        }
        app_version = "33.0.5"
        capabilities = [
          {
            description = "Cron, Nextcloud, Nginx are able to change file ownership arbitrarily"
            name        = "CHOWN"
          },
          {
            description = "Cron, Nextcloud, Nginx are able to bypass file permission checks"
            name        = "DAC_OVERRIDE"
          },
          {
            description = "Cron, Nextcloud, Nginx are able to bypass permission checks for file operations"
            name        = "FOWNER"
          },
          {
            description = "Cron, Nextcloud, Nginx are able to bind to privileged ports (< 1024)"
            name        = "NET_BIND_SERVICE"
          },
          {
            description = "Cron, Nextcloud, Nginx are able to use raw and packet sockets"
            name        = "NET_RAW"
          },
          {
            description = "Cron, Nextcloud, Nginx are able to change group ID of processes"
            name        = "SETGID"
          },
          {
            description = "Cron, Nextcloud, Nginx are able to change user ID of processes"
            name        = "SETUID"
          },
          {
            description = "Imaginary is able to modify process scheduling priority"
            name        = "SYS_NICE"
          },
        ]
        categories       = ["productivity"]
        changelog_url    = "https://nextcloud.com/changelog/"
        date_added       = "2024-08-07"
        description      = "A file sharing server that puts the control and security of your own data back into your hands."
        home             = "https://nextcloud.com/"
        host_mounts      = []
        icon             = "https://media.sys.truenas.net/apps/nextcloud/icons/icon.svg"
        keywords         = ["nextcloud", "storage", "sync", "http", "web", "php"]
        lib_version      = "2.3.4"
        lib_version_hash = "2e3a8847308fb2eb0da046018f287c73822c094b5950a10377c3235794ff1242"
        maintainers = [
          {
            email = "dev@truenas.com"
            name  = "truenas"
            url   = "https://www.truenas.com/"
          },
        ]
        name = "nextcloud"
        run_as_context = [
          {
            description = "Container [cron] runs as root user and group."
            gid         = 0
            group_name  = "Host group is [root]"
            uid         = 0
            user_name   = "Host user is [root]"
          },
          {
            description = "Container [imaginary] can run as any non-root user and group."
            gid         = 568
            group_name  = "Host group is [apps]"
            uid         = 568
            user_name   = "Host user is [apps]"
          },
          {
            description = "Container [nextcloud] runs as root user and group."
            gid         = 0
            group_name  = "Host group is [root]"
            uid         = 0
            user_name   = "Host user is [root]"
          },
          {
            description = "Container [nginx] runs as root user and group."
            gid         = 0
            group_name  = "Host group is [root]"
            uid         = 0
            user_name   = "Host user is [root]"
          },
          {
            description = "Container [postgres] runs as non-root user and group."
            gid         = 999
            group_name  = "Host group is [docker]"
            uid         = 999
            user_name   = "Host user is [netdata]"
          },
          {
            description = "Container [redis] can run as any non-root user and group."
            gid         = 568
            group_name  = "Host group is [apps]"
            uid         = 568
            user_name   = "Host user is [apps]"
          },
        ]
        screenshots = ["https://media.sys.truenas.net/apps/nextcloud/screenshots/screenshot1.png", "https://media.sys.truenas.net/apps/nextcloud/screenshots/screenshot2.png", "https://media.sys.truenas.net/apps/nextcloud/screenshots/screenshot3.png"]
        sources     = ["https://apps.truenas.com/catalog/nextcloud_stable/", "https://github.com/nextcloud/docker"]
        title       = "Nextcloud"
        train       = "stable"
        version     = "2.3.39"
      }
      app_name         = "nextcloud"
      is_install       = false
      is_rollback      = false
      is_update        = true
      is_upgrade       = false
      operation        = "UPDATE"
      scale_version    = "TrueNAS-25.10.3.1"
      upgrade_metadata = {}
    }
    ix_volumes = {}
    labels     = []
    network = {
      certificate_id = null
      dns_opts       = []
      networks       = []
      web_port = {
        bind_mode   = "published"
        host_ips    = []
        port_number = 30027
      }
    }
    nextcloud = {
      additional_envs = []
      admin_password  = var.truenas_nextcloud_admin_password
      admin_user      = "root"
      apt_packages    = []
      cron = {
        enabled = false
      }
      data_dir_path = "/var/www/html/data"
      db_password   = var.truenas_nextcloud_db_password
      db_user       = "nextcloud"
      host          = ""
      imaginary = {
        enabled = false
      }
      max_execution_time               = 30
      op_cache_interned_strings_buffer = 32
      op_cache_memory_consumption      = 128
      php_memory_limit                 = 512
      php_upload_limit                 = 3
      postgres_image_selector          = "postgres_18_image"
      redis_password                   = var.truenas_nextcloud_redis_password
      tesseract_languages              = []
    }
    release_name = "nextcloud"
    resources = {
      gpus = {
        kfd_device_exists    = false
        nvidia_gpu_selection = {}
        use_all_gpus         = false
      }
      limits = {
        cpus   = 2
        memory = 4096
      }
    }
    storage = {
      additional_storage = [
        {
          host_path_config = {
            acl_enable = false
            path       = "/mnt/apps/apps/nextcloud/config"
          }
          mount_path = "/var/www/html/config"
          read_only  = false
          type       = "host_path"
        },
      ]
      data = {
        host_path_config = {
          acl_enable = false
          path       = "/mnt/apps/apps/nextcloud/userdata"
        }
        type = "host_path"
      }
      html = {
        host_path_config = {
          acl_enable = false
          path       = "/mnt/apps/apps/nextcloud/appdata"
        }
        type = "host_path"
      }
      postgres_data = {
        host_path_config = {
          acl_enable       = false
          auto_permissions = true
          path             = "/mnt/apps/apps/nextcloud/pgdata"
        }
        type = "host_path"
      }
    }
  }

  cloudflared_app_config = {
    cloudflared = {
      additional_args = []
      additional_envs = []
      tunnel_token    = var.truenas_cloudflared_tunnel_token
    }
    ix_certificate_authorities = {}
    ix_certificates            = {}
    ix_context = {
      app_metadata = {
        annotations = {
          min_scale_version = "24.10.2.2"
        }
        app_version      = "2026.6.0"
        capabilities     = []
        categories       = ["networking"]
        changelog_url    = "https://github.com/cloudflare/cloudflared/blob/master/RELEASE_NOTES"
        date_added       = "2024-08-02"
        description      = "Cloudflared is a client for Cloudflare Tunnel, a daemon that exposes private services through the Cloudflare edge."
        home             = "https://github.com/cloudflare/cloudflared"
        host_mounts      = []
        icon             = "https://media.sys.truenas.net/apps/cloudflared/icons/icon.svg"
        keywords         = ["network", "cloudflare", "tunnel"]
        lib_version      = "2.3.4"
        lib_version_hash = "2e3a8847308fb2eb0da046018f287c73822c094b5950a10377c3235794ff1242"
        maintainers = [
          {
            email = "dev@truenas.com"
            name  = "truenas"
            url   = "https://www.truenas.com/"
          },
        ]
        name = "cloudflared"
        run_as_context = [
          {
            description = "Container [cloudflared] can run as any non-root user and group."
            gid         = 568
            group_name  = "Host group is [apps]"
            uid         = 568
            user_name   = "Host user is [apps]"
          },
        ]
        screenshots = []
        sources     = ["https://apps.truenas.com/catalog/cloudflared_community/", "https://github.com/cloudflare/cloudflared", "https://hub.docker.com/r/cloudflare/cloudflared"]
        title       = "Cloudflared"
        train       = "community"
        version     = "2.0.9"
      }
      app_name         = "cloudflared"
      is_install       = false
      is_rollback      = false
      is_update        = true
      is_upgrade       = false
      operation        = "UPDATE"
      scale_version    = "TrueNAS-25.10.3.1"
      upgrade_metadata = {}
    }
    ix_volumes = {}
    labels     = []
    network = {
      host_network = true
    }
    release_name = "cloudflared"
    resources = {
      limits = {
        cpus   = 2
        memory = 4096
      }
    }
    run_as = {
      group = 568
      user  = 568
    }
    storage = {
      additional_storage = []
    }
  }

}

data "truenas_app_config_document" "nextcloud" {
  config = local.nextcloud_app_config
}

data "truenas_app_config_document" "cloudflared" {
  config = local.cloudflared_app_config
}

resource "truenas_app_config" "nextcloud" {
  name        = "nextcloud"
  config_json = data.truenas_app_config_document.nextcloud.json
}

resource "truenas_app_config" "cloudflared" {
  name        = "cloudflared"
  config_json = data.truenas_app_config_document.cloudflared.json
}
