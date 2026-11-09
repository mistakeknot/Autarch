#!/bin/bash
# Install the Home "your move" build (PR 17 build: chunks A, B, C; schema V4, expand-only; later additive builds reuse it, ending at their own CODE_VERSION).
#   scripts/install-home-your-move.sh --check    [--commit <40-hex sha>] [REPORT_THREAD]
#   scripts/install-home-your-move.sh --go       --commit <40-hex sha>  [REPORT_THREAD]
#   scripts/install-home-your-move.sh --rollback [--previous-plugin DIR] [--backup FILE] [REPORT_THREAD]
# Run from the checkout on the host as mk (or as root; mk's commands then go through runuser). Reports through report-tell to
# REPORT_THREAD (env VIZIER_REPORT_THREAD, or the last argument). Stops at the first failure (set -e, report via the EXIT trap).
#
# --check changes nothing in Home, git or the plugin. It WRITES only, under a fresh 0700 directory
# ($HOME_YOUR_MOVE_OUT_DIR or /tmp)/home-your-move-<UTC stamp>-<random>/ (made by mktemp -d; as root the override must sit in a root-owned directory others cannot write): install.log, report.txt and moves-before.json (the read-only
# `bb home moves --json` export), and it sends report.txt through report-tell when a thread is given. Its read-only steps are
# git rev-parse/branch/status, `bb plugin list --json`, `bb home moves --json` and sqlite3 -readonly. A failing export fails
# the check, because --go would abort at the same step.
# Exception, read from data.db with sqlite3 mode=ro: when schema_meta schema_version is below 4 there is no moves table, so
# --check and --go write moves-before.json as {"moves":[],"schema":N,"note":...} and continue. Version 4 or more, unreadable
# or unknown keeps the export fatal. --rollback always keeps it fatal.
# --go and --rollback both take that export FIRST, and abort hard (before disabling or changing anything) if it fails or is
# empty or is not JSON with a "moves" array.
# disabled-plugin export: begin
# One narrow exception (mk-hd2ks): `bb home moves` is a command the plugin itself provides, so it exists only while the plugin is
# enabled and running. When `bb plugin list` shows the plugin as NOT enabled-and-running (a failed or interrupted earlier run left
# it disabled, the case that aborted a rollback), no process writes the moves table, and the export is taken from the same table
# with sqlite3 mode=ro instead: same rows, same order, the same {"moves":[...]} shape (without the derived "groups"). It is
# refused, and the export stays fatal as above, when the plugin list is unreadable, when something holds data.db, when schema_meta
# schema_version is not a number of 4 or more, when the moves table cannot be read, or when the result is not a JSON array. A
# plugin that is enabled and running always goes through `bb home moves`, with no fallback.
# disabled-plugin export: end
#
# Commands used are only those the existing Home deploy (scripts/home-upgrade-v3.bash) already uses: bb plugin list|disable|
# install --yes|enable|logs, plus sqlite3, fuser and git, and `bb home moves --json` (the plugin's own read-only export).
# --rollback: export, `bb plugin disable autarch` (fatal on failure), then reinstall the previous build with
# `bb plugin install --yes DIR` when --previous-plugin is given, enable, and verify. An expand-only schema (V4, V5) is opened by the previous build as it is; the previous build
# restoring the data.db backup has no safe scripted procedure here (home-restore-v2.sh is for
# the v2 backup only), so that part prints MANUAL: lines.
set -euo pipefail
export PATH=/usr/local/bin:/usr/bin:/bin
umask 077

