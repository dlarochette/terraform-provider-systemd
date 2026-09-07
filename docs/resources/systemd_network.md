# systemd_network

Manages a systemd-networkd `.network` profile.

The file is written to `/etc/systemd/network/{filename}` and the provider
runs `networkctl reload` on apply. There is no `enable` / `active` attribute —
networkd picks files up by name and `Match` sections. On destroy the file is
removed and `networkctl reload` runs again.

## Example Usage

```hcl
resource "systemd_network" "br0" {
  filename = "30-br0.network"

  section {
    name = "Match"
    entry { key = "Name", value = "br0" }
  }
  section {
    name = "Network"
    entry { key = "Address", value = "192.0.2.10/24" }
    entry { key = "Gateway", value = "192.0.2.1" }
  }
}
```

## Typed properties

Beyond raw `content` and generic `section` blocks, this resource exposes **typed
properties**: one snake_case block per systemd-networkd section, whose attributes
are networkd directives with the same names and validation rules as the networkd
parsers.

Example:

```hcl
resource "systemd_network" "br0" {
  filename = "30-br0.network"

  match {
    name = "br0"
  }

  network {
    dhcp    = "no"
    dns     = "192.0.2.1"
  }
}
```

Repeatable sections (e.g. `[Address]`, `[Route]`, `[WireGuardPeer]`) appear as
list blocks — one block instance per file section. The catalog is generated
from the systemd-networkd gperf tables of systemd v249-v257; directives not in
the catalog can always be set through `section` blocks, and a directive must
not be set both ways.

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `filename` | Required | Filename under `/etc/systemd/network` (must end with `.network`). Forces replacement when changed. |
| `content` | Optional | Raw file contents. Mutually exclusive with `section` blocks. |
| `section` | Optional | Structured INI representation (see below). Mutually exclusive with `content`. |

`content` and `section` are mutually exclusive. Duplicate keys within a section
(for example multiple `ExecStart` entries) are allowed. When `section` blocks
are used, `content` is computed from the rendered INI file.

| Block | Attribute | Required | Description |
|-------|-----------|----------|-------------|
| `section` | `name` | Required | Section name without brackets (e.g. `Unit`, `Service`). |
| `section.entry` | `key` | Required | Key name as written before `=`. |
| `section.entry` | `value` | Required | Value as written after `=` (may be empty). |

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Resource identifier used in Terraform state. |
| `content` | When `section` blocks are used, the rendered INI file contents. |
