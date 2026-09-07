# systemd_credential

Writes a secret to the host credential store, consumable by units via
`LoadCredential=` (plaintext) or `LoadCredentialEncrypted=` (encrypted, the
default). `data` is never read back or decrypted from the remote host — but it
**is persisted in Terraform state** (marked `Sensitive`, so it is redacted from
CLI output and logs, not from the state file itself). Keep `data` out of
version control (e.g. a `sensitive` variable) and treat your Terraform state as
a secret.

Changing `encrypted` forces replacement: flipping it switches the remote path
between `/etc/credstore` and `/etc/credstore.encrypted`, so Terraform destroys
the credential at the old path before creating it at the new one.

Two things this resource does *not* do, since there is no way to signal it to
the OS or existing processes over SSH:

- **No import**: the resource has no `ImportState` — the content is
  unrecoverable from the remote host (it is encrypted, or simply never read
  back), so there is nothing Terraform could populate `data` with.
- **No consumer restart on rotation**: writing a new value only updates the
  file in the credstore. Units already running with
  `LoadCredential=` / `LoadCredentialEncrypted=` keep the credential they
  loaded at their last start; restart them yourself (e.g. via
  `systemd_instance` or an explicit apply step) to pick up the new value.

Destroy only removes the credential file at the path matching state
(`/etc/credstore/{name}` or `/etc/credstore.encrypted/{name}`) — no
stop/disable and no `daemon-reload`.

## Example Usage

```hcl
resource "systemd_credential" "db" {
  name      = "db-pass"
  data      = var.db_password # sensitive
  encrypted = true
  with_key  = "host"
}

resource "systemd_unit" "app" {
  name = "app.service"

  section {
    name = "Service"
    entry {
      key   = "LoadCredentialEncrypted"
      value = "db-pass"
    }
  }
}
```

Relative credential names search `/etc/credstore.encrypted/` (or
`/etc/credstore/` for `LoadCredential=`). An absolute path is also accepted,
e.g. `db-pass:/etc/credstore.encrypted/db-pass`.

## Argument Reference

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | Required | Credential filename under the credstore (e.g. `db-pass`). Forces replacement when changed. |
| `data` | Required | Credential content. Never read back from the remote host, but persisted in Terraform state (marked `Sensitive`). |
| `encrypted` | Optional | Encrypt the credential with `systemd-creds encrypt` under `/etc/credstore.encrypted` (default `true`) instead of storing it plaintext under `/etc/credstore`. Forces replacement when changed. |
| `with_key` | Optional | Key source passed to `systemd-creds encrypt --with-key` (`auto`, `host`, `tpm2`, or `host+tpm2`). Only meaningful when `encrypted` is true. |

## Attribute Reference

| Attribute | Description |
|-----------|-------------|
| `id` | Resource identifier used in Terraform state. |
