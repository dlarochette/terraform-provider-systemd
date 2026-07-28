#!/bin/bash
# Runs inside the ACC guest to build /var/tmp/tf-acc-portable.tar
set -euo pipefail
FIX=/var/tmp/tf-acc-portable-src
UNIT_DIR="$FIX/usr/lib/systemd/system"
rm -rf "$FIX" /var/tmp/tf-acc-portable.tar
mkdir -p "$FIX/etc" "$UNIT_DIR" "$FIX/usr/bin"
printf 'ID=debian\nVERSION_ID=12\nNAME=Debian\n' >"$FIX/etc/os-release"
if [[ -x /bin/true ]]; then
  TRUE=/bin/true
else
  TRUE=/usr/bin/true
fi
cp -a "$TRUE" "$FIX/usr/bin/true"
cat >"$UNIT_DIR/tf-acc-portable.service" <<'UNIT'
[Unit]
Description=TF ACC portable oneshot

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/bin/true

[Install]
WantedBy=multi-user.target
UNIT
tar -C "$FIX" -cf /var/tmp/tf-acc-portable.tar .
ls -la "$UNIT_DIR/tf-acc-portable.service" /var/tmp/tf-acc-portable.tar
