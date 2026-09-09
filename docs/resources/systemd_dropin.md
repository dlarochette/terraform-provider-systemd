# systemd_dropin

Manages a drop-in configuration file for a unit, written to
`/etc/systemd/system/{unit}.d/{dropin}`. On apply the provider runs
`systemctl daemon-reload`. On destroy the drop-in file is removed and
`daemon-reload` runs again.

## Example Usage

```hcl
resource "systemd_dropin" "sshd_restart" {
  unit   = "sshd.service"
  dropin = "10-restart.conf"

  section {
    name = "Unit"
    entry {
      key   = "X-RestartIfChanged"
      value = "true"
    }
  }
}
```

## Typed properties

Beyond raw `content` and generic `section` blocks, this resource exposes the
unit typed blocks (one snake_case block per systemd section, same validation
rules as the systemd parsers). Because `unit` is the resource attribute
holding the parent unit name, the typed `[Unit]` section is exposed under
`unit_section` instead; `[Install]` is meaningless inside a drop-in (systemd
ignores it) but remains available.

```hcl
resource "systemd_dropin" "sshd" {
  unit   = "sshd.service"
  dropin = "10-hardening.conf"

  service {
    protect_system = "strict"
    protect_home   = "yes"
  }
}
```

Same exclusivity rules as the other resources: a directive must not be set
both via a typed block and via a `section` block.

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `unit` | Required | Parent unit filename (e.g. `sshd.service`). Forces replacement when changed. |
| `dropin` | Required | Drop-in filename ending in `.conf` (e.g. `override.conf`). Forces replacement when changed. |
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

- [systemd.unit](https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html)
- [systemd.exec](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html)
