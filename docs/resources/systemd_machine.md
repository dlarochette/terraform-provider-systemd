# systemd_machine

Manages a systemd-nspawn machine: imports the image (`machinectl import-tar` /
`import-raw` / `pull-tar` / `pull-raw` / `pull-dkr`), optionally writes
`/etc/systemd/nspawn/{name}.nspawn` settings, and manages
`systemd-nspawn@{name}.service` enable/active state via `systemctl`.

On destroy, `delete_image` defaults to `true` and runs `machinectl remove`.

## Example Usage

```hcl
resource "systemd_machine" "demo" {
  name         = "demo"
  enable       = true
  active       = true
  delete_image = true

  image {
    type   = "local" # or tar | raw | oci
    source = "/var/tmp/demo.tar"
  }

  content = <<-EOT
    [Exec]
    Boot=no
    PrivateUsers=no
    Parameters=/usr/bin/sleep infinity
  EOT
}
```

For an OCI image use `image { type = "oci", source = "docker.io/library/debian:bookworm" }`
(`machinectl pull-dkr`).

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | Required | Machine name (also the image name). Forces replacement when changed. |
| `image.type` | Required | Image source kind: `local` (import-tar / import-raw), `tar` (pull-tar), `raw` (pull-raw), or `oci` (pull-dkr). |
| `image.source` | Required | Remote path (local), HTTPS URL (tar/raw), or OCI reference (oci). |
| `enable` | Optional | Whether `systemd-nspawn@{name}.service` should be enabled. |
| `active` | Optional | Whether the machine should be started (`systemctl start` / `stop`). |
| `delete_image` | Optional | On destroy, run `machinectl remove` for the image (default `true`). |
| `content` | Optional | Raw `.nspawn` settings file contents. Mutually exclusive with `section` blocks. |
| `section` | Optional | Structured INI representation of the `.nspawn` file (see below). Mutually exclusive with `content`. |

{CONTENT_SECTION_NOTE}
{section_block_table()}
## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Resource identifier used in Terraform state. |
| `content` | When `section` blocks are used, the rendered `.nspawn` file contents. |
