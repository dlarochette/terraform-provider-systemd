#!/usr/bin/env bash
# acc-nspawn-ssh.sh — same ACC guest as acc-nspawn.sh, but run TestAcc* over
# remote.Dial (SSH/SFTP) against sshd in the guest on 127.0.0.1:2222.
#
# See docs/superpowers/specs/2026-07-30-acc-ssh-design.md
set -euo pipefail
export ACC_TRANSPORT=ssh
exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/acc-nspawn.sh"
