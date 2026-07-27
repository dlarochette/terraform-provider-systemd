# Terraform Provider: systemd

Manage **systemd units** and **systemd-networkd** files on remote Linux hosts over SSH.

Provider address: `dlarochette/systemd`  
Module: `github.com/toxn/terraform-provider-systemd`  
Repository: https://github.com/toxn/terraform-provider-systemd

## How it works

Same idea as `systemctl --host=user@host`:

1. Provider opens an SSH session to the target.
2. Unit / network files are written with **SFTP** (atomic temp + rename).
3. Desired state is applied with remote **`systemctl`** / **`networkctl`**.

No agent, no Unix socket, no extra daemon on the host.

See [design spec](docs/superpowers/specs/2026-07-27-terraform-systemd-design.md).

## Resources

| Resource | Purpose |
|----------|---------|
| `systemd_unit` | `/etc/systemd/system/{name}` + optional enable/active |
| `systemd_dropin` | drop-in under `{unit}.d/` |
| `systemd_network` | `.network` file |
| `systemd_netdev` | `.netdev` file |
| `systemd_link` | `.link` file |

Data sources: `systemd_unit` (`systemctl show`), `systemd_link` (`networkctl status`).

## Quick start

```hcl
terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.1.0"
    }
  }
}

provider "systemd" {
  alias = "ymir"
  host  = "ymir.example"
  user  = "root"
}

resource "systemd_unit" "demo" {
  provider = systemd.ymir
  name     = "demo.service"
  enable   = true
  content  = file("${path.module}/demo.service")
}
```

Until a public registry listing exists, use a [GitHub Release](https://github.com/toxn/terraform-provider-systemd/releases) binary or Terraform `dev_overrides`:

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "dlarochette/systemd" = "/path/to/terraform-provider-systemd/bin"
  }
  direct {}
}
```

## Build

```bash
make build   # bin/terraform-provider-systemd
make test
make schema  # writes schemas/provider.json for terraform validate / IDE lint
```

Provider and resource schemas include validators (unit suffixes, network filename suffixes, path-segment safety, port ranges, `private_key` vs `private_key_path` conflicts) so `terraform validate` / `tofu validate` can lint `.tf` configs against the provider schema.

## Tags / release

Tags use Semantic Versioning **without** a `v` prefix (`0.1.0`). Pushing a tag runs GoReleaser via GitHub Actions.

## License

See repository license file when published.
