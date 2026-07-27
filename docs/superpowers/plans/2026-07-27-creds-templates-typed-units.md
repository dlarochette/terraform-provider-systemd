# Credentials, Templates & Typed Units Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `systemd_socket`/`systemd_target`, template-name support, `systemd_instance`, and `systemd_credential` to the SSH-only Terraform systemd provider.

**Architecture:** Reuse `unitLikeResource` for socket/target. Allow `@` in unit names. New instance resource only calls `systemctl` (no file). New credential resource writes `/etc/credstore` or encrypts via `systemd-creds` into `/etc/credstore.encrypted`. Unit wiring stays `section`/`entry`.

**Tech Stack:** Go, Terraform Plugin Framework, golang.org/x/crypto/ssh, github.com/pkg/sftp, existing `internal/remote` + `internal/provider`.

**Spec:** `docs/superpowers/specs/2026-07-27-systemd-creds-templates-design.md`

## Global Constraints

- Tags: SemVer without `v` prefix
- Conventional Commits, English messages
- No secrets in logs or test output; `data` is Framework `Sensitive: true`
- Root SSH assumed; paths `/etc/credstore`, `/etc/credstore.encrypted`
- Unit wiring for credentials: `section`/`entry` only (no typed blocks)
- Docs (README + examples + schema dump) updated with each feature commit
- Working tree already has unfinished `NewTargetResource` — finish it in Task 1

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/provider/resource_unit_like.go` | Add `NewSocketResource`, ensure `NewTargetResource` |
| `internal/provider/validators.go` | Allow `@` in unit names; template/instance/credential validators |
| `internal/provider/instance_name.go` | Parse/build `app@bar.service` ↔ template+instance |
| `internal/provider/resource_instance.go` | `systemd_instance` resource |
| `internal/provider/resource_credential.go` | `systemd_credential` resource |
| `internal/remote/host.go` | Extend `Host` with credential methods + constants |
| `internal/remote/ssh.go` | Implement credential ops (+ `runWithStdin`) |
| `internal/remote/fake.go` | Fake credential ops |
| `internal/provider/client.go` | Thin wrappers |
| `internal/provider/provider.go` | Register resources |
| `examples/basic/main.tf`, `README.md`, `schemas/provider.json` | Docs |

---

### Task 1: `systemd_socket` + finish `systemd_target`

**Files:**
- Modify: `internal/provider/resource_unit_like.go`
- Modify: `internal/provider/provider.go`
- Modify: `internal/provider/schema_test.go`
- Modify: `README.md`, `examples/basic/main.tf`, `docs/superpowers/specs/2026-07-27-terraform-systemd-design.md`
- Run: `make schema`

**Interfaces:**
- Produces: `NewSocketResource() resource.Resource`, `NewTargetResource() resource.Resource` (typeNames `_socket`, `_target`)

- [ ] **Step 1: Ensure constructors exist**

In `resource_unit_like.go`, ensure both:

```go
func NewSocketResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_socket",
		suffix:   ".socket",
		doc:      "Manages a systemd `.socket` unit under `/etc/systemd/system`. Pair with a matching `.service`.",
	}
}

func NewTargetResource() resource.Resource {
	return &unitLikeResource{
		typeName: "_target",
		suffix:   ".target",
		doc:      "Manages a systemd `.target` unit under `/etc/systemd/system` (grouping / synchronization point for other units).",
	}
}
```

- [ ] **Step 2: Register + schema tests**

In `provider.go` `Resources()` and `schema_test.go` `TestResourceSchemas`, include `NewSocketResource` and `NewTargetResource` (after automount, before dropin).

- [ ] **Step 3: Docs + example**

Add rows to README resources table. In `examples/basic/main.tf` add a minimal `systemd_socket` and keep/finish `systemd_target`. Bump example `version = ">= 0.3.0"`.

- [ ] **Step 4: Test + schema**

```bash
cd /home/dla/Projets/david.larochette/terraform-provider-systemd
go test ./internal/provider/ -count=1
make schema VERSION=0.3.0
```

Expected: PASS; `schemas/provider.json` lists `systemd_socket` and `systemd_target`.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/resource_unit_like.go internal/provider/provider.go internal/provider/schema_test.go \
  README.md examples/basic/main.tf docs/superpowers/specs/2026-07-27-terraform-systemd-design.md schemas/provider.json
git commit -m "$(cat <<'EOF'
feat: add systemd_socket and systemd_target resources

EOF
)"
```

