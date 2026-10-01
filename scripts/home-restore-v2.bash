# home-restore-v2.bash: body, started only by the home-restore-v2.sh launcher.
# Restores Home (the autarch bb plugin) to its pre-v3 backup.
# Run on zklw from the root-owned copy (installed by the generated home-v3-run-<sha12>.sh package): sudo /usr/local/libexec/home-v3-<sha12>/home-restore-v2.sh --thread <thr_...> --repo <Autarch checkout> [--backup <path>] [--check]
# Exit codes: 0 restored (or --check passed), 2 backup failed verification, 3 Home still holds the DB,
# 4 build or install failed, 5 the plugin did not start on the backup, 6 the install could not be verified.
# Tested in test mode by integrations/bb-plugin-autarch/__tests__/restore-script.test.ts (Task 2.8a); the sudo
# launch, the real npm ci and bb plugin build, and the real runuser are unprobed (beads mk-schu.4, 2.11).
set -euo pipefail
. "${BASH_SOURCE[0]%/*}/home-common.bash"
parse_common "$@"
set -- "${REST[@]}"
REPO=; BACKUP=; CHECK_ONLY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --thread) THREAD=${2:?}; shift 2 ;;
    --repo) REPO=${2:?}; shift 2 ;;
    --backup) BACKUP=${2:?}; shift 2 ;;
    --check) CHECK_ONLY=1; shift ;;
    *) echo "unknown argument: $1" >&2; exit 64 ;;
  esac
