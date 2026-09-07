# systemd_instance

Manages the enable/active lifecycle of a specific instance of a template unit
(e.g. `app@bar.service`) without duplicating the unit file per instance. This
resource is lifecycle-only: it writes no file of its own, and destroy just
stops and disables the instance.

## Example Usage

```hcl
resource "systemd_unit" "app_template" {
  name = "app@.service"

  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "App instance %i"
    }
  }
  section {
    name = "Service"
    entry {
      key   = "ExecStart"
      value = "/usr/local/bin/app %i"
    }
  }
}

resource "systemd_instance" "app_bar" {
  template = systemd_unit.app_template.name
  instance = "bar"
  enable   = true
  active   = true
}
```

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `template` | Required | Template unit name (e.g. `app@.service`). Forces replacement when changed. |
| `instance` | Required | Instance string spliced into the template (e.g. `bar` for `app@bar.service`). Forces replacement when changed. |
| `enable` | Optional | Whether the instance should be enabled (`systemctl enable` / `disable`). |
| `active` | Optional | Whether the instance should be started (`systemctl start` / `stop`). Start failure fails the apply. |

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Instantiated unit name (e.g. `app@bar.service`). |