---

### Task 2: Allow `@` in unit names + template/instance helpers

**Files:**
- Modify: `internal/provider/validators.go`
- Modify: `internal/provider/validators_test.go`
- Create: `internal/provider/instance_name.go`
- Create: `internal/provider/instance_name_test.go`

**Interfaces:**
- Produces:
  - `templateNameValidator() validator.String`
  - `instanceStringValidator() validator.String`
  - `func InstantiateUnit(template, instance string) (string, error)`
  - `func ParseInstantiatedUnit(name string) (template, instance string, err error)`

- [ ] **Step 1: Failing tests for validators and parse helpers**

Add to `validators_test.go`:

```go
func TestUnitNameAllowsTemplate(t *testing.T) {
	t.Parallel()
	v := unitNameValidator()
	for _, ok := range []string{"app@.service", "app@bar.service"} {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(ok)}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("%s: %v", ok, resp.Diagnostics)
		}
	}
}

func TestTemplateNameValidator(t *testing.T) {
	t.Parallel()
	v := templateNameValidator()
	ok := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue("app@.service")}, ok)
	if ok.Diagnostics.HasError() {
		t.Fatal(ok.Diagnostics)
	}
	bad := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue("app@bar.service")}, bad)
	if !bad.Diagnostics.HasError() {
		t.Fatal("expected reject instantiated name as template")
	}
}
```

Create `instance_name_test.go`:

```go
package provider

import "testing"

func TestInstantiateUnit(t *testing.T) {
	got, err := InstantiateUnit("app@.service", "bar")
	if err != nil || got != "app@bar.service" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestParseInstantiatedUnit(t *testing.T) {
	tpl, inst, err := ParseInstantiatedUnit("app@bar.service")
	if err != nil || tpl != "app@.service" || inst != "bar" {
		t.Fatalf("%q %q %v", tpl, inst, err)
	}
	if _, _, err := ParseInstantiatedUnit("app@.service"); err == nil {
		t.Fatal("expected error for template-only name")
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/provider/ -run 'TestUnitNameAllowsTemplate|TestTemplateNameValidator|TestInstantiateUnit|TestParseInstantiatedUnit' -count=1
```

Expected: FAIL (undefined helpers / template names rejected).

- [ ] **Step 3: Implement**

Update `unitNameValidator` regex to allow `@` in the basename:

```go
mustCompile(`^[^/@]+(@[^/@]*)?\.(service|socket|timer|path|mount|automount|swap|target|slice|scope|device)$`),
```

(`unitSuffixValidator` already only checks suffix; `@` is fine via `noPathSegment`.)

Add:

```go
func templateNameValidator() validator.String {
	return stringvalidator.All(
		noPathSegment(),
		stringvalidator.RegexMatches(
			mustCompile(`^[^/@]+@\.(service|socket|timer|path|mount|automount|swap|target|slice|scope|device)$`),
			"must be a template unit name (e.g. app@.service)",
		),
	)
}

func instanceStringValidator() validator.String {
	return stringvalidator.All(
		stringvalidator.LengthAtLeast(1),
		stringvalidator.RegexMatches(
			mustCompile(`^[^/@]+$`),
			"instance must not contain '@' or '/'",
		),
	)
}
```

`instance_name.go`:

