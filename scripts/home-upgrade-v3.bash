# home-upgrade-v3.bash: body, started only by the home-upgrade-v3.sh launcher.
# Upgrades Home (the autarch bb plugin) to the v3 build: disable, verify no holder, install, enable.
# Run on the host only from the root-owned copy installed by the generated home-v3-run-<sha12>.sh package (scripts/home-build-root-package.sh); never run a checkout copy with sudo.
# Arguments: --thread <thr_...> --plugin <v3 build dir> [--check: verify the install and plugin dir, change nothing]
# Tested in test mode by integrations/bb-plugin-autarch/__tests__/restore-script.test.ts (Task 2.8a); the sudo
# launch itself is unprobed (bead mk-schu.4): mk's dry run covers it.
set -euo pipefail
. "${BASH_SOURCE[0]%/*}/home-common.bash"
parse_common "$@"
set -- "${REST[@]}"
PLUGIN=; CHECK_ONLY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --thread) THREAD=${2:?}; shift 2 ;;
    --plugin) PLUGIN=${2:?}; shift 2 ;;
    --check) CHECK_ONLY=1; shift ;;
    *) echo "unknown argument: $1" >&2; exit 64 ;;
  esac
done
[ -n "$THREAD" ] && [ -n "$PLUGIN" ] || { echo "usage: $0 --thread <thr_...> --plugin <dir> [test-mode flags]" >&2; exit 64; }
case $PLUGIN in /*) ;; *) echo "--plugin must be an absolute path" >&2; exit 64 ;; esac
common_setup home-upgrade
verify_install
[ -f "$PLUGIN/package.json" ] || { say "no plugin build at $PLUGIN"; exit 4; }
if [ "$CHECK_ONLY" = 1 ]; then say "--check: install verified, plugin build present; nothing changed"; exit 0; fi

# 1. Stop Home, then require that nothing holds the database.
"${AS[@]}" "$BB" plugin disable autarch
HOLDERS=$(db_holders)
if [ -n "$HOLDERS" ]; then
  say "Home still holds the DB; re-enabling the existing build. Holders:"
  say "$HOLDERS"
  say "(bb's plugin-state snapshot may hold it open: bead mk-schu.2)"
  "${AS[@]}" "$BB" plugin enable autarch || true
  exit 3
fi
# 2. Install the v3 build and enable it; the open migrates (quiesce, backup, verify, DDL).
"${AS[@]}" "$BB" plugin install --yes "$PLUGIN" || { say "install failed; plugin left disabled on the unchanged v2 DB"; exit 4; }
"${AS[@]}" "$BB" plugin enable autarch || { say "enable failed; plugin left disabled"; exit 5; }
# 3. Report, and require success evidence: a healthy plugin status AND a v3 migration_log row (read as mk).
# Enable can return before activation fails, so poll briefly; no evidence means failure (plan 1.3.9).
if [ "$TEST" = 1 ]; then TRIES=2; WAIT=0; else TRIES=12; WAIT=5; fi
STATUS=; MIGROW=; HEALTHY=0
for _ in $(seq "$TRIES"); do
  sleep "$WAIT"
  plugin_state; STATUS=$PSTATE
  MIGROW=$("${AS0[@]}" sqlite3 -readonly "$DATA/data.db" "select version, at, backup_path, digest from migration_log where version >= 3 order by version desc limit 1" 2>&1 || true)
  HEALTHY=$PHEALTHY
  [ "$HEALTHY" = 1 ] && printf '%s' "$MIGROW" | grep -Eq '^[0-9]+\|' && break
done
say "plugin status: $STATUS"
say "migration_log: ${MIGROW:-no v3 row}"
# The refusal markers are fixed tokens the plugin writes (backup.ts REFUSED_QUIESCE/REFUSED_BACKUP, logged by store.ts and
# carried in the thrown error bb reports); the plain-text messages and the retry line are matched too, so a build that
# predates the tokens, or a bb that rewraps the message, is still caught. The LAST matching line decides: an old
# refusal followed by a migrated line is a success.
PATTERN='autarch: schema [0-9]+ →|home-refused:|another connection holds data\.db|pre-migration backup not verified|store not ready, retrying'
LOGLINES=$("${AS[@]}" "$BB" plugin logs autarch 2>&1 | grep -E "$PATTERN" | tail -3 || true)
LOGLINE=$(printf '%s\n' "$LOGLINES" | tail -1)
say "bb.log: ${LOGLINES:-no migration line found}"
case "$LOGLINE" in
  *home-refused:*|*"another connection holds data.db"*|*"pre-migration backup not verified"*|*"store not ready, retrying"*)
    "${AS[@]}" "$BB" plugin disable autarch || true
    say "migration refused; the plugin is left disabled on the unchanged v2 DB. Recovery: run the restore from the root-owned copy, $(dirname "$(readlink -f "$0")")/home-restore-v2.sh --thread $THREAD --repo <Autarch checkout> [--backup <path>] (the backup path is in migration_log and bb.log), or retry this upgrade after fixing the cause."
    exit 5 ;;
esac
if [ "$HEALTHY" != 1 ]; then
  "${AS[@]}" "$BB" plugin disable autarch || true
  say "plugin never became healthy after enable; the plugin is left disabled on the unchanged v2 DB. Recovery: run the restore from the root-owned copy, $(dirname "$(readlink -f "$0")")/home-restore-v2.sh --thread $THREAD --repo <Autarch checkout> [--backup <path>] (the backup path is in migration_log and bb.log), or retry this upgrade after fixing the cause."
  exit 5
fi
if ! printf '%s' "$MIGROW" | grep -Eq '^[0-9]+\|'; then
  "${AS[@]}" "$BB" plugin disable autarch || true
  say "no v3 migration_log row after enable; the plugin is left disabled on the unchanged v2 DB. Recovery: run the restore from the root-owned copy, $(dirname "$(readlink -f "$0")")/home-restore-v2.sh --thread $THREAD --repo <Autarch checkout> [--backup <path>] (the backup path is in migration_log and bb.log), or retry this upgrade after fixing the cause."
  exit 5
fi
