#!/usr/bin/env bash
# acc-nspawn.sh — build/start a systemd-nspawn machine and run `go test -run
# TestAcc` against it via remote.Nspawn (machinectl / systemd-run, no SSH).
#
# The rootfs is debootstrapped directly under /var/lib/machines/<name>, which
# is exactly where machinectl/systemd-nspawn auto-discover directory-tree
# images — no separate "registration" step is required.
#
# Required host packages (Debian/Ubuntu names; Fedora: systemd-container +
# debootstrap from the main repos):
#   systemd-container (machinectl, systemd-nspawn, systemd-run)
#   debootstrap
#   dbus (systemd-run/machinectl talk to systemd over D-Bus)
#
# Env:
#   SYSTEMD_ACC_MACHINE   machine name (default: tf-systemd-acc)
#   ACC_DEBIAN_SUITE      debootstrap suite (default: bookworm)
#   ACC_DEBIAN_MIRROR     debootstrap mirror (default: http://deb.debian.org/debian)
#   ACC_REBUILD=1         force a rootfs rebuild even if one already looks bootable
#   ACC_WIPE=1            remove the machine image after the run (success or failure)
#   ACC_READY_RETRIES     readiness poll attempts, 2s apart (default: 90 => ~3min)
#
# Exit status is `go test`'s exit status (0 on pass, non-zero otherwise), or
# 1 if the machine never becomes ready or a prerequisite is missing.

set -euo pipefail

MACHINE="${SYSTEMD_ACC_MACHINE:-tf-systemd-acc}"
ROOT="/var/lib/machines/${MACHINE}"
SUITE="${ACC_DEBIAN_SUITE:-bookworm}"
MIRROR="${ACC_DEBIAN_MIRROR:-http://deb.debian.org/debian}"
READY_RETRIES="${ACC_READY_RETRIES:-90}"
NSPAWN_CONF="/etc/systemd/nspawn/${MACHINE}.nspawn"

log() { echo "acc-nspawn: $*" >&2; }
die() { log "$*"; exit 1; }

[[ "$(id -u)" -eq 0 ]] || die "must run as root (try: sudo make testacc)"

for bin in machinectl systemd-nspawn systemd-run debootstrap; do
  command -v "${bin}" >/dev/null 2>&1 \
    || die "missing required tool '${bin}' (install systemd-container + debootstrap)"
done

machine_state() {
  machinectl show "${MACHINE}" -P State 2>/dev/null || true
}

machine_is_active() {
  [[ "$(machine_state)" == "running" ]]
}

stop_machine() {
  machine_is_active || return 0
  log "stopping ${MACHINE}"
  machinectl stop "${MACHINE}" >/dev/null 2>&1 || true
  for _ in $(seq 1 30); do
    machine_is_active || return 0
    sleep 1
  done
  log "warning: ${MACHINE} did not report stopped after 30s"
}

rootfs_looks_bootable() {
  [[ -x "${ROOT}/usr/lib/systemd/systemd" || -x "${ROOT}/lib/systemd/systemd" ]]
}

build_rootfs() {
  log "debootstrapping ${SUITE} into ${ROOT} (mirror: ${MIRROR})"
  debootstrap --include=systemd,dbus,systemd-sysv,libpam-systemd,systemd-resolved \
    "${SUITE}" "${ROOT}" "${MIRROR}"

  systemd-machine-id-setup --root="${ROOT}"

  mkdir -p "/etc/systemd/nspawn"
  cat > "${NSPAWN_CONF}" <<EOF
# Managed by scripts/acc-nspawn.sh — regenerated on every rootfs rebuild.
[Exec]
Boot=yes

[Files]
BindReadOnly=/etc/resolv.conf
EOF
}

if [[ "${ACC_REBUILD:-}" == "1" ]] || ! rootfs_looks_bootable; then
  stop_machine
  rm -rf "${ROOT}"
  build_rootfs
fi

CLEANUP_DONE=0
cleanup() {
  [[ "${CLEANUP_DONE}" -eq 1 ]] && return
  CLEANUP_DONE=1
  [[ -n "${OUT:-}" ]] && rm -f "${OUT}"
  stop_machine
  if [[ "${ACC_WIPE:-}" == "1" ]]; then
    if machine_is_active; then
      # stop_machine already warned above; never rm -rf a rootfs backing a
      # machine that is still (or again) active — that can corrupt/orphan a
      # running container. Overrides whatever status the script was about to
      # exit with: a refused wipe is itself a hard failure to surface.
      log "refusing to wipe '${MACHINE}': still active after stop timeout"
      exit 1
    fi
    log "wiping ${MACHINE} image"
    machinectl remove "${MACHINE}" >/dev/null 2>&1 || rm -rf "${ROOT}"
    rm -f "${NSPAWN_CONF}"
  fi
}
trap cleanup EXIT

if ! machine_is_active; then
  log "starting ${MACHINE}"
  machinectl start "${MACHINE}"
fi

log "waiting for ${MACHINE} to become ready (up to $((READY_RETRIES * 2))s)"
ready=0
state="unknown"
for ((i = 1; i <= READY_RETRIES; i++)); do
  if systemd-run -M "${MACHINE}" -q -P --wait -- true >/dev/null 2>&1; then
    state="$(systemd-run -M "${MACHINE}" -q -P --wait -- systemctl is-system-running 2>/dev/null || true)"
    case "${state}" in
      running | degraded)
        ready=1
        break
        ;;
    esac
  fi
  sleep 2
done

[[ "${ready}" -eq 1 ]] \
  || die "machine '${MACHINE}' did not become ready within $((READY_RETRIES * 2))s (last systemctl is-system-running: '${state}')"

log "${MACHINE} ready (systemctl is-system-running: ${state})"

export TF_ACC=1
export SYSTEMD_ACC_MACHINE="${MACHINE}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$(mktemp)"

set +e
(
  cd "${REPO_ROOT}"
  go test ./internal/provider/ -run 'TestAcc' -count=1 -timeout 45m -v
) 2>&1 | tee "${OUT}"
status="${PIPESTATUS[0]}"
set -e

if [[ "${status}" -eq 0 ]] && ! grep -q '^=== RUN[[:space:]]*TestAcc' "${OUT}"; then
  log "warning: no TestAcc* tests matched -run 'TestAcc' (expected until the ACC test matrix lands)"
fi

exit "${status}"
