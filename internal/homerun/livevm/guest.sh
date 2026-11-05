#!/bin/bash
# Runs inside the throwaway guest, as root, from cloud-init. It builds the accounts the live tests expect,
# runs the cross-built test binary against the guest's own system systemd, and reports on the serial console
# (and on a 9p share when the guest kernel has one). Nothing here touches the host.
set -u
export PATH=/usr/sbin:/usr/bin:/sbin:/bin
SEED=${1:?seed mount dir}
OUT=/var/hrout
mkdir -p "$OUT"
say() { echo "$@" > /dev/ttyS0; }
say "HRLIVE-BEGIN $(date -u +%FT%TZ)"
{ systemctl --version | head -2; uname -r; stat -fc 'cgroup-fs=%T' /sys/fs/cgroup; . /etc/os-release; echo "$PRETTY_NAME"; } > "$OUT/env.txt" 2>&1
getent group hrlog >/dev/null || groupadd hrlog
id runner >/dev/null 2>&1 || useradd -m -s /bin/bash runner
id rootalias >/dev/null 2>&1 || useradd -o -u 0 -g 0 -M -s /bin/bash rootalias
echo 'runner ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/runner
chmod 440 /etc/sudoers.d/runner
install -m 0755 "$SEED/live.test" /root/live.test
cd /root
RUN=Live; [ -f "$SEED/run.txt" ] && RUN=$(cat "$SEED/run.txt")
HR_OUT="$OUT" /root/live.test -test.v -test.run "$RUN" -test.timeout 50m > "$OUT/test.log" 2>&1
echo "exit=$?" > "$OUT/exit.txt"
journalctl -b --no-pager -u 'hr-*' > "$OUT/journal-units.txt" 2>&1
# Report: the 9p share if it mounts, always the serial console.
mkdir -p /mnt/out
if mount -t 9p -o trans=virtio,version=9p2000.L out /mnt/out 2>/dev/null; then cp -r "$OUT"/. /mnt/out/ && say "HRLIVE-9P-OK"; fi
for f in env.txt exit.txt obs.txt test.log systemd-analyze-security.txt; do
  [ -f "$OUT/$f" ] || continue
  say "HRLIVE-FILE-BEGIN $f"
  base64 -w 76 "$OUT/$f" > /dev/ttyS0
  say "HRLIVE-FILE-END $f"
done
say "HRLIVE-END $(date -u +%FT%TZ)"
sync
poweroff
