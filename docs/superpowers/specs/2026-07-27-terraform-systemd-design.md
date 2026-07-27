# Design: Terraform provider systemd

Date: 2026-07-27  
Status: approved (SSH-only revision)  
Module: `github.com/dlarochette/terraform-provider-systemd`  
Provider address: `dlarochette/systemd`  
Binary: `terraform-provider-systemd`  
Remote: `github.com/dlarochette/terraform-provider-systemd`

## Goal

Manage systemd units and systemd-networkd configuration on a remote Linux fleet from Terraform/OpenTofu over **SSH only** — same idea as `systemctl --host=user@host`.

## Scope (MVP)

**In:**

- Units under `/etc/systemd/system/`
- Drop-ins under `/etc/systemd/system/{unit}.d/`
- networkd files under `/etc/systemd/network/` (`.network`, `.netdev`, `.link`)
- Enable / disable / start / stop via remote `systemctl`
- networkd reload / link status via remote `networkctl`
- Remote hosts reached by SSH (+ optional bastion)

**Out:**

- Custom remote agent / Unix socket / HTTP API
- sudo / non-root (root SSH assumed)
- Dedicated timer/mount/resolved resources
- Public Terraform Registry (phase 2)
- Bulk import of existing hosts
- D-Bus via `systemd-stdio-bridge` in-process (CLI is enough)

## Architecture

```text
Terraform  →  provider  →  SSH
                            ├─ SFTP  →  /etc/systemd/{system,network}
                            └─ exec  →  systemctl / networkctl
```

No bootstrap of an extra daemon. The provider dials SSH, writes files with SFTP (atomic temp+rename), then runs the same tools an admin would.

### Terraform schemas (lint `.tf`)

Provider / resource / data source schemas are the contract for `terraform validate` and editor tooling:

- Attribute validators (unit name suffixes, `.network`/`.netdev`/`.link` filenames, drop-in `.conf`, no path traversal, SSH port range)
- `private_key` conflicts with `private_key_path`
- Markdown descriptions on all attributes
- Dump: `make schema` → `schemas/provider.json` (`terraform providers schema -json`)
- Unit tests call `Schema.ValidateImplementation` for every type

### Host targeting

One provider configuration per host, using aliases:

```hcl
provider "systemd" {
  alias = "ymir"
  host  = "ymir.example"
  user  = "root"
}

resource "systemd_unit" "demo" {
  provider = systemd.ymir
  name     = "demo.service"
  content  = file("demo.service")
}
```

### HCL content model

Each file-backed resource accepts **either**:

- structured `section` / `entry` blocks (mapped to systemd INI), **or**
- a raw `content` string

These modes are mutually exclusive. Validation fails if both or neither are set. When using `section`, `content` is computed from the rendered INI after apply.

## Resources and data sources

| Type | Path / identity | Notes |
|------|-----------------|-------|
| `systemd_unit` | `/etc/systemd/system/{name}` | `name` is ID; optional enable/active |
| `systemd_dropin` | `/etc/systemd/system/{unit}.d/{dropin}.conf` | ID = `unit/dropin` |
| `systemd_network` | `/etc/systemd/network/{filename}` | ID = `filename` |
| `systemd_netdev` | `/etc/systemd/network/{filename}` | ID = `filename` |
| `systemd_link` | `/etc/systemd/network/{filename}` | ID = `filename` |
| data `systemd_unit` | `systemctl show` | load/active/sub/unit-file state |
| data `systemd_link` | `networkctl status` | operational/setup state |

Destroy: stop/disable (best effort) → remove file → `daemon-reload` / `networkctl reload`. Start failure fails the apply.

## Provider configuration schema

| Attribute | Required | Description |
|-----------|----------|-------------|
| `host` | yes | SSH hostname or IP |
| `user` | no | SSH user (default `root`) |
| `port` | no | SSH port (default `22`) |
| `private_key` | no | PEM private key contents |
| `private_key_path` | no | path to key file |
| `ssh_agent` | no | use local SSH agent (default true if no key) |
| `bastion_host` | no | optional jump host |
| `bastion_user` | no | jump user |
| `bastion_port` | no | jump port |
| `insecure_ignore_host_key` | no | skip known_hosts (dev only) |

## Package layout

```text
main.go
internal/provider/     Plugin Framework provider + resources
internal/remote/       SSH dial, SFTP, systemctl/networkctl
internal/unitfile/     systemd INI helpers
examples/
docs/
.github/workflows/ci.yml
.goreleaser.yml
```

## Release

- Tags: Semantic Versioning **without** `v` prefix (`0.1.0`).
- GoReleaser builds the provider binary; GitHub Release on tag.