```go
package provider

import (
	"fmt"
	"strings"
)

func InstantiateUnit(template, instance string) (string, error) {
	at := strings.Index(template, "@.")
	if at < 0 {
		return "", fmt.Errorf("not a template unit name: %q", template)
	}
	if instance == "" || strings.ContainsAny(instance, "@/") {
		return "", fmt.Errorf("invalid instance %q", instance)
	}
	return template[:at+1] + instance + template[at+1:], nil
}

func ParseInstantiatedUnit(name string) (template, instance string, err error) {
	at := strings.Index(name, "@")
	if at < 0 {
		return "", "", fmt.Errorf("not an instantiated unit: %q", name)
	}
	rest := name[at+1:]
	sufAt := strings.LastIndex(rest, ".")
	if sufAt <= 0 {
		return "", "", fmt.Errorf("not an instantiated unit: %q", name)
	}
	instance = rest[:sufAt]
	if instance == "" {
		return "", "", fmt.Errorf("template name has empty instance: %q", name)
	}
	suffix := rest[sufAt:] // e.g. ".service"
	template = name[:at+1] + suffix // "app@" + ".service"
	return template, instance, nil
}
```

(Fix: `template = name[:at+1] + suffix` yields `app@.service` — correct.)

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/provider/ -run 'TestUnitNameAllowsTemplate|TestTemplateNameValidator|TestInstantiateUnit|TestParseInstantiatedUnit' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/provider/validators.go internal/provider/validators_test.go \
  internal/provider/instance_name.go internal/provider/instance_name_test.go
git commit -m "$(cat <<'EOF'
feat: support systemd template unit names and instance parsing

