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
SUITE="${ACC_DEBIAN_SUITE:-bookworm}"
MIRROR="${ACC_DEBIAN_MIRROR:-http://deb.debian.org/debian}"
READY_RETRIES="${ACC_READY_RETRIES:-90}"

log() { echo "acc-nspawn: $*" >&2; }
die() { log "$*"; exit 1; }

# MACHINE feeds ROOT (an rm -rf target) and machinectl/systemd-run -M below —
# reject anything that isn't a plain identifier before it's used anywhere,
# so a hostile/typo'd SYSTEMD_ACC_MACHINE can't turn into a path traversal or
# option injection.
[[ "${MACHINE}" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] \
  || die "invalid SYSTEMD_ACC_MACHINE '${MACHINE}' (must match ^[a-zA-Z0-9][a-zA-Z0-9_.-]*\$)"

ROOT="/var/lib/machines/${MACHINE}"
NSPAWN_CONF="/etc/systemd/nspawn/${MACHINE}.nspawn"

[[ "$(id -u)" -eq 0 ]] || die "must run as root (Makefile runs: sudo ./scripts/acc-nspawn.sh)"

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
  debootstrap --include=systemd,dbus,systemd-sysv,libpam-systemd,systemd-resolved,systemd-container \
    "${SUITE}" "${ROOT}" "${MIRROR}"

  systemd-machine-id-setup --root="${ROOT}"

  mkdir -p "/etc/systemd/nspawn"
  cat > "${NSPAWN_CONF}" <<EOF
# Managed by scripts/acc-nspawn.sh — regenerated on every rootfs rebuild.
# Capability=all + PrivateUsers=no so nested systemd_machine ACC can start
# an inner nspawn (sysfs mount / UID map) inside this guest.
[Exec]
Boot=yes
PrivateUsers=no
Capability=all

[Files]
BindReadOnly=/etc/resolv.conf
EOF
}

if [[ "${ACC_REBUILD:-}" == "1" ]] || ! rootfs_looks_bootable; then
  stop_machine
  if machine_is_active; then
    # Same invariant as the ACC_WIPE path below: never rm -rf a rootfs
    # backing a machine that is still (or again) active — that can
    # corrupt/orphan a running container.
    die "refusing to rebuild '${MACHINE}': still active after stop timeout"
  fi
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
  # Keep outer .nspawn aligned even when rootfs is reused (no ACC_REBUILD).
  mkdir -p "/etc/systemd/nspawn"
  cat > "${NSPAWN_CONF}" <<EOF
# Managed by scripts/acc-nspawn.sh
[Exec]
Boot=yes
PrivateUsers=no
Capability=all

[Files]
BindReadOnly=/etc/resolv.conf
EOF
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

# Nested systemd_machine ACC needs machinectl inside the guest. Fresh rootfs
# installs systemd-container via debootstrap; existing images may lack it, and
# the guest often has no working DNS — so download .debs on the host and dpkg -i.
ensure_guest_machinectl() {
  if systemd-run -M "${MACHINE}" -q -P --wait -- test -x /usr/bin/machinectl; then
    return 0
  fi
  die "machinectl missing in guest (rebuild with: sudo ACC_REBUILD=1 make testacc — debootstrap must include systemd-container)"
}

ensure_guest_machinectl

# Mini Boot=no rootfs tar for TestAccMachineLifecycle (local import only).
log "staging nested machine fixture tar in ${MACHINE}"
systemd-run -M "${MACHINE}" -P --wait -- bash -c '
  set -euo pipefail
  FIX=/var/tmp/tf-acc-mini
  rm -rf "$FIX" /var/tmp/tf-acc-mini.tar
  mkdir -p "$FIX/usr/bin"
  cp -a "$(command -v sleep)" "$FIX/usr/bin/sleep"
  tar -C "$FIX" -cf /var/tmp/tf-acc-mini.tar .
' || die "failed to stage /var/tmp/tf-acc-mini.tar in ${MACHINE}"

