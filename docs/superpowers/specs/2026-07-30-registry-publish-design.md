# Design: Publish to Terraform + OpenTofu Registries

Date: 2026-07-30  
Status: approved  
Module: `github.com/dlarochette/terraform-provider-systemd`  
Provider address: `dlarochette/systemd`  
First signed release: `0.10.1`

## Goal

Make `dlarochette/systemd` installable via:

- Terraform Registry: `registry.terraform.io/dlarochette/systemd`
- OpenTofu Registry: `registry.opentofu.org/dlarochette/systemd`

by producing **GPG-signed** GitHub releases and completing the registry submission flows.

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Registries | **Both** (Terraform + OpenTofu) |
| Signing key | **Dedicated** RSA key — UID `Terraform Provider systemd <david@larochette.me>` (not personal/Move keys; private key only for this CI use) |
| Key algorithm | RSA 4096 (Terraform Registry requires RSA or DSA — not ed25519) |
| Key expiry | ~3 years (extend before expiry) |
| First signed version | **0.10.1** (do not rewrite unsigned `0.10.0`) |
| Git tags | **No `v` prefix** (workspace rule) |
| GitHub release **name** | `v{{.Version}}` (OpenTofu requires release name starting with `v`) |
| Release tooling | Extend existing GoReleaser + CI (not HashiCorp composite action) |
| Artifact platforms | Keep linux/darwin amd64+arm64 (no Windows in this release) |

## Current gaps

Release `0.10.0` has zips + `SHA256SUMS` only — **no** `SHA256SUMS.sig`, no `terraform-registry-manifest.json` in archives, release title is not `v…`.

## Architecture

```text
git tag 0.10.1  (no v)
  └─ CI release job
        ├─ import dedicated GPG from secrets.GPG_PRIVATE_KEY (+ PASSPHRASE)
        ├─ goreleaser release
        │     ├─ zip binaries (filenames without v: …_0.10.1_os_arch.zip)
        │     ├─ …_0.10.1_manifest.json  ← registry protocol metadata (release asset)
        │     ├─ SHA256SUMS (includes zips + manifest)
        │     ├─ SHA256SUMS.sig   ← detached GPG signature
        │     └─ GitHub Release name = v0.10.1
        └─ (manual) submit public key + provider to both registries
```

## Dedicated GPG key lifecycle

1. Generate locally (interactive passphrase via pinentry):

```bash
gpg --full-generate-key
# RSA and RSA, 4096 bits, expire 3y
# Real name: Terraform Provider systemd
# Email: david@larochette.me
```

2. Export **public** (safe to paste into registries / docs):

```bash
gpg --armor --export "Terraform Provider systemd <david@larochette.me>"
```

3. Export **private** only into a temp file for `gh secret set`, then shred — never commit:

```bash
gpg --armor --export-secret-keys "Terraform Provider systemd <david@larochette.me>" > /tmp/tf-systemd-gpg.asc
# gh secret set GPG_PRIVATE_KEY < /tmp/tf-systemd-gpg.asc
# shred -u /tmp/tf-systemd-gpg.asc
```

4. Backup private key offline (password manager / encrypted backup). If the GitHub secret is compromised, revoke and replace the key in both registries.

Personal keys (`david@larochette.me` day-to-day, Move package keys) stay **out** of GitHub Actions.

## GoReleaser changes

1. **`signs`** — sign `SHA256SUMS` with `--batch --yes --detach-sign` (binary `.sig`, not ASCII armor).
2. **`release.name_template: "v{{.Version}}"`** — OpenTofu crawler requirement; tag stays `0.10.1`.
3. **`terraform-registry-manifest.json`** at repo root:

```json
{
  "version": 1,
  "metadata": {
    "protocol_versions": ["6.0"]
  }
}
```

Publish as a separate release asset renamed to
`{{ .ProjectName }}_{{ .Version }}_manifest.json` via GoReleaser
`checksum.extra_files` + `release.extra_files` (HashiCorp scaffolding pattern —
must appear in `SHA256SUMS`).

4. Keep binary name `terraform-provider-systemd_v{{ .Version }}` inside zips.

5. CI `release` job: import GPG before goreleaser:

```yaml
- name: Import GPG key
  uses: crazy-max/ghaction-import-gpg@v6
  with:
    gpg_private_key: ${{ secrets.GPG_PRIVATE_KEY }}
    passphrase: ${{ secrets.PASSPHRASE }}
```

Pass fingerprint to goreleaser env as required by `signs`.

## GitHub secrets (repo `dlarochette/terraform-provider-systemd`)

| Secret | Content |
|--------|---------|
| `GPG_PRIVATE_KEY` | ASCII-armored **dedicated** secret key |
| `PASSPHRASE` | Passphrase for that key |

## Manual registry steps (operator)

OpenTofu submissions **must** use the GitHub issue **form UI** (not `gh` / API).

### Terraform Registry (`registry.terraform.io`)

1. Sign in with GitHub (`dlarochette`).
2. **User Settings → Signing Keys** → paste ASCII-armored **public** key for the dedicated UID.
3. **Publish → Provider** → select `dlarochette/terraform-provider-systemd`.
4. After signed `0.10.1` exists, publish / sync versions.

### OpenTofu Registry (`registry.opentofu.org`)

1. [Submit Provider Signing Key](https://github.com/opentofu/registry/issues/new?template=provider_key.yml) via issue form UI.
2. [Submit new Provider](https://github.com/opentofu/registry/issues/new?template=provider.yml) for `dlarochette/systemd`.
3. Wait for approval / automation.

## Docs / SemVer

- README: remove “not on the public Terraform Registry yet”; document Registry install with `version = ">= 0.10.1"`.
- Latest release badge → `0.10.1`.
- MVP design: Public Registry phase 2 → link this spec as shipped path.
- Tag **0.10.1** after CI green with signed assets.

## Verification checklist

- [ ] Release titled `v0.10.1` (git tag `0.10.1`) contains `*_SHA256SUMS` and `*_SHA256SUMS.sig`
- [ ] `gpg --verify` succeeds with the dedicated UID
- [ ] Release includes `*_manifest.json` and it is listed in `SHA256SUMS`
- [ ] `terraform init` / `tofu init` resolve `dlarochette/systemd` after listings go live

## Out of scope

- Putting personal/Move private keys in GitHub
- Republishing unsigned `0.10.0` with a signature
- Windows / FreeBSD builds
- Automating OpenTofu issue-form submission via API/CLI

## Success criteria

Provider listed on both registries; clean `terraform init` / `tofu init` with `source = "dlarochette/systemd"` downloads and verifies `0.10.1`.