EOF
)"
```

---

### Task 3: `systemd_instance` resource

**Files:**
- Create: `internal/provider/resource_instance.go`
- Modify: `internal/provider/client.go` — add `ApplyUnitLifecycle(ctx, name string, enable, active *bool) error` extracted from PutUnit’s enable/active block (optional refactor; or call Enable/Start via Host through thin methods)
- Modify: `internal/provider/provider.go`, `schema_test.go`
- Modify: `examples/basic/main.tf`, `README.md`
- Run: `make schema`

**Interfaces:**
- Consumes: `InstantiateUnit`, `ParseInstantiatedUnit`, `templateNameValidator`, `instanceStringValidator`, `Client.Host.EnableUnit/DisableUnit/StartUnit/StopUnit`
- Produces: `NewInstanceResource() resource.Resource` → type `systemd_instance`

- [ ] **Step 1: Add `ApplyUnitLifecycle` on Client**

In `client.go`:

```go
func (c *Client) ApplyUnitLifecycle(ctx context.Context, name string, enable, active *bool) error {
	_ = ctx
	if enable != nil {
		if *enable {
			if err := c.Host.EnableUnit(name); err != nil {
				return err
			}
		} else if err := c.Host.DisableUnit(name); err != nil {
			return err
		}
	}
	if active != nil {
		if *active {
			if err := c.Host.StartUnit(name); err != nil {
				return fmt.Errorf("start %s: %w", name, err)
			}
		} else if err := c.Host.StopUnit(name); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) DeleteUnitLifecycle(ctx context.Context, name string) {
	_ = ctx
	_ = c.Host.StopUnit(name)
	_ = c.Host.DisableUnit(name)
}
```

Refactor `PutUnit` / `DeleteUnit` to call these (behavior unchanged).

- [ ] **Step 2: Implement `resource_instance.go`**

Schema attributes: `template` (Required, RequiresReplace), `instance` (Required, RequiresReplace), optional `enable`/`active`, computed `id`.

Create/Update: `name, err := InstantiateUnit(...)` then `ApplyUnitLifecycle`. Set `id` to instantiated name.  
Delete: `DeleteUnitLifecycle`.  
Import: passthrough on `id`, then in Read parse into template+instance (or ImportState sets `id` and Read fills fields — prefer custom ImportState that sets template/instance/id via `ParseInstantiatedUnit`).

Read: `UnitStatus`; if LoadState is `not-found` and unit file state empty, remove resource; else keep config attributes from state (lifecycle desired state is config-driven).

- [ ] **Step 3: Register + schema test + example**

Example (depends on a template unit):

```hcl
resource "systemd_unit" "app_template" {
  name = "app@.service"
  section {
    name = "Unit"
    entry {
      key   = "Description"
      value = "App instance %i"
    }
  }
  section {
    name = "Service"
    entry {
      key   = "ExecStart"
      value = "/usr/local/bin/app %i"
    }
  }
}

resource "systemd_instance" "app_bar" {
  template = systemd_unit.app_template.name
  instance = "bar"
  enable   = true
  active   = true
}
```

- [ ] **Step 4: Test**

```bash
go test ./internal/provider/ -count=1
make schema VERSION=0.3.0
```

Expected: PASS; schema includes `systemd_instance`.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/resource_instance.go internal/provider/client.go \
  internal/provider/provider.go internal/provider/schema_test.go \
  examples/basic/main.tf README.md schemas/provider.json
git commit -m "$(cat <<'EOF'
feat: add systemd_instance for template unit lifecycle

EOF
)"
```

---

### Task 4: Remote credential host API

**Files:**
- Modify: `internal/remote/host.go`
- Modify: `internal/remote/ssh.go`
- Modify: `internal/remote/fake.go`
- Create: `internal/remote/credential_test.go` (fake-based)
- Modify: `internal/provider/client.go`

**Interfaces:**
- Produces on `Host`:
  - `WriteCredential(name, data string) error` → `/etc/credstore/{name}` mode 0600
  - `WriteCredentialEncrypted(name, data, withKey string) error` → encrypt to `/etc/credstore.encrypted/{name}`
  - `CredentialExists(name string, encrypted bool) (bool, error)`
  - `RemoveCredential(name string, encrypted bool) error`
- Constants: `DefaultCredstoreDir = "/etc/credstore"`, `DefaultCredstoreEncryptedDir = "/etc/credstore.encrypted"`

- [ ] **Step 1: Failing Fake test**

```go
func TestFakeCredentialRoundTrip(t *testing.T) {
	f := NewFake()
	if err := f.WriteCredential("db", "secret"); err != nil {
		t.Fatal(err)
	}
	ok, err := f.CredentialExists("db", false)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if err := f.WriteCredentialEncrypted("db", "secret", "host"); err != nil {
		t.Fatal(err)
	}
	ok, err = f.CredentialExists("db", true)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if err := f.RemoveCredential("db", true); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (interface incomplete)

```bash
go test ./internal/remote/ -run TestFakeCredentialRoundTrip -count=1
```

- [ ] **Step 3: Implement Host + Fake + SSH**

`host.go`: add methods to interface; add path helpers `credPath(encrypted bool, name string)`.

`fake.go`: store under those paths; for encrypted, store placeholder `"encrypted:"+data` and note command `systemd-creds encrypt …`.

`ssh.go`:
- `WriteCredential`: `atomicWrite` then `sftp.Chmod(path, 0o600)` (extend `atomicWrite` with optional mode or chmod after write).
- `WriteCredentialEncrypted`: `runWithStdin` piping data:

```go
func (c *Client) runWithStdin(cmd string, stdin []byte) (string, error) {
	session, err := c.ssh.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	session.Stdin = bytes.NewReader(stdin)
	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf
	err = session.Run(cmd)
	return buf.String(), err
}
```

Command (mkdir first via SFTP `MkdirAll`):

```text
systemd-creds encrypt --name=<name> [--with-key=<key>] - <dest>
```

Omit `--with-key` when `withKey` is empty or `auto`.

`CredentialExists`: try `sftp.Stat`.  
`RemoveCredential`: `removeFile`.

- [ ] **Step 4: Client wrappers**

```go
func (c *Client) PutCredential(ctx context.Context, name, data string, encrypted bool, withKey string) error
func (c *Client) HasCredential(ctx context.Context, name string, encrypted bool) (bool, error)
func (c *Client) DeleteCredential(ctx context.Context, name string, encrypted bool) error
```

- [ ] **Step 5: Test + commit**

```bash
go test ./internal/remote/ ./internal/provider/ -count=1
git add internal/remote/ internal/provider/client.go
git commit -m "$(cat <<'EOF'
feat: add remote credential store operations

EOF
)"
```

---

### Task 5: `systemd_credential` resource + docs

**Files:**
- Create: `internal/provider/resource_credential.go`
- Create: `internal/provider/resource_credential_test.go` (schema + with_key validator)
- Modify: `internal/provider/provider.go`, `schema_test.go`
- Modify: `examples/basic/main.tf`, `README.md`
- Modify: design table in `docs/superpowers/specs/2026-07-27-terraform-systemd-design.md` (pointer to creds-templates spec)
- Run: `make schema`

**Interfaces:**
- Consumes: `Client.PutCredential/HasCredential/DeleteCredential`
- Produces: `NewCredentialResource() resource.Resource`

- [ ] **Step 1: Schema + CRUD**

Attributes:
- `name` (Required, RequiresReplace, `noPathSegment`)
- `data` (Required, Sensitive, `nonEmptyContent`)
- `encrypted` (Optional, default `true` via `booldefault.StaticBool(true)`)
- `with_key` (Optional; validator one of `auto|host|tpm2|host+tpm2`; only meaningful when encrypted)
- `id` computed = name

Create/Update: `PutCredential`.  
Read: if `!HasCredential` → RemoveResource; keep `data` from state (do not refresh from remote).  
Delete: `DeleteCredential` with encrypted flag from state.

ValidateConfig: if `encrypted=false` and `with_key` set → error.

- [ ] **Step 2: Example wiring**

```hcl
resource "systemd_credential" "db" {
  name      = "db-pass"
  data      = var.db_password   # sensitive
  encrypted = true
  with_key  = "host"
}

