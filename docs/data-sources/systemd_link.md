# Data Source: systemd_link

Reads link state from `networkctl status` on the remote host.

## Example Usage

```hcl
data "systemd_link" "br0" {
  name = "br0"
}
```

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | Required | Interface name (e.g. `eth0`). |

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Same as `name`. |
| `operational_state` | Parsed operational state from `networkctl status`. |
| `setup_state` | Parsed setup state from `networkctl status` when present. |
