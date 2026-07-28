# systemd_machine (nspawn) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship phase 1 of the containers design — `systemd_machine` manages nspawn images + `/etc/systemd/nspawn/<name>.nspawn` + `systemd-nspawn@<name>` lifecycle over SSH.

**Architecture:** Extend `remote.Host` with machine image/settings helpers (`machinectl` for images, SFTP for `.nspawn`, `systemctl` for enable/start). Provider `Client` orchestrates CRUD; new Framework resource mirrors unit `content` XOR `section` patterns. ACC uses a local mini rootfs fixture inside the existing nspawn guest (no OCI/HTTP pull in CI). Phase 2 `systemd_portable` is **out of this plan**.

**Tech Stack:** Go, terraform-plugin-framework, `machinectl` / `systemctl`, existing `remote.Client` (SSH) + `remote.Nspawn` (ACC), GitHub Actions `testacc`.

**Spec:** `docs/superpowers/specs/2026-07-28-systemd-containers-design.md`

## Global Constraints

- SSH-only product path (SFTP + CLI); no agent
- SemVer tags without `v`; this feature is a `feat` → minor (e.g. `0.7.0`)
- Conventional Commits, English messages
- `delete_image` default **true**
- Image `type`/`source` changes → RequiresReplace
- Runtime lifecycle: **systemctl** on `systemd-nspawn@<name>.service` only
- Image CRUD: **machinectl** only
- ACC: local fixture only (no `pull-dkr` / HTTP in CI)
- Do not implement `systemd_portable` in this plan
- Nested ACC machine name: `tf-acc-nested` (fixed in tests)

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/remote/host.go` | Extend `Host` + `MachineStatus` + path helpers |
| `internal/remote/machine.go` | Shared helpers: `nspawnUnitName`, `MachineImageType`, argv builders (pure) |
| `internal/remote/machine_test.go` | Unit tests for argv / path / type validation |
| `internal/remote/ssh.go` | SSH impl of new Host methods |
| `internal/remote/nspawn.go` | Nspawn ACC impl of new Host methods |
| `internal/remote/fake.go` | Fake Host + recorded machinectl commands / image set |
| `internal/provider/client.go` | `PutMachine` / `GetMachine` / `DeleteMachine` |
| `internal/provider/client_machine_test.go` | Client tests against `remote.Fake` |
| `internal/provider/resource_machine.go` | `systemd_machine` resource |
| `internal/provider/resource_machine_test.go` | Schema / validate / render tests |
| `internal/provider/provider.go` | Register `NewMachineResource` |
| `internal/provider/schema_test.go` | Include machine in schema matrix |
| `internal/provider/acc_machine_test.go` | `TestAccMachineLifecycle` |
| `scripts/acc-nspawn.sh` | Install `systemd-container` in guest; stage mini fixture |
| `examples/machine/main.tf` | Example config |
| `README.md` | Document `systemd_machine` |

---

### Task 1: Pure machine helpers + Host interface

**Files:**
- Create: `internal/remote/machine.go`
- Create: `internal/remote/machine_test.go`
- Modify: `internal/remote/host.go`

**Interfaces:**
- Produces:
  - `type MachineImageType string` with constants `MachineImageLocal`, `MachineImageTar`, `MachineImageRaw`, `MachineImageOCI`
  - `func ParseMachineImageType(s string) (MachineImageType, error)`
  - `func MachinectlEnsureArgs(name string, t MachineImageType, source string) ([]string, error)` — returns argv **after** `machinectl` (e.g. `["pull-dkr", ref, name]`)
  - `func NspawnUnitName(machine string) string` → `systemd-nspawn@<machine>.service`
  - `func nspawnSettingsPath(name string) (string, error)` → `/etc/systemd/nspawn/<name>.nspawn`
  - `type MachineStatus struct { ImagePresent bool; Unit UnitStatus; Settings string }`
  - Host methods (signatures only in this task — Fake stubs returning `errNotImplemented` are OK until Task 2):

```go
EnsureMachineImage(name, imageType, source string) error
RemoveMachineImage(name string) error
WriteNspawnFile(name, content string) error
ReadNspawnFile(name string) (string, error)
RemoveNspawnFile(name string) error
ShowMachine(name string) (MachineStatus, error)
```

- Consumes: existing `safeName`, `UnitStatus`

- [ ] **Step 1: Write failing tests for argv mapping**

```go
package remote

