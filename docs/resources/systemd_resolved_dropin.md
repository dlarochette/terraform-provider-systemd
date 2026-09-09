# systemd_resolved_dropin

Manages a drop-in configuration file for systemd-resolved, written to
`/etc/systemd/resolved.conf.d/{name}`. On apply the provider writes the file
then runs `systemctl restart systemd-resolved.service`; the same restart runs
on destroy after the file is removed.

## Example Usage

```hcl
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
```

## Typed properties

Beyond raw `content` and generic `section` blocks, this resource exposes the
**typed** `[Resolve]` section: the resolved.conf directives as snake_case
attributes with systemd validation rules (enums for `LLMNR`, `MulticastDNS`,
`DNSSEC`, `DNSOverTLS`, `Cache`, `DNSStubListener`; lists for `DNS`,
`FallbackDNS`, `Domains`, `DNSStubListenerExtra`; booleans; a time span for
`StaleRetentionSec`).

Example:

```hcl
resource "systemd_resolved" "main" {
  resolve {
    dns               = ["1.1.1.1", "9.9.9.9"]
    llmnr             = "resolve"
    dnssec            = "allow-downgrade"
    dns_over_tls      = "opportunistic"
    dns_stub_listener = "yes"
  }
}
```

The catalog comes from the resolved gperf tables of systemd v249-v257;
directives not in the catalog can always be set through `section` blocks, and
a directive must not be set both ways.

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | Required | Drop-in filename ending in `.conf`. Forces replacement when changed. |
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

- [resolved.conf](https://www.freedesktop.org/software/systemd/man/latest/resolved.conf.html)
- [systemd-resolved](https://www.freedesktop.org/software/systemd/man/latest/systemd-resolved.html)