done
[ -n "$THREAD" ] && [ -n "$REPO" ] || { echo "usage: $0 --thread <thr_...> --repo <checkout> [--backup <path>] [--check]" >&2; exit 64; }
case $REPO in /*) ;; *) echo "--repo must be an absolute path" >&2; exit 64 ;; esac
case ${BACKUP:-/} in /*) ;; *) echo "--backup must be an absolute path" >&2; exit 64 ;; esac
common_setup home-restore
BUILD=${BUILD_SET:-/home/mk/.local/share/autarch-home-v2}   # a9853e2 worktree for the v2 build
V2_COMMIT=a9853e2
MOVED=()
reverse_steps() {
  say "To undo: $BB plugin disable autarch; then, for each file moved aside below, mv it back over $DATA/<name>; then enable the v3 build."
}
fail_after_move() { # <exit code> <message>
  say "FAILED: $2"
  say "Moved aside (never deleted): ${MOVED[*]:-nothing}"
  reverse_steps
  exit "$1"
}

# 0. Verify the install; failure exits 6 before anything is touched.
verify_install
say "verified install: $URL, runtime pid $PINNED_PID"

# 1. The backup: --backup, else the newest home-v2-backup-*.db; it must be inside $DATA.
if [ -z "$BACKUP" ]; then
  BACKUP=$(ls -1t "$DATA"/home-v2-backup-*.db 2>/dev/null | head -1 || true)
fi
[ -n "$BACKUP" ] && [ -f "$BACKUP" ] || { say "no backup found in $DATA"; exit 2; }
REAL_DATA=$(realpath -e "$DATA" 2>/dev/null) || { say "no data directory at $DATA"; exit 2; }
REAL_BACKUP=$(realpath -e "$BACKUP") || { say "backup unreadable: $BACKUP"; exit 2; }
case "$REAL_BACKUP" in "$REAL_DATA"/*) ;; *) say "backup is not inside $DATA: $REAL_BACKUP"; exit 2 ;; esac
BACKUP=$REAL_BACKUP

# 2. Verify it, as mk, read-only.
IC=$("${AS0[@]}" sqlite3 -readonly "$BACKUP" 'PRAGMA integrity_check' 2>&1 || true)
[ "$IC" = ok ] || { say "backup failed integrity_check: $(printf '%s' "$IC" | head -3 | tr '\n' ' ')"; exit 2; }
SV=$("${AS0[@]}" sqlite3 -readonly "$BACKUP" "select value from schema_meta where key='schema_version'" 2>&1 || true)
MR=$("${AS0[@]}" sqlite3 -readonly "$BACKUP" "select value from schema_meta where key='min_reader_version'" 2>&1 || true)
case "$SV" in 1|2) ;; *) say "backup schema_version is '$SV', not 1 or 2"; exit 2 ;; esac
case "${MR:-0}" in ""|0|1|2) ;; *) say "backup min_reader_version is '$MR', above 2"; exit 2 ;; esac
say "backup verified: $BACKUP (integrity ok, schema_version $SV, min_reader_version ${MR:-0})"
if [ "$CHECK_ONLY" = 1 ]; then say "--check: nothing moved"; exit 0; fi

# 3. Stop Home, then require that nothing holds the database.
"${AS[@]}" "$BB" plugin disable autarch
HOLDERS=$(db_holders)
if [ -n "$HOLDERS" ]; then
  say "Home still holds the DB; re-enabling the existing build, nothing moved. Holders:"
  say "$HOLDERS"
  say "(bb's plugin-state snapshot may hold it open: bead mk-schu.2)"
  "${AS[@]}" "$BB" plugin enable autarch || true
  exit 3
fi

# 4. Move aside, never delete.
TS=$(date -u +%Y%m%dT%H%M%SZ)
# data.db-journal: a v3 migration runs in journal_mode=DELETE, so a crash during the hold leaves a hot rollback journal
# that would be replayed into the restored pages by the next connection (plan 8.9 item 2).
for f in data.db data.db-wal data.db-shm data.db-journal; do
  if [ -e "$DATA/$f" ] || [ -L "$DATA/$f" ]; then
    "${AS0[@]}" mv "$DATA/$f" "$DATA/${f/data.db/data.db.v3-$TS}"
    MOVED+=("$DATA/${f/data.db/data.db.v3-$TS}")
  fi
done
# 5. Install the backup as mk.
"${AS0[@]}" cp --no-clobber "$BACKUP" "$DATA/data.db" || fail_after_move 4 "could not install the backup"
"${AS0[@]}" chmod 0600 "$DATA/data.db"

# 6. The v2 build, as mk.
PD=$BUILD/integrations/bb-plugin-autarch
if [ "$TEST" = 0 ]; then
  if [ ! -e "$BUILD" ]; then
    "${AS0[@]}" git -C "$REPO" worktree add "$BUILD" "$V2_COMMIT" || fail_after_move 4 "git worktree add failed"
  fi
  HEAD=$("${AS0[@]}" git -C "$BUILD" rev-parse HEAD)
  case "$HEAD" in "$V2_COMMIT"*) ;; *) fail_after_move 4 "worktree HEAD is $HEAD, not $V2_COMMIT" ;; esac
  "${AS[@]}" /usr/bin/env -C "$PD" npm ci || fail_after_move 4 "npm ci failed"
  "${AS[@]}" /usr/bin/env -C "$PD" "$BB" plugin build || fail_after_move 4 "bb plugin build failed"
else
  HEAD=$V2_COMMIT-test
fi
"${AS[@]}" "$BB" plugin install --yes "$PD" || fail_after_move 4 "bb plugin install failed"

# 7. Enable and wait for a healthy start. Health is read from `bb plugin list --json` (enabled and status running).
"${AS[@]}" "$BB" plugin enable autarch || fail_after_move 5 "bb plugin enable failed"
WAIT=60; [ "$TEST" = 1 ] && WAIT=3
STATUS=
healthy=0
for _ in $(seq 1 "$WAIT"); do
  plugin_state; STATUS=$PSTATE
  if [ "$PHEALTHY" = 1 ]; then healthy=1; break; fi
  sleep 1
done
LOGS=$("${AS[@]}" "$BB" plugin logs autarch 2>&1 | tail -5 || true)
OPEN_ASKS=$("${AS0[@]}" sqlite3 -readonly "$DATA/data.db" "select count(*) from decisions d where d.withdrawn_at is null and d.resolved_at is null and not exists (select 1 from picks k where k.decision_id = d.id) and not exists (select 1 from decisions r where r.supersedes = d.id)" 2>&1 || true)

# 8. Report.
say "restored backup: $BACKUP"
say "moved aside: ${MOVED[*]:-nothing}"
say "build commit: $HEAD"
say "plugin status: $STATUS"
say "bb.log tail: $LOGS"
say "open asks in the restored DB: $OPEN_ASKS"
[ "$healthy" = 1 ] || fail_after_move 5 "the plugin did not report a healthy start within ${WAIT}s"