import "testing"

func TestMachinectlEnsureArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ, source string
		want        []string
	}{
		{"tar", "https://example.com/a.tar", []string{"pull-tar", "https://example.com/a.tar", "web"}},
		{"raw", "https://example.com/a.raw", []string{"pull-raw", "https://example.com/a.raw", "web"}},
		{"oci", "docker.io/library/debian:bookworm", []string{"pull-dkr", "docker.io/library/debian:bookworm", "web"}},
		{"local", "/var/tmp/mini.tar", []string{"import-tar", "/var/tmp/mini.tar", "web"}},
	}
	for _, tc := range cases {
		got, err := MachinectlEnsureArgs("web", mustType(t, tc.typ), tc.source)
		if err != nil {
			t.Fatalf("%s: %v", tc.typ, err)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %#v want %#v", tc.typ, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %#v want %#v", tc.typ, got, tc.want)
			}
		}
	}
}

func TestMachinectlEnsureArgsLocalDir(t *testing.T) {
	t.Parallel()
	// Directory sources use clone/copy semantics documented as import via
	// machinectl clone when source is already under /var/lib/machines, else
	// the helper returns an error directing callers to use a .tar/.raw path.
	// For MVP: paths ending in / or without .tar/.raw/.tar.gz/.tgz/.tar.xz
	// are rejected with a clear error (ACC fixture is always a .tar).
	_, err := MachinectlEnsureArgs("web", MachineImageLocal, "/var/tmp/rootfs")
	if err == nil {
		t.Fatal("expected error for local directory path")
	}
}