# Root only: resolve an output-directory override ($1) once and print the resolved path. Every ancestor up to $3 (default /, the
# anchor itself is not checked) must be owned by $2 (uid 0 in real use; an argument so the rule can be tested without root) and
# not group/other writable; the final directory itself may instead be sticky and owned by $2 (like /tmp). A writable or foreign
# link in the chain would allow a symlink or rename race against root's writes. Callers use only the printed path afterwards.
out_parent_resolve() {   # $1 = directory, $2 = required owner uid, $3 = anchor (optional)
  local p top=${3:-/} mode final=1
  case $1 in *[[:cntrl:]]*) return 1 ;; esac   # trailing newlines are lost by command substitution below
  p=$(realpath -e -- "$1" 2>/dev/null) && [ -d "$p" ] || return 1
  [ "$top" = / ] || top=$(realpath -e -- "$top") || return 1
  case $p in *[[:cntrl:]]*) return 1 ;; esac   # a newline would be lost by dirname below and defeat the walk
  printf '%s\n' "$p"
  while :; do
    [ "$p" != "$top" ] || return 0
    [ "$(stat -c %u -- "$p")" = "$2" ] || return 1
    mode=$((8#$(stat -c %a -- "$p"))) || return 1
    if [ $((mode & 0022)) -ne 0 ]; then
      { [ "$final" = 1 ] && [ $((mode & 01000)) -ne 0 ]; } || return 1
    fi
    [ "$p" != / ] || return 0
    p=$(dirname -- "$p"); final=0
  done
}
# A reused pid is not the bb server: the runtime file ($1) must name a live pid (under proc root $2, /proc unless a test passes
# another) with one argv element exactly equal to the file's entryPath or to its resolved (realpath) form. No substring or
# basename match.
runtime_names_live_process() {   # $1 = bb-app-runtime.json, $2 = proc root
  local pid entry real arg
  pid=$(jq -r '.pid // empty' "$1" 2>/dev/null || true)
  entry=$(jq -r '.entryPath // empty' "$1" 2>/dev/null || true)
  [[ "$pid" =~ ^[0-9]+$ ]] && [ -n "$entry" ] || return 1
  [ "$2" != /proc ] || kill -0 "$pid" 2>/dev/null || return 1
  [ -r "$2/$pid/cmdline" ] || return 1
  real=$(realpath -e -- "$entry" 2>/dev/null || true)
  while IFS= read -r -d '' arg || [ -n "$arg" ]; do
    if [ "$arg" = "$entry" ] || { [ -n "$real" ] && [ "$arg" = "$real" ]; }; then return 0; fi
  done <"$2/$pid/cmdline"
  return 1
}
# Sourced (by the tests), the function above is all that is defined.
[ "${BASH_SOURCE[0]}" = "$0" ] || return 0

MODE= WANT= ARG_THREAD= PREV= BKFILE=
while [ $# -gt 0 ]; do
  case "$1" in
    --check) MODE=check; shift ;;
    --go) MODE=go; shift ;;
    --rollback) MODE=rollback; shift ;;
    --previous-plugin) PREV=${2:?--previous-plugin needs a directory}; shift 2 ;;
    --backup) BKFILE=${2:?--backup needs a file}; shift 2 ;;
    --commit) WANT=${2:?--commit needs a sha}; shift 2 ;;
    -*) echo "unknown option $1" >&2; exit 64 ;;
    *) ARG_THREAD=$1; shift ;;
  esac
done
[ -n "$MODE" ] || { echo "usage: $0 --check|--go|--rollback [--commit <40-hex sha>] [REPORT_THREAD]" >&2; exit 64; }
[ "$MODE" != go ] || [[ "$WANT" =~ ^[0-9a-f]{40}$ ]] || { echo "--go needs --commit <full 40-hex sha>" >&2; exit 64; }
[ -z "$WANT" ] || [[ "$WANT" =~ ^[0-9a-f]{40}$ ]] || { echo "--commit must be 40 lowercase hex" >&2; exit 64; }

