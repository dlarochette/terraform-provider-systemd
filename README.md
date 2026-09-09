# Terraform Provider: systemd

Manage **systemd units**, **systemd-networkd**, and **systemd-resolved** on remote Linux hosts over SSH — same idea as `systemctl --host=user@host`.

| | |
|---|---|
| **Provider address** | `dlarochette/systemd` |
| **Go module** | `github.com/dlarochette/terraform-provider-systemd` |
| **Repository** | https://github.com/dlarochette/terraform-provider-systemd |
| **Releases** | https://github.com/dlarochette/terraform-provider-systemd/releases |
| **License** | MIT |

## How it works

1. The provider opens an **SSH** session to the target host (optional bastion).
2. Unit / network / resolved files are written with **SFTP** (atomic temp file + rename).
3. Desired state is applied with remote **`systemctl`**, **`networkctl`**, and **`resolvectl`**.

No agent, no Unix socket API, no extra daemon on the host. Root SSH is assumed for the MVP.

Design notes: [docs/superpowers/specs/2026-07-27-terraform-systemd-design.md](docs/superpowers/specs/2026-07-27-terraform-systemd-design.md).

## Requirements

- Terraform ≥ 1.5 or OpenTofu ≥ 1.6
- Target host: Linux with systemd; `networkctl` if you manage networkd files; `resolvectl` / `systemd-resolved` for resolved resources
- SSH access as a user that can write `/etc/systemd` and run `systemctl` (typically `root`)

## Install

From **0.10.1**, signed releases are published for the Terraform and OpenTofu registries
(`registry.terraform.io/dlarochette/systemd`, `registry.opentofu.org/dlarochette/systemd`).
Until both listings are live, you can still install a [GitHub Release](https://github.com/dlarochette/terraform-provider-systemd/releases) binary or use `dev_overrides` while developing:

```hcl
terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.10.1"
    }
  }
}
```

```hcl
# ~/.terraformrc  (or ~/.tofurc) — local development only
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

Publishing details: [docs/superpowers/specs/2026-07-30-registry-publish-design.md](docs/superpowers/specs/2026-07-30-registry-publish-design.md).

## Quick start

```hcl
terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.10.1"
    }
  }
}