func TestNspawnUnitName(t *testing.T) {
	t.Parallel()
	if got := NspawnUnitName("web"); got != "systemd-nspawn@web.service" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL (undefined)**

Run: `go test ./internal/remote/ -run 'TestMachinectlEnsureArgs|TestNspawnUnitName' -count=1`
Expected: FAIL compile or undefined symbols

- [ ] **Step 3: Implement helpers + extend Host**

In `machine.go`:

```go
package remote

import (
	"fmt"
	"path"
	"strings"
)

const DefaultNspawnDir = "/etc/systemd/nspawn"

type MachineImageType string

const (
	MachineImageLocal MachineImageType = "local"
	MachineImageTar   MachineImageType = "tar"
	MachineImageRaw   MachineImageType = "raw"
	MachineImageOCI   MachineImageType = "oci"
)

func ParseMachineImageType(s string) (MachineImageType, error) {
	switch MachineImageType(s) {
	case MachineImageLocal, MachineImageTar, MachineImageRaw, MachineImageOCI:
		return MachineImageType(s), nil
	default:
		return "", fmt.Errorf("invalid image.type %q (want local|tar|raw|oci)", s)
	}
}

func NspawnUnitName(machine string) string {
	return "systemd-nspawn@" + machine + ".service"
}

func nspawnSettingsPath(name string) (string, error) {
	if err := safeName(name); err != nil {
		return "", err
	}
	return path.Join(DefaultNspawnDir, name+".nspawn"), nil
}

// MachinectlEnsureArgs returns machinectl subcommand argv (without "machinectl").
func MachinectlEnsureArgs(name string, t MachineImageType, source string) ([]string, error) {
	if err := safeName(name); err != nil {
		return nil, err
	}
	if source == "" {
		return nil, fmt.Errorf("image.source is required")
	}
	switch t {
	case MachineImageTar:
		return []string{"pull-tar", source, name}, nil
	case MachineImageRaw:
		return []string{"pull-raw", source, name}, nil
	case MachineImageOCI:
		return []string{"pull-dkr", source, name}, nil
	case MachineImageLocal:
		lower := strings.ToLower(source)
		switch {
		case strings.HasSuffix(lower, ".raw"):
			return []string{"import-raw", source, name}, nil
		case strings.HasSuffix(lower, ".tar"),
			strings.HasSuffix(lower, ".tar.gz"),
			strings.HasSuffix(lower, ".tgz"),
			strings.HasSuffix(lower, ".tar.xz"):
			return []string{"import-tar", source, name}, nil
		default:
			return nil, fmt.Errorf("local image.source %q must be a .tar/.tar.gz/.tgz/.tar.xz/.raw file path", source)
		}
	default:
		return nil, fmt.Errorf("unsupported image type %q", t)
	}
}

type MachineStatus struct {
	ImagePresent bool
	Unit         UnitStatus
	Settings     string // empty if no .nspawn file
}
```

Add the six methods to `Host` in `host.go`.

Add stub methods on `Fake` that return `fmt.Errorf("not implemented")` so the tree compiles (full Fake in Task 2). Same temporary stubs on `*Client` (SSH) and `*Nspawn` if the compiler requires them — or implement empty bodies that `panic("TODO")` only if you cannot leave the build broken between tasks; prefer compiling stubs.

- [ ] **Step 4: Run tests — expect PASS**

Run: `go test ./internal/remote/ -run 'TestMachinectlEnsureArgs|TestNspawnUnitName|TestMachinectlEnsureArgsLocalDir' -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/remote/host.go internal/remote/machine.go internal/remote/machine_test.go internal/remote/fake.go internal/remote/ssh.go internal/remote/nspawn.go
git commit -m "$(cat <<'EOF'
feat: add machine image helpers and Host method stubs

EOF
)"
```

---

### Task 2: Fake + SSH + Nspawn Host implementations

**Files:**
- Modify: `internal/remote/fake.go`
- Modify: `internal/remote/ssh.go`
- Modify: `internal/remote/nspawn.go`
- Create or modify: `internal/remote/fake_test.go` (machine cases)

**Interfaces:**
- Consumes: `MachinectlEnsureArgs`, `nspawnSettingsPath`, `NspawnUnitName`, `MachineStatus`
- Produces: full Host machine methods on Fake, `*Client` (SSH), `*Nspawn`

**Create / image-exists policy (locked for this plan):** If `machinectl show-image <name>` succeeds before Ensure, **remove** it first (`machinectl remove`) then pull/import. Document in method godoc. Avoids ambiguous “adopt” fingerprinting.

- [ ] **Step 1: Fake implementation + test**

```go
// on Fake — add fields:
Images map[string]bool // name → present

func (f *Fake) EnsureMachineImage(name, imageType, source string) error {
	t, err := ParseMachineImageType(imageType)
	if err != nil {
		return err
	}
	args, err := MachinectlEnsureArgs(name, t, source)
	if err != nil {
		return err
	}
	if err := f.note("machinectl " + strings.Join(args, " ")); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Images == nil {
		f.Images = map[string]bool{}
	}
	f.Images[name] = true
	return nil
}

func (f *Fake) RemoveMachineImage(name string) error {
	if err := safeName(name); err != nil {
		return err
	}
	if err := f.note("machinectl remove " + name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Images, name)
	return nil
}

func (f *Fake) WriteNspawnFile(name, content string) error {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return err
	}
	return f.write(p, content)
}

func (f *Fake) ReadNspawnFile(name string) (string, error) {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return "", err
	}
	return f.read(p)
}

func (f *Fake) RemoveNspawnFile(name string) error {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return err
	}
	return f.remove(p)
}

func (f *Fake) ShowMachine(name string) (MachineStatus, error) {
	if err := safeName(name); err != nil {
		return MachineStatus{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st := MachineStatus{ImagePresent: f.Images[name]}
	st.Unit = f.Statuses[NspawnUnitName(name)]
	if c, ok := f.Files[path.Join(DefaultNspawnDir, name+".nspawn")]; ok {
		st.Settings = c
	}
	return st, nil
}
```

Test: Ensure → Show ImagePresent → WriteNspawn → Read → Remove image.

- [ ] **Step 2: SSH `*Client` methods**

Pattern (mirror existing `run` / `atomicWrite`):

```go
func (c *Client) EnsureMachineImage(name, imageType, source string) error {
	t, err := ParseMachineImageType(imageType)
	if err != nil {
		return err
	}
	args, err := MachinectlEnsureArgs(name, t, source)
	if err != nil {
		return err
	}
	// If image exists, remove first (ignore errors from show-image miss).
	if _, err := c.run("machinectl show-image " + shellQuote(name)); err == nil {
		_ = c.mustOK("machinectl remove " + shellQuote(name))
	}
	return c.mustOK("machinectl " + shellJoin(args))
}
```

Implement `shellQuote` / `shellJoin` if not present — prefer appending carefully escaped tokens. Existing code often concatenates; follow the same style as `EnableUnit` for consistency, but **quote** `name` and URL/ref with `strconv.Quote` or a small helper to avoid injection.

`WriteNspawnFile`: `atomicWrite` to `nspawnSettingsPath` after `mkdir -p /etc/systemd/nspawn`.  
`ShowMachine`: `machinectl show-image` (present if exit 0); `UnitStatus(NspawnUnitName(name))`; optional read settings file (missing → empty Settings).

- [ ] **Step 3: Nspawn Host methods**

Same semantics via `n.run("machinectl", ...)` / copy helpers already used for files. For `WriteNspawnFile`, write via existing copy-to path used by `WriteUnit`.

- [ ] **Step 4: Compile + unit tests**

Run: `go test ./internal/remote/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/remote/
git commit -m "$(cat <<'EOF'
feat: implement Host machine image and nspawn file ops

EOF
)"
```

---

### Task 3: Provider Client orchestration

**Files:**
- Modify: `internal/provider/client.go`
- Create: `internal/provider/client_machine_test.go`

**Interfaces:**
- Consumes: `remote.Host` machine methods, `remote.NspawnUnitName`, `ApplyUnitLifecycle`
- Produces:

```go
func (c *Client) PutMachine(ctx context.Context, name, imageType, source string, settings *string, enable, active *bool) error
func (c *Client) GetMachine(ctx context.Context, name string) (remote.MachineStatus, error)
func (c *Client) DeleteMachine(ctx context.Context, name string, deleteImage bool) error
```

Semantics:

**PutMachine**
1. `EnsureMachineImage(name, imageType, source)`
2. If `settings != nil` && `*settings != ""` → `WriteNspawnFile`; if `settings != nil` && `*settings == ""` → `RemoveNspawnFile` (explicit clear). If `settings == nil` → leave file untouched (Create always passes pointer when config has body; resource layer decides).
3. `DaemonReload()`
4. `ApplyUnitLifecycle(ctx, remote.NspawnUnitName(name), enable, active)`

**DeleteMachine**
1. `DeleteUnitLifecycle(ctx, remote.NspawnUnitName(name))`
2. `RemoveNspawnFile` (ignore not exist)
3. If `deleteImage` → `RemoveMachineImage`

- [ ] **Step 1: Failing Client test against Fake**

```go
func TestPutDeleteMachine(t *testing.T) {
	t.Parallel()
	f := remote.NewFake()
	c := &Client{Host: f}
	settings := "[Exec]\nBoot=no\n"
	en, act := true, true
	if err := c.PutMachine(context.Background(), "web", "local", "/tmp/a.tar", &settings, &en, &act); err != nil {
		t.Fatal(err)
	}
	st, err := c.GetMachine(context.Background(), "web")
	if err != nil || !st.ImagePresent || st.Settings == "" {
		t.Fatalf("status=%+v err=%v", st, err)
	}
	if err := c.DeleteMachine(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	st, _ = c.GetMachine(context.Background(), "web")
	if st.ImagePresent {
		t.Fatal("image should be gone")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/provider/ -run TestPutDeleteMachine -count=1`
Expected: FAIL undefined

- [ ] **Step 3: Implement Client methods**

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./internal/provider/ -run TestPutDeleteMachine -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/provider/client.go internal/provider/client_machine_test.go
git commit -m "$(cat <<'EOF'
feat: add Client PutMachine GetMachine DeleteMachine

EOF
)"
```

---

### Task 4: `systemd_machine` resource

**Files:**
- Create: `internal/provider/resource_machine.go`
- Create: `internal/provider/resource_machine_test.go`
- Modify: `internal/provider/provider.go` (register)
- Modify: `internal/provider/schema_test.go` (add `NewMachineResource`)

**Interfaces:**
- Consumes: `Client.PutMachine` / `GetMachine` / `DeleteMachine`, `resolveContent` / section helpers from `sections.go`, validators from `validators.go`
- Produces: `NewMachineResource() resource.Resource` → type name `systemd_machine`

**Model:**

```go
type machineImageModel struct {
	Type   types.String `tfsdk:"type"`
	Source types.String `tfsdk:"source"`
}