REPORT_THREAD=${VIZIER_REPORT_THREAD:-$ARG_THREAD}
REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
PLUGIN=$REPO/integrations/bb-plugin-autarch
# The service user's home and bb come from the account database, never from a literal path. Overrides, for a host that keeps
# them elsewhere: HOME_BB_BIN (the bb launcher) and HOME_BB_DATA (bb's data directory; by default the one directory under
# the service user's home that holds a bb-app-runtime.json, and an ambiguous or missing one is fatal).
MKHOME=$(getent passwd mk 2>/dev/null | cut -d: -f6 || true)
DATA= BB=
# shared destination lock: begin (mk-zq345.5)
# --go and --rollback take <state dir>/lock, the file the update runner already locks, before they resolve bb or read anything about
# the installed build, and hold it to the end of the process (--check changes nothing and takes none). A caller that already holds
# it (the runner's fd 9, or BPD_LOCK_FD) is recognised by descriptor, never by environment alone: the fd must be open, name the same
# device:inode as the lock file, and pass flock -n. Otherwise this opens its own descriptor and waits, at most HOME_UI_LOCK_WAIT
# seconds (default 2100, 35 minutes). HOME_UI_LOCK=skip lets --rollback run without the lock (mk only, emergency).
lock_holder() {   # $1 = lock file; prints the pid /proc/locks names, or nothing
  local ino; ino=$(stat -c %i "$1" 2>/dev/null) || return 0
  awk -v i="$ino" '$2 != "->" { n = split($6, a, ":"); if (a[n] == i) { print $5; exit } }' /proc/locks 2>/dev/null || true
}
take_lock() {   # $1 = mode
  local st=${HOME_UI_STATE_DIR:-$MKHOME/.local/state/autarch-home-update} l f a b w=${HOME_UI_LOCK_WAIT:-2100}
  l=$st/lock
  if [ "${HOME_UI_LOCK:-}" = skip ]; then
    if [ "$1" = rollback ]; then
      echo "HOME_UI_LOCK=skip: running WITHOUT the destination lock. RACE RISK: the update runner or a tool deploy can install a build between this command's steps. Emergency use only." >&2
      return 0
    fi
    echo "HOME_UI_LOCK=skip applies to --rollback only; taking the lock" >&2
  fi
  [ -d "$st" ] || { echo "WARNING: no state dir $st; running without the destination lock" >&2; return 0; }
  f=${BPD_LOCK_FD:-9}; case $f in ''|*[!0-9]*) f=9 ;; esac
  a=$(stat -L -c %d:%i "/proc/self/fd/$f" 2>/dev/null || true); b=$(stat -c %d:%i "$l" 2>/dev/null || true)
  if [ -z "$a" ] || [ "$a" != "$b" ]; then
    if [ ! -e "$l" ]; then
      : >>"$l" || { echo "cannot create the destination lock $l" >&2; exit 64; }
      [ "$(id -u)" -ne 0 ] || chown --reference="$st" "$l"   # as root, the file stays the service user's, as the runner needs it
    fi
    exec {f}>>"$l" || { echo "cannot open the destination lock $l" >&2; exit 64; }
  fi
  if ! flock -n "$f"; then
    echo "waiting for the destination lock (held by pid $(lock_holder "$l")); giving up after ${w}s" >&2
    flock -w "$w" "$f" || { echo "another deploy holds the lock (pid $(lock_holder "$l"))" >&2; exit 75; }
  fi
  export BPD_LOCK_FD="$f"   # children reuse this descriptor (2.1: a holder exports it and keeps it open)
}
case $MODE in go|rollback) take_lock "$MODE" ;; esac
# shared destination lock: end
# Test hook, honoured ONLY with --check: HOME_YOUR_MOVE_TEST_DATA_DIR (a dir holding data.db) and HOME_YOUR_MOVE_TEST_BB (a stub
# bb). Under it the script runs as the current user and skips the git checks. --go and --rollback ignore and refuse it.
TESTHOOK=0
if [ -n "${HOME_YOUR_MOVE_TEST_DATA_DIR:-}${HOME_YOUR_MOVE_TEST_BB:-}" ]; then
  # Guards, in this order, before the hook can take effect: never as root (any mode), both variables or neither, --check only.
  [ "$(id -u)" -ne 0 ] || { echo "the test hook is refused as root" >&2; exit 64; }
  { [ -n "${HOME_YOUR_MOVE_TEST_DATA_DIR:-}" ] && [ -n "${HOME_YOUR_MOVE_TEST_BB:-}" ]; } || { echo "the test hook needs both HOME_YOUR_MOVE_TEST_DATA_DIR and HOME_YOUR_MOVE_TEST_BB" >&2; exit 64; }
  [ "$MODE" = check ] || { echo "the test hook is for --check only" >&2; exit 64; }
  TESTHOOK=1; DATA=${HOME_YOUR_MOVE_TEST_DATA_DIR:?}; BB=${HOME_YOUR_MOVE_TEST_BB:?}
fi
if [ "$TESTHOOK" = 0 ]; then
  [ -n "$MKHOME" ] || { echo "the service user mk has no home directory in the account database" >&2; exit 64; }
  BB=${HOME_BB_BIN:-$MKHOME/.local/bin/bb}
  BBDATA=${HOME_BB_DATA:-}
  if [ -z "$BBDATA" ]; then
    mapfile -t FOUND < <(find "$MKHOME" -maxdepth 3 -name bb-app-runtime.json -not -path '*/node_modules/*' 2>/dev/null | sort)
    [ "${#FOUND[@]}" -eq 1 ] || { echo "cannot tell bb's data directory (${#FOUND[@]} candidates); set HOME_BB_DATA" >&2; exit 64; }
    # The one candidate must also name a live process, or it is a stale leftover.
    runtime_names_live_process "${FOUND[0]}" /proc || { echo "${FOUND[0]} does not name a live bb process (pid and entryPath); set HOME_BB_DATA" >&2; exit 64; }
    BBDATA=$(dirname "${FOUND[0]}")
  fi
  DATA=$BBDATA/plugins/autarch
