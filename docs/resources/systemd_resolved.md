# systemd_resolved

Manages the global systemd-resolved configuration `/etc/systemd/resolved.conf`.
This resource is a singleton (no `name`). On apply the provider writes the file
then runs `systemctl restart systemd-resolved.service`. On destroy the file is
**removed**, so the vendor defaults apply again, and the service is restarted.

## Example Usage

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
```

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
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
| `id` | Always `resolved.conf`. |
| `content` | When `section` blocks are used, the rendered INI file contents. |