provider "systemd" {
  alias = "main"
  host  = "host.example.com"
  user  = "root"
}
```

### Raw `content`

```hcl
resource "systemd_unit" "demo" {
  provider = systemd.main
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
  provider = systemd.main
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

### Typed systemd properties

For units, `systemd_unit` and every typed unit resource also expose **typed
properties**: one snake_case block per systemd section (`unit`, `service`,
`timer`, `socket`, `mount`, `install`, …) whose attributes are systemd
directives, with the same names (in snake_case) and the same validation rules
as the systemd parsers (enum values, booleans, time spans, byte sizes, octal
modes, signals, resource limits, weight ranges, int ranges):

```hcl
resource "systemd_unit" "demo" {
  name = "demo.service"

  unit {
    description = "Demo unit managed by Terraform"
    after       = ["network.target"]
  }
  service {
    type              = "oneshot"
    exec_start        = ["/bin/true"] # repeatable directives are lists
    timeout_start_sec = "2min"
    cpu_quota         = "50%"
    memory_max        = "2G"
    protect_home      = "read-only"
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}
```

The catalog is generated from the systemd `load-fragment` gperf table
(systemd v257): `go generate ./internal/sdprops`. The networkd resources
(`systemd_network`, `systemd_netdev`, `systemd_link`) follow the same model
with the networkd catalogs; repeatable sections (`[Address]`, `[Route]`, ...)
are list blocks. `systemd_resolved` and
`systemd_resolved_dropin` expose the typed `[Resolve]` block. Repeatable directives are
list attributes; other values are strings / bools / ints per the systemd
parser. `content`, `section` and typed blocks are mutually exclusive at file
level, and a directive set via a typed block cannot also be set via `section`.

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

### systemd-resolved

Global config and drop-ins use the same `content` XOR `section` model. Apply writes the file then
runs `systemctl restart systemd-resolved.service`. Destroying `systemd_resolved` **removes**
`/etc/systemd/resolved.conf` (vendor defaults apply again).

`systemd_resolve_link` is **runtime-only** (`resolvectl`); settings are cleared on reboot unless
also configured in a `.network` file. Destroy runs `resolvectl revert <link>`.

```hcl
resource "systemd_resolved" "main" {
  section {
    name = "Resolve"
    entry {
      key   = "DNS"
      value = "1.1.1.1"
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

resource "systemd_resolve_link" "eth0" {
  link          = "eth0"
  dns           = ["1.1.1.1", "1.0.0.1"]
  domains       = ["~example.com"]
  default_route = true
}

data "systemd_resolve_status" "eth0" {
  link = "eth0"
}
```

See also [`examples/resolved`](examples/resolved) and [`examples/basic`](examples/basic).

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
| `verify` | no | Unit file validation with the remote systemd parser: `off` / `warn` (default) / `error` (fails the apply and rolls the file back) |
| `systemd_version` | no | Pin the target systemd release (e.g. `251`) for directive availability checks instead of detecting it on the host |

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
| `systemd_machine` | image + `/etc/systemd/nspawn/{name}.nspawn` | nspawn image lifecycle (`machinectl`) + settings + `systemd-nspawn@` enable/active; `delete_image` default true |
| `systemd_portable` | `/var/lib/portables/{name}` + attach | `portablectl` attach/detach; image types `local` \| `tar` \| `raw` (no `oci`) |
| `systemd_resolved` | `/etc/systemd/resolved.conf` | Singleton; restart `systemd-resolved` on apply/destroy; destroy removes the file |
| `systemd_resolved_dropin` | `/etc/systemd/resolved.conf.d/{name}` | `name` must end with `.conf`; restart on apply/destroy |
| `systemd_resolve_link` | n/a (`resolvectl`) | Runtime-only per-link DNS; destroy → `resolvectl revert` |

Destroy: stop/disable (best effort) → remove file → `daemon-reload` (and `networkctl reload` for networkd files;
`systemctl restart systemd-resolved.service` for resolved files).
`systemd_instance` is lifecycle-only (no file of its own): destroy just stops/disables the instance,
there is nothing to remove on disk.
`systemd_credential` is not a unit, so its destroy is a carve-out: it only removes the credential
file at the path matching state (`/etc/credstore/{name}` or `/etc/credstore.encrypted/{name}`) —
no stop/disable and no `daemon-reload`.
`systemd_resolve_link` destroy only runs `resolvectl revert` (no file on disk).

## Data sources

| Data source | Backend |
|-------------|---------|
| `systemd_unit` | `systemctl show` (`load_state`, `active_state`, `sub_state`, `unit_file_state`) |
| `systemd_link` | `networkctl status` (`operational_state`, `setup_state`) |
| `systemd_resolve_status` | `resolvectl status [link]` (raw `status` text) |
| `systemd_version` | `systemctl --version` (`version`, `release`) |

## Examples

Reproductions of the official systemd examples ([systemd.service(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html),
[systemd.timer(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.timer.html),
[systemd.network(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.network.html), ...) in HCL and JSON syntax live in the
[examples guide](docs/guides/examples.md). The [`examples/`](examples) directory holds complete configurations.

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

ACC tests exercise the provider against a **real systemd** inside a `systemd-nspawn`
machine. Two transports:

| Target | Command | Host impl |
|--------|---------|-----------|
| nspawn (default) | `make testacc` | `remote.Nspawn` (`machinectl` / `systemd-run`) |
| SSH | `make testacc-ssh` | `remote.Dial` → guest `sshd` on `127.0.0.1:2222` |

```bash
sudo apt-get install -y systemd-container debootstrap dbus openssh-client
make testacc      # nspawn transport
make testacc-ssh  # same guest over SSH/SFTP
```

The first run debootstraps a Debian rootfs under `/var/lib/machines/`, which takes a few
minutes; subsequent runs reuse it. Env vars (see [`scripts/acc-nspawn.sh`](scripts/acc-nspawn.sh)
and [`docs/superpowers/specs/2026-07-30-acc-ssh-design.md`](docs/superpowers/specs/2026-07-30-acc-ssh-design.md)):

| Var | Default | Purpose |
|-----|---------|---------|
| `SYSTEMD_ACC_MACHINE` | `tf-systemd-acc` | nspawn machine name (nspawn ACC) |
| `ACC_REBUILD` | unset | `=1` forces a rootfs rebuild |
| `ACC_WIPE` | unset | `=1` removes the machine image after the run |
| `SYSTEMD_ACC_SSH_HOST` | (unset) | If set, ACC uses SSH instead of nspawn |
| `SYSTEMD_ACC_SSH_PORT` | `2222` | Guest sshd port (SSH ACC harness) |
| `SYSTEMD_ACC_SSH_KEY` | harness temp | Private key path (required with SSH host) |
| `SYSTEMD_ACC_SSH_INSECURE` | `1` (harness) | Skip `known_hosts` for lab Dial |

CI runs both `testacc` and `testacc-ssh` before a tag release. Tags use Semantic Versioning
**without** a `v` prefix (`0.1.1`, not `v0.1.1`).

## License

[MIT](LICENSE) © David Larochette
