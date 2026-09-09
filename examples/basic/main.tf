terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.6.0"
    }
  }
}

provider "systemd" {
  host                     = var.host
  user                     = var.user
  insecure_ignore_host_key = var.insecure
  verify                   = "warn"
  # systemd_version = "251" # pin the target systemd release instead of detecting it
}

variable "host" {
  type = string
}

variable "user" {
  type    = string
  default = "root"
}

variable "insecure" {
  type    = bool
  default = false
}

variable "db_password" {
  type      = string
  sensitive = true
}

# Typed properties: one block per systemd section, snake_case directive
# attributes with systemd validation rules.
resource "systemd_unit" "backup" {
  name   = "backup.service"
  enable = true
  active = false

  unit {
    description = "Nightly backup"
  }
  service {
    type       = "oneshot"
    exec_start = ["/usr/local/bin/backup.sh"]
  }
}

# The same file written with generic section blocks (equivalent result).
resource "systemd_timer" "backup" {
  name   = "backup.timer"
  enable = true
  active = true

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "Run backup.service nightly"
    }
  }
  section {
    name = "Timer"
    entry {
      key   = "OnCalendar"
      value = "*-*-* 02:30:00"
    }
    entry {
      key   = "Persistent"
      value = "true"
    }
  }
  section {
    name = "Install"
    entry {
      key   = "WantedBy"
      value = "timers.target"
    }
  }
}

resource "systemd_mount" "data" {
  name   = "data.mount"
  enable = true
  active = true

  unit {
    description = "Data volume"
  }
  mount {
    what  = "/dev/disk/by-label/DATA"
    where = "/data"
    type  = "ext4"
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}

resource "systemd_automount" "data" {
  name   = "data.automount"
  enable = true
  active = true

  unit {
    description = "Automount /data"
  }
  automount {
    where = "/data"
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}

resource "systemd_socket" "backup" {
  name   = "backup.socket"
  enable = true
  active = true

  unit {
    description = "Backup socket activation"
  }
  socket {
    listen_stream = ["8080"]
  }
  install {
    wanted_by = ["sockets.target"]
  }
}

resource "systemd_unit" "watch_inbox" {
  name   = "watch-inbox.service"
  enable = false
  active = false

  unit {
    description = "Process new inbox files"
  }
  service {
    type       = "oneshot"
    exec_start = ["/usr/local/bin/process-inbox.sh"]
  }
}

resource "systemd_path" "watch_inbox" {
  name   = "watch-inbox.path"
  enable = true
  active = true

  unit {
    description = "Watch /var/inbox for new files"
  }
  path {
    path_exists         = "/var/inbox"
    directory_not_empty = "/var/inbox"
  }
  install {
    wanted_by = ["paths.target"]
  }
}

resource "systemd_slice" "app" {
  name   = "app.slice"
  enable = true
  active = true

  unit {
    description = "Application workload slice"
  }
  slice {
    memory_max = "1G"
  }
}

resource "systemd_swap" "var_swap" {
  name   = "var-swapfile.swap"
  enable = true
  active = false

  unit {
    description = "Swap file under /var"
  }
  swap {
    what = "/var/swapfile"
  }
  install {
    wanted_by = ["swap.target"]
  }
}

resource "systemd_unit" "app_template" {
  name = "app@.service"

  unit {
    description = "App instance %i"
  }
  service {
    exec_start = ["/usr/local/bin/app %i"]
  }
}

resource "systemd_instance" "app_bar" {
  template = systemd_unit.app_template.name
  instance = "bar"
  enable   = true
  active   = true
}

resource "systemd_credential" "db" {
  name      = "db-pass"
  data      = var.db_password
  encrypted = true
  with_key  = "host"
}

resource "systemd_unit" "app" {
  name = "app.service"

  unit {
    description = "App using a host credential"
  }
  service {
    exec_start                = ["/usr/local/bin/app"]
    load_credential_encrypted = ["db-pass"]
  }
}

resource "systemd_target" "app" {
  name   = "app.target"
  enable = true
  active = true

  unit {
    description = "Application stack"
    wants       = ["backup.service"]
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}

# --- systemd-networkd ---

resource "systemd_link" "eth0" {
  filename = "10-eth0.link"

  match {
    mac_address = "aa:bb:cc:dd:ee:ff"
  }
  link {
    name = "eth0"
  }
}

resource "systemd_netdev" "br0" {
  filename = "20-br0.netdev"

  netdev {
    name = "br0"
    kind = "bridge"
  }
}

resource "systemd_network" "br0" {
  filename = "30-br0.network"

  match {
    name = "br0"
  }
  network {
    address = "192.0.2.10/24"
    gateway = "192.0.2.1"
    dns     = "9.9.9.9"
  }
}
