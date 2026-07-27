# Nspawn Acceptance Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `remote.Nspawn` Host, a rootfs harness script, Go `TestAcc*` tests, and a GitHub Actions `testacc` job — validating real systemd without SSH.

**Architecture:** Third `Host` impl talks to a machine via `machinectl copy-to/from` and `systemd-run -M`. Harness builds Debian/Ubuntu rootfs with debootstrap, starts the machine, runs ACC tests, stops it. Production SSH path unchanged.

**Tech Stack:** Go, systemd-nspawn / machinectl, debootstrap, GitHub Actions `ubuntu-latest`, existing `internal/provider.Client`.

**Spec:** `docs/superpowers/specs/2026-07-27-acc-nspawn-design.md`

## Global Constraints

- ACC runtime: systemd-nspawn only (no Docker, no QEMU)
- ACC access: machinectl / systemd-run -M — **not** SSH
- SSH ACC out of scope for v1
- Default machine name: `tf-systemd-acc` (override `SYSTEMD_ACC_MACHINE`)
- Rebuild rootfs only if missing or `ACC_REBUILD=1`
- Wipe machine only if `ACC_WIPE=1`
- `TestAcc*` skip when `TF_ACC` unset; CI must **fail** if nspawn cannot run (no silent skip)
- Conventional Commits, English; SemVer tags without `v`
- No production code path depends on `Nspawn`
- Distro v1: Debian/Ubuntu via debootstrap

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/remote/nspawn.go` | `Nspawn` struct implementing `Host` |
| `internal/remote/nspawn_test.go` | Unit tests that skip without machine; helper parsing |
| `internal/provider/acc_helpers_test.go` | `accClient(t)`, skip helpers |
| `internal/provider/acc_unit_test.go` | `TestAccUnitLifecycle` |
| `internal/provider/acc_typed_test.go` | `TestAccTimerPathSocket` |
| `internal/provider/acc_dropin_test.go` | `TestAccDropin` |
| `internal/provider/acc_network_test.go` | `TestAccNetwork` |
| `internal/provider/acc_credential_test.go` | `TestAccCredential` |
| `internal/provider/acc_instance_test.go` | `TestAccInstance` |
| `scripts/acc-nspawn.sh` | Build/start/test/stop harness |
| `Makefile` | `testacc` → script |
| `.github/workflows/ci.yml` | `testacc` job |
| `README.md` | How to run ACC |

---

### Task 1: `remote.Nspawn` Host

**Files:**
- Create: `internal/remote/nspawn.go`
- Create: `internal/remote/nspawn_test.go`
- Modify: none of SSH/Fake behaviour

**Interfaces:**
- Produces: `type Nspawn struct { Machine string }` with `func NewNspawn(machine string) *Nspawn`
- Produces: full `Host` implementation
- Consumes: existing `unitPath`, `dropinPath`, `networkPath`, `credPath`, `safeName`, `Default*` dirs

- [ ] **Step 1: Scaffold with compile-time interface check**

```go
package remote

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

// Nspawn implements Host against a systemd-nspawn machine (ACC only).
type Nspawn struct {
	Machine string
}

func NewNspawn(machine string) *Nspawn {
	if machine == "" {
		machine = "tf-systemd-acc"
	}
	return &Nspawn{Machine: machine}
}

var _ Host = (*Nspawn)(nil)

