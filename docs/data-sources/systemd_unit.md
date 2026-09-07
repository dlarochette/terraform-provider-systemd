# Data Source: systemd_unit

Reads unit state properties from `systemctl show` on the remote host.

## Example Usage

```hcl
data "systemd_unit" "sshd" {
  name = "sshd.service"
}
```

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | Required | Unit filename (e.g. `sshd.service`). |

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Same as `name`. |
| `load_state` | systemd `LoadState` property (e.g. `loaded`). |
| `active_state` | systemd `ActiveState` property (e.g. `active`). |
| `sub_state` | systemd `SubState` property (e.g. `running`). |
| `unit_file_state` | systemd `UnitFileState` property (enabled/disabled/…). |
