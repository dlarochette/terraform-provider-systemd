# Terraform Provider: systemd

Manage **systemd units** and **systemd-networkd** files on remote Linux hosts over SSH — same idea as `systemctl --host=user@host`.

| | |
|---|---|
| **Provider address** | `dlarochette/systemd` |
| **Go module** | `github.com/dlarochette/terraform-provider-systemd` |
| **Repository** | https://github.com/dlarochette/terraform-provider-systemd |
| **Latest release** | [0.8.0](https://github.com/dlarochette/terraform-provider-systemd/releases/tag/0.8.0) |
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
      version = ">= 0.8.0"
    }
  }
}

provider "systemd" {
  alias = "ymir"
  host  = "ymir.example"
  user  = "root"
}
```

### Raw `content`

```hcl
resource "systemd_unit" "demo" {
  provider = systemd.ymir
  name     = "demo.service"
  enable   = true
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

### Structured `section` blocks

`content` and `section` are mutually exclusive. Duplicate keys (e.g. multiple `ExecStart`) are allowed.

```hcl
resource "systemd_unit" "demo" {
  provider = systemd.ymir
  name     = "demo.service"
  enable   = true

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "Demo unit managed by Terraform"
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
```

The same `section` / `entry` model works on `systemd_timer`, `systemd_mount`, `systemd_automount`, `systemd_socket`, `systemd_path`, `systemd_swap`, `systemd_slice`, `systemd_target`, `systemd_dropin`, and the networkd resources.

### Machines (systemd-nspawn)

`systemd_machine` manages an nspawn image (`machinectl import-*` / `pull-tar` / `pull-raw` / `pull-dkr`), optional
`/etc/systemd/nspawn/{name}.nspawn` settings (`content` XOR `section`), and `systemd-nspawn@{name}.service`
via `systemctl`. On destroy, `delete_image` defaults to `true` (`machinectl remove`).

```hcl
resource "systemd_machine" "demo" {
  name         = "demo"
  enable       = true
  active       = true
  delete_image = true

  image {
    type   = "local" # or tar | raw | oci
    source = "/var/tmp/demo.tar"
  }

  content = <<-EOT
    [Exec]
    Boot=no
    PrivateUsers=no
    Parameters=/usr/bin/sleep infinity
  EOT
}
```

OCI example: `image { type = "oci", source = "docker.io/library/debian:bookworm" }` (`machinectl pull-dkr`).
Acceptance tests use a local tar fixture only (no registry pull in CI). Nested container
**start** is not exercised in ACC (cgroup mounts fail inside the outer nspawn guest); ACC
covers image import, `.nspawn` write/update, enable, and `delete_image`.

### Portable services

`systemd_portable` materializes an image under `/var/lib/portables/{name}` (or `{name}.raw`) and
attaches it with `portablectl` (`--profile=trusted`). `image.type` is `local` | `tar` | `raw`
(`oci` is not supported for portable images).

```hcl
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
```

The image must contain matching unit files (prefix = image name) and an `os-release`. Unit
overrides after attach use existing `systemd_dropin` / unit resources.

### Template units and instances

`systemd_unit` (and the other typed unit resources) also accept template unit names like `app@.service`.
Use `systemd_instance` to manage the enable/active lifecycle of a specific instance (e.g. `app@bar.service`)
without duplicating the unit file per instance:

```hcl
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
```

### Credentials

`systemd_credential` writes a secret to the host credential store, consumable by units via
`LoadCredential=` (plaintext) or `LoadCredentialEncrypted=` (encrypted, the default). `data` is
never read back or decrypted from the remote host — but it **is persisted in Terraform state**
(marked `Sensitive`, so it's redacted from CLI output/logs, not from the state file itself).
Keep `data` out of version control (e.g. a `sensitive` variable, as below) and treat your
Terraform state as a secret.

Changing `encrypted` forces replacement (`RequiresReplace`): flipping it switches the remote
path between `/etc/credstore` and `/etc/credstore.encrypted`, so Terraform destroys the
credential at the old path before creating it at the new one instead of leaving an orphan
behind.

Two more things this resource does *not* do, since there's no way to signal it to the OS or
existing processes over SSH:

- **No import**: `systemd_credential` has no `ImportState` — the content is unrecoverable
  from the remote host (it's encrypted, or simply never read back), so there is nothing
  Terraform could populate `data` with.
- **No consumer restart on rotation**: writing a new value only updates the file in the
  credstore. Units already running with `LoadCredential=`/`LoadCredentialEncrypted=` keep
  the credential they loaded at their last start; restart them yourself (e.g. via
  `systemd_instance` or an explicit `terraform apply` step) to pick up the new value.

```hcl
resource "systemd_credential" "db" {
  name      = "db-pass"
  data      = var.db_password # sensitive
  encrypted = true
  with_key  = "host"
}

resource "systemd_unit" "app" {
  name = "app.service"
  # …
  section {
    name = "Service"
    entry {
      key   = "LoadCredentialEncrypted"
      value = "db-pass"
    }
  }
}
```

Relative credential names search `/etc/credstore.encrypted/` (or `/etc/credstore/` for
`LoadCredential=`). You can also pass an absolute path:
`db-pass:/etc/credstore.encrypted/db-pass`.

### networkd (`.network` / `.netdev` / `.link`)

Same `section` / `entry` (or raw `content`) model. Apply writes under `/etc/systemd/network/`
then runs `networkctl reload`. There is no `enable`/`active` — networkd picks files up by
name/Match.

```hcl
resource "systemd_link" "eth0" {
  filename = "10-eth0.link"
  section {
    name = "Match"
    entry {
      key   = "MACAddress"
      value = "aa:bb:cc:dd:ee:ff"
    }
  }
  section {
    name = "Link"
    entry {
      key   = "Name"
      value = "eth0"
    }
  }
}

resource "systemd_netdev" "br0" {
  filename = "20-br0.netdev"
  section {
    name = "NetDev"
    entry {
      key   = "Name"
      value = "br0"
    }
    entry {
      key   = "Kind"
      value = "bridge"
    }
  }
}

resource "systemd_network" "br0" {
  filename = "30-br0.network"
  section {
    name = "Match"
    entry {
      key   = "Name"
      value = "br0"
    }
  }
  section {
    name = "Network"
    entry {
      key   = "Address"
      value = "192.0.2.10/24"
    }
    entry {
      key   = "Gateway"
      value = "192.0.2.1"
    }
  }
}

data "systemd_link" "br0" {
  name = "br0"
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
| `systemd_unit` | `/etc/systemd/system/{name}` | Generic units (`.service`, `.socket`, …); optional `enable` / `active` |
| `systemd_timer` | `/etc/systemd/system/{name}` | `.timer` only; pair with a `.service` |
| `systemd_mount` | `/etc/systemd/system/{name}` | `.mount` only (e.g. `data.mount` → `/data`) |
| `systemd_automount` | `/etc/systemd/system/{name}` | `.automount` only; usually with a `.mount` |
| `systemd_socket` | `/etc/systemd/system/{name}` | `.socket` only; pair with a `.service` |
| `systemd_path` | `/etc/systemd/system/{name}` | `.path` only; pair with a `.service` |
| `systemd_swap` | `/etc/systemd/system/{name}` | `.swap` only |
| `systemd_slice` | `/etc/systemd/system/{name}` | `.slice` only; cgroup hierarchy |
| `systemd_target` | `/etc/systemd/system/{name}` | `.target` only; grouping / sync point |
| `systemd_dropin` | `/etc/systemd/system/{unit}.d/{dropin}` | `dropin` must end with `.conf` |
| `systemd_instance` | n/a (lifecycle only) | Enable/start a template unit instance (e.g. `app@bar.service`); `template` + `instance`, both `RequiresReplace` |
| `systemd_network` | `/etc/systemd/network/{filename}` | Filename must end with `.network` |
| `systemd_netdev` | `/etc/systemd/network/{filename}` | Filename must end with `.netdev` |
| `systemd_link` | `/etc/systemd/network/{filename}` | Filename must end with `.link` |
| `systemd_credential` | `/etc/credstore{,.encrypted}/{name}` | `data` is `Sensitive`, persisted in state, never read back; `encrypted` defaults to `true` and forces replacement; not importable |

Destroy: stop/disable (best effort) → remove file → `daemon-reload` (and `networkctl reload` for networkd files).
`systemd_instance` is lifecycle-only (no file of its own): destroy just stops/disables the instance,
there is nothing to remove on disk.
`systemd_credential` is not a unit, so its destroy is a carve-out: it only removes the credential
file at the path matching state (`/etc/credstore/{name}` or `/etc/credstore.encrypted/{name}`) —
no stop/disable and no `daemon-reload`.

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

## Acceptance tests

Unit tests: `make test` (fake host, no privileges).

ACC tests exercise the provider's resources against a **real systemd** running inside a
`systemd-nspawn` container (not SSH — see [`internal/remote/nspawn.go`](internal/remote)):

```bash
sudo apt-get install -y systemd-container debootstrap dbus
make testacc
```

The first run debootstraps a Debian rootfs under `/var/lib/machines/`, which takes a few
minutes; subsequent runs reuse it. Env vars (see [`scripts/acc-nspawn.sh`](scripts/acc-nspawn.sh)):

| Var | Default | Purpose |
|-----|---------|---------|
| `SYSTEMD_ACC_MACHINE` | `tf-systemd-acc` | nspawn machine name |
| `ACC_REBUILD` | unset | `=1` forces a rootfs rebuild |
| `ACC_WIPE` | unset | `=1` removes the machine image after the run |

CI and releases run on GitHub Actions. Tags use Semantic Versioning **without** a `v` prefix (`0.1.1`, not `v0.1.1`). Pushing a tag builds and publishes binaries with GoReleaser. The `testacc` job must pass before a tag release publishes.

## License

[MIT](LICENSE) © David Larochette
