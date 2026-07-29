terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.9.0"
    }
  }
}

provider "systemd" {
  host = "ymir.example"
  user = "root"
}

# Global resolved.conf (singleton). Destroy removes the file and restarts the service.
resource "systemd_resolved" "main" {
  section {
    name = "Resolve"
    entry {
      key   = "DNS"
      value = "1.1.1.1"
    }
    entry {
      key   = "FallbackDNS"
      value = "9.9.9.9"
    }
  }
}

resource "systemd_resolved_dropin" "lab" {
  name = "10-lab.conf"
  section {
    name = "Resolve"
    entry {
      key   = "Domains"
      value = "~lab.example"
    }
  }
}

# Runtime-only: cleared on reboot unless also set in a .network file.
resource "systemd_resolve_link" "eth0" {
  link          = "eth0"
  dns           = ["1.1.1.1", "1.0.0.1"]
  domains       = ["~example.com", "lan"]
  default_route = true
  llmnr         = "no"
}

data "systemd_resolve_status" "eth0" {
  link = "eth0"
}

output "resolve_status" {
  value = data.systemd_resolve_status.eth0.status
}
