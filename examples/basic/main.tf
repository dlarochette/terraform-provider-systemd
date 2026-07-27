terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.3.0"
    }
  }
}

provider "systemd" {
  host                     = var.host
  user                     = var.user
  insecure_ignore_host_key = var.insecure
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

resource "systemd_unit" "backup" {
  name   = "backup.service"
  enable = true
  active = false

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "Nightly backup"
    }
  }
  section {
    name = "Service"
    entry {
      key   = "Type"
      value = "oneshot"
    }
    entry {
      key   = "ExecStart"
      value = "/usr/local/bin/backup.sh"
    }
  }
}

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

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "Data volume"
    }
  }
  section {
    name = "Mount"
    entry {
      key   = "What"
      value = "/dev/disk/by-label/DATA"
    }
    entry {
      key   = "Where"
      value = "/data"
    }
    entry {
      key   = "Type"
      value = "ext4"
    }
  }
  section {
    name = "Install"
    entry {
      key   = "WantedBy"
      value = "multi-user.target"
    }
  }
}

resource "systemd_automount" "data" {
  name   = "data.automount"
  enable = true
  active = true

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "Automount /data"
    }
  }
  section {
    name = "Automount"
    entry {
      key   = "Where"
      value = "/data"
    }
  }
  section {
    name = "Install"
    entry {
      key   = "WantedBy"
      value = "multi-user.target"
    }
  }
}

resource "systemd_socket" "backup" {
  name   = "backup.socket"
  enable = true
  active = true

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "Backup socket activation"
    }
  }
  section {
    name = "Socket"
    entry {
      key   = "ListenStream"
      value = "8080"
    }
  }
  section {
    name = "Install"
    entry {
      key   = "WantedBy"
      value = "sockets.target"
    }
  }
}

resource "systemd_unit" "app_template" {
  name = "app@.service"

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "App instance %i"
    }
  }
  section {
    name = "Service"
    entry {
      key   = "ExecStart"
      value = "/usr/local/bin/app %i"
    }
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

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "App using a host credential"
    }
  }
  section {
    name = "Service"
    entry {
      key   = "LoadCredentialEncrypted"
      value = "db-pass"
    }
    entry {
      key   = "ExecStart"
      value = "/usr/local/bin/app"
    }
  }
}

resource "systemd_target" "app" {
  name   = "app.target"
  enable = true
  active = true

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "Application stack"
    }
    entry {
      key   = "Wants"
      value = "backup.service"
    }
  }
  section {
    name = "Install"
    entry {
      key   = "WantedBy"
      value = "multi-user.target"
    }
  }
}
