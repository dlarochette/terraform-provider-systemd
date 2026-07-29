# Design: systemd-resolved resources

Date: 2026-07-29  
Status: approved (scope C)  
Module: `github.com/dlarochette/terraform-provider-systemd`  
Provider address: `dlarochette/systemd`  
Target release: `0.9.0`

## Goal

Manage **systemd-resolved** on remote hosts over SSH:

1. Global config file `/etc/systemd/resolved.conf`
2. Drop-ins under `/etc/systemd/resolved.conf.d/`
3. Per-link runtime DNS via `resolvectl`
4. Read-only status via `resolvectl status`

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Surfaces | **C** — files + status DS + per-link `resolvectl` resource |
| Global apply | `systemctl restart systemd-resolved.service` |
| Per-link persistence | Runtime only; reboot clears unless mirrored in `.network` |
| Out | `/etc/resolv.conf` symlink management, `.dnssd` files |

## Resources

### `systemd_resolved`

Owns full `/etc/systemd/resolved.conf`. `content` XOR `section`. Computed `id` = `resolved.conf`.  
Create/Update: write file → restart. Delete: remove file → restart (vendor defaults apply).

### `systemd_resolved_dropin`

`name` (must end `.conf`, RequiresReplace), `content` XOR `section`.  
Path: `/etc/systemd/resolved.conf.d/{name}`. Restart after write/delete.

### `systemd_resolve_link`

Per-interface runtime config via `resolvectl`:

| Attribute | Maps to |
|-----------|---------|
| `link` | interface name (RequiresReplace) |
| `dns` | `resolvectl dns LINK …` |
| `domains` | `resolvectl domain LINK …` |
| `default_route` | `resolvectl default-route LINK BOOL` |
| `llmnr` / `mdns` / `dnssec` / `dnsovertls` | matching resolvectl commands (optional strings) |

Delete: `resolvectl revert LINK`.

### `systemd_resolve_status` (data source)

Optional `link`. Computed `status` = stdout of `resolvectl status [link]`.

## Transport

Extend `remote.Host` with resolved file ops, `ResolvedRestart`, and resolvectl helpers. Implement on SSH, Nspawn (ACC), Fake.

## ACC

- Guest already includes `systemd-resolved`.
- Drop-in write/read/delete + restart.
- Resolve link: create dummy iface (`ip link add … type dummy`), set DNS, assert, revert.

## SemVer

`feat` → minor bump **0.9.0** (no `v` prefix).
