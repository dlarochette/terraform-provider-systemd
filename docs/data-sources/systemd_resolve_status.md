# Data Source: systemd_resolve_status

Returns the raw output of `resolvectl status` (optionally scoped to one link)
from the remote host.

## Example Usage

```hcl
data "systemd_resolve_status" "eth0" {
  link = "eth0"
}
```

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `link` | Optional | Interface name. When unset, returns global status. |

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Link name, or `global` when unset. |
| `status` | Raw stdout from `resolvectl status`. |

## See also

- [resolvectl](https://www.freedesktop.org/software/systemd/man/latest/resolvectl.html)
- [systemd-resolved](https://www.freedesktop.org/software/systemd/man/latest/systemd-resolved.html)
