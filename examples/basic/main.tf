terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.1.3"
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

resource "systemd_unit" "demo" {
  name   = "terraform-demo.service"
  enable = true
  active = false

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "Terraform demo unit"
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
      value = "/bin/true"
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

resource "systemd_network" "eth0" {
  filename = "10-eth0.network"

  section {
    name = "Match"
    entry {
      key   = "Name"
      value = "eth0"
    }
  }
  section {
    name = "Network"
    entry {
      key   = "DHCP"
      value = "yes"
    }
  }
}
