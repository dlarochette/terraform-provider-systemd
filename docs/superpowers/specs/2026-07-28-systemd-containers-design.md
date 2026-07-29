# Design: systemd containers (nspawn + portable)

Date: 2026-07-28  
Status: shipped (`systemd_machine` in **0.7.0**, `systemd_portable` in **0.8.0**)  
Module: `github.com/dlarochette/terraform-provider-systemd`  
Provider address: `dlarochette/systemd`  
Tracks: [#4](https://github.com/dlarochette/terraform-provider-systemd/issues/4)

## Goal

Manage systemd containers on remote Linux hosts over **SSH only** (SFTP + remote CLI), without an agent:

1. **Phase 1:** `systemd-nspawn` machines — image lifecycle + `.nspawn` settings + enable/start
2. **Phase 2:** portable services — image lifecycle + `portablectl` attach/detach

Same transport and INI patterns as units / networkd / credentials.

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Surfaces in one design | **C** — nspawn + portable; implement nspawn first |
| Image ownership | **B** — Terraform owns config **and** image lifecycle |
| Image sources (MVP) | **D** — local import, HTTP `pull-tar`/`pull-raw`, OCI `pull-dkr` |
| Destroy | **C** — `delete_image` attribute, **default `true`** |
| Resource shape (nspawn) | **A** — single `systemd_machine` (image + settings + runtime) |
| Approach | **1** — CLI `machinectl` + SFTP `.nspawn` (not unit-first, not split resources) |
| Settings body | **B** — `content` XOR `section`/`entry` (same as units) |
| Quadlet / `*.container` | Out of scope |
| Registry auth beyond `machinectl pull-dkr` | Out of scope |
| Dedicated network resources for machines | Out of scope (use `.nspawn` sections and/or existing networkd) |

## Architecture

```text
Terraform
  ├─ systemd_machine (phase 1)
  │    image  → machinectl import-* | pull-tar | pull-raw | pull-dkr
  │    file   → SFTP /etc/systemd/nspawn/<name>.nspawn
  │    run    → systemctl enable/disable/start/stop systemd-nspawn@<name>
  ├─ systemd_portable (phase 2)
  │    image  → same image block → store then portablectl attach
  │    run    → portablectl attach/detach + systemctl on attached units
  └─ existing → unit-like, instance, credential, dropin, networkd unchanged
```

Transport: extend `remote.Host` (SSH and ACC `Nspawn`) with machine/portable helpers. No new daemon.

## Phase 1 — `systemd_machine`

### Schema

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | yes | Machine name; also image name under `/var/lib/machines/<name>` |
| `image` | yes | Nested block (exactly one) |
| `image.type` | yes | `local` \| `tar` \| `raw` \| `oci` |
| `image.source` | yes | Remote path (local), HTTPS URL (tar/raw), or OCI ref (`pull-dkr`) |
| `enable` | no | Enable/disable `systemd-nspawn@<name>.service` |
| `active` | no | Start/stop the machine |
| `delete_image` | no | On destroy, `machinectl remove <name>` if true (**default true**) |
| `content` | no | Raw `.nspawn` body; mutually exclusive with `section` |
| `section` / `entry` | no | Structured `.nspawn`; mutually exclusive with `content` |
| `id` | computed | Same as `name` |

Omit both `content` and `section` → no `.nspawn` file (systemd defaults).

Changing `image.type` or `image.source` → **RequiresReplace**.

### Example

```hcl
resource "systemd_machine" "web" {
  name         = "web"
  enable       = true
  active       = true
  delete_image = true

  image {
    type   = "oci"
    source = "docker.io/library/debian:bookworm"
  }

  section {
    name = "Network"
    entry {
      key   = "VirtualEthernet"
      value = "yes"
    }
  }
}
```

### Lifecycle

**Create**

1. Ensure image present via `machinectl`:
   - `local` → `import-tar` / `import-raw` / copy directory as appropriate from remote `source` path
   - `tar` → `pull-tar <url> <name>`
   - `raw` → `pull-raw <url> <name>`
   - `oci` → `pull-dkr <ref> <name>` (or equivalent supported by host systemd)
2. If settings set: atomic SFTP write `/etc/systemd/nspawn/<name>.nspawn`
3. `daemon-reload` if required by the host toolchain
4. Apply `enable` / `active`

**Update**

- Image fields → replace
- Settings / enable / active → in place: rewrite `.nspawn` if needed; stop → write → start when `active` and settings changed
- Clearing settings (both content and sections removed) → delete `.nspawn` file if previously managed

**Read**

- Image presence: `machinectl list-images` / `show-image`
- Runtime: `systemctl show systemd-nspawn@<name>` and/or `machinectl status`
- Settings: read `.nspawn` if present

**Delete**

1. Stop + disable (best effort)
2. Remove managed `.nspawn` if present
3. If `delete_image`: `machinectl remove <name>`

**Import**

- ID = machine `name`; refresh image presence and runtime; settings from file if any. `image.source` may be unknown after import — document that import is best-effort for image provenance (operator may need to set `image` to match reality or taint).

### Remote API sketch

```go
// on remote.Host
EnsureMachineImage(ctx, name, imageType, source string) error
RemoveMachineImage(ctx, name string) error
WriteNspawnFile(ctx, name, content string) error
DeleteNspawnFile(ctx, name string) error
ApplyMachineLifecycle(ctx, name string, enable, active *bool) error
DeleteMachineLifecycle(ctx, name string) error
ShowMachine(ctx, name string) (MachineStatus, error)
```

**Image ops:** `machinectl` only. **Runtime:** `systemctl` on `systemd-nspawn@<name>` only (same enable/active model as other resources).

## Phase 2 — `systemd_portable`

Same design document; separate release after phase 1.

| Attribute | Required | Description |
|-----------|----------|-------------|
| `name` | yes | Portable image / attachment name |
| `image` | yes | Same nested block as `systemd_machine` |
| `enable` / `active` | no | Applied to attached unit(s) where portablectl exposes them |
| `delete_image` | no | Default **true**; remove image after detach |
| `id` | computed | `name` |

No `.nspawn` `content`/`section` on this resource. Unit overrides use existing `systemd_dropin` / unit resources.

**Create:** ensure image → `portablectl attach` → optional enable/active  
**Delete:** stop related units (best effort) → `portablectl detach` → optional image remove  

If `portablectl` is missing on the host, Create fails with a clear error. ACC skips when absent.

## Acceptance tests

### Phase 1

- Run inside existing ACC nspawn guest (`TF_ACC` + `scripts/acc-nspawn.sh`)
- Use a **local fixture** rootfs/tar prepared by the harness (no OCI/HTTP pull in CI)
- Cases: create + active → update settings → destroy with `delete_image=true` (image gone)
- Optional local-only case: `delete_image=false` leaves image
- Pure Go unit tests: map `image.type` → expected `machinectl` argv
- Note: nested container **start** (`active=true`) is not reliable inside the ACC nspawn guest
  (cgroup mounts); ACC covers import + `.nspawn` + enable + delete_image instead

### Phase 2

- ACC with `portablectl` if available in guest; otherwise skip
- Attach / detach / delete_image smoke only

## Documentation & versioning

- README + examples for `systemd_machine` at phase 1 ship
- Phase 2 docs when `systemd_portable` lands
- SemVer: no `v` prefix; each phase is a `feat` → **minor** bump (e.g. `0.6.0` → `0.7.0` for machines)
- Update [#4](https://github.com/dlarochette/terraform-provider-systemd/issues/4) with link to this spec; close when both phases done (or split issues if preferred at plan time)

## Non-goals (both phases)

- Quadlet / Podman `*.container` generators
- Building rootfs with debootstrap from Terraform (ACC harness may still debootstrap for fixtures)
- Rich registry credentials UI beyond what `machinectl pull-dkr` already supports via environment/host config
- Managing nested Terraform provider config *inside* the machine
- Replacing the ACC host transport with SSH for product features (ACC stays machinectl/`systemd-run -M`)

## Open implementation details (plan may decide)

- Exact `local` import subcommand when `source` is a directory vs `.tar` vs `.raw`
- Whether Create fails if an image named `name` already exists, or adopts it when fingerprint matches
- Import of `image.source` after `terraform import` (document limitation if unrecoverable)

## Success criteria

- [x] Spec approved; implementation plan written for phase 1
- [x] `systemd_machine` CRUD works over SSH against a real host
- [x] ACC green with local image fixture
- [x] Tagged release with docs/examples (0.7.0)
- [x] Phase 2 (`systemd_portable`) implemented (0.8.0); `oci` not supported for portable
