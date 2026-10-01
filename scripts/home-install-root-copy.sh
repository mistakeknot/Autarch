#!/bin/sh
# Run by mk with sudo, on the host, from an Autarch checkout:  sudo scripts/home-install-root-copy.sh --thread <thr_...> [--expect-head <sha>] [--dest DIR]
# Copies the five Home upgrade/restore script files into a root-owned directory (default /usr/local/libexec/home-v3),
# after checking each against the git-tracked blob at HEAD, so root never runs a file mk (or an agent) could edit.
# This script is itself run by root, so mk should read it (or pipe the reviewed commit's copy:
#   git -C <checkout> show <sha>:scripts/home-install-root-copy.sh | sudo sh -s -- --src <checkout>/scripts --thread <thr> --expect-head <sha>).
# Finally reports to the creating thread via bb. Test mode (--test-as-current-user, never as root) skips the root
# requirement and the bb report, and installs as the current user.
FILES="home-upgrade-v3.sh home-upgrade-v3.bash home-restore-v2.sh home-restore-v2.bash home-common.bash"
THREAD=
EXPECT_HEAD=
SRC=
BB=$(getent passwd mk | cut -d: -f6)/.local/bin/bb
DEST=/usr/local/libexec/home-v3
TEST=0
while [ $# -gt 0 ]; do
  case "$1" in
    --dest) DEST=${2:?}; shift 2 ;;
    --thread) THREAD=${2:?}; shift 2 ;;
    --src) SRC=${2:?}; shift 2 ;;
    --expect-head) EXPECT_HEAD=${2:?}; shift 2 ;;
    --test-as-current-user) TEST=1; shift ;;
    *) echo "unknown argument: $1" >&2; exit 64 ;;
  esac
done
if [ "$TEST" = 1 ]; then
  [ "$(id -u)" != 0 ] || { echo "--test-as-current-user is refused as root" >&2; exit 64; }
else
  [ "$(id -u)" = 0 ] || { echo "run as root: sudo $0 --thread <thr_...>" >&2; exit 64; }
  [ -n "$THREAD" ] || { echo "--thread <thr_...> is required (the report goes there)" >&2; exit 64; }
  case "$DEST" in /root/home-v3-scripts|/usr/local/libexec/home-v3) ;; *) echo "--dest must be /root/home-v3-scripts or /usr/local/libexec/home-v3 as root" >&2; exit 64 ;; esac
fi
if [ -z "$SRC" ]; then SRC=$(readlink -f -- "$0") && SRC=${SRC%/*}; fi
SRC=$(readlink -f -- "$SRC") || exit 1
REPORT=$(mktemp /tmp/home-install-report.XXXXXX) || exit 1
say() { printf '%s\n' "$*" | tee -a "$REPORT"; }
finish() {
  rc=$?
  say "home-install-root-copy: exit $rc"
  if [ "$TEST" = 0 ] && [ -n "$THREAD" ] && [ -x "$BB" ]; then
    runuser -u mk -- env -i HOME="$(getent passwd mk | cut -d: -f6)" USER=mk LOGNAME=mk PATH=/usr/bin:/bin "$BB" thread tell "$THREAD" --message-file "$REPORT" || echo "report not delivered; read $REPORT" >&2
  else
    echo "report not sent (no --thread or test mode); read $REPORT" >&2
  fi
}
trap finish EXIT
GITDIR=${SRC%/scripts}
mkdir -p "${DEST%/*}"
G="git -c safe.directory=$GITDIR -C $GITDIR --no-replace-objects"
HEADSHA=$($G rev-parse HEAD) || { say "not a git checkout"; exit 1; }
say "git HEAD $HEADSHA (compare with the reviewed commit)"
[ -z "$EXPECT_HEAD" ] || [ "$EXPECT_HEAD" = "$HEADSHA" ] || { say "HEAD is not the expected $EXPECT_HEAD; nothing installed"; exit 7; }
STAGE=$(mktemp -d "${DEST%/*}/.home-v3-stage.XXXXXX") || { say "cannot create staging dir beside $DEST"; exit 1; }
trap 'rm -rf "$STAGE"; finish' EXIT
for f in $FILES; do
  # Read each file once into staging, hash that copy against the committed blob, and install only the verified copy.
  cp -- "$SRC/$f" "$STAGE/$f" || { say "cannot read $f"; exit 1; }
  want=$($G show "HEAD:scripts/$f" | sha256sum | cut -d' ' -f1)
  have=$(sha256sum < "$STAGE/$f" | cut -d' ' -f1)
  [ -n "$want" ] && [ "$want" = "$have" ] || { say "MISMATCH $f: working copy differs from git HEAD blob; nothing installed"; exit 7; }
  say "ok $f sha256 $have"
done
if [ "$TEST" = 0 ]; then chown -R 0:0 "$STAGE"; fi
chmod 0755 "$STAGE/home-upgrade-v3.sh" "$STAGE/home-restore-v2.sh"
chmod 0644 "$STAGE/home-upgrade-v3.bash" "$STAGE/home-restore-v2.bash" "$STAGE/home-common.bash"
chmod 0755 "$STAGE"
rm -rf "$DEST.old"; [ ! -e "$DEST" ] || mv "$DEST" "$DEST.old"
mv "$STAGE" "$DEST" || { say "install failed"; exit 1; }
rm -rf "$DEST.old"
say "installed to $DEST (root:root, launchers 0755, bodies 0644)"
say "Next, run as root (use the copy, never the checkout):"
say "  sudo $DEST/home-upgrade-v3.sh --thread <thr_...> --plugin <v3 build dir>"
say "  sudo $DEST/home-restore-v2.sh --thread <thr_...> --repo <Autarch checkout> [--backup <path>]"
