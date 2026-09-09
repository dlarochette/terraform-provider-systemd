# Examples from the systemd documentation

This guide reproduces the examples of the official systemd man pages in
provider syntax — typed properties in HCL, and the same configurations as
OpenTofu **JSON** (`.tf.json`, the JSON variant of HCL). Every example links
back to the man page it comes from.

Official documentation index: [systemd.directives(7)](https://www.freedesktop.org/software/systemd/man/latest/systemd.directives.html) lists every
directive and the man page that documents it.

All examples assume a configured provider:

```hcl
provider "systemd" {
  host = "host.example.com"
}
```

The same file in JSON syntax (`main.tf.json`):

```json
{
  "provider": {
    "systemd": { "host": "host.example.com" }
  }
}
```

Note: OpenTofu accepts HCL (`.tf`) and JSON (`.tf.json`) only — not YAML. In
JSON syntax, repeatable blocks are arrays.

## Simple service (systemd.service(5))

Source: [systemd.service(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html#Examples)

Original INI:

```ini
[Unit]
Description=Simple service

[Service]
ExecStart=/usr/bin/simple

[Install]
WantedBy=multi-user.target
```

HCL, with typed properties:

```hcl
resource "systemd_unit" "simple" {
  name = "simple.service"

  unit {
    description = "Simple service"
  }
  service {
    exec_start = ["/usr/bin/simple"]
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}
```

The same unit in JSON syntax (`simple.tf.json`):

```json
{
  "resource": {
    "systemd_unit": {
      "simple": {
        "name": "simple.service",
        "unit": {
          "description": "Simple service"
        },
        "service": {
          "exec_start": ["/usr/bin/simple"]
        },
        "install": {
          "wanted_by": ["multi-user.target"]
        }
      }
    }
  }
}
```

The same file written with generic `section` blocks:

```hcl
resource "systemd_unit" "simple_sections" {
  name = "simple.service"

  section {
    name = "Unit"
    entry { key = "Description" value = "Simple service" }
  }
  section {
    name = "Service"
    entry { key = "ExecStart" value = "/usr/bin/simple" }
  }
  section {
    name = "Install"
    entry { key = "WantedBy" value = "multi-user.target" }
  }
}
```

## Forking daemon (systemd.service(5))

Source: [systemd.service(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html#Examples)

```hcl
resource "systemd_unit" "daemon" {
  name = "daemon.service"

  unit {
    description = "Some simple daemon"
  }
  service {
    type     = "forking"
    exec_start = ["/usr/sbin/fooford"]
    exec_stop  = ["/bin/kill $MAINPID"]
    pid_file   = "/run/foofifo.pid"
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}
```

`type`, `exec_start` and `pid_file` are validated: `type` only accepts the
systemd service types (`simple`, `forking`, `exec`, `dbus`, `notify`,
`notify-reload`, `oneshot`), `pid_file` is a path.

## Socket activation (systemd.socket(5))

Source: [systemd.socket(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.socket.html#Examples)

Original INI:

```ini
[Socket]
ListenStream=9999
SocketMode=0660
SocketUser=foo
SocketGroup=bar
```

HCL:

```hcl
resource "systemd_socket" "foo" {
  name = "foo.socket"

  unit {
    description = "Foo socket"
  }
  socket {
    listen_stream = ["9999"]
    socket_mode   = "0660"
    socket_user   = "foo"
    socket_group  = "foo"
  }
  install {
    wanted_by = ["sockets.target"]
  }
}

# The service triggered by the socket (systemd.service(5), DefaultDependencies).
resource "systemd_unit" "foo_service" {
  name = "foo.service"

  unit {
    description = "Foo service"
  }
  service {
    exec_start     = ["/usr/bin/fooserver"]
    standard_input = "socket"
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}
```

JSON:

```json
{
  "resource": {
    "systemd_socket": {
      "foo": {
        "name": "foo.socket",
        "socket": {
          "listen_stream": ["9999"],
          "socket_mode": "0660",
          "socket_user": "foo",
          "socket_group": "foo"
        },
        "install": { "wanted_by": ["sockets.target"] }
      }
    }
  }
}
```

`socket_mode` accepts octal notation (`0660`), `socket_user`/`socket_group`
are strings as written after `=`.

## Timer (systemd.timer(5))

Source: [systemd.timer(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.timer.html#Examples)

Original INI (`bootchk.timer`):

```ini
[Unit]
Description=Run bootchk at boot and at a recurring interval

[Timer]
OnBootSec=50
OnUnitActiveSec=2min

[Install]
WantedBy=timers.target
```

HCL:

```hcl
resource "systemd_timer" "bootchk" {
  name = "bootchk.timer"

  unit {
    description = "Run bootchk at boot and at a recurring interval"
  }
  timer {
    on_boot_sec       = ["50"]
    on_unit_active_sec = ["2min"]
  }
  install {
    wanted_by = ["timers.target"]
  }
}
```

Time spans are validated (`50` = 50 seconds, `2min`, `1h 30m`, `infinity`);
see [systemd.time(7)](https://www.freedesktop.org/software/systemd/man/latest/systemd.time.html).

## Mount and automount (systemd.mount(5), systemd.automount(5))

Source: [systemd.mount(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.mount.html#Examples)

Original INI:

```ini
[Unit]
Description=Encrypt /home

[Mount]
What=/dev/mapper/home
Where=/home
Type=ext4
```

HCL:

```hcl
resource "systemd_mount" "home" {
  name = "home.mount"

  unit {
    description = "Encrypt /home"
  }
  mount {
    what  = "/dev/mapper/home"
    where = "/home"
    type  = "ext4"
  }
}

resource "systemd_automount" "home" {
  name = "home.automount"

  unit {
    description = "Automount /home"
  }
  automount {
    where             = "/home"
    timeout_idle_sec  = "10min"
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}
```

See also [systemd.automount(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.automount.html).

## Resource control (systemd.resource-control(5))

Source: [systemd.resource-control(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.resource-control.html)

Original INI:

```ini
[Service]
CPUQuota=50%
MemoryMax=2G
```

HCL:

```hcl
resource "systemd_slice" "app" {
  name = "app.slice"

  slice {
    cpu_quota  = "50%"
    memory_max = "2G"
  }
}

resource "systemd_unit" "app_worker" {
  name = "app-worker.service"

  unit {
    slice = "app.slice"
  }
  service {
    exec_start = ["/usr/local/bin/app"]
  }
}
```

`memory_max` accepts byte sizes (`2G`, `512MiB`), percentages and
`infinity`; `cpu_quota` is written as systemd accepts it (`50%`).

## Hardening drop-in (systemd.exec(5))

Source: [systemd.exec(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html)

A typical `/etc/systemd/system/sshd.service.d/10-hardening.conf`:

```hcl
resource "systemd_dropin" "sshd_hardening" {
  unit   = "sshd.service"
  dropin = "10-hardening.conf"

  service {
    protect_system              = "strict"
    protect_home                = "yes"
    private_tmp                 = true
    memory_deny_write_execute   = true
    restrict_address_families   = ["AF_UNIX", "AF_INET", "AF_INET6"]
  }
}
```

`protect_system` and `protect_home` only accept the systemd enum values
(`no`, `yes`, `full`, `strict` and `no`, `yes`, `read-only`, `tmpfs`).

## Dependencies and ordering (systemd.unit(5))

Source: [systemd.unit(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html)

```hcl
resource "systemd_unit" "collector" {
  name = "collector.service"

  unit {
    description = "Log collector"
    after       = ["network-online.target"]
    wants       = ["network-online.target"]
    requires    = ["collector.socket"]
    conflicts   = ["shutdown.target"]
  }
  service {
    exec_start = ["/usr/local/bin/collector"]
  }
}
```

`after`, `wants`, `requires`, `conflicts`, `binds_to`, `part_of`, `upholds`,
`joins_namespace_of` and `requires_mounts_for` are list attributes — repeatable
directives, one value per line in the rendered file.

## Template units and instances (systemd.unit(5))

Source: [systemd.unit(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html#Template_Units)

```hcl
resource "systemd_unit" "app_template" {
  name = "app@.service"

  unit {
    description = "App instance %i"
  }
  service {
    exec_start = ["/usr/local/bin/app %i"]
  }
}

resource "systemd_instance" "app_bar" {
  template = systemd_unit.app_template.name
  instance = "bar"
  enable   = true
  active   = true
}
```

## Static network (systemd.network(5))

Source: [systemd.network(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.network.html#Examples)

Original INI:

```ini
[Match]
Name=eth0

[Network]
Address=192.168.0.15/24
Gateway=192.168.0.1
DNS=192.168.0.1
```

HCL:

```hcl
resource "systemd_network" "eth0_static" {
  filename = "10-eth0.network"

  match {
    name = "eth0"
  }
  network {
    address = "192.168.0.15/24"
    gateway = "192.168.0.1"
    dns     = "192.168.0.1"
  }
}
```

JSON:

```json
{
  "resource": {
    "systemd_network": {
      "eth0_static": {
        "filename": "10-eth0.network",
        "match":   { "name": "eth0" },
        "network": {
          "address": "192.168.0.15/24",
          "gateway": "192.168.0.1",
          "dns":     "192.168.0.1"
        }
      }
    }
  }
}
```

## DHCP network (systemd.network(5))

Source: [systemd.network(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.network.html#Examples)

```hcl
resource "systemd_network" "enp2s0_dhcp" {
  filename = "20-enp2s0.network"

  match {
    name = "enp2s0"
  }
  network {
    dhcp = "ipv4"
  }
  dhcpv4 {
    use_dns    = true
    use_ntp    = true
    route_metric = "100"
  }
}
```

## Bridge netdev (systemd.netdev(5))

Source: [systemd.netdev(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.netdev.html#Examples)

Original INI:

```ini
[NetDev]
Name=bridge0
Kind=bridge

[Bridge]
STP=true
```

HCL:

```hcl
resource "systemd_netdev" "bridge0" {
  filename = "25-bridge0.netdev"

  netdev {
    name = "bridge0"
    kind = "bridge"
  }
  bridge {
    stp = true
  }
}
```

`kind` is an enum of all netdev kinds (`bridge`, `bond`, `vxlan`,
`wireguard`, …).

## WireGuard netdev (systemd.netdev(5))

Source: [systemd.netdev(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.netdev.html#Examples)

`[WireGuardPeer]` is a repeatable section — one list block per peer:

```hcl
resource "systemd_netdev" "wg0" {
  filename = "30-wg0.netdev"

  netdev {
    name = "wg0"
    kind = "wireguard"
  }
  wireguard {
    private_key_file = "/etc/wireguard/wg0.key"
    listen_port      = "51820"
  }
  wireguard_peer {
    public_key = "PEER-PUBLIC-KEY"
    endpoint   = "peer.example.com:51820"
    allowed_ips  = "0.0.0.0/0"
  }
}
```

## Link file (systemd.link(5))

Source: [systemd.link(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.link.html#Examples)

Original INI:

```ini
[Match]
Driver=broadcom

[Link]
MTUBytes=4096
NamePolicy=keep
```

HCL:

```hcl
resource "systemd_link" "gigabit" {
  filename = "10-gigabit.link"

  match {
    driver = "broadcom"
  }
  link {
    mtu_bytes     = "4096"
    name_policy   = "keep"
  }
}
```

## DNS resolver (resolved.conf(5))

Source: [resolved.conf(5)](https://www.freedesktop.org/software/systemd/man/latest/resolved.conf.html)

Original INI:

```ini
[Resolve]
DNS=192.0.2.1 192.0.2.10
Domains=example.com
```

HCL:

```hcl
resource "systemd_resolved" "main" {
  resolve {
    dns     = ["192.0.2.1", "192.0.2.10"]
    domains = ["company.example"]
  }
}
```

`domains` accepts search domains; prefix `~` for routing-only domains
(`~company.example`).

## Runtime DNS on one link (resolvectl(1))

Source: [resolvectl(1)](https://www.freedesktop.org/software/systemd/man/latest/resolvectl.html)

The equivalent of `resolvectl dns eth0 192.0.2.1 && resolvectl domain eth0
~company.example` — runtime only, cleared on reboot:

```hcl
resource "systemd_resolve_link" "eth0" {
  link    = "eth0"
  dns     = ["192.0.2.1"]
  domains = ["~company.example"]
}
```

## Credentials (systemd.exec(5), systemd-creds(1))

Source: [systemd.exec(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html#Credentials) and
[systemd-creds(1)](https://www.freedesktop.org/software/systemd/man/latest/systemd-creds.html)

```hcl
resource "systemd_credential" "db" {
  name      = "db-pass"
  data      = var.db_password # sensitive
  encrypted = true
  with_key  = "host"
}

resource "systemd_unit" "app" {
  name = "app.service"

  service {
    exec_start = ["/usr/local/bin/app"]
    load_credential_encrypted = ["db-pass"]
  }
}
```

## Path unit (systemd.path(5))

Source: [systemd.path(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.path.html#Examples)

Original INI:

```ini
[Path]
PathExists=/var/run/mysqld/mysqld.sock

[Install]
WantedBy=multi-user.target
```

HCL:

```hcl
resource "systemd_path" "mysql_sock" {
  name = "mysql-sock.path"

  path {
    path_exists = ["/var/run/mysqld/mysqld.sock"]
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}
```

## Target and ordering (systemd.target(5))

Source: [systemd.target(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.target.html)

```hcl
resource "systemd_target" "app" {
  name = "app.target"

  unit {
    description = "App stack"
    requires    = ["app-db.service"]
    after       = ["app-db.service"]
  }
  install {
    wanted_by = ["multi-user.target"]
  }
}
```

## Swap (systemd.swap(5))

Source: [systemd.swap(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.swap.html)

Original INI:

```ini
[Swap]
What=/swapfile
Priority=10
```

HCL:

```hcl
resource "systemd_swap" "var_swap" {
  name = "var-swapfile.swap"

  swap {
    what     = "/swapfile"
    priority = "10"
  }
}
```

## Nspawn machine (systemd-nspawn(1), machinectl(1))

Source: [systemd-nspawn(1)](https://www.freedesktop.org/software/systemd/man/latest/systemd-nspawn.html),
[machinectl(1)](https://www.freedesktop.org/software/systemd/man/latest/machinectl.html)

```hcl
resource "systemd_machine" "demo" {
  name   = "demo"
  enable = true
  active = true

  image {
    type   = "tar"
    source = "https://example.com/images/demo.tar"
  }

  # /etc/systemd/nspawn/demo.nspawn settings
  exec {
    boot       = false
    private_users = false
  }
  files {
    bind = ["/srv/data:/srv/data"]
  }
}
```

## Portable service (portablectl(1))

Source: [portablectl(1)](https://www.freedesktop.org/software/systemd/man/latest/portablectl.html)

```hcl
resource "systemd_portable" "app" {
  name   = "app"
  enable = true

  image {
    type   = "local"
    source = "/var/tmp/app.tar"
  }
}
```

## Further reading

- [systemd.directives(7)](https://www.freedesktop.org/software/systemd/man/latest/systemd.directives.html) — index of every directive
- [systemd.unit(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html), [systemd.service(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html), [systemd.exec(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html), [systemd.kill(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.kill.html)
- [systemd.resource-control(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.resource-control.html), [systemd.scope(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.scope.html), [systemd.slice(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.slice.html)
- [systemd.timer(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.timer.html), [systemd.socket(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.socket.html), [systemd.mount(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.mount.html), [systemd.path(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.path.html), [systemd.swap(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.swap.html), [systemd.target(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.target.html), [systemd.automount(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.automount.html)
- [systemd.network(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.network.html), [systemd.netdev(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.netdev.html), [systemd.link(5)](https://www.freedesktop.org/software/systemd/man/latest/systemd.link.html), [systemd.networkd(8)](https://www.freedesktop.org/software/systemd/man/latest/systemd-networkd.html)
- [resolved.conf(5)](https://www.freedesktop.org/software/systemd/man/latest/resolved.conf.html), [systemd-resolved(8)](https://www.freedesktop.org/software/systemd/man/latest/systemd-resolved.html), [resolvectl(1)](https://www.freedesktop.org/software/systemd/man/latest/resolvectl.html)
- [systemd-nspawn(1)](https://www.freedesktop.org/software/systemd/man/latest/systemd-nspawn.html), [machinectl(1)](https://www.freedesktop.org/software/systemd/man/latest/machinectl.html), [portablectl(1)](https://www.freedesktop.org/software/systemd/man/latest/portablectl.html)
- [systemd-analyze(1) verify](https://www.freedesktop.org/software/systemd/man/latest/systemd-analyze.html#verify)
