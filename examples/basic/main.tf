terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.1.0"
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
  content = <<-EOT
    [Unit]
    Description=Terraform demo unit
    [Service]
    Type=oneshot
    ExecStart=/bin/true
    [Install]
    WantedBy=multi-user.target
  EOT
}

resource "systemd_network" "eth0" {
  filename = "10-eth0.network"
  content  = <<-EOT
    [Match]
    Name=eth0
    [Network]
    DHCP=yes
  EOT
}
