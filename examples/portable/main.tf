terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.8.0"
    }
  }
}

provider "systemd" {
  host = "host.example.com"
  user = "root"
}

# Local portable image (tar of an OS tree with matching unit prefix + os-release).
resource "systemd_portable" "app" {
  name         = "app"
  enable       = true
  active       = false
  delete_image = true

  image {
    type   = "local"
    source = "/var/tmp/app.tar"
  }
}
