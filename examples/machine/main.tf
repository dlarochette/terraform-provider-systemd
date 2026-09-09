terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.7.0"
    }
  }
}

provider "systemd" {
  host = "host.example.com"
  user = "root"
}

# Local tar import (path is on the remote host). For OCI use:
#   image { type = "oci", source = "docker.io/library/debian:bookworm" }
resource "systemd_machine" "demo" {
  name         = "demo"
  enable       = true
  active       = true
  delete_image = true

  image {
    type   = "local"
    source = "/var/tmp/demo.tar"
  }

  content = <<-EOT
    [Exec]
    Boot=no
    PrivateUsers=no
    Parameters=/usr/bin/sleep infinity
  EOT
}
