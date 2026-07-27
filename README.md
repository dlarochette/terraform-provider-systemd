# Terraform Provider: systemd

Manage **systemd units** and **systemd-networkd** files on remote Linux hosts over SSH — same idea as `systemctl --host=user@host`.

| | |
|---|---|
| **Provider address** | `dlarochette/systemd` |
| **Repository** | https://github.com/dlarochette/terraform-provider-systemd |
| **Latest release** | [0.1.1](https://github.com/dlarochette/terraform-provider-systemd/releases/tag/0.1.1) |
| **License** | MIT |

## How it works

1. The provider opens an **SSH** session to the target host (optional bastion).
2. Unit / network files are written with **SFTP** (atomic temp file + rename).
3. Desired state is applied with remote **`systemctl`** and **`networkctl`**.

No agent, no Unix socket API, no extra daemon on the host. Root SSH is assumed for the MVP.

Design notes: [docs/superpowers/specs/2026-07-27-terraform-systemd-design.md](docs/superpowers/specs/2026-07-27-terraform-systemd-design.md).

## Requirements

- Terraform ≥ 1.5 or OpenTofu ≥ 1.6
- Target host: Linux with systemd; `networkctl` if you manage networkd files
- SSH access as a user that can write `/etc/systemd` and run `systemctl` (typically `root`)

## Install

The provider is not on the public Terraform Registry yet. Install a [GitHub Release](https://github.com/dlarochette/terraform-provider-systemd/releases) binary, or use `dev_overrides` while developing:

```hcl
# ~/.terraformrc  (or ~/.tofurc)
provider_installation {
  dev_overrides {
    "dlarochette/systemd" = "/absolute/path/to/terraform-provider-systemd/bin"
  }
  direct {}
}
```

Build locally:

```bash
git clone https://github.com/dlarochette/terraform-provider-systemd.git
cd terraform-provider-systemd
make build   # → bin/terraform-provider-systemd
```

Terraform expects the binary name `terraform-provider-systemd` in that directory.

## Quick start

```hcl
terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.1.1"
    }
  }
}

provider "systemd" {
  alias = "ymir"
  host  = "ymir.example"
  user  = "root"
  # private_key_path = "~/.ssh/id_ed25519"
  # ssh_agent        = true
}

resource "systemd_unit" "demo" {
  provider = systemd.ymir
  name     = "demo.service"
  enable   = true
  active   = false
  content  = <<-EOT
    [Unit]
    Description=Demo unit managed by Terraform
    [Service]
    Type=oneshot
    ExecStart=/bin/true
    [Install]
    WantedBy=multi-user.target
  EOT
}
```

See also [`examples/basic`](examples/basic).

## Provider configuration

| Attribute | Required | Description |
|-----------|----------|-------------|
| `host` | yes | SSH hostname or IP |
| `user` | no | SSH user (default `root`) |
| `port` | no | SSH port (default `22`) |
| `private_key` | no | PEM private key contents (conflicts with `private_key_path`) |
| `private_key_path` | no | Path to PEM private key |
| `ssh_agent` | no | Use `SSH_AUTH_SOCK` (default true when no key is set) |
| `bastion_host` | no | Jump host |
| `bastion_user` | no | Jump user (defaults to `user`) |
| `bastion_port` | no | Jump port (default `22`) |
| `insecure_ignore_host_key` | no | Skip `known_hosts` (lab only) |

Use a **provider alias per host** when managing a fleet.

## Resources

| Resource | Remote path | Notes |
|----------|-------------|-------|
| `systemd_unit` | `/etc/systemd/system/{name}` | Optional `enable` / `active`; start failure fails apply |
| `systemd_dropin` | `/etc/systemd/system/{unit}.d/{dropin}` | `dropin` must end with `.conf` |
| `systemd_network` | `/etc/systemd/network/{filename}` | Filename must end with `.network` |
| `systemd_netdev` | `/etc/systemd/network/{filename}` | Filename must end with `.netdev` |
| `systemd_link` | `/etc/systemd/network/{filename}` | Filename must end with `.link` |

Destroy: stop/disable (best effort) → remove file → `daemon-reload` / `networkctl reload`.

## Data sources

| Data source | Backend |
|-------------|---------|
| `systemd_unit` | `systemctl show` (`load_state`, `active_state`, `sub_state`, `unit_file_state`) |
| `systemd_link` | `networkctl status` (`operational_state`, `setup_state`) |

## Schema validation / linting

Provider and resource schemas include validators (unit suffixes, network filename suffixes, path-segment safety, port ranges, key conflicts) so `terraform validate` / `tofu validate` can catch bad `.tf` configs early.

```bash
make schema   # dumps schemas/provider.json for IDE / custom lint tooling
```

## Development

```bash
make test
make build
make schema
```

CI and releases run on GitHub Actions. Tags use Semantic Versioning **without** a `v` prefix (`0.1.1`, not `v0.1.1`). Pushing a tag builds and publishes binaries with GoReleaser.

## License

[MIT](LICENSE) © David Larochette
