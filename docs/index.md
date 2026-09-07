# systemd Provider

Manage **systemd units**, **systemd-networkd**, and **systemd-resolved** on remote Linux hosts over SSH — the same idea as `systemctl --host=user@host`.

The provider opens an SSH session to the target host (optionally through a bastion), writes unit / network / resolved files with SFTP (atomic temp file + rename), and applies the desired state with remote `systemctl`, `networkctl`, and `resolvectl`.

No agent, no Unix socket API, and no extra daemon are required on the host. SSH access as a user that can write `/etc/systemd` and run `systemctl` (typically `root`) is assumed.

## Requirements

- Terraform >= 1.5 or OpenTofu >= 1.6
- Target host: Linux with systemd
- `networkctl` on the host if you manage networkd files
- `resolvectl` / `systemd-resolved` on the host for resolved resources

## Example Usage

```hcl
terraform {
  required_providers {
    systemd = {
      source  = "dlarochette/systemd"
      version = ">= 0.10.1"
    }
  }
}

provider "systemd" {
  host = "host.example.com"
  user = "root"
}

resource "systemd_unit" "demo" {
  name   = "demo.service"
  enable = true
  content = <<-EOT
    [Unit]
    Description=Demo unit managed by Terraform
    [Service]
    Type=oneshot
    ExecStart=/bin/true
    [Install]
    WantedBy=multi-user.target
  EOT
}
```

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `host` | Required | SSH hostname or IP address of the target machine. |
| `user` | Optional | SSH user (default `root`). |
| `port` | Optional | SSH port (default `22`). |
| `private_key` | Optional | PEM-encoded private key contents. Conflicts with `private_key_path`. |
| `private_key_path` | Optional | Path to a PEM private key file. Conflicts with `private_key`. |
| `ssh_agent` | Optional | Use the local SSH agent (`SSH_AUTH_SOCK`). Defaults to true when no key is set. |
| `bastion_host` | Optional | SSH jump host. |
| `bastion_user` | Optional | SSH user on the bastion (defaults to `user`). |
| `bastion_port` | Optional | Bastion SSH port (default `22`). |
| `insecure_ignore_host_key` | Optional | Skip `known_hosts` verification (lab only). |
| `verify` | Optional | Validate unit files with the remote systemd parser: `off` / `warn` (default) / `error` (fails the apply and rolls the file back). |

## Fleet usage

Define one provider alias per host when managing a fleet:

```hcl
provider "systemd" {
  alias = "main"
  host  = "host.example.com"
}

provider "systemd" {
  alias = "backup"
  host  = "backup.example.com"
}

resource "systemd_unit" "demo" {
  provider = systemd.main
  name     = "demo.service"
  enable   = true
  content  = "..."
}
```

## File model

Most resources share the same `content` XOR `section` model:

- `content`: the raw file contents written to the remote host.
- `section` blocks: a structured INI representation. Each `section` has a `name`
  (without brackets) and `entry` blocks with a `key` and a `value`. Duplicate
  keys (e.g. multiple `ExecStart`) are allowed.

- **typed properties** (unit resources): one snake_case block per systemd
  section (`unit { … }`, `service { … }`, `install { … }`, …). Attributes are
  systemd directives with the same names and the same validation rules as the
  systemd parsers, generated from the systemd `load-fragment` gperf table
  (systemd v257): enum attributes only accept the documented systemd values,
  time spans / sizes / modes / signals / limits are regex-validated, and
  repeatable directives are list attributes. A directive set via a typed
  block must not also be set via a `section` block; directives outside the
  catalog always remain settable through `section` or `content`.