type machineModel struct {
	Name        types.String      `tfsdk:"name"`
	Image       []machineImageModel `tfsdk:"image"` // list nesting; MaxItems 1
	Content     types.String      `tfsdk:"content"`
	Sections    []sectionModel    `tfsdk:"section"`
	Enable      types.Bool        `tfsdk:"enable"`
	Active      types.Bool        `tfsdk:"active"`
	DeleteImage types.Bool        `tfsdk:"delete_image"`
	ID          types.String      `tfsdk:"id"`
}
```

Use `schema.ListNestedBlock` for `image` with `listvalidator.SizeBetween(1, 1)` (same pattern as other nested blocks in the codebase if any; otherwise `SingleNestedBlock` if Framework version supports it — prefer **SingleNestedBlock** `image` if available in the module’s framework version).

Schema highlights:
- `name`: Required, `RequiresReplace`, `noPathSegment()` (reuse)
- `image.type`: Required, validator one of `local|tar|raw|oci`, `RequiresReplace`
- `image.source`: Required, non-empty, `RequiresReplace`
- `delete_image`: Optional+Computed, `booldefault.StaticBool(true)`
- `content` / `section`: same XOR validation as units (`ValidateConfig`)
- `enable` / `active`: optional bools like units

**CRUD:**
- Create/Update: resolve body via existing section render; pass `*string` for settings (nil if neither content nor sections — do not write file; empty string if clearing on update from prior settings — track via state Read)
- For Create with no settings: pass `nil` and do not call Write/Remove
- Delete: `DeleteMachine(ctx, name, deleteImage)` using planned/state `delete_image` (default true)
- Read: `GetMachine`; if `!ImagePresent` → remove resource from state; set content from Settings when possible (raw only — if state used sections, keep sections from state like other resources that don’t round-trip INI parse — **follow `unitResource` Read behaviour**)
- Import: `path.Root("id")` ← name; set `name`; leave image unknown → require operator to set image in config (document)

- [ ] **Step 1: Schema unit test**

Add `NewMachineResource` to `TestResourceSchemas`. Add `ValidateConfig` test: missing image fails; content+section fails; good OCI config OK.

- [ ] **Step 2: Run schema tests — FAIL until resource exists**

Run: `go test ./internal/provider/ -run 'TestResourceSchemas|TestMachineValidate' -count=1`

- [ ] **Step 3: Implement resource + register**

Mirror structure of `resource_credential.go` + unit Create/Update/Delete wiring from `resource_unit.go`.

- [ ] **Step 4: Full provider unit tests**

Run: `go test ./internal/provider/ -count=1`
Expected: PASS (ACC skipped without `TF_ACC`)

- [ ] **Step 5: Commit**

```bash
git add internal/provider/
git commit -m "$(cat <<'EOF'
feat: add systemd_machine resource