fi
MKUID=$(id -u mk 2>/dev/null || id -u)
if [ "$TESTHOOK" = 1 ]; then AS=(); AS0=(env -i HOME="${HOME:-/}" PATH=/usr/bin:/bin)
else
  if [ "$(id -u)" -eq 0 ]; then AS=(runuser -u mk --); elif [ "$(id -un)" = mk ]; then AS=(); else echo "run as mk or root" >&2; exit 64; fi
  AS0=("${AS[@]}" env -i HOME="$MKHOME" USER=mk LOGNAME=mk PATH=/usr/bin:/bin XDG_RUNTIME_DIR=/run/user/$MKUID)
fi

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
OUTPARENT=${HOME_YOUR_MOVE_OUT_DIR:-/tmp}
if [ "$(id -u)" -eq 0 ] && [ -n "${HOME_YOUR_MOVE_OUT_DIR:-}" ]; then
  # Resolved and checked once; only the resolved path is used from here on.
  OUTPARENT=$(out_parent_resolve "$OUTPARENT" 0) || { echo "HOME_YOUR_MOVE_OUT_DIR $OUTPARENT must resolve to a directory owned by root, with every ancestor root-owned and not group/other writable (the directory itself may be root-owned sticky); refusing as root" >&2; exit 64; }
fi
# A fresh 0700 directory made atomically by mktemp, never a predictable name; every output file lives only inside it.
OUT=$(mktemp -d "$OUTPARENT/home-your-move-$STAMP-XXXXXX") || { echo "cannot create an output directory under $OUTPARENT" >&2; exit 64; }
chmod 0700 "$OUT"
LOG=$OUT/install.log
MSG=$OUT/report.txt
: >"$MSG"
say() { printf '%s\n' "$*" | tee -a "$LOG" "$MSG"; }
manual() { say "MANUAL: $*"; }

finish() {
  rc=$?
  say "install-home-your-move ($MODE): exit $rc; output dir $OUT"
  if [ -n "$REPORT_THREAD" ]; then
    if [ "$(id -u)" -eq 0 ] || [ "$(id -un)" = mk ]; then
      chmod 0644 "$MSG" 2>/dev/null || true
      "${AS[@]}" "${MKHOME:-/nonexistent}/.local/bin/report-tell" "$REPORT_THREAD" --message-file "$MSG" >>"$LOG" 2>&1 || echo "report-tell failed; read $MSG" >&2
    fi
  else
    echo "no report thread given (VIZIER_REPORT_THREAD or argument); report is in $MSG" >&2
  fi
}
trap finish EXIT

# A failed precondition: --go and --rollback stop at the first ($2 is the exit code); --check records it, goes on, and exits with
# the first code after the last check, so one run lists everything that would stop --go.
FAILS=0 FIRSTRC=
bad() {   # $1 = message, $2 = exit code
  say "$1"
  [ "$MODE" = check ] || exit "$2"
  FAILS=$((FAILS+1)); [ -n "$FIRSTRC" ] || FIRSTRC=$2
}

run() { say "+ $*"; "$@" >>"$LOG" 2>&1; }
gitmk() { "${AS[@]}" git -C "$REPO" "$@"; }   # via mk, never safe.directory