# Portable service tree for TestAccPortableLifecycle (local tar → /var/lib/portables).
# Portable service tree for TestAccPortableLifecycle (local tar → /var/lib/portables).
log "staging portable fixture tar in ${MACHINE}"
STAGE_SCRIPT_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/acc-stage-portable.sh"
[[ -f "${STAGE_SCRIPT_SRC}" ]] || die "missing ${STAGE_SCRIPT_SRC}"
chmod 755 /var/lib/machines
mkdir -p /var/lib/machines/.tf-provider-stage
chmod 1777 /var/lib/machines/.tf-provider-stage
STAGE_SCRIPT_HOST="/var/lib/machines/.tf-provider-stage/acc-stage-portable.sh"
cp -a "${STAGE_SCRIPT_SRC}" "${STAGE_SCRIPT_HOST}"
chmod 0755 "${STAGE_SCRIPT_HOST}"
# Drop the home-dir SELinux label so machinectl (sd-copy) can open the file.
if command -v chcon >/dev/null 2>&1; then
  chcon -t systemd_machined_var_lib_t "${STAGE_SCRIPT_HOST}" >/dev/null 2>&1 \
    || restorecon -F "${STAGE_SCRIPT_HOST}" >/dev/null 2>&1 \
    || true
elif command -v restorecon >/dev/null 2>&1; then
  restorecon -F "${STAGE_SCRIPT_HOST}" >/dev/null 2>&1 || true
fi
machinectl copy-to --force "${MACHINE}" "${STAGE_SCRIPT_HOST}" /var/tmp/acc-stage-portable.sh \
  || die "machinectl copy-to acc-stage-portable.sh failed"
systemd-run -M "${MACHINE}" -P --wait -- bash /var/tmp/acc-stage-portable.sh \
  || die "failed to stage /var/tmp/tf-acc-portable.tar in ${MACHINE}"

export TF_ACC=1
export SYSTEMD_ACC_MACHINE="${MACHINE}"

# Staging dir for machinectl copy-to/from (SELinux-friendly). Writable by the
# user who will run go test so we do not leave root-owned junk in GOCACHE.
# /var/lib/machines is often mode 0700; open traverse so SUDO_USER can reach
# the sticky stage dir (otherwise Go falls back to /tmp and SELinux denies
# systemd_machined_t open on user_tmp_t → "Failed to copy: Access denied").
STAGE_DIR="/var/lib/machines/.tf-provider-stage"
chmod 755 /var/lib/machines
mkdir -p "${STAGE_DIR}"
chmod 1777 "${STAGE_DIR}"
if command -v restorecon >/dev/null 2>&1; then
  restorecon -R "${STAGE_DIR}" >/dev/null 2>&1 || true
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$(mktemp)"

# Run go test as the invoking user when elevated via sudo, so module/build
# caches under $HOME stay owned by that user (not root/root).
run_go_test() {
  cd "${REPO_ROOT}"
  go test ./internal/provider/ -run 'TestAcc' -count=1 -timeout 45m -v
}

set +e
if [[ -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
  TEST_HOME="$(getent passwd "${SUDO_USER}" | cut -d: -f6)"
  # Capture the user's Go env before dropping (paths under their home).
  USER_GOCACHE="$(sudo -u "${SUDO_USER}" go env GOCACHE)"
  USER_GOMODCACHE="$(sudo -u "${SUDO_USER}" go env GOMODCACHE)"
  USER_GOPATH="$(sudo -u "${SUDO_USER}" go env GOPATH)"
  (
    sudo -u "${SUDO_USER}" --preserve-env=TF_ACC,SYSTEMD_ACC_MACHINE,PATH \
      env HOME="${TEST_HOME}" \
          GOCACHE="${USER_GOCACHE}" \
          GOMODCACHE="${USER_GOMODCACHE}" \
          GOPATH="${USER_GOPATH}" \
          bash -c "cd \"${REPO_ROOT}\" && go test ./internal/provider/ -run 'TestAcc' -count=1 -timeout 45m -v"
  ) 2>&1 | tee "${OUT}"
  status="${PIPESTATUS[0]}"
else
  ( run_go_test ) 2>&1 | tee "${OUT}"
  status="${PIPESTATUS[0]}"
fi
set -e

if [[ "${status}" -eq 0 ]] && ! grep -q '^=== RUN[[:space:]]*TestAcc' "${OUT}"; then
  die "no TestAcc* tests matched -run 'TestAcc' (harness ran nothing — that's a hard failure, not a pass)"
fi

exit "${status}"
