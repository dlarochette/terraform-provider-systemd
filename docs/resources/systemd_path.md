# systemd_path

A `.path` unit. Pair it with the matching `.service` unit.

The file is written to `/etc/systemd/system/{name}`. On apply the provider
runs `systemctl daemon-reload`, then `systemctl enable` / `disable` and
`systemctl start` / `stop` when requested. On destroy it stops and disables the
unit (best effort), removes the file, and runs `systemctl daemon-reload`.

## Example Usage

```hcl
resource "systemd_path" "watch" {
  name   = "watch.path"
  enable = true
  active = true

  section {
    name = "Path"
    entry { key = "PathModified", value = "/etc/app" }
  }
  section {
    name = "Install"
    entry { key = "WantedBy", value = "multi-user.target" }
  }
}
```

## Typed properties

Beyond raw `content` and generic `section` blocks, this resource exposes **typed
properties**: one snake_case block per systemd section, whose attributes are
systemd directives with the same names and validation rules as the systemd
parsers (enums, booleans, time spans, byte sizes, octal modes, signals,
resource limits, weight ranges).

Example — the equivalent of `ExecStart`, `Restart` and `WantedBy` without any
`entry` boilerplate:

```hcl
resource "systemd_path" "demo" {
  name = "watch.path"

  unit {
    description = "Typed demo"
    after       = ["network.target"]
  }

  path {
    path_modified = "/etc/app"
  }

  install {
    wanted_by = ["multi-user.target"]
  }
}
```

The catalog covers 10 directives from systemd v257 ([Unit], [Install], [Path] and the
other unit sections). Directives not in the catalog (or exotic value forms) can
always be set through `section` blocks — but a directive must not be set both
ways. See the provider documentation for the full class map: list-valued
attributes correspond to repeatable directives.

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | Required | Unit filename ending with `.path`. Forces replacement when changed. |
| `enable` | Optional | Whether the unit should be enabled (`systemctl enable` / `disable`). |
| `active` | Optional | Whether the unit should be started (`systemctl start` / `stop`). Start failure fails the apply. |
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