# disabled-plugin export: begin
# Returns 0 with $EXPORT written, or 1 (having said why) so the caller falls through to `bb home moves`, which then fails fatally as
# before. Read only: sqlite3 -readonly, mode=ro, and fuser; nothing is disabled, enabled or installed.
export_from_db() {
  local pstate sv held rows snap n nulq nulw nulc pcols
  pstate=$(printf '%s' "${LIST:-}" | jq -r 'if (.plugins | type) != "array" then "unknown" elif any(.plugins[]; .id == "autarch" and .enabled == true and .status == "running") then "running" else "not-running" end' 2>/dev/null || true)
  [ "$pstate" = not-running ] || return 1   # running: bb answers; unknown: do not guess
  held=$(fuser -v "$DATA"/data.db* 2>&1 || true)
  [ -z "$held" ] || { say "export: the plugin is not running but something holds data.db ($held); not reading it directly"; return 1; }
  sv=$("${AS0[@]}" sqlite3 -readonly "file:$DATA/data.db?mode=ro" "select value from schema_meta where key = 'schema_version'" 2>/dev/null || true)
  { [[ "$sv" =~ ^[0-9]+$ ]] && [ "$sv" -ge 4 ]; } || { say "export: the plugin is not running and schema_version '${sv:-unreadable}' is not a number of 4 or more; no direct read"; return 1; }
  # sqlite3 -json cuts a TEXT value at an embedded NUL (the plugin's own export keeps it), which would write a parseable but
  # incomplete record: refuse, for any column, a NUL byte or a BLOB.
  pcols=$("${AS0[@]}" sqlite3 -readonly "file:$DATA/data.db?mode=ro" "select group_concat(name, ',') from pragma_table_info('moves')" 2>/dev/null || true)   # listed before the guard, and compared with the list inside the transaction below
  nulq="select group_concat('instr(CAST(\"' || name || '\" AS BLOB), x''00'') > 0 OR typeof(\"' || name || '\") = ''blob''', ' OR ') from pragma_table_info('moves')"
  nulw=$("${AS0[@]}" sqlite3 -readonly "file:$DATA/data.db?mode=ro" "$nulq" 2>/dev/null || true)
  [ -n "$nulw" ] || { say "export: the moves columns could not be listed; no direct read"; return 1; }
  # One connection and one read transaction: the guard counts, the column list, the schema version and the rows all come from one
  # snapshot, so a writer that starts after the checks above cannot slip a NUL in between a check and the read.
  rows=$OUT/moves-rows.json snap=$OUT/moves-snapshot.txt
  "${AS0[@]}" sqlite3 -bail -readonly "file:$DATA/data.db?mode=ro" "BEGIN" "select count(*) from moves where $nulw" "select count(*) from moves" \
    "select group_concat(name, ',') from pragma_table_info('moves')" "select value from schema_meta where key = 'schema_version'" \
    ".mode json" "SELECT * FROM moves ORDER BY opened_at DESC, task_id, generation" "COMMIT" >"$snap" 2>>"$LOG" \
    || { say "export: the plugin is not running and the moves table cannot be read from data.db"; return 1; }
  nulc=$(sed -n 1p "$snap") n=$(sed -n 2p "$snap")
  [ "$nulc" = 0 ] || { say "export: the moves table holds a NUL byte or a BLOB in ${nulc:-an unreadable} row(s), which sqlite3 -json cannot export faithfully; no direct read"; return 1; }
  { [ "$(sed -n 3p "$snap")" = "$pcols" ] && [ "$(sed -n 4p "$snap")" = "$sv" ]; } || { say "export: the moves table changed while it was being read; no direct read"; return 1; }
  [[ "$n" =~ ^[0-9]+$ ]] || { say "export: the moves rows could not be counted in data.db"; return 1; }
  tail -n +5 "$snap" >"$rows"
  if [ "$n" = 0 ]; then   # sqlite3 -json prints nothing for zero rows: only a counted zero is an empty table
    [ ! -s "$rows" ] || { say "export: the moves rows could not be read from data.db"; return 1; }
    echo '[]' >"$rows"
  fi
  [ "$(jq 'length' "$rows" 2>/dev/null || true)" = "$n" ] || { say "export: the moves read from data.db do not match the $n counted rows"; return 1; }
  rm -f "$snap"
  jq -e 'type == "array"' "$rows" >/dev/null 2>&1 || { say "export: the moves read from data.db are not a JSON array"; return 1; }
  jq -c --argjson schema "$sv" '{moves: ., schema: $schema, source: "data.db, read only (the plugin was not running)"}' "$rows" >"$EXPORT" || return 1
  rm -f "$rows"
  say "export: the plugin is not running, so 'bb home moves' is unavailable; read the moves table from data.db read only"
  say "export: $EXPORT ($(wc -c <"$EXPORT") bytes, $(jq '.moves | length' "$EXPORT") moves)"
}
# disabled-plugin export: end
export_moves() {   # fatal where moves can exist: a rollback record must exist, non-empty and parseable, before any change
  EXPORT=$OUT/moves-before.json
  # Schema below 4 has no moves table, so there is nothing to lose; a version that is unreadable or unknown is NOT that case.
  SV=$("${AS0[@]}" sqlite3 -readonly "file:$DATA/data.db?mode=ro" "select value from schema_meta where key = 'schema_version'" 2>/dev/null || true)
  if [[ "$SV" =~ ^[0-9]+$ ]] && [ "$SV" -lt 4 ] && [ "$MODE" != rollback ]; then
    printf '{"moves":[],"schema":%s,"note":"no moves table at schema %s"}\n' "$SV" "$SV" >"$EXPORT"
    say "export: schema $SV has no moves table, nothing to lose; wrote $EXPORT"
    return 0
  fi
  # disabled-plugin export: begin
  if export_from_db; then return 0; fi
  # disabled-plugin export: end
  say "+ bb home moves --json > $EXPORT"
  if ! "${AS0[@]}" "$BB" home moves --json >"$EXPORT" 2>>"$LOG"; then
    bad "ABORT: 'bb home moves --json' failed; no rollback export, so nothing was changed (a plugin build before chunk A has no such command)" 3
    return 0
  fi
  [ -s "$EXPORT" ] || { bad "ABORT: the moves export is empty; nothing was changed" 3; return 0; }
  jq -e '(.moves | type) == "array"' "$EXPORT" >/dev/null 2>&1 || { bad "ABORT: the moves export is not JSON with a moves array; nothing was changed" 3; return 0; }
  say "export: $EXPORT ($(wc -c <"$EXPORT") bytes, $(jq '.moves | length' "$EXPORT") moves)"
}

