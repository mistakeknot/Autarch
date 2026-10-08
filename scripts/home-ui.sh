#!/bin/bash
# Home UI deploy and live preview, for changes that do not touch the database schema.
#   scripts/home-ui.sh check    [--commit SHA | --path DIR]   gates only; changes nothing
#   scripts/home-ui.sh deploy   --commit <40-hex sha>         backup, install that merged-main commit, verify, print the rollback
#   scripts/home-ui.sh preview  [--path DIR]                  backup, run DIR (default: this checkout) as Home with hot reload on the
#                                                             live data; Ctrl+C restores the previous build
#   scripts/home-ui.sh rollback BACKUP_DIR [--restore-data]   reinstall the build the backup recorded (data restore only if asked)
#   scripts/home-ui.sh migrate-check [--path DIR]             run DIR's migrations on a COPY of a fresh backup; never touches live
# Run as mk (refused as root). Not for schema changes: those still go to mk as scripts (install-home-your-move.sh).
#
# Why this is safe to run by an agent:
#   * Gate 1 (UI-only): the target's CODE_VERSION must equal the live schema_version, and its migrations.ts must be byte-identical
#     to the installed build's. Anything else is refused, before any backup or change. A missing or unreadable input fails closed.
#   * deploy takes only a commit that is an ancestor of origin/main (after a fetch), builds it in a fresh worktree (npm ci).
#   * A backup (sqlite3 .backup, integrity-checked, 0700) is taken before every deploy and at the start of every preview session.
#   * Writes made in preview go through the plugin's own code path on the live database, exactly as Home does: picks, notes and
#     Other answers reach the owning threads the same way. Nothing here writes the database itself.
# Env overrides (tests, other installs): HOME_BB_BIN, HOME_BB_DATA, HOME_UI_BUILDS (where deploy builds go).
set -euo pipefail
export PATH=/usr/local/bin:/usr/bin:/bin
umask 077

die() { echo "home-ui: $*" >&2; exit "${2:-1}"; }
[ "$(id -u)" != 0 ] || die "run as mk, not root" 64

MKHOME=${HOME:?}
BB=${HOME_BB_BIN:-$MKHOME/.local/bin/bb}
if [ -z "${HOME_BB_DATA:-}" ]; then   # discover the one BB data directory that holds the Home database
  mapfile -t found < <(find "$MKHOME" -maxdepth 4 -path '*/plugins/autarch/data.db' -type f 2>/dev/null)
  [ ${#found[@]} -eq 1 ] || die "cannot pick one Home database (found ${#found[@]}); set HOME_BB_DATA to the BB data directory" 64
  HOME_BB_DATA=${found[0]%/plugins/autarch/data.db}
fi
BBDATA=$HOME_BB_DATA
DB=$BBDATA/plugins/autarch/data.db
BUILDS=${HOME_UI_BUILDS:-$MKHOME/bb-coordinator-evidence/autarch}
REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
PLUGIN_REL=integrations/bb-plugin-autarch
STAMP=$(date -u +%Y%m%dT%H%M%SZ)

SUB=${1:-}; [ $# -gt 0 ] && shift || true
COMMIT=; PATH_ARG=; RESTORE_DATA=0; BACKUP_ARG=
case $SUB in
  rollback) BACKUP_ARG=${1:?rollback needs the backup directory}; shift ;;
  check|deploy|preview|migrate-check) ;;
  *) die "usage: $0 check|deploy|preview|rollback|migrate-check (see the header)" 64 ;;
esac
while [ $# -gt 0 ]; do
  case $1 in
    --commit) COMMIT=${2:?--commit needs a sha}; shift 2 ;;
    --path) PATH_ARG=${2:?--path needs a directory}; shift 2 ;;
    --restore-data) RESTORE_DATA=1; shift ;;
    *) die "unknown argument: $1" 64 ;;
  esac
done
[[ -z $COMMIT || $COMMIT =~ ^[0-9a-f]{40}$ ]] || die "--commit must be a full 40-hex sha" 64
[ "$SUB" != deploy ] || [ -n "$COMMIT" ] || die "deploy needs --commit <40-hex sha>" 64

