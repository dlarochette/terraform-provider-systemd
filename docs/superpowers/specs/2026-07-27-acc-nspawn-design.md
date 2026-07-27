# Design: Acceptance tests via systemd-nspawn

Date: 2026-07-27  
Status: approved  
Module: `github.com/dlarochette/terraform-provider-systemd`

## Goal

Run acceptance tests against a **real systemd** inside a **systemd-nspawn** machine, locally and on GitHub Actions — without exercising the SSH/SFTP transport in ACC v1.

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Where | Local (`make testacc`) **and** GitHub Actions |
| Runtime | **systemd-nspawn** only (no Docker, no QEMU for ACC v1) |
| Access from tests | **machinectl / systemd-run -M** (not SSH) |
| SSH in ACC | **Out of scope** for v1 (still covered by Fake unit tests + manual use) |
| Approach | Third `remote.Host` implementation (`Nspawn`) + harness script + `TestAcc*` |

## Architecture

```text
make testacc
  └─ scripts/acc-nspawn.sh
        ├─ build/import rootfs → machine "tf-systemd-acc"
        ├─ machinectl start + wait ready
        ├─ TF_ACC=1 SYSTEMD_ACC_MACHINE=tf-systemd-acc \
        │     go test ./internal/provider/ -run 'TestAcc' -count=1 -timeout 45m
        └─ machinectl stop (+ optional wipe)

TestAcc*
  └─ Client{ Host: remote.Nspawn{Machine: env} }
        ├─ file ops  → machinectl copy-to / copy-from (+ chmod via systemd-run)
        └─ exec      → systemd-run -M <machine> -P --wait -- <cmd>
```

| Host impl | Used by |
|-----------|---------|
| `remote.SSH` | Production provider |
| `remote.Fake` | Unit tests |
| `remote.Nspawn` | Acceptance tests only |

## Rootfs

- **Distro (v1):** Debian or Ubuntu minimal via `debootstrap` (simpler on GHA `ubuntu-latest`).
- **Host packages:** `systemd-container` (provides `machinectl`, `systemd-nspawn`).
- **Guest packages:** `systemd`, dbus as needed, `systemd-networkd`, tools for `systemd-creds`.
- **Location:** `/var/lib/machines/tf-systemd-acc` (or import via `machinectl`).
- **Rebuild:** only if image missing or `ACC_REBUILD=1`.
- Machine name override: `SYSTEMD_ACC_MACHINE` (default `tf-systemd-acc`).

## `remote.Nspawn` behaviour

Implements the existing `remote.Host` interface:

- `WriteUnit` / `WriteDropin` / `WriteNetwork` / credential writes: stage locally, `machinectl copy-to`, set mode (`0600` for plaintext credentials) via `systemd-run -M … -- chmod`.
- `Read*` / `Remove*`: `copy-from` or `systemd-run -M … -- rm` / `test -e`.
- `DaemonReload`, `EnableUnit`, `StartUnit`, `StopUnit`, `UnitStatus`, `NetworkReload`, `LinkStatus`, `systemd-creds encrypt`: `systemd-run -M <machine> -P --wait -- …`.
- `Close`: no-op (machine lifecycle owned by the harness).

Privileges: harness and tests expect **root** (or equivalent) on the host to manage machines.

## Test style (v1)

Go acceptance tests (same pattern as Fake-backed CRUD tests), gated by:

```go
if os.Getenv("TF_ACC") == "" {
    t.Skip("TF_ACC not set")
}
```

Not Terraform `resource.TestCase` / plugin-sdk ACC helpers in v1 (YAGNI). Direct `Client` / Framework resource exercise against `Nspawn` is enough to validate real systemd behaviour.

### Matrix

| Test | Coverage |
|------|----------|
| `TestAccUnitLifecycle` | write unit, enable, start, `systemctl show`, destroy |
| `TestAccTimerPathSocket` | typed `.timer` / `.path` / `.socket` + enable |
| `TestAccDropin` | drop-in + daemon-reload |
| `TestAccNetwork` | `.network` write + `networkctl reload` (link data source if feasible) |
| `TestAccCredential` | plaintext + encrypted credstore; skip encrypt if host key unavailable |
| `TestAccInstance` | template unit file + instance enable/start |

## Harness & Make

```make
testacc:
	./scripts/acc-nspawn.sh
```

`scripts/acc-nspawn.sh`:

1. Check root / `machinectl` availability (exit non-zero in CI; local may document sudo).
2. Build/import machine if needed.
3. `machinectl start` + ready probe (`systemd-run -M … -- systemctl is-system-running` with timeout).
4. Export `TF_ACC=1`, `SYSTEMD_ACC_MACHINE=…`, run Go ACC tests.
5. `machinectl stop`; wipe only if `ACC_WIPE=1`.

## CI (GitHub Actions)

- Keep existing `test` job: `go test ./...` + build.
- Add `testacc` job on `ubuntu-latest`:
  - install `systemd-container`, debootstrap deps
  - `sudo ./scripts/acc-nspawn.sh` (or `make testacc` under sudo)
  - timeout ~45m
  - **fail** if nspawn cannot run (no silent skip on CI)
- Local: developers may skip when not root; document in README.

## Out of scope (v1)

- SSH/SFTP acceptance tests
- Bastion / multi-host
- Docker or QEMU ACC runners
- mkosi-based fancy images
- Publishing prebuilt machine images to a registry
- Plugin Framework `resource.Test` harness
- Testing portable services / containers as product features (see issue #4)

## Success criteria

- `make test` unchanged (fast, no nspawn).
- `sudo make testacc` on a Linux host with `systemd-container` boots a machine and passes `TestAcc*`.
- GHA `testacc` job green on `main` / PRs.
- No production code path depends on `Nspawn` (ACC-only wiring in tests).

## Package touch points

```text
internal/remote/nspawn.go       # Host implementation
internal/remote/nspawn_test.go  # smoke if possible without full machine
internal/provider/*_acc_test.go # TestAcc*
scripts/acc-nspawn.sh           # harness
Makefile                        # testacc → script
.github/workflows/ci.yml        # testacc job
README.md                       # how to run ACC
```
