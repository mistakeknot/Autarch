#!/bin/bash
# Builds the cloud-init NoCloud seed (volume label cidata) that carries the test binary and guest.sh.
# usage: build-seed.sh LIVE_TEST_BINARY OUT_ISO [TEST_RUN_REGEXP]  (default: Live)
set -eu
export PATH=/usr/bin:/bin
here=$(cd "$(dirname "$0")" && pwd)
bin=${1:?test binary}; iso=${2:?output iso}
d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
cp "$bin" "$d/live.test"; cp "$here/guest.sh" "$d/guest.sh"; echo "${3:-Live}" > "$d/run.txt"
printf 'instance-id: hrlive-%s\nlocal-hostname: hrlive\n' "$(date +%s)" > "$d/meta-data"
cat > "$d/user-data" <<'UD'
#cloud-config
ssh_pwauth: false
runcmd:
  - [ bash, -c, "mkdir -p /mnt/seed && mount -o ro LABEL=cidata /mnt/seed && exec bash /mnt/seed/guest.sh /mnt/seed" ]
UD
genisoimage -quiet -output "$iso" -volid cidata -joliet -rock "$d"
