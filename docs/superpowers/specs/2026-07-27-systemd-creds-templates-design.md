# Design: typed units, templates/instances, credentials

Date: 2026-07-27  
Status: approved (approach 1 — minimal SSH)  
Module: `github.com/dlarochette/terraform-provider-systemd`  
Provider address: `dlarochette/systemd`

## Goal

Extend the SSH-only systemd provider with:

1. More typed unit resources (`.socket`, `.target`)
2. Template units (`foo@.service`) and separate instance lifecycle (`foo@bar.service`)
3. Host credential store management (`/etc/credstore` / `.encrypted`) plus unit-side wiring via existing `section`/`entry`

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Credentials scope | **C** — file on host **and** unit wiring |
| Unit wiring for creds | Existing `section`/`entry` only in v1 (no typed `load_credential` blocks) |
| Templates | **B** — template file resource + separate instance resource |
| Transport | SSH + SFTP + `systemctl` / `systemd-creds` (no agent) |
| Approach | Minimal SSH (not rich schema, not external secret backend) |

## Architecture

```text
Terraform
  ├─ unit-like files     → SFTP /etc/systemd/system/{name}
  │    systemd_unit | _socket | _timer | _mount | _automount | _target
  │    (name may be a template: app@.service)
  ├─ systemd_instance    → systemctl enable/start/stop/disable foo@bar.service
  │                        (no unit file written)
  ├─ systemd_credential  → SFTP /etc/credstore[/encrypted]/ + systemd-creds encrypt
  └─ existing            → dropin, networkd unchanged
```

## Resources

### Typed unit-like (existing pattern)

| Type | Suffix | Notes |
|------|--------|-------|
| `systemd_socket` | `.socket` | new |
| `systemd_target` | `.target` | new (may already be in tree) |
| `systemd_timer` / `_mount` / `_automount` | as today | unchanged |

Shared factory: `unitLikeResource` (`typeName`, `suffix`, docs). Same schema as today: `name`, `content` xor `section`, `enable`, `active`, `id`.

### Template files

Template units are **normal unit-like / `systemd_unit` files** whose `name` contains `@` before the suffix, e.g. `app@.service`.

Validators must allow `@` in unit names (still a single path segment, no `/`). Examples:

- Template: `app@.service`
- Instantiated name (for `systemd_unit` generic use or import): `app@bar.service`

`%i` / `%I` / `%n` etc. appear only inside unit **content**; Terraform does not expand them.

### `systemd_instance`

Manages lifecycle of an instantiated template unit. **Does not write a unit file.**

| Attribute | Required | Description |
|-----------|----------|-------------|
| `template` | yes | Template unit name, e.g. `app@.service` (must match `^[^@]+@\.(service\|socket\|timer\|…)$` — empty instance between `@` and suffix) |
| `instance` | yes | Instance string, e.g. `bar` (no `@`, no `/`) |
| `enable` | no | `systemctl enable` / `disable` |
| `active` | no | `systemctl start` / `stop` (start failure fails apply) |
| `id` | computed | Instantiated unit name, e.g. `app@bar.service` |

**Create / Update:** resolve `id` from `template` + `instance` → optional enable/active via existing `Client` helpers. No SFTP write. Caller is responsible for having the template file present (separate resource + `depends_on`).

**Read:** `systemctl show` / unit-file state if useful; absence of template file is not this resource’s job. If the unit cannot be shown and was never enabled, keep id from state.

**Delete:** stop + disable (best effort).

**Import:** ID = instantiated unit name (`app@bar.service`); parse into `template` + `instance`.

### `systemd_credential`

Places a credential on the host for units to load.

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | yes | Credential id / filename (single path segment) |
| `data` | yes | Plaintext secret (**sensitive** in Terraform) |
| `encrypted` | no | Default `true` |
| `with_key` | no | Passed to `systemd-creds encrypt --with-key=` when encrypted: `auto` (omit flag / systemd default), `host`, `tpm2`, `host+tpm2` |
| `id` | computed | = `name` |

**Paths:**

- `encrypted=false` → `/etc/credstore/{name}` (mode `0600`, mkdir parent `0700` if needed)
- `encrypted=true` → pipe `data` to  
  `systemd-creds encrypt --name={name} [--with-key=…] - /etc/credstore.encrypted/{name}`

**Read:** check file existence only. **Never** decrypt or return secret contents into state. Plan compares desired `data` from config (sensitive) to force update when config changes; remote ciphertext is opaque.

**Update:** rewrite file (re-encrypt if encrypted).

**Delete:** remove the credential file.

**Unit wiring (documentation + examples only):**

```hcl
entry {
  key   = "LoadCredentialEncrypted"
  value = "db-pass"   # searches /etc/credstore.encrypted/
}
# or explicit path:
entry {
  key   = "LoadCredentialEncrypted"
  value = "db-pass:/etc/credstore.encrypted/db-pass"
}
```

For plaintext store use `LoadCredential=` with `/etc/credstore/`.

## Validator changes

- `unitNameValidator` / `unitSuffixValidator`: allow `@` in the name (template or instance form).
- New: `templateNameValidator` — must contain `@.` pattern with empty instance (`foo@.service`).
- New: `instanceIdValidator` — non-empty, no `/`, no `@`.
- Credential `name`: reuse `noPathSegment()`.

## Out of scope (this design)

- Typed HCL blocks for `LoadCredential*` / `SetCredential*`
- External secret backends (Vault, etc.)
- User credentials (`--user` / `~/.config/credstore`)
- Writing `SetCredentialEncrypted=` ciphertext into unit drop-ins via provider
- Expanding `%i` in Terraform
- networkd / resolved changes
- Non-root / sudo

## Delivery order

1. `systemd_socket` + `systemd_target` (unit-like) → patch/minor release
2. Template name validators + `systemd_instance`
3. `systemd_credential` + README/examples
4. Tag release (SemVer without `v`; `feat` → minor, e.g. `0.3.0` or next after pending target)

## Testing

- Schema `ValidateImplementation` for new resources
- Unit tests: template/instance name parsing; credential path selection; validators reject bad `@` forms
- Fake `remote.Host`: add hooks for credential write/encrypt command and instance enable/start if needed
- No acceptance test requirement in v1 (same as today)

## Package touch points

```text
internal/provider/resource_unit_like.go   # NewSocketResource, NewTargetResource
internal/provider/resource_instance.go    # new
internal/provider/resource_credential.go  # new
internal/provider/validators.go           # @ and credential validators
internal/remote/                          # WriteCredential, EncryptCredential, paths
internal/provider/client.go               # wrappers
examples/basic/                           # socket, target, template+instance, credential
README.md + schemas/provider.json
```

## Release

- Tags: SemVer **without** `v` prefix
- Docs updated in the same commit(s) as the feature
- Secrets never logged; `data` marked sensitive in the Framework schema