EOF
)"
```

---

### Task 5: ACC fixture harness + `TestAccMachineLifecycle`

**Files:**
- Modify: `scripts/acc-nspawn.sh`
- Create: `internal/provider/acc_machine_test.go`

**Interfaces:**
- Consumes: `accClient(t)`, `Client.PutMachine` / `DeleteMachine` / `GetMachine`, Host methods
- Produces: green `TestAccMachineLifecycle` under `make testacc`

**Harness changes:**
1. Add `systemd-container` to debootstrap `--include=` so the guest has `machinectl`.
2. After machine ready, build fixture **inside the guest**:

```bash
# Minimal Boot=no rootfs as a tar the provider will import-tar.
systemd-run -M "${MACHINE}" -P --wait -- bash -c '
  set -euo pipefail
  FIX=/var/tmp/tf-acc-mini
  rm -rf "$FIX" /var/tmp/tf-acc-mini.tar
  mkdir -p "$FIX/usr/bin"
  cp -a /usr/bin/sleep "$FIX/usr/bin/sleep"
  tar -C "$FIX" -cf /var/tmp/tf-acc-mini.tar .
'
```

3. Document that nested machines need the guest capable of nspawn (privileged ACC host already is).

**ACC test sketch:**

```go
func TestAccMachineLifecycle(t *testing.T) {
	c := accClient(t)
	ctx := context.Background()
	name := "tf-acc-nested"
	settings := "[Exec]\nBoot=no\nParameters=/usr/bin/sleep infinity\n"
	en, act := true, true
	t.Cleanup(func() {
		_ = c.DeleteMachine(context.Background(), name, true)
	})
	if err := c.PutMachine(ctx, name, "local", "/var/tmp/tf-acc-mini.tar", &settings, &en, &act); err != nil {
		t.Fatalf("create: %v", err)
	}
	st, err := c.GetMachine(ctx, name)
	if err != nil || !st.ImagePresent {
		t.Fatalf("read: %+v %v", st, err)
	}
	if st.Unit.ActiveState != "active" {
		t.Fatalf("want active, got %+v", st.Unit)
	}
	settings2 := settings + "[Network]\nVirtualEthernet=no\n"
	if err := c.PutMachine(ctx, name, "local", "/var/tmp/tf-acc-mini.tar", &settings2, &en, &act); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := c.DeleteMachine(ctx, name, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	st, _ = c.GetMachine(ctx, name)
	if st.ImagePresent {
		t.Fatal("image should be removed")
	}
}
```

If nested start is flaky on GHA, fail hard with the systemctl error (no skip) — fix harness (e.g. drop `PrivateUsers` limitations; ACC outer `.nspawn` may need `[Exec] Capability=all` or `SystemCallFilter=` tweaks). Prefer fixing harness over skipping.

- [ ] **Step 1: Update harness include + fixture staging**

- [ ] **Step 2: Write ACC test**

- [ ] **Step 3: Run ACC**

Run: `make testacc`
Expected: all `TestAcc*` PASS including `TestAccMachineLifecycle`

- [ ] **Step 4: Commit**

```bash
git add scripts/acc-nspawn.sh internal/provider/acc_machine_test.go
git commit -m "$(cat <<'EOF'
test: add ACC coverage for systemd_machine via local tar fixture

EOF
)"
```

---

### Task 6: Docs, example, issue, release prep

**Files:**
- Create: `examples/machine/main.tf`
- Modify: `README.md` (resource section + bump “Latest release” only when tagging)
- Modify: GitHub issue #4 body/comment with spec + plan links

**Example:**

```hcl
resource "systemd_machine" "demo" {
  name         = "demo"
  enable       = true
  active       = true
  delete_image = true

  image {
    type   = "local"
    source = "/var/tmp/demo.tar"
  }

  content = <<-EOT
    [Exec]
    Boot=no
    Parameters=/usr/bin/sleep infinity
  EOT
}
```

Also document OCI/`pull-dkr` in README with a short note that ACC does not exercise OCI.

- [ ] **Step 1: Write example + README section**

- [ ] **Step 2: `go test ./...` and `make testacc`**

- [ ] **Step 3: Commit docs**

```bash
git add examples/machine/main.tf README.md
git commit -m "$(cat <<'EOF'
docs: document systemd_machine resource and example

EOF
)"
```

- [ ] **Step 4: Comment on GitHub #4** with links to spec + this plan (phase 1). Leave issue open until phase 2.

- [ ] **Step 5: Tag only after CI green on main** — `0.7.0` (no `v`), push tag; do **not** tag from this task unless the user explicitly asks.

---

## Spec coverage checklist

| Spec item | Task |
|-----------|------|
| `systemd_machine` schema (image, delete_image, content XOR section) | 4 |
| Image sources local/tar/raw/oci argv | 1–2 |
| SFTP `.nspawn` | 2–3 |
| systemctl on `systemd-nspawn@` | 3–4 |
| delete_image default true | 4 |
| RequiresReplace on image.* | 4 |
| Host SSH + Nspawn + Fake | 2 |
| ACC local fixture, no OCI in CI | 5 |
| Docs / example | 6 |
| `systemd_portable` | deferred (not in plan) |

## Placeholder / consistency self-review

- No TBD steps; local directory import explicitly rejected (tar/raw only) — matches ACC fixture and open point in spec.
- Pre-existing image → remove then re-import (locked).
- Runtime path consistently `systemctl` via `ApplyUnitLifecycle` + `NspawnUnitName`.
