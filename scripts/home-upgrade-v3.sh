#!/bin/sh
# Launcher. As root it refuses (exit 6) unless this launcher, its .bash body and home-common.bash are root:root,
# not group/other-writable, and on a path whose every directory is root-owned and not group/other-writable
# (the generated home-v3-run-<sha12>.sh package makes such a copy). HOME_LAUNCHER_ASSUME_ROOT=1 only makes the check apply
# to a non-root caller (used by tests); nothing here ever skips it for root.
_d=$(readlink -f -- "$0") && _d=${_d%/*} || exit 6
if [ "$(id -u)" = 0 ] || [ "${HOME_LAUNCHER_ASSUME_ROOT:-}" = 1 ]; then
  _trusted() {
    _r=$(readlink -f -- "$1") || return 1
    [ -f "$_r" ] || return 1
    [ "$(stat -c '%u %g' -- "$_r")" = "0 0" ] || return 1
    [ $(( 0$(stat -c %a -- "$_r") & 18 )) -eq 0 ] || return 1
    while [ "$_r" != / ]; do
      _r=${_r%/*}; [ -n "$_r" ] || _r=/
      [ "$(stat -c '%u %g' -- "$_r")" = "0 0" ] || return 1
      [ $(( 0$(stat -c %a -- "$_r") & 18 )) -eq 0 ] || return 1
    done
  }
  for _f in "$0" "$_d/home-upgrade-v3.bash" "$_d/home-common.bash"; do
    _trusted "$_f" || { echo "refusing to run as root: $_f is not a root-owned, non-group/other-writable file on a root-owned path; use the root-owned copy from the generated home-v3-run package" >&2; exit 6; }
  done
fi
exec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /bin/bash --noprofile --norc "$_d/home-upgrade-v3.bash" "$@"