resource "systemd_unit" "app" {
  name = "app.service"
  # …
  section {
    name = "Service"
    entry {
      key   = "LoadCredentialEncrypted"
      value = "db-pass"
    }
  }
}
```

Document that relative `LoadCredentialEncrypted=name` searches `/etc/credstore.encrypted/`.

- [ ] **Step 3: Test + schema**

```bash
go test ./... -count=1
make schema VERSION=0.3.0
```

- [ ] **Step 4: Commit**

```bash
git add internal/provider/resource_credential.go internal/provider/resource_credential_test.go \
  internal/provider/provider.go internal/provider/schema_test.go \
  examples/basic/main.tf README.md schemas/provider.json \
  docs/superpowers/specs/2026-07-27-terraform-systemd-design.md
git commit -m "$(cat <<'EOF'
feat: add systemd_credential for host credstore management

EOF
)"
```

---

### Task 6: Release 0.3.0

**Files:** none beyond ensuring README latest release link is `0.3.0`

- [ ] **Step 1: Verify clean tree and tests**

```bash
go test ./... -count=1
git status -sb
```

- [ ] **Step 2: Push + tag**

```bash
git push origin main
git tag 0.3.0
git push origin 0.3.0
```

- [ ] **Step 3: Watch release**

```bash
gh run watch --repo dlarochette/terraform-provider-systemd
gh release view 0.3.0 --repo dlarochette/terraform-provider-systemd
```

Expected: non-draft release with linux/darwin amd64+arm64 zips + SHA256SUMS.

---

## Spec coverage check

| Spec item | Task |
|-----------|------|
| `systemd_socket` / `systemd_target` | 1 |
| Template names on unit-like | 2 |
| `systemd_instance` | 3 |
| `systemd_credential` + encrypt paths | 4–5 |
| Unit wiring via section/entry | 5 (docs/example) |
| Validators `@` / template / instance | 2 |
| Out of scope items not implemented | — |
| SemVer tag without `v` | 6 |