# Home state (read only), common to all modes.
read_state() {
  LIST=$("${AS0[@]}" "$BB" plugin list --json 2>&1) || { bad "bb plugin list failed: $(printf '%s' "$LIST" | head -c 200)" 3; LIST='{}'; }
  say "plugin: $(printf '%s' "$LIST" | jq -c '[.plugins[]? | select(.id == "autarch") | {enabled, status}]' 2>&1)"
  [ -f "$DATA/data.db" ] || bad "no $DATA/data.db" 3
  say "schema: $("${AS0[@]}" sqlite3 -readonly "$DATA/data.db" 'select max(version) from migration_log' 2>&1)"
}

disable_plugin() {   # fatal: a plugin that is still running must not be installed over or rolled back under
  say "+ bb plugin disable autarch"
  "${AS0[@]}" "$BB" plugin disable autarch >>"$LOG" 2>&1 || { say "ABORT: bb plugin disable autarch failed; the plugin may still be running; nothing further was changed"; exit 5; }
  HOLD=$(fuser -v "$DATA"/data.db* 2>&1 || true)
  [ -z "$HOLD" ] || { say "ABORT: something still holds data.db: $HOLD"; exit 5; }
}

enable_and_verify() {   # $1 = expected max migration_log version
  # redundant-enable skip: begin
  # `bb plugin install` leaves the plugin enabled and running. `bb plugin enable` on a running plugin reloads it: bb stops every service
  # within 5 s each and marks the plugin degraded ("service home-queue did not stop") when one is mid-handler, and it stays degraded until
  # the next reload. So when the plugin is already enabled and running, the unchanged enable line below runs against a stand-in bb that
  # answers for it, and the verification loop that follows still checks the running state and the migration version.
  local BB_KEEP=$BB
  if "${AS0[@]}" timeout -k 2 30 "$BB" plugin list --json 2>/dev/null | jq -e '[.plugins[]? | select(.id == "autarch")] | length == 1 and (.[0].enabled == true) and (.[0].status == "running")' >/dev/null 2>&1; then
    say "autarch is already enabled and running after the install; skipping the redundant 'bb plugin enable' (it would reload the plugin and stop every service again)"
    BB=/bin/true
  fi
  # redundant-enable skip: end
  "${AS0[@]}" "$BB" plugin enable autarch >>"$LOG" 2>&1 || { say "enable failed; see 'bb plugin logs autarch'"; exit 5; }
  # redundant-enable skip: begin
  BB=$BB_KEEP
  # redundant-enable skip: end
  OK=0 S= M=
  for _ in $(seq 12); do
    sleep 5
    S=$("${AS0[@]}" "$BB" plugin list --json 2>&1 | jq -e '[.plugins[]? | select(.id == "autarch")] | length == 1 and (.[0].enabled == true) and (.[0].status == "running")' 2>/dev/null || true)
    M=$("${AS0[@]}" sqlite3 -readonly "$DATA/data.db" 'select max(version) from migration_log' 2>&1 || true)
    [ "$S" = true ] && { [ -z "$1" ] || [ "$M" = "$1" ]; } && { OK=1; break; }
  done
  say "status running: ${S:-unknown}; migration_log max version: ${M:-unknown}"
  [ "$OK" = 1 ] || { say "no evidence of a running plugin; see 'bb plugin logs autarch'"; exit 6; }
}
# install-error recheck: begin
# A path-plugin `bb plugin install` builds only the frontend (app.js, app.css, app.meta.json, each written by renaming a staged
# file); the server bundle runs from bb's own runtime cache, so dist/server.js and dist/server.meta.json are not rebuilt and are
# not checked. A finished install leaves all three frontend artifacts newer than $1 and app.meta.json naming the plugin; a
# half-written dist (some artifacts rebuilt, the rest old) does not pass.
dist_rebuilt() {   # $1 = marker file touched just before the install began
  local d="$PLUGIN/dist" f
  for f in app.js app.css app.meta.json; do
    if [ -s "$d/$f" ] && [ "$d/$f" -nt "$1" ]; then :; else return 1; fi
  done
  jq -e '.pluginId == "autarch"' "$d/app.meta.json" >/dev/null 2>&1 || return 1
}
# `bb plugin install` can exit non-zero on the client (a reset connection while it waits) after the server has finished the install.
# Returns 0 when the install succeeded, or failed on the client but autarch is installed from $PLUGIN, in a state install leaves
# (running, or disabled and waiting for the enable step), with dist fully rebuilt after $1 was touched; returns 1 on a real mismatch,
# after a bounded poll. The enable step that follows still checks the running plugin and the migration version.
install_rechecked() {   # $1 = marker file touched just before the install began
  local rc=0 wait=${HOME_YOUR_MOVE_RECHECK_WAIT:-60} step=${HOME_YOUR_MOVE_RECHECK_INTERVAL:-3} row newer end rem grace=2
  case $wait in ''|*[!0-9]*) wait=60 ;; esac     # a bad value falls back to the default: the poll must end
  case $step in ''|*[!0-9]*|0) step=3 ;; esac
  "${AS0[@]}" "$BB" plugin install --yes "$PLUGIN" >>"$LOG" 2>&1 || rc=$?
  [ "$rc" -ne 0 ] || return 0
  say "install: 'bb plugin install' exited $rc ($(tail -n 1 "$LOG" | head -c 160)); checking for up to ${wait}s (plus a ${grace}s kill grace on a stuck request) whether the server completed it"
  end=$((SECONDS+wait))   # wall-clock deadline: each status request below is held to the time left, then killed after the grace
  while :; do
    rem=$((end-SECONDS)); [ "$rem" -ge 1 ] || rem=1
    row=$("${AS0[@]}" timeout -k "$grace" "$rem" "$BB" plugin list --json 2>/dev/null | jq -r --arg p "$PLUGIN" '[.plugins[]? | select(.id == "autarch")] | if length == 1 and ((.[0].source // "") | sub("/+$"; "")) == ("path:" + ($p | sub("/+$"; ""))) and ((.[0].enabled == true and .[0].status == "running") or .[0].enabled == false) then "match" else "no" end' 2>/dev/null || true)
    if [ "$row" = match ]; then
      newer=; dist_rebuilt "$1" && newer=yes
      if [ -n "$newer" ]; then
        say "WARNING: 'bb plugin install' exited $rc, but the server completed the install: autarch is installed from $PLUGIN and its frontend build (dist/app.js, app.css, app.meta.json) was rebuilt after the install began; going on to enable and verify"
        return 0
      fi
    fi
    rem=$((end-SECONDS)); [ "$rem" -gt 0 ] || return 1
    [ "$step" -le "$rem" ] || step=$rem
    sleep "$step"
  done
}
# install-error recheck: end

