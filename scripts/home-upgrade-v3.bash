# home-upgrade-v3.bash: body, started only by the home-upgrade-v3.sh launcher.
# Upgrades Home (the autarch bb plugin) to the v3 build: disable, verify no holder, install, enable.
# Run on zklw: sudo scripts/home-upgrade-v3.sh --thread <thr_...> --plugin <v3 build dir>
# Tested in test mode by integrations/bb-plugin-autarch/__tests__/restore-script.test.ts (Task 2.8a); the sudo
# launch itself is unprobed (bead mk-schu.4): mk's dry run covers it.
set -euo pipefail
. "${BASH_SOURCE[0]%/*}/home-common.bash"
parse_common "$@"
set -- "${REST[@]}"
PLUGIN=
while [ $# -gt 0 ]; do
  case "$1" in
    --thread) THREAD=${2:?}; shift 2 ;;
    --plugin) PLUGIN=${2:?}; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 64 ;;
  esac
done
[ -n "$THREAD" ] && [ -n "$PLUGIN" ] || { echo "usage: $0 --thread <thr_...> --plugin <dir> [test-mode flags]" >&2; exit 64; }
common_setup home-upgrade
verify_install
[ -f "$PLUGIN/package.json" ] || { say "no plugin build at $PLUGIN"; exit 4; }

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
"${AS[@]}" "$BB" plugin install "$PLUGIN" || { say "install failed; plugin left disabled on the unchanged v2 DB"; exit 4; }
"${AS[@]}" "$BB" plugin enable autarch || { say "enable failed; plugin left disabled"; exit 5; }
# 3. Report the backup path and the bb.log line.
if [ "$TEST" = 1 ]; then sleep 0; else sleep 5; fi
say "plugin status: $("${AS[@]}" "$BB" plugin status autarch 2>&1 | head -5)"
say "migration_log: $("${AS0[@]}" sqlite3 -readonly "$DATA/data.db" "select version, at, backup_path, digest from migration_log order by rowid desc limit 1" 2>&1 || true)"
LOGLINE=$("${AS[@]}" "$BB" plugin logs autarch 2>&1 | grep -E 'autarch: schema 2|QuiesceRequired|BackupNotVerified' | tail -3 || true)
say "bb.log: ${LOGLINE:-no migration line found}"
case "$LOGLINE" in
  *QuiesceRequired*|*BackupNotVerified*)
    "${AS[@]}" "$BB" plugin disable autarch || true
    say "migration refused; plugin left disabled on the unchanged v2 DB. Re-enable the old build."
    exit 5 ;;
esac
