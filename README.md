# Terraform Provider: systemd

Manage **systemd units** and **systemd-networkd** files on remote Linux hosts over SSH — same idea as `systemctl --host=user@host`.

| | |
|---|---|
| **Provider address** | `dlarochette/systemd` |
| **Go module** | `github.com/dlarochette/terraform-provider-systemd` |
| **Repository** | https://github.com/dlarochette/terraform-provider-systemd |
| **Latest release** | [0.3.1](https://github.com/dlarochette/terraform-provider-systemd/releases/tag/0.3.1) |
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
      version = ">= 0.3.0"
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

The same `section` / `entry` model works on `systemd_timer`, `systemd_mount`, `systemd_automount`, `systemd_socket`, `systemd_target`, `systemd_dropin`, and the networkd resources.

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

A relative `LoadCredentialEncrypted=db-pass` (no `/`) makes systemd look up `db-pass` under
`/etc/credstore.encrypted/`; plaintext `LoadCredential=` looks under `/etc/credstore/`.

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

CI and releases run on GitHub Actions. Tags use Semantic Versioning **without** a `v` prefix (`0.1.1`, not `v0.1.1`). Pushing a tag builds and publishes binaries with GoReleaser.

## License

[MIT](LICENSE) © David Larochette