if [ "$MODE" = rollback ]; then
  read_state
  export_moves   # first, before anything is disabled or reverted
  if [ -z "$PREV" ]; then
    manual "no --previous-plugin DIR given: build the previous plugin version in a directory, then rerun with --previous-plugin DIR (this script will not guess which build to restore)"
    exit 7
  fi
  [ -d "$PREV" ] || { say "--previous-plugin $PREV is not a directory"; exit 2; }
  disable_plugin
  say "+ bb plugin install --yes $PREV"
  "${AS0[@]}" "$BB" plugin install --yes "$PREV" >>"$LOG" 2>&1 || { say "install of the previous build failed; the plugin stays disabled"; exit 5; }
  enable_and_verify ""
  say "rolled back to the build in $PREV. Export taken before: $EXPORT"
  manual "the data.db is left as it is: an expand-only schema is opened by the previous build. To restore a data.db backup instead there is no scripted procedure: with the plugin disabled, as mk, copy ${BKFILE:-the pre-install backup} over $DATA/data.db, check that fuser -v $DATA/data.db* shows no holder, then run bb plugin enable autarch"
  exit 0
fi

# 1. Branch and commit.
if [ "$TESTHOOK" = 1 ]; then say "test hook: git checks skipped"; BRANCH=home-your-move; HEAD=test
else
BRANCH=$(gitmk branch --show-current)
HEAD=$(gitmk rev-parse HEAD)
say "branch ${BRANCH:-(detached)}, HEAD $HEAD"   # the pinned --commit, not a branch name, says what is installed
if [ -n "$WANT" ]; then
  [ "$HEAD" = "$WANT" ] || bad "HEAD is not the requested commit $WANT" 2