live_schema() {
  [ -r "$DB" ] || die "cannot read the Home database at $DB"
  local v
  v=$(sqlite3 -readonly "file:$DB?mode=ro" "SELECT value FROM schema_meta WHERE key='schema_version'") || die "cannot read schema_version"
  [[ $v =~ ^[0-9]+$ ]] || die "schema_version is not a number: '$v'"
  echo "$v"
}
code_version() {   # $1 = plugin dir
  local v
  v=$(sed -n 's/^export const CODE_VERSION = \([0-9][0-9]*\);.*/\1/p' "$1/migrations.ts" 2>/dev/null | head -n1)
  [[ $v =~ ^[0-9]+$ ]] || die "no CODE_VERSION in $1/migrations.ts"
  echo "$v"
}
installed_dir() {
  local s
  s=$("$BB" plugin source autarch 2>/dev/null | sed -n 's/^ *resolved: path://p' | head -n1)
  [ -n "$s" ] && [ -d "$s" ] || die "cannot tell the installed Home build (bb plugin source autarch); refusing"
  echo "$s"
}
gate_ui_only() {   # $1 = plugin dir to run
  local new=$1 live code cur
  [ -f "$new/package.json" ] && [ -f "$new/migrations.ts" ] || die "$new is not a Home plugin build"
  [ -d "$new/node_modules" ] || die "$new has no node_modules (run npm ci there as mk)"
  live=$(live_schema); code=$(code_version "$new")
  [ "$live" = "$code" ] || die "NOT UI-only: the build is schema $code, the live database is schema $live. This is a migration; it goes to mk as a script." 3
  cur=$(installed_dir)
  cmp -s "$cur/migrations.ts" "$new/migrations.ts" || die "NOT UI-only: migrations.ts differs from the installed build ($cur). This goes to mk as a script." 3
  ui_files_only "$cur" "$new"
  echo "gate ok: schema $live, only ui/ and __tests__/ differ from the installed build"
}
ui_files_only() {   # $1 = installed build, $2 = target; every difference must be under ui/ or __tests__/
  local out
  out=$(diff -rq -x node_modules -x .git -x dist "$1" "$2" 2>&1 | grep -Ev "^(Files .*/(ui|__tests__)/.* differ|Only in [^:]*/(ui|__tests__)(/[^:]*)?: |Only in [^:]*: (ui|__tests__)$)" || true)
  [ -z "$out" ] || die "NOT UI-only: files outside ui/ and __tests__/ differ from the installed build:
$out
This goes to mk as a script (or a reviewed PR + deploy of a build that only touches ui/)." 3
}
backup() {   # prints the backup directory
  local dir=$BBDATA/home-backups/$STAMP prev
  prev=$(installed_dir)
  mkdir -p -m 700 "$BBDATA/home-backups" && mkdir -m 700 "$dir" || die "cannot create $dir"
  sqlite3 -readonly "file:$DB?mode=ro" ".backup '$dir/data.db'" || die "backup failed; nothing was changed"
  [ "$(sqlite3 -readonly "$dir/data.db" 'PRAGMA integrity_check')" = ok ] || die "backup failed its integrity check; nothing was changed"
  [ "$(sqlite3 -readonly "$dir/data.db" "SELECT value FROM schema_meta WHERE key='schema_version'")" = "$(live_schema)" ] || die "backup schema differs from live; nothing was changed"
  printf '%s\n' "$prev" >"$dir/previous-source.txt"
  echo "$dir"
}
plugin_running() {
  "$BB" plugin list --json 2>/dev/null | jq -e '[.plugins[]? | select(.id == "autarch")] | length == 1 and (.[0].enabled == true) and (.[0].status == "running")' >/dev/null 2>&1
}
wait_running() {
  local i; for i in $(seq 1 "${HOME_UI_WAIT:-30}"); do plugin_running && return 0; sleep 1; done; return 1
}
install_build() {   # $1 = plugin dir
  "$BB" plugin disable autarch || die "bb plugin disable failed; nothing else was changed"
  if ! "$BB" plugin install --yes "$1"; then
    echo "home-ui: install of $1 failed; Home is disabled. Restore with the rollback command." >&2; return 5
  fi
  "$BB" plugin enable autarch || return 5
  wait_running || { echo "home-ui: Home did not come back running" >&2; return 6; }
}
rollback_cmd() { printf 'rollback: bash %q rollback %q\n' "$REPO/scripts/home-ui.sh" "$1"; }

case $SUB in
check)
  if [ -n "$COMMIT" ]; then
    git -C "$REPO" fetch -q origin main || die "fetch failed"
    git -C "$REPO" merge-base --is-ancestor "$COMMIT" origin/main || die "$COMMIT is not on origin/main"
    echo "commit is on origin/main; the schema gate runs after the build exists (deploy)"
  else
    gate_ui_only "${PATH_ARG:-$REPO/$PLUGIN_REL}"
  fi ;;

