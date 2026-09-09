# systemd_netdev

Manages a systemd-networkd `.netdev` (virtual device) profile.

The file is written to `/etc/systemd/network/{filename}` and the provider
runs `networkctl reload` on apply. There is no `enable` / `active` attribute —
networkd picks files up by name and `Match` sections. On destroy the file is
removed and `networkctl reload` runs again.

## Example Usage

```hcl
resource "systemd_netdev" "br0" {
  filename = "20-br0.netdev"

  section {
    name = "NetDev"
    entry { key = "Name", value = "br0" }
    entry { key = "Kind", value = "bridge" }
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
resource "systemd_netdev" "br0" {
  filename = "20-br0.netdev"

  match {
    name = "br0"
  }

  netdev {
    kind    = "bridge"
    name    = "br0"
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
| `filename` | Required | Filename under `/etc/systemd/network` (must end with `.netdev`). Forces replacement when changed. |
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


## See also

- [systemd.netdev](https://www.freedesktop.org/software/systemd/man/latest/systemd.netdev.html)
- [systemd-networkd](https://www.freedesktop.org/software/systemd/man/latest/systemd-networkd.html)
