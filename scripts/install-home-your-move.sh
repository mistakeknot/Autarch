#!/bin/bash
# Install the Home "your move" build (branch home-your-move: chunks A, B, C; schema V4, expand-only).
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
#
# Commands used are only those the existing Home deploy (scripts/home-upgrade-v3.bash) already uses: bb plugin list|disable|
# install --yes|enable|logs, plus sqlite3, fuser and git, and `bb home moves --json` (the plugin's own read-only export).
# --rollback: export, `bb plugin disable autarch` (fatal on failure), then reinstall the previous build with
# `bb plugin install --yes DIR` when --previous-plugin is given, enable, and verify. V4 is expand-only, so the previous build
# opens the V4 database as it is; restoring the data.db backup has no safe scripted procedure here (home-restore-v2.sh is for
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

run() { say "+ $*"; "$@" >>"$LOG" 2>&1; }
gitmk() { "${AS[@]}" git -C "$REPO" "$@"; }   # via mk, never safe.directory

export_moves() {   # fatal where moves can exist: a rollback record must exist, non-empty and parseable, before any change
  EXPORT=$OUT/moves-before.json
  # Schema below 4 has no moves table, so there is nothing to lose; a version that is unreadable or unknown is NOT that case.
  SV=$("${AS0[@]}" sqlite3 -readonly "file:$DATA/data.db?mode=ro" "select value from schema_meta where key = 'schema_version'" 2>/dev/null || true)
  if [[ "$SV" =~ ^[0-9]+$ ]] && [ "$SV" -lt 4 ] && [ "$MODE" != rollback ]; then
    printf '{"moves":[],"schema":%s,"note":"no moves table at schema %s"}\n' "$SV" "$SV" >"$EXPORT"
    say "export: schema $SV has no moves table, nothing to lose; wrote $EXPORT"
    return 0
  fi
  say "+ bb home moves --json > $EXPORT"
  if ! "${AS0[@]}" "$BB" home moves --json >"$EXPORT" 2>>"$LOG"; then
    say "ABORT: 'bb home moves --json' failed; no rollback export, so nothing was changed (a plugin build before chunk A has no such command)"
    exit 3
  fi
  [ -s "$EXPORT" ] || { say "ABORT: the moves export is empty; nothing was changed"; exit 3; }
  jq -e '(.moves | type) == "array"' "$EXPORT" >/dev/null 2>&1 || { say "ABORT: the moves export is not JSON with a moves array; nothing was changed"; exit 3; }
  say "export: $EXPORT ($(wc -c <"$EXPORT") bytes, $(jq '.moves | length' "$EXPORT") moves)"
}

# Home state (read only), common to all modes.
read_state() {
  LIST=$("${AS0[@]}" "$BB" plugin list --json 2>&1) || { say "bb plugin list failed: $(printf '%s' "$LIST" | head -c 200)"; exit 3; }
  say "plugin: $(printf '%s' "$LIST" | jq -c '[.plugins[]? | select(.id == "autarch") | {enabled, status}]' 2>&1)"
  [ -f "$DATA/data.db" ] || { say "no $DATA/data.db"; exit 3; }
  say "schema: $("${AS0[@]}" sqlite3 -readonly "$DATA/data.db" 'select max(version) from migration_log' 2>&1)"
}

disable_plugin() {   # fatal: a plugin that is still running must not be installed over or rolled back under
  say "+ bb plugin disable autarch"
  "${AS0[@]}" "$BB" plugin disable autarch >>"$LOG" 2>&1 || { say "ABORT: bb plugin disable autarch failed; the plugin may still be running; nothing further was changed"; exit 5; }
  HOLD=$(fuser -v "$DATA"/data.db* 2>&1 || true)
  [ -z "$HOLD" ] || { say "ABORT: something still holds data.db: $HOLD"; exit 5; }
}

enable_and_verify() {   # $1 = expected max migration_log version
  "${AS0[@]}" "$BB" plugin enable autarch >>"$LOG" 2>&1 || { say "enable failed; see 'bb plugin logs autarch'"; exit 5; }
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
  manual "the data.db is left as it is: V4 is expand-only and the previous build opens it. To restore a data.db backup instead there is no scripted procedure: with the plugin disabled, as mk, copy ${BKFILE:-the pre-install backup} over $DATA/data.db, check that fuser -v $DATA/data.db* shows no holder, then run bb plugin enable autarch"
  exit 0
fi

# 1. Branch and commit.
if [ "$TESTHOOK" = 1 ]; then say "test hook: git checks skipped"; BRANCH=home-your-move; HEAD=test
else
BRANCH=$(gitmk branch --show-current)
HEAD=$(gitmk rev-parse HEAD)
say "branch $BRANCH, HEAD $HEAD"
[ "$BRANCH" = home-your-move ] || { say "wrong branch: need home-your-move"; exit 2; }
if [ -n "$WANT" ]; then
  [ "$HEAD" = "$WANT" ] || { say "HEAD is not the requested commit $WANT"; exit 2; }
else
  say "no --commit given: the sha is not pinned (fine for --check only)"
fi
[ -z "$(gitmk status --porcelain --untracked-files=no)" ] || { say "tracked files are modified; install only a clean commit"; exit 2; }
fi
[ "$TESTHOOK" = 1 ] || [ -d "$PLUGIN/node_modules" ] || manual "in $PLUGIN run 'npm install' as mk (the README step); node_modules is missing"

# 2. Current Home state and the rollback export (both read only; the export is fatal on failure).
read_state
export_moves
BACKUP=$OUT/data.db.pre-your-move
if [ "$MODE" = check ]; then
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
"${AS0[@]}" "$BB" plugin install --yes "$PLUGIN" >>"$LOG" 2>&1 || { say "install failed; plugin is disabled; backup $BACKUP; to revert use --rollback"; exit 5; }
enable_and_verify 4
say "installed $HEAD. Rollback data: $BACKUP and $EXPORT"
manual "open Home and check the Your move panel and one card conversation by eye; this script cannot see the UI"