deploy)
  git -C "$REPO" fetch -q origin main || die "fetch failed"
  git -C "$REPO" merge-base --is-ancestor "$COMMIT" origin/main || die "$COMMIT is not on origin/main; deploy takes only merged commits"
  B=$BUILDS/build-${COMMIT:0:12}
  [ -d "$B" ] || git -C "$REPO" worktree add -q --detach "$B" "$COMMIT" || die "worktree add failed"
  ( cd "$B/$PLUGIN_REL" && npm ci --no-audit --no-fund ) || die "npm ci failed in $B; nothing was changed"
  [ "$(git -C "$B" rev-parse HEAD)" = "$COMMIT" ] || die "$B is not at $COMMIT"
  [ -z "$(git -C "$B" status --porcelain --untracked-files=no)" ] || die "$B has modified tracked files"
  gate_ui_only "$B/$PLUGIN_REL"
  BK=$(backup); echo "backup: $BK"
  echo "$(rollback_cmd "$BK")"
  if ! install_build "$B/$PLUGIN_REL"; then
    echo "home-ui: deploy failed; reinstalling the previous build" >&2
    install_build "$(cat "$BK/previous-source.txt")" || echo "home-ui: automatic rollback failed; run the rollback command above" >&2
    exit 5
  fi
  echo "deployed $COMMIT; Home is running"
  echo "$(rollback_cmd "$BK")" ;;

preview)
  D=${PATH_ARG:-$REPO/$PLUGIN_REL}
  gate_ui_only "$D"
  BK=$(backup); echo "backup: $BK"
  PREV=$(cat "$BK/previous-source.txt")
  RC=0
  restore() {
    local rc=$?; trap - EXIT INT TERM
    echo "home-ui: restoring the previous Home build $PREV"
    if ! install_build "$PREV"; then echo "home-ui: restore failed; run: $(rollback_cmd "$BK")" >&2; rc=7; fi
    if ! ui_files_only "$PREV" "$D" 2>/dev/null; then echo "home-ui: WARNING: the preview build now differs from the installed build outside ui/ and __tests__/; review before merging" >&2; fi
    exit "$rc"
  }
  trap restore EXIT; trap 'exit 130' INT TERM
  install_build "$D" || exit 5
  echo "Preview is live: Home now runs $D with hot reload, on the live data. mk sees it in the Home tab of Aleph; there is nothing to share."
  echo "Ctrl+C or stopping this command restores $PREV. If this process is killed hard: $(rollback_cmd "$BK")"
  "$BB" plugin dev "$D" || RC=$?
  exit "$RC" ;;

rollback)
  [ -d "$BACKUP_ARG" ] && [ -f "$BACKUP_ARG/previous-source.txt" ] && [ -f "$BACKUP_ARG/data.db" ] || die "$BACKUP_ARG is not a home-ui backup"
  PREV=$(cat "$BACKUP_ARG/previous-source.txt"); [ -d "$PREV" ] || die "previous build $PREV is gone"
  if [ "$RESTORE_DATA" = 1 ]; then
    [ "$(sqlite3 -readonly "$BACKUP_ARG/data.db" 'PRAGMA integrity_check')" = ok ] || die "backup fails its integrity check; nothing was changed"
    command -v fuser >/dev/null || die "fuser is needed to prove nothing holds the database; nothing was changed"
    "$BB" plugin disable autarch || die "bb plugin disable failed"
    sleep 1
    if fuser "$DB" "$DB-wal" >/dev/null 2>&1; then die "something still holds $DB open; Home is disabled, data NOT restored. Retry, or stop the holder"; fi
    sqlite3 "$BACKUP_ARG/data.db" ".backup '$DB.restore'" || die "data restore failed; Home stays disabled"
    rm -f "$DB-wal" "$DB-shm"; mv -f "$DB.restore" "$DB" || die "data restore failed at the final move; Home stays disabled"
    echo "data restored from $BACKUP_ARG (everything written after it is lost)"
  fi
  install_build "$PREV" || exit 5
  echo "rolled back to $PREV" ;;

migrate-check)
  D=${PATH_ARG:-$REPO/$PLUGIN_REL}
  [ -f "$D/migrations.ts" ] && [ -d "$D/node_modules" ] || die "$D is not a Home plugin build with node_modules"
  BK=$(backup); T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
  cp "$BK/data.db" "$T/copy.db"
  ( cd "$D" && npx --no-install tsx -e '
    import Database from "better-sqlite3";
    import { migrate, readSchemaState } from "./migrations.ts";
    const db = new Database(process.argv[1]);
    const before = readSchemaState(db);
    migrate(db);
    const ic = db.pragma("integrity_check", { simple: true });
    console.log(JSON.stringify({ before, after: readSchemaState(db), integrity: ic }));
    process.exit(ic === "ok" ? 0 : 1);
  ' "$T/copy.db" ) || die "migration failed on the COPY (live untouched)"
  echo "migration ran on a copy of $BK; live was not touched" ;;
esac
