# Design: Acceptance tests over SSH

Date: 2026-07-30  
Status: approved  
Module: `github.com/dlarochette/terraform-provider-systemd`  
Target release: `0.10.0`

## Goal

Exercise the production **SSH + SFTP** transport (`remote.Dial`) in acceptance tests, in addition to the existing **nspawn** ACC path (`remote.Nspawn` via `machinectl` / `systemd-run`).

Local and GitHub Actions. SSH ACC failures are hard failures on CI (no soft skip).

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Surfaces | **C** — nspawn default + SSH when env set; optional external host via same env |
| Test shape | **1** — same `TestAcc*` suite; transport selected by `accClient` |
| CI | Dedicated job `testacc-ssh` (fail hard); `release` needs it |
| Reachability (nspawn guest) | Dial `127.0.0.1` on a **dedicated guest sshd port** (default `2222`) |
| Auth | Ephemeral `ssh-keygen` key → guest `authorized_keys`; `insecure_ignore_host_key` for lab |
| SemVer | **0.10.0** (`feat` / test infrastructure) |

## Why not nspawn `Port=` + localhost

systemd-nspawn **excludes loopback** from `Port=` DNAT. The current ACC guest also **shares the host network** (no `VirtualEthernet`), so nspawn port mapping is not used today.

MVP approach: force the ACC guest onto the **host network** (`VirtualEthernet=no` in
`.nspawn`, overriding machinectl’s default `--network-veth`), install `openssh-server`,
bind sshd to port **2222** (avoid clashing with host `:22`), Dial `127.0.0.1:2222`.

Out of scope for this release: keeping private veth + `Port=` DNAT to a non-loopback host IP.

## Architecture

```text
make testacc
  └─ scripts/acc-nspawn.sh
        └─ TF_ACC=1 → go test -run TestAcc → Host = Nspawn

make testacc-ssh
  └─ scripts/acc-nspawn-ssh.sh
        ├─ reuse / start same machine as acc-nspawn (shared rootfs)
        ├─ ensure openssh-server + Port 2222 + root login keys-only
        ├─ generate ephemeral key; install pubkey in guest
        ├─ wait for sshd on 127.0.0.1:2222
        ├─ export SYSTEMD_ACC_SSH_* + TF_ACC=1
        └─ go test -run TestAcc → Host = SSH (remote.Dial)

accClient(t):
  if SYSTEMD_ACC_SSH_HOST != "" → Dial(Config{…})
  else → NewNspawn(SYSTEMD_ACC_MACHINE)
```

| Host impl | Used by |
|-----------|---------|
| `remote.SSH` | Production + SSH ACC |
| `remote.Nspawn` | Default ACC |
| `remote.Fake` | Unit tests |

## Environment

| Variable | Default (SSH harness) | Notes |
|----------|----------------------|-------|
| `TF_ACC` | `1` (set by harness) | Required or tests skip |
| `SYSTEMD_ACC_MACHINE` | `tf-systemd-acc` | Same machine as nspawn ACC |
| `SYSTEMD_ACC_SSH_HOST` | `127.0.0.1` | If unset → Nspawn path |
| `SYSTEMD_ACC_SSH_PORT` | `2222` | Guest sshd listen port |
| `SYSTEMD_ACC_SSH_USER` | `root` | |
| `SYSTEMD_ACC_SSH_KEY` | harness temp path | PEM private key |
| `SYSTEMD_ACC_SSH_INSECURE` | `1` | Maps to `InsecureIgnoreHostKey` |

External host: set the same `SYSTEMD_ACC_SSH_*` vars manually (no nspawn prepare). Without them, `make testacc` behaviour unchanged.

## Host API change

Add to `remote.Host`:

```go
Exec(args ...string) error
```

- **Nspawn:** existing `mustOK` / `Exec`
- **SSH:** `mustOK(shellJoin(args))`
- **Fake:** `note("exec " + strings.Join(args, " "))`

ACC tests that currently type-assert `*remote.Nspawn` (e.g. `ip link add` for resolve-link) must use `Host.Exec` so they run over SSH.

## Harness (`scripts/acc-nspawn-ssh.sh`)

1. Require root (like `acc-nspawn.sh`).
2. Ensure machine exists/bootable (delegate to shared helpers or call into the same rootfs logic as `acc-nspawn.sh`; prefer factoring common start/stop/ready into a sourced snippet rather than duplicating debootstrap).
3. Ensure guest packages: `openssh-server` (apt-get inside guest if missing). Rebuild rootfs with `--include=openssh-server` on next `ACC_REBUILD` so CI cold starts are faster.
4. Write `/etc/ssh/sshd_config.d/99-tf-acc.conf` (or drop-in): `Port 2222`, `PermitRootLogin prohibit-password`, `PasswordAuthentication no`.
5. `ssh-keygen -t ed25519 -N '' -f "$KEY"`; install pubkey to `/root/.ssh/authorized_keys` (mode 0600 / dir 0700).
6. `systemctl enable --now ssh` (or `ssh.service` / `sshd`) in guest; reset-failed if needed.
7. Poll `ssh -o BatchMode=yes -o StrictHostKeyChecking=no -i "$KEY" -p 2222 root@127.0.0.1 true` until ready (bounded retries).
8. Export SSH env; run `go test` as `SUDO_USER` (same ownership rules as nspawn harness).
9. Cleanup: stop machine (optional wipe); delete ephemeral key material.

Do **not** leave the ephemeral private key in the image after the run when `ACC_WIPE=1`; always remove host-side temp key on exit.

## CI (`.github/workflows/ci.yml`)

```yaml
testacc-ssh:
  runs-on: ubuntu-latest
  timeout-minutes: 45
  steps:
    - checkout + setup-go
    - apt install systemd-container debootstrap dbus openssh-client
    - run: make testacc-ssh

release:
  needs: [test, testacc, testacc-ssh]
```

`Makefile`: `testacc-ssh` target → `sudo ./scripts/acc-nspawn-ssh.sh`.

## Docs

- README Development section: document `make testacc` vs `make testacc-ssh` and env vars.
- Update [2026-07-27-acc-nspawn-design.md](2026-07-27-acc-nspawn-design.md): SSH ACC no longer “Out of scope”; link here.

## Out of scope

- Bastion in ACC
- Password auth
- Terraform Plugin SDK `resource.TestCase`
- Private-network nspawn + `Port=` DNAT
- Soft-fail / `continue-on-error` on CI

## Success criteria

- `make testacc` still green (Nspawn).
- `make testacc-ssh` green locally and on GHA; at least one test proves SFTP write (unit/dropin/network/resolved) and remote `systemctl` via SSH.
- Resolve-link ACC works over SSH via `Host.Exec`.
- Tag **0.10.0** after CI green.