else
  say "no --commit given: the sha is not pinned (fine for --check only)"
fi
[ -z "$(gitmk status --porcelain --untracked-files=no)" ] || bad "tracked files are modified; install only a clean commit" 2
fi
[ "$TESTHOOK" = 1 ] || [ -d "$PLUGIN/node_modules" ] || manual "in $PLUGIN run 'npm install' as mk (the README step); node_modules is missing"

# 2. Current Home state and the rollback export (both read only; the export is fatal on failure).
read_state
export_moves
BACKUP=$OUT/data.db.pre-your-move
if [ "$MODE" = check ]; then
  if [ "$FAILS" -gt 0 ]; then
    say "--check: $FAILS precondition(s) failed; --go would stop at the first of them. Nothing was changed; only $OUT was written."
    exit "$FIRSTRC"
  fi
  say "--check: would back up $DATA/data.db to $BACKUP,"
  say "--check: then run 'bb plugin disable autarch', 'bb plugin install --yes $PLUGIN', 'bb plugin enable autarch', and poll status and migration_log."
  say "--check: Home, git and the plugin were not changed; only $OUT was written."
  exit 0
fi

# 3. Backup (the plugin also writes its own verified pre-migration backup on open).
"${AS0[@]}" sqlite3 -readonly "$DATA/data.db" ".backup '$BACKUP'" >>"$LOG" 2>&1 || { say "backup failed"; exit 4; }
[ "$("${AS0[@]}" sqlite3 -readonly "$BACKUP" 'pragma integrity_check' 2>&1)" = ok ] || { say "backup integrity_check failed"; exit 4; }
say "backup: $BACKUP ($(sha256sum "$BACKUP" | cut -d' ' -f1))"

# 4. Install and enable, as scripts/home-upgrade-v3.bash does.
disable_plugin
say "+ bb plugin install --yes $PLUGIN"
# install-error recheck: begin
# The install runs (and is rechecked) here; the unchanged line below then runs against a stand-in bb that answers for the outcome, so
# it neither installs twice nor changes its failure message.
INSTALL_MARK=$OUT/install-started BB_REAL=$BB
: >"$INSTALL_MARK"
if install_rechecked "$INSTALL_MARK"; then BB=/bin/true; else BB=/bin/false; fi
# install-error recheck: end
"${AS0[@]}" "$BB" plugin install --yes "$PLUGIN" >>"$LOG" 2>&1 || { say "install failed; plugin is disabled; backup $BACKUP; to revert use --rollback"; exit 5; }
# install-error recheck: begin
BB=$BB_REAL
# install-error recheck: end
# The migration version this build ends at is the build's own CODE_VERSION (4 for the original your-move build, 5 once
# root_fsid arrived); a hard-coded number would roll back every later additive build.
EXPECT=$(sed -n 's/^export const CODE_VERSION = \([0-9][0-9]*\);.*/\1/p' "$PLUGIN/migrations.ts" 2>/dev/null | head -1)
[[ "$EXPECT" =~ ^[0-9]+$ ]] || [ "$TESTHOOK" = 1 ] || { say "cannot read CODE_VERSION from $PLUGIN/migrations.ts; the plugin is disabled; backup $BACKUP; to revert use --rollback"; exit 5; }
enable_and_verify "${EXPECT:-4}"
say "installed $HEAD. Rollback data: $BACKUP and $EXPORT"
manual "open Home and check the Your move panel and one card conversation by eye; this script cannot see the UI"
