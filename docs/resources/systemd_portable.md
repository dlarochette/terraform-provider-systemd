# systemd_portable

Attaches a portable service image. The image is materialized under
`/var/lib/portables/{name}` (or `{name}.raw`) and attached with `portablectl`
(`--profile=trusted`). The image must contain matching unit files (prefix =
image name) and an `os-release`. Unit overrides after attach use existing
`systemd_dropin` / unit resources.

On destroy, `delete_image` defaults to `true` and removes the image from disk.

## Example Usage

```hcl
resource "systemd_portable" "app" {
  name         = "app"
  enable       = true
  active       = false
  delete_image = true

  image {
    type   = "local"
    source = "/var/tmp/app.tar"
  }
}
```

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | Required | Portable image name (also the unit prefix, e.g. `app` → `app.service`). Forces replacement when changed. |
| `image.type` | Required | `local` (tar extract or raw copy), `tar` (HTTP pull + extract), or `raw` (HTTP pull to `.raw`). `oci` is not supported for portable images. |
| `image.source` | Required | Remote path (local) or HTTPS URL (tar/raw). |
| `enable` | Optional | Pass `--enable` to `portablectl attach` / manage the primary `{name}.service` unit. |
| `active` | Optional | Pass `--now` to `portablectl attach` / start the primary `{name}.service` unit. |
| `delete_image` | Optional | On destroy, remove the portable image from disk (default `true`). |

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Resource identifier used in Terraform state. |


## See also

- [portablectl](https://www.freedesktop.org/software/systemd/man/latest/portablectl.html)
- [systemd-portabled](https://www.freedesktop.org/software/systemd/man/latest/systemd-portabled.html)