func (n *Nspawn) Close() error { return nil }
```

- [ ] **Step 2: Implement `run` / `copyTo` / `copyFrom` helpers**

```go
func (n *Nspawn) run(args ...string) (string, error) {
	// systemd-run -M <machine> -P --wait -- <args...>
	cmd := exec.Command("systemd-run", append([]string{
		"-M", n.Machine, "-P", "--wait", "--",
	}, args...)...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func (n *Nspawn) mustOK(args ...string) error {
	out, err := n.run(args...)
	if err != nil {
		return fmt.Errorf("%v: %w (%s)", args, err, strings.TrimSpace(out))
	}
	return nil
}

func (n *Nspawn) copyTo(localPath, remotePath string) error {
	cmd := exec.Command("machinectl", "copy-to", n.Machine, localPath, remotePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("machinectl copy-to: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (n *Nspawn) writeFile(remotePath, content string, mode os.FileMode) error {
	tmp, err := os.CreateTemp("", "nspawn-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Ensure parent dir exists inside machine
	if err := n.mustOK("mkdir", "-p", path.Dir(remotePath)); err != nil {
		return err
	}
	if err := n.copyTo(tmp.Name(), remotePath); err != nil {
		return err
	}
	return n.mustOK("chmod", fmt.Sprintf("%04o", mode), remotePath)
}
```

Mirror SSH semantics for unit files (`0644`), credentials plaintext (`0600`). For encrypted creds: pipe via `systemd-run` with stdin:

```go
func (n *Nspawn) runWithStdin(stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("systemd-run", append([]string{
		"-M", n.Machine, "-P", "--wait", "--",
	}, args...)...)
	cmd.Stdin = bytes.NewReader(stdin)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
```

- [ ] **Step 3: Implement all `Host` methods**

Pattern (units — adapt for dropin/network using path helpers):

```go
func (n *Nspawn) WriteUnit(name, content string) error {
	p, err := unitPath(DefaultUnitDir, name)
	if err != nil {
		return err
	}
	return n.writeFile(p, content, 0o644)
}

func (n *Nspawn) ReadUnit(name string) (string, error) {
	p, err := unitPath(DefaultUnitDir, name)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp("", "nspawn-read-*")
	if err != nil {
		return "", err
	}
	_ = tmp.Close()
	defer os.Remove(tmp.Name())
	cmd := exec.Command("machinectl", "copy-from", n.Machine, p, tmp.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("copy-from: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	b, err := os.ReadFile(tmp.Name())
	return string(b), err
}

func (n *Nspawn) RemoveUnit(name string) error {
	p, err := unitPath(DefaultUnitDir, name)
	if err != nil {
		return err
	}
	_, _ = n.run("rm", "-f", p) // best-effort like SSH
	return nil
}

func (n *Nspawn) DaemonReload() error { return n.mustOK("systemctl", "daemon-reload") }
func (n *Nspawn) EnableUnit(name string) error {
	return n.mustOK("systemctl", "enable", "--", name)
}
// DisableUnit, StartUnit, StopUnit similarly
func (n *Nspawn) UnitStatus(name string) (UnitStatus, error) {
	out, err := n.run("systemctl", "show",
		"--property=LoadState,ActiveState,SubState,UnitFileState", "--", name)
	if err != nil {
		return UnitStatus{}, err
	}
	return parseUnitStatus(out), nil // extract shared parser from ssh.go or duplicate small helper
}
```

**Refactor note:** If `parseUnitStatus` / `parseLinkStatus` live only in `ssh.go`, move them to `status_parse.go` in the same package and use from both SSH and Nspawn. Do this in the same task if needed to avoid duplication.

Credential methods: match SSH (`WriteCredential` → writeFile 0600 under `/etc/credstore`; encrypt via `systemd-creds encrypt --name=…` with stdin).

- [ ] **Step 4: Unit test without machine**

```go
func TestNewNspawnDefaultMachine(t *testing.T) {
	n := NewNspawn("")
	if n.Machine != "tf-systemd-acc" {
		t.Fatalf("got %q", n.Machine)
	}
}
```

Optional: `TestNspawnImplementsHost` already via `var _ Host`.

- [ ] **Step 5: `go test ./internal/remote/ -count=1` (no TF_ACC) — PASS**

- [ ] **Step 6: Commit**

```bash
git add internal/remote/nspawn.go internal/remote/nspawn_test.go internal/remote/status_parse.go # if created
git commit -m "feat: add remote.Nspawn Host for acceptance tests"
```

---

### Task 2: Harness `scripts/acc-nspawn.sh` + Makefile

**Files:**
- Create: `scripts/acc-nspawn.sh` (executable)
- Modify: `Makefile`

**Interfaces:**
- Produces: machine ready with `SYSTEMD_ACC_MACHINE` exported; exit ≠ 0 on failure
- Consumes: root, `debootstrap`, `machinectl`, `systemd-nspawn`

- [ ] **Step 1: Write harness**

```bash
#!/usr/bin/env bash
set -euo pipefail

MACHINE="${SYSTEMD_ACC_MACHINE:-tf-systemd-acc}"
ROOT="/var/lib/machines/${MACHINE}"
SUITE="${ACC_DEBIAN_SUITE:-bookworm}" # or jammy if Ubuntu host preferred
MIRROR="${ACC_DEBIAN_MIRROR:-http://deb.debian.org/debian}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "acc-nspawn: must run as root (try: sudo make testacc)" >&2
  exit 1
fi
command -v machinectl >/dev/null
command -v debootstrap >/dev/null

build_rootfs() {
  debootstrap --include=systemd,dbus,systemd-sysv,libpam-systemd,systemd-resolved \
    "${SUITE}" "${ROOT}" "${MIRROR}"
  # Prefer networkd for ACC network tests; enable in machine after first boot or via preset
  mkdir -p "${ROOT}/etc/systemd/system/multi-user.target.wants"
  # Ensure machine-id
  systemd-machine-id-setup --root="${ROOT}" || true
  # Minimal nspawn settings: no private users issues for copy-to
  mkdir -p "/etc/systemd/nspawn"
  cat > "/etc/systemd/nspawn/${MACHINE}.nspawn" <<EOF
[Exec]
Boot=yes
[Files]
BindReadOnly=/etc/resolv.conf
EOF
}

if [[ ! -d "${ROOT}/usr/lib/systemd/systemd" ]] || [[ "${ACC_REBUILD:-}" == "1" ]]; then
  rm -rf "${ROOT}"
  build_rootfs
fi

machinectl start "${MACHINE}" || systemd-nspawn --machine="${MACHINE}" --boot &
# Prefer machinectl start when registered; if only raw dir, use:
# machinectl start works when image is under /var/lib/machines/<name>

# Ready probe
for i in $(seq 1 60); do
  if systemd-run -M "${MACHINE}" -P --wait -- systemctl is-system-running --wait 2>/dev/null | grep -Eq 'running|degraded'; then
    break
  fi
  # softer probe:
  if systemd-run -M "${MACHINE}" -P --wait -- true 2>/dev/null; then
    if systemd-run -M "${MACHINE}" -P --wait -- systemctl is-system-running 2>/dev/null | grep -Eq 'running|degraded|starting'; then
      [[ $i -gt 5 ]] && systemd-run -M "${MACHINE}" -P --wait -- systemctl is-system-running | grep -Eq 'running|degraded' && break
    fi
  fi
  sleep 2
  if [[ $i -eq 60 ]]; then
    echo "acc-nspawn: machine not ready" >&2
    exit 1
  fi
done

export TF_ACC=1
export SYSTEMD_ACC_MACHINE="${MACHINE}"

# Run tests as the invoking user if SUDO_USER set, else root
TEST_USER="${SUDO_USER:-root}"
if [[ "${TEST_USER}" != "root" ]]; then
  # Go module cache must be writable; run go test as root for v1 simplicity
  :
fi
(
  cd "$(dirname "$0")/.."
  go test ./internal/provider/ -run 'TestAcc' -count=1 -timeout 45m -v
)
status=$?

machinectl stop "${MACHINE}" || true
if [[ "${ACC_WIPE:-}" == "1" ]]; then
  machinectl remove "${MACHINE}" || rm -rf "${ROOT}"
fi
exit "${status}"
```

Tune the start/ready logic during implementation until `systemd-run -M … -- true` works reliably on Ubuntu GHA. Document required packages: `systemd-container`, `debootstrap`, `dbus`.

- [ ] **Step 2: Makefile**

```make
testacc:
	./scripts/acc-nspawn.sh
```

- [ ] **Step 3: Manual smoke (if root available)**

```bash
sudo apt-get install -y systemd-container debootstrap
sudo ACC_REBUILD=1 ./scripts/acc-nspawn.sh
```

Expected: builds, starts, runs tests (may fail until Task 3 adds TestAcc — then harness should still exit because no tests matched… use `-run 'TestAcc'` which exits 0 with “no tests to run” in older go — in Go 1.20+ `testing` exits 0 if zero tests. **Force failure if zero ACC tests** in harness:

```bash
go test … -run 'TestAcc' … | tee /tmp/acc.out
grep -q '^ok' /tmp/acc.out  # or check that at least one TestAcc ran
```

Better: after Task 3, always have tests. For Task 2 alone, commit harness that runs `go test` and accept empty match until Task 3.

- [ ] **Step 4: Commit**

```bash
chmod +x scripts/acc-nspawn.sh
git add scripts/acc-nspawn.sh Makefile
git commit -m "feat: add systemd-nspawn acceptance test harness"
```

---

### Task 3: ACC helpers + core `TestAcc*` matrix

**Files:**
- Create: `internal/provider/acc_helpers_test.go`
- Create: `internal/provider/acc_unit_test.go`
- Create: `internal/provider/acc_typed_test.go`
- Create: `internal/provider/acc_dropin_test.go`
- Create: `internal/provider/acc_network_test.go`
- Create: `internal/provider/acc_credential_test.go`
- Create: `internal/provider/acc_instance_test.go`

**Interfaces:**
- Consumes: `remote.NewNspawn`, `Client`, existing resource types
- Produces: `TestAcc*` functions

- [ ] **Step 1: Helpers**

```go
//go:build !skip_acc

package provider

import (
	"os"
	"testing"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

func accSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
}

func accClient(t *testing.T) *Client {
	t.Helper()
	accSkip(t)
	machine := os.Getenv("SYSTEMD_ACC_MACHINE")
	if machine == "" {
		machine = "tf-systemd-acc"
	}
	return &Client{Host: remote.NewNspawn(machine)}
}
```

Do **not** use a build tag that excludes ACC from normal `go test ./...` — skip via env only so CI unit job stays fast and green without nspawn.

- [ ] **Step 2: `TestAccUnitLifecycle`**

Use `Client.PutUnit` / `GetUnit` / `UnitStatus` / `DeleteUnit` with a unique name `tf-acc-demo.service`, content oneshot `ExecStart=/bin/true`, enable+active true. Assert ActiveState active or inactive for oneshot after start (oneshot → inactive/dead after success — assert LoadState=loaded and start succeeded). Destroy and assert read fails / file gone.

- [ ] **Step 3: Remaining matrix tests**

- **Typed:** write `tf-acc.timer` (OnCalendar unlikely needed — use `OnUnitActiveSec=1h` or just enable a timer with `Persistent=false`), `tf-acc.path` with PathExists=/tmp, `tf-acc.socket` ListenStream=/run/tf-acc.sock — each via Client.PutUnit + EnableUnit.
- **Dropin:** PutDropin on a throwaway unit or on `tf-acc-demo.service`.
- **Network:** PutNetwork `90-tf-acc.network` with Match Name=lo or a dummy — NetworkReload must succeed.
- **Credential:** PutCredential plaintext; try encrypted and `t.Skip` if encrypt fails.
- **Instance:** Write `tf-acc@.service` template, ApplyUnitLifecycle on `tf-acc@x.service`.

Keep names prefixed `tf-acc-` for cleanup/debug.

- [ ] **Step 4: Verify**

```bash
go test ./internal/provider/ -count=1   # must skip ACC, PASS
sudo make testacc                      # must PASS all TestAcc*
```

- [ ] **Step 5: Commit**

```bash
git add internal/provider/acc_*.go
git commit -m "test: add nspawn acceptance tests for unit, typed, network, creds"
```

---

### Task 4: CI job + README

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `README.md`
- Push design commit if still local-only

- [ ] **Step 1: CI**

```yaml
  testacc:
    runs-on: ubuntu-latest
    timeout-minutes: 45
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
      - name: Install nspawn tooling
        run: |
          sudo apt-get update
          sudo apt-get install -y systemd-container debootstrap dbus
      - name: Acceptance tests (nspawn)
        run: sudo make testacc
```

Keep `test` job unchanged. `testacc` should run on PR + push to main (same triggers). Do not make release depend on `testacc` initially if flaky — **spec says fail if nspawn cannot run**; release `needs: [test]` only (not testacc) for v1 to avoid blocking tags on infra flakes — **OR** `needs: [test, testacc]` per success criteria “GHA testacc green”. Prefer `needs: [test, testacc]` for release to match success criteria.

- [ ] **Step 2: README section**

```markdown
## Acceptance tests

Unit tests: `make test` (Fake host, no privileges).

ACC against real systemd in a nspawn machine (not SSH):

```bash
sudo apt-get install -y systemd-container debootstrap
sudo make testacc
```

Env: `SYSTEMD_ACC_MACHINE` (default `tf-systemd-acc`), `ACC_REBUILD=1`, `ACC_WIPE=1`.
```

- [ ] **Step 3: Commit + push**

```bash
git add .github/workflows/ci.yml README.md
git commit -m "ci: run nspawn acceptance tests on GitHub Actions"
git push origin main
```

- [ ] **Step 4: Watch CI**

```bash
gh run watch --repo dlarochette/terraform-provider-systemd
```

Fix harness until `testacc` is green.

---

## Spec coverage

| Spec item | Task |
|-----------|------|
| `remote.Nspawn` Host | 1 |
| Harness + Makefile | 2 |
| TestAcc matrix | 3 |
| GHA testacc + README | 4 |
| SSH out of ACC | — (not implemented) |
| No Docker/QEMU | — |
