# home-upgrade-v3.bash: body, started only by the home-upgrade-v3.sh launcher.
# Upgrades Home (the autarch bb plugin) to the v3 build: disable, verify no holder, install, enable.
# Run on zklw: sudo scripts/home-upgrade-v3.sh --thread <thr_...> --plugin <v3 build dir>
# UNTESTED in Task 2.3; its tests are Task 2.8a.
set -euo pipefail
BBDATA=/home/mk/.bb-machines/autarch.getbb.app
DATA=$BBDATA/plugins/autarch
BB=/home/mk/.local/bin/bb
MKUID=$(id -u mk)
THREAD=; PLUGIN=; URL=; AS=()
while [ $# -gt 0 ]; do
  case "$1" in
    --thread) THREAD=${2:?}; shift 2 ;;
    --plugin) PLUGIN=${2:?}; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 64 ;;
  esac
done
[ -n "$THREAD" ] && [ -n "$PLUGIN" ] || { echo "usage: $0 --thread <thr_...> --plugin <dir>" >&2; exit 64; }
[ "$(id -u)" -eq 0 ] || { echo "run as root: sudo $0 ..." >&2; exit 64; }
[ "$(hostname -s)" = zklw ] || { echo "zklw only" >&2; exit 64; }
AS0=(runuser -u mk -- env -i HOME=/home/mk USER=mk LOGNAME=mk PATH=/usr/bin:/bin
     XDG_RUNTIME_DIR=/run/user/$MKUID)
REPORT=$("${AS0[@]}" mktemp /tmp/home-upgrade-report.XXXXXX)
say()    { printf '%s\n' "$*" | "${AS0[@]}" tee -a "$REPORT" >/dev/null; printf '%s\n' "$*"; }
finish() { rc=$?; say "home-upgrade-v3: exit $rc"
           if [ -n "$URL" ]; then "${AS[@]}" "$BB" thread tell "$THREAD" --message-file "$REPORT" \
             || echo "report not delivered; read $REPORT" >&2
           else echo "install not verified; report not sent; read $REPORT" >&2; fi; }
trap finish EXIT

# 0. Verify the install (plan 1.3.9): runtime file, pid, entry path, listener, open file under BBDATA.
RT=$BBDATA/bb-app-runtime.json
{ [ -f "$RT" ] && [ ! -L "$RT" ] && [ "$(stat -c %u "$RT")" = "$MKUID" ]; } || { say "runtime file not trusted"; exit 6; }
[ -x "$BB" ] && [ "$(stat -c %u "$BB")" = "$MKUID" ] || { say "bb binary missing or not owned by mk"; exit 6; }
# mk-writable input: read it as mk, never as root, and validate before it reaches /proc paths.
URL_C=$("${AS0[@]}" jq -r .serverUrl "$RT"); PID=$("${AS0[@]}" jq -r .pid "$RT"); ENTRY=$("${AS0[@]}" jq -r .entryPath "$RT")
case "$PID" in ""|*[!0-9]*) say "runtime pid not numeric"; exit 6 ;; esac
case "$URL_C" in http://127.0.0.1:[0-9]*) ;; *) say "serverUrl not loopback: $URL_C"; exit 6 ;; esac
PORT=${URL_C##*:}
case "$PORT" in ""|*[!0-9]*) say "runtime port not numeric"; exit 6 ;; esac
[ "$(stat -c %u "/proc/$PID" 2>/dev/null)" = "$MKUID" ] || { say "runtime pid $PID not alive as mk"; exit 6; }
[ "$(readlink -f "$ENTRY")" = "$BBDATA/npm/bin/bb-app" ] || { say "entryPath does not resolve to the pinned bb-app"; exit 6; }
tr '\0' ' ' < "/proc/$PID/cmdline" | grep -qF "$ENTRY" || { say "pid $PID does not run entryPath"; exit 6; }
SET=$(pgrep -P "$PID" | tr '\n' ' ')"$PID"
INODE=$(awk -v p="$(printf '%04X' "$PORT")" '$2 ~ ":"p"$" && $4=="0A" {print $10}' /proc/net/tcp | head -1)
[ -n "$INODE" ] || { say "no listener on port $PORT"; exit 6; }
HELD=0; OPEN=0
for p in $SET; do
  for fd in /proc/$p/fd/*; do
    t=$(readlink "$fd" 2>/dev/null || true)
    [ "$t" = "socket:[$INODE]" ] && HELD=1
    case "$t" in "$BBDATA"/*) OPEN=1 ;; esac
  done
done
[ "$HELD" = 1 ] && [ "$OPEN" = 1 ] || { say "listener or data directory not held by the runtime"; exit 6; }
URL=$URL_C
AS=("${AS0[@]}" BB_DATA_DIR="$BBDATA" BB_SERVER_URL="$URL" NODE_ENV=production)
[ -f "$PLUGIN/package.json" ] || { say "no plugin build at $PLUGIN"; exit 4; }

# 1. Stop Home, then require that nothing holds the database.
"${AS[@]}" "$BB" plugin disable autarch
if fuser "$DATA"/data.db* >/dev/null 2>&1; then
  say "Home still holds the DB; re-enabling the existing build"
  "${AS[@]}" "$BB" plugin enable autarch || true
  exit 3
fi
# 2. Install the v3 build and enable it; the open migrates (quiesce, backup, verify, DDL).
"${AS[@]}" "$BB" plugin install "$PLUGIN" || { say "install failed; plugin left disabled on the unchanged v2 DB"; exit 4; }
"${AS[@]}" "$BB" plugin enable autarch || { say "enable failed; plugin left disabled"; exit 5; }
# 3. Report the backup path and the bb.log line.
sleep 5
say "plugin status: $("${AS[@]}" "$BB" plugin status autarch 2>&1 | head -5)"
say "migration_log: $("${AS0[@]}" sqlite3 -readonly "$DATA/data.db" "select version, at, backup_path, digest from migration_log order by rowid desc limit 1" 2>&1 || true)"
LOGLINE=$("${AS[@]}" "$BB" plugin logs autarch 2>&1 | grep -E 'autarch: schema 2|QuiesceRequired|BackupNotVerified' | tail -3 || true)
say "bb.log: ${LOGLINE:-no migration line found}"
case "$LOGLINE" in
  *QuiesceRequired*|*BackupNotVerified*) say "migration refused; plugin is on the unchanged v2 DB. Disable and re-enable the old build."; exit 5 ;;
esac
