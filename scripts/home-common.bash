# home-common.bash: shared preamble, sourced by home-upgrade-v3.bash and home-restore-v2.bash.
# Never executed on its own, and never reached except through a /bin/sh launcher that has already cleared the
# environment (BASH_ENV and ENV are never read; see plan 1.3.9).
#
# Test mode (--test-as-current-user, --bbdata, --build, restore --repo checks): honoured only when NOT root.
# It skips the root, hostname and runuser steps, points BBDATA at a temp install, calls the pinned
# $BBDATA/npm/bin/bb stub directly, skips the real git worktree and npm ci, and shortens waits. As root
# (the only way mk runs the scripts) every one of those flags is refused.

BBDATA=/home/mk/.bb-machines/autarch.getbb.app   # constant; --bbdata only in test mode
TEST=0; BBDATA_SET=0; BUILD_SET=
REST=()
URL=; AS=(); AS0=(); REPORT=; THREAD=; NAME=home-script

parse_common() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --test-as-current-user) TEST=1; shift ;;
      --bbdata) BBDATA=${2:?--bbdata needs a path}; BBDATA_SET=1; shift 2 ;;
      --build) BUILD_SET=${2:?--build needs a path}; shift 2 ;;
      *) REST+=("$1"); shift ;;
    esac
  done
  if [ "$TEST" = 0 ] && { [ "$BBDATA_SET" = 1 ] || [ -n "$BUILD_SET" ]; }; then
    echo "--bbdata and --build are test-mode flags (--test-as-current-user, not as root)" >&2; exit 64
  fi
  if [ "$TEST" = 1 ] && [ "$(id -u)" -eq 0 ]; then
    echo "--test-as-current-user is refused as root" >&2; exit 64
  fi
}

# plugin_state: read Home's health from `bb plugin list --json` (this bb has no `plugin status`). Sets PSTATE (one report
# line) and PHEALTHY=1 only for enabled=true and status=running, the real bb's two fields; anything else is unhealthy.
plugin_state() {
  local raw rc=0
  raw=$("${AS[@]}" "$BB" plugin list --json 2>&1) || rc=$?
  PHEALTHY=0; PSTATE=
  if [ "$rc" != 0 ]; then
    PSTATE="bb plugin list failed (exit $rc): $(printf '%s' "$raw" | head -c 200)"
    return 0
  fi
  # Exact JSON types and values: enabled is the boolean true and status is exactly the string "running".
  if printf '%s' "$raw" | jq -e '[.plugins[]? | select(.id == "autarch")] | length == 1 and (.[0].enabled == true) and (.[0].status == "running")' >/dev/null 2>&1; then PHEALTHY=1; fi
  PSTATE=$(printf '%s' "$raw" | jq -r '[.plugins[]? | select(.id == "autarch")][0] // empty | "enabled=\(.enabled | tojson) status=\(.status | tojson) detail=\(.statusDetail | tojson)"' 2>/dev/null) || PSTATE=
  [ -n "$PSTATE" ] || PSTATE="autarch is not in the plugin list: $(printf '%s' "$raw" | head -c 200)"
}

# common_setup <name>: identity, mk-command prefix, report file and the finish trap.
common_setup() {
  NAME=$1
  # bb.js spawns its child with cwd: process.cwd(); as root the caller's cwd is often /root, which mk cannot read (EACCES).
  # Paths are required absolute (below), so leaving the caller's directory loses nothing.
  cd / || exit 64
  DATA=$BBDATA/plugins/autarch
  if [ "$TEST" = 1 ]; then
    MKUID=$(id -u)
    MKHOME=$(getent passwd "$MKUID" | cut -d: -f6)
    MKNAME=$(id -un)
    AS0=(env -i HOME="$MKHOME" USER="$MKNAME" LOGNAME="$MKNAME" PATH=/usr/bin:/bin XDG_RUNTIME_DIR=/run/user/$MKUID)
    BB=$BBDATA/npm/bin/bb
  else
    [ "$(id -u)" -eq 0 ] || { echo "run as root, from the root-owned copy" >&2; exit 64; }
    [ "$(hostname -s)" = zklw ] || { echo "zklw only" >&2; exit 64; }
    MKUID=$(id -u mk)
    AS0=(runuser -u mk -- env -i HOME=/home/mk USER=mk LOGNAME=mk PATH=/usr/bin:/bin XDG_RUNTIME_DIR=/run/user/$MKUID)
    BB=/home/mk/.local/bin/bb   # execs $BB_DATA_DIR/npm/bin/bb once BB_DATA_DIR is pinned
  fi
  REPORT=$("${AS0[@]}" mktemp "/tmp/$NAME-report.XXXXXX")
  trap finish EXIT
}

