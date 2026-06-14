locals {
  nextcloud_userdata_path = "/mnt/tank/apps/tnextcloud/userdata"
  calibre_data_path       = "/mnt/tank/apps/calibre"

  nextcloud_userdata_posix_acl = [
    {
      tag     = "USER_OBJ"
      default = false
      id      = -1
      who     = null
      perms = {
        READ    = true
        WRITE   = true
        EXECUTE = true
      }
    },
    {
      tag     = "USER"
      default = false
      id      = 3000
      who     = null
      perms = {
        READ    = true
        WRITE   = true
        EXECUTE = true
      }
    },
    {
      tag     = "GROUP_OBJ"
      default = false
      id      = -1
      who     = null
      perms = {
        READ    = true
        WRITE   = true
        EXECUTE = true
      }
    },
    {
      tag     = "MASK"
      default = false
      id      = -1
      who     = null
      perms = {
        READ    = true
        WRITE   = true
        EXECUTE = true
      }
    },
    {
      tag     = "OTHER"
      default = false
      id      = -1
      who     = null
      perms = {
        READ    = false
        WRITE   = false
        EXECUTE = false
      }
    },
    {
      tag     = "USER_OBJ"
      default = true
      id      = -1
      who     = null
      perms = {
        READ    = true
        WRITE   = true
        EXECUTE = true
      }
    },
    {
      tag     = "USER"
      default = true
      id      = 3000
      who     = null
      perms = {
        READ    = true
        WRITE   = true
        EXECUTE = true
      }
    },
    {
      tag     = "GROUP_OBJ"
      default = true
      id      = -1
      who     = null
      perms = {
        READ    = true
        WRITE   = true
        EXECUTE = true
      }
    },
    {
      tag     = "MASK"
      default = true
      id      = -1
      who     = null
      perms = {
        READ    = true
        WRITE   = true
        EXECUTE = true
      }
    },
    {
      tag     = "OTHER"
      default = true
      id      = -1
      who     = null
      perms = {
        READ    = false
        WRITE   = false
        EXECUTE = false
      }
    },
  ]
}

resource "truenas_filesystem_acl" "calibre" {
  path      = local.calibre_data_path
  uid       = 568
  gid       = 568
  acltype   = "POSIX1E"
  acl_json  = jsonencode(local.nextcloud_userdata_posix_acl)
  recursive = true

  depends_on = [
    truenas_dataset.datasets["tank_apps_calibre"],
    truenas_dataset.datasets["tank_apps_tnextcloud_userdata"],
  ]
}

resource "truenas_smb_share_copy" "calibre" {
  source_path = local.nextcloud_userdata_path
  name        = "calibre"
  path        = local.calibre_data_path

  depends_on = [
    truenas_filesystem_acl.calibre,
  ]
}
