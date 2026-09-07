# systemd_resolve_link

Configures per-link DNS settings at runtime via `resolvectl`. This resource is
**runtime-only**: settings are cleared on reboot unless also configured in a
`.network` file (see `systemd_network`). Destroy runs `resolvectl revert
<link>`; there is no file on disk.

## Example Usage

```hcl
resource "systemd_resolve_link" "eth0" {
  link          = "eth0"
  dns           = ["1.1.1.1", "1.0.0.1"]
  domains       = ["~example.com"]
  default_route = true
}
```

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `link` | Required | Network interface name (e.g. `eth0`). Forces replacement when changed. |
| `dns` | Optional | DNS servers for the link (`resolvectl dns`). |
| `domains` | Optional | Search/routing domains (`resolvectl domain`). Use a `~` prefix for route-only domains. |
| `default_route` | Optional | Whether the link is used as a default route for DNS queries. |
| `dnssec` | Optional | DNSSEC mode as accepted by `resolvectl dnssec`. |
| `dnsovertls` | Optional | DNS-over-TLS mode as accepted by `resolvectl dnsovertls`. |
| `llmnr` | Optional | LLMNR mode as accepted by `resolvectl llmnr` (e.g. `yes`, `no`, `resolve`). |
| `mdns` | Optional | mDNS mode as accepted by `resolvectl mdns`. |

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Resource identifier used in Terraform state. |
