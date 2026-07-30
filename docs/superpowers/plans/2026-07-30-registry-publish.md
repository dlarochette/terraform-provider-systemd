# Registry Publish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce GPG-signed GitHub releases so `dlarochette/systemd` can be listed on Terraform and OpenTofu registries, starting at `0.10.1`.

**Architecture:** Extend existing GoReleaser + CI with HashiCorp-style `signs`, registry manifest as a release asset (checksummed + signed), and a dedicated RSA GPG key imported from GitHub secrets. Operator completes registry UI submissions manually.

**Tech Stack:** GoReleaser v2, GitHub Actions, GPG (RSA 4096), Terraform Registry / OpenTofu Registry

## Global Constraints

- Git tags: **no `v` prefix** (e.g. `0.10.1`)
- GitHub release **name**: `v{{.Version}}` (OpenTofu requirement)
- First signed version: **0.10.1** (do not rewrite `0.10.0`)
- GPG: dedicated key UID `Terraform Provider systemd <david@larochette.me>`, RSA 4096, ~3y expiry
- Private key only in GitHub secrets (`GPG_PRIVATE_KEY`, `PASSPHRASE`) — never commit
- Platforms: linux/darwin amd64+arm64 (no Windows)
- OpenTofu issue forms: UI only (not `gh`/API)

---

### Task 1: Registry manifest + GoReleaser signing

**Files:**
- Create: `terraform-registry-manifest.json`
- Modify: `.goreleaser.yml`
- Spec: `docs/superpowers/specs/2026-07-30-registry-publish-design.md`

**Interfaces:**
- Produces: release assets `*_manifest.json`, `*_SHA256SUMS`, `*_SHA256SUMS.sig`; release name `v0.10.1`

- [ ] **Step 1: Add `terraform-registry-manifest.json`**

```json
{
  "version": 1,
  "metadata": {
    "protocol_versions": ["6.0"]
  }
}
```

- [ ] **Step 2: Update `.goreleaser.yml`**

Add (aligned with HashiCorp scaffolding):

```yaml
checksum:
  name_template: "{{ .ProjectName }}_{{ .Version }}_SHA256SUMS"
  algorithm: sha256
  extra_files:
    - glob: "terraform-registry-manifest.json"
      name_template: "{{ .ProjectName }}_{{ .Version }}_manifest.json"

signs:
  - artifacts: checksum
    args:
      - "--batch"
      - "--local-user"
      - "{{ .Env.GPG_FINGERPRINT }}"
      - "--output"
      - "${signature}"
      - "--detach-sign"
      - "${artifact}"

release:
  github:
    owner: dlarochette
    name: terraform-provider-systemd
  name_template: "v{{.Version}}"
  make_latest: true
  extra_files:
    - glob: "terraform-registry-manifest.json"
      name_template: "{{ .ProjectName }}_{{ .Version }}_manifest.json"
```

Keep existing `builds` / `archives` / `changelog` blocks.

- [ ] **Step 3: Dry-run without signing** (optional sanity)

```bash
goreleaser check
```

Expected: config validates (or note if `signs` needs env at check time).

- [ ] **Step 4: Commit**

```bash
git add terraform-registry-manifest.json .goreleaser.yml docs/superpowers/specs/2026-07-30-registry-publish-design.md
git commit -m "feat: add GPG signing and registry manifest for releases"
```

---

### Task 2: CI GPG import before GoReleaser

**Files:**
- Modify: `.github/workflows/ci.yml` (release job)

**Interfaces:**
- Consumes: `secrets.GPG_PRIVATE_KEY`, `secrets.PASSPHRASE`
- Produces: `GPG_FINGERPRINT` env for goreleaser `signs`

- [ ] **Step 1: Insert import step before goreleaser**

```yaml
      - name: Import GPG key
        id: import_gpg
        uses: crazy-max/ghaction-import-gpg@v6
        with:
          gpg_private_key: ${{ secrets.GPG_PRIVATE_KEY }}
          passphrase: ${{ secrets.PASSPHRASE }}
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          GPG_FINGERPRINT: ${{ steps.import_gpg.outputs.fingerprint }}
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: import dedicated GPG key before goreleaser release"
```

---

### Task 3: README + design doc updates

**Files:**
- Modify: `README.md` (Install section, latest release badge, version constraint)
- Modify: `docs/superpowers/specs/2026-07-27-terraform-systemd-design.md` (phase 2 note)
- Modify: `docs/superpowers/specs/2026-07-30-registry-publish-design.md` if manifest embedding wording differs from HashiCorp pattern

- [ ] **Step 1: README Install**

Replace “not on the public Terraform Registry yet” with registry install instructions; keep `dev_overrides` as secondary. Bump badge and `version = ">= 0.10.1"`.

- [ ] **Step 2: Link registry publish design from MVP design phase 2**

- [ ] **Step 3: Commit**

```bash
git add README.md docs/superpowers/specs/
git commit -m "docs: document registry install path for 0.10.1"
```

---

### Task 4: Dedicated GPG key + GitHub secrets

**Files:** none in repo (secrets only)

- [ ] **Step 1: Generate key** (interactive passphrase via pinentry)

```bash
gpg --full-generate-key
# RSA and RSA, 4096, expire 3y
# Real name: Terraform Provider systemd
# Email: david@larochette.me
```

Or batch equivalent with passphrase from pinentry-qt.

- [ ] **Step 2: Export public key** (print fingerprint + armored pubkey for registries)

```bash
gpg --list-secret-keys --with-fingerprint "Terraform Provider systemd"
gpg --armor --export "Terraform Provider systemd <david@larochette.me>"
```

- [ ] **Step 3: Set GitHub secrets** (private key via temp file + shred)

```bash
gpg --armor --export-secret-keys "Terraform Provider systemd <david@larochette.me>" > /tmp/tf-systemd-gpg.asc
gh secret set GPG_PRIVATE_KEY -R dlarochette/terraform-provider-systemd < /tmp/tf-systemd-gpg.asc
# PASSPHRASE via pinentry → gh secret set PASSPHRASE
shred -u /tmp/tf-systemd-gpg.asc
```

- [ ] **Step 4: Confirm secrets exist** (names only)

```bash
gh secret list -R dlarochette/terraform-provider-systemd
```

---

### Task 5: Tag `0.10.1` and verify signed release

- [ ] **Step 1: Ensure working tree clean; push commits**

```bash
git push origin main
```

- [ ] **Step 2: Tag and push**

```bash
git tag 0.10.1
git push origin 0.10.1
```

- [ ] **Step 3: Wait for CI release job**

```bash
gh run watch -R dlarochette/terraform-provider-systemd
```

- [ ] **Step 4: Verify assets**

Release name `v0.10.1`; assets include `*_SHA256SUMS`, `*_SHA256SUMS.sig`, `*_manifest.json`.

```bash
gh release view 0.10.1 -R dlarochette/terraform-provider-systemd
gpg --verify …_SHA256SUMS.sig …_SHA256SUMS
```

- [ ] **Step 5: Hand off manual registry UI steps** to operator (Terraform Signing Keys + Publish; OpenTofu provider_key + provider forms).

---

## Self-review

| Spec item | Task |
|-----------|------|
| Both registries | Task 5 handoff |
| Dedicated RSA key + UID | Task 4 |
| 0.10.1 first signed | Task 5 |
| Tag no `v`, release name `v…` | Task 1 + 5 |
| GoReleaser signs + manifest | Task 1 |
| CI GPG import | Task 2 |
| Docs | Task 3 |
| OpenTofu form UI only | Task 5 (no automation) |
