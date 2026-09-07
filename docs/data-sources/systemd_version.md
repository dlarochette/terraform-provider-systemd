# Data Source: systemd_version

Reads the systemd release of the target host (`systemctl --version`). Use it
to gate or branch unit configurations on the remote systemd version; unit
file validation always uses this same version via `systemd-analyze verify`
(see the provider `verify` attribute).

## Example Usage

```hcl
data "systemd_version" "host" {}

locals {
  supports_notify_reload = tonumber(data.systemd_version.host.version) >= 255
}
```

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Same as `version`. |
| `version` | Systemd release number (e.g. `257`). |
| `release` | Full first line of `systemctl --version` (e.g. `systemd 257 (257.3-1)`). |
