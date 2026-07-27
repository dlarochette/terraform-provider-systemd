#!/usr/bin/env bash
# Dump provider schemas as JSON for terraform validate / IDE / custom linters.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if ! command -v terraform >/dev/null 2>&1 && ! command -v tofu >/dev/null 2>&1; then
  echo "terraform or tofu required to dump provider schemas" >&2
  exit 1
fi
TF=terraform
command -v tofu >/dev/null 2>&1 && TF=tofu

make build
mkdir -p schemas .schema-work
BIN="$ROOT/bin"
cat >.schema-work/terraform.tf <<'EOF'
terraform {
  required_providers {
    systemd = {
      source = "dlarochette/systemd"
    }
  }
}

provider "systemd" {
  host = "127.0.0.1"
}
EOF

cat >.schema-work/.terraformrc <<EOF
provider_installation {
  dev_overrides {
    "dlarochette/systemd" = "$BIN"
  }
  direct {}
}
EOF

export TF_CLI_CONFIG_FILE="$ROOT/.schema-work/.terraformrc"
cd .schema-work
# init may warn with dev_overrides; schema dump still works after providers schema
set +e
$TF init -backend=false -input=false >/dev/null 2>&1
$TF providers schema -json >"$ROOT/schemas/provider.json"
status=$?
set -e
if [[ $status -ne 0 ]] || [[ ! -s "$ROOT/schemas/provider.json" ]]; then
  echo "failed to dump schemas" >&2
  exit 1
fi
echo "wrote schemas/provider.json"
