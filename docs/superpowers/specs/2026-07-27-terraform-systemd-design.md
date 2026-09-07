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

- Units under `/etc/systemd/system/` (generic `systemd_unit` plus dedicated `systemd_timer`, `systemd_mount`, `systemd_automount`, `systemd_socket`, `systemd_path`, `systemd_swap`, `systemd_slice`, `systemd_target`)
- Drop-ins under `/etc/systemd/system/{unit}.d/`
- networkd files under `/etc/systemd/network/` (`.network`, `.netdev`, `.link`)
- Enable / disable / start / stop via remote `systemctl`
- networkd reload / link status via remote `networkctl`
- Remote hosts reached by SSH (+ optional bastion)

**Out:**

- Custom remote agent / Unix socket / HTTP API
- sudo / non-root (root SSH assumed)
- Public Terraform Registry (phase 2) — path: [2026-07-30-registry-publish-design.md](2026-07-30-registry-publish-design.md) (first signed release `0.10.1`)
- Bulk import of existing hosts
- D-Bus via `systemd-stdio-bridge` in-process (CLI is enough)
- Managing `/etc/resolv.conf` symlink / `.dnssd` files

**Shipped after MVP** (see dedicated specs):

- Credentials / templates / typed units — [2026-07-27-systemd-creds-templates-design.md](2026-07-27-systemd-creds-templates-design.md)
- Containers (nspawn + portable) — [2026-07-28-systemd-containers-design.md](2026-07-28-systemd-containers-design.md) ([#4](https://github.com/dlarochette/terraform-provider-systemd/issues/4) closed)
- systemd-resolved — [2026-07-29-systemd-resolved-design.md](2026-07-29-systemd-resolved-design.md)

## Architecture

```text
Terraform  →  provider  →  SSH
                            ├─ SFTP  →  /etc/systemd/{system,network,resolved.conf(.d),nspawn}
                            └─ exec  →  systemctl / networkctl / resolvectl / machinectl / portablectl
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
  alias = "main"
  host  = "host.example.com"
  user  = "root"
}

resource "systemd_unit" "demo" {
  provider = systemd.main
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

> Extended by [2026-07-27-systemd-creds-templates-design.md](2026-07-27-systemd-creds-templates-design.md):
> `systemd_socket`, `systemd_target`, template units + `systemd_instance`, and `systemd_credential`.

| Type | Path / identity | Notes |
|------|-----------------|-------|
| `systemd_unit` | `/etc/systemd/system/{name}` | generic units; `name` is ID; optional enable/active |
| `systemd_timer` | `/etc/systemd/system/{name}` | `.timer` only |
| `systemd_mount` | `/etc/systemd/system/{name}` | `.mount` only |
| `systemd_automount` | `/etc/systemd/system/{name}` | `.automount` only |
| `systemd_socket` | `/etc/systemd/system/{name}` | `.socket` only |
| `systemd_path` | `/etc/systemd/system/{name}` | `.path` only |
| `systemd_swap` | `/etc/systemd/system/{name}` | `.swap` only |
| `systemd_slice` | `/etc/systemd/system/{name}` | `.slice` only |
| `systemd_target` | `/etc/systemd/system/{name}` | `.target` only |
| `systemd_dropin` | `/etc/systemd/system/{unit}.d/{dropin}.conf` | ID = `unit/dropin` |
| `systemd_network` | `/etc/systemd/network/{filename}` | ID = `filename` |
| `systemd_netdev` | `/etc/systemd/network/{filename}` | ID = `filename` |
| `systemd_link` | `/etc/systemd/network/{filename}` | ID = `filename` |
| data `systemd_unit` | `systemctl show` | load/active/sub/unit-file state |
| data `systemd_link` | `networkctl status` | operational/setup state |
| `systemd_machine` | machinectl + `.nspawn` | see containers design |
| `systemd_portable` | portablectl | see containers design |
| `systemd_resolved` | `/etc/systemd/resolved.conf` | restart resolved; see resolved design |
| `systemd_resolved_dropin` | `resolved.conf.d/{name}` | restart resolved |
| `systemd_resolve_link` | `resolvectl` | runtime-only; see resolved design |
| data `systemd_resolve_status` | `resolvectl status` | raw status text |

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