say() { printf '%s\n' "$*" | "${AS0[@]}" tee -a "$REPORT" >/dev/null; printf '%s\n' "$*"; }
finish() {
  rc=$?
  say "$NAME: exit $rc"
  if [ -n "$URL" ]; then
    "${AS[@]}" "$BB" thread tell "$THREAD" --message-file "$REPORT" || echo "report not delivered; read $REPORT" >&2
  else
    echo "install not verified; report not sent; read $REPORT" >&2
  fi
}

# Step 0: verify the install (plan 1.3.9) before anything is touched. Any failure exits 6.
verify_install() {
  local RT=$BBDATA/bb-app-runtime.json
  { [ -f "$RT" ] && [ ! -L "$RT" ] && [ "$(stat -c %u "$RT")" = "$MKUID" ]; } || { say "runtime file not trusted"; exit 6; }
  { [ -f "$BBDATA/npm/bin/bb" ] && [ "$(stat -c %u "$BBDATA/npm/bin/bb")" = "$MKUID" ]; } || { say "pinned bb missing or not owned by mk"; exit 6; }
  # mk-writable input: read it as mk, never as root, and validate before it reaches /proc paths.
  local URL_C PID ENTRY PORT SET INODE HELD=0 OPEN=0 p fd t kids next
  URL_C=$("${AS0[@]}" jq -r .serverUrl "$RT"); PID=$("${AS0[@]}" jq -r .pid "$RT"); ENTRY=$("${AS0[@]}" jq -r .entryPath "$RT")
  case "$PID" in ""|*[!0-9]*) say "runtime pid not numeric"; exit 6 ;; esac
  # Exactly http://127.0.0.1:<digits>: a decoy such as http://127.0.0.1:9999/x:38886 must not yield a port.
  printf '%s' "$URL_C" | grep -Eq '^http://127\.0\.0\.1:[0-9]+$' || { say "serverUrl not loopback: $URL_C"; exit 6; }
  PORT=${URL_C##*:}
  case "$PORT" in ""|*[!0-9]*) say "runtime port not numeric"; exit 6 ;; esac
  [ "$(stat -c %u "/proc/$PID" 2>/dev/null)" = "$MKUID" ] || { say "runtime pid $PID not alive as mk"; exit 6; }
  # The real install is a symlink chain (npm/bin/bb-app -> ../lib/node_modules/bb-app/dist/bb-app.js): resolve BOTH sides.
  # The pinned side must exist and sit under BBDATA/npm/lib/node_modules/bb-app/ (or be the bin file itself in a plain layout).
  local PINNED_REAL ENTRY_REAL
  PINNED_REAL=$(readlink -e "$BBDATA/npm/bin/bb-app" 2>/dev/null || true); ENTRY_REAL=$(readlink -e "$ENTRY" 2>/dev/null || true)
  { [ -n "$PINNED_REAL" ] && [ -n "$ENTRY_REAL" ] && [ "$ENTRY_REAL" = "$PINNED_REAL" ]; } || { say "entryPath does not resolve to the pinned bb-app"; exit 6; }
  case "$PINNED_REAL" in "$BBDATA"/npm/*) ;; *) say "pinned bb-app resolves outside $BBDATA/npm"; exit 6 ;; esac
  [ "$(stat -c %u "$PINNED_REAL")" = "$MKUID" ] || { say "pinned bb-app is not owned by mk"; exit 6; }
  tr '\0' ' ' < "/proc/$PID/cmdline" | grep -qF "$ENTRY" || { say "pid $PID does not run entryPath"; exit 6; }
  # The owned set: the pid and every descendant.
  SET=$PID; next=$PID
  while [ -n "$next" ]; do
    kids=
    for p in $next; do kids="$kids $({ pgrep -P "$p" || true; } | tr "\n" " ")"; done
    next=$(echo $kids); SET="$SET $next"
  done
  INODE=$(awk -v p="$(printf '%04X' "$PORT")" '$2 ~ ":"p"$" && $4=="0A" {print $10}' /proc/net/tcp | head -1)
  [ -n "$INODE" ] || { say "no listener on port $PORT"; exit 6; }
  for p in $SET; do
    for fd in /proc/$p/fd/*; do
      t=$(readlink "$fd" 2>/dev/null || true)
      [ "$t" = "socket:[$INODE]" ] && HELD=1
      case "$t" in "$BBDATA"/*) OPEN=1 ;; esac
    done
  done
  [ "$HELD" = 1 ] && [ "$OPEN" = 1 ] || { say "listener or data directory not held by the runtime"; exit 6; }
  URL=$URL_C
  PINNED_PID=$PID
  AS=("${AS0[@]}" BB_DATA_DIR="$BBDATA" BB_SERVER_URL="$URL" NODE_ENV=production)
}

# db_holders: print who holds the Home database, empty when nobody does. bb's plugin-state snapshot may keep the
# file open after disable (bead mk-schu.2); the holder list is reported so mk can see which process it is.
db_holders() {
  local out
  out=$(fuser -v "$DATA"/data.db* 2>&1) && echo "$out" || true
}
