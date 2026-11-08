#!/bin/bash
# Home deploy gates, for the shared bb plugin updater (see integrations/bb-plugin-autarch/deploy.json).
# The gate logic is a copy of the gates in scripts/home-ui.sh (gate_ui_only, plugin_code_only, gate_collect, migrate-check).
# home-ui.sh keeps its own inline copy, and a parity test (__tests__/home-gates-parity.test.ts) requires the same verdicts.
# Read only: this script changes nothing, calls no bb, and opens the live database read-only. Nothing executes it yet.
#
#   scripts/home-gates.sh gate          --class plugin-deploy --installed DIR --target DIR --data BBDATA
#   scripts/home-gates.sh post-install  --data BBDATA [--plugin-dir DIR]
#   scripts/home-gates.sh check         --installed DIR --target DIR --data BBDATA [--plugin-code]
#   scripts/home-gates.sh migrate-check --path DIR --data BBDATA
#
# gate          the deploy.json pre-install check. Class plugin-deploy passes only when the live schema_version equals the
#               target's CODE_VERSION, migrations.ts is byte-identical to the installed build's, and either only ui/ and
#               __tests__/ differ or the change is plugin-code-only (no dependency change, no schema statement added).
#               Class additive-schema is refused (exit 10): it stays on scripts/install-home-your-move.sh.
# post-install  the deploy.json health check: max(migration_log.version) equals the build's CODE_VERSION. The build is the
#               checkout this script lives in unless --plugin-dir names another.
# check         home-ui.sh check, for two directories: lists every failed gate, exit 3 on any. Without --plugin-code only ui/
#               and __tests__/ may differ (the home-ui.sh default); with it, plugin code may differ too.
# migrate-check runs DIR's migrations on a COPY of the live database (a temp dir); the live database is never written.
#
# Check protocol (deploy.json): exit 0 = pass; 10 = refused (nothing to install under this class); 20 = needs mk (unused here);
# anything else = error, including unreadable inputs, which always fail closed. 64 = usage. gate and post-install print one JSON
# line per gate on stdout, {"gate":..,"ok":..,"reason":..}, and the last line summarises. Human text goes to stderr.
set -euo pipefail
export PATH=/usr/local/bin:/usr/bin:/bin
umask 077

die() { echo "home-gates: $1" >&2; exit "${2:-1}"; }

SUB=${1:-}; [ $# -gt 0 ] && shift || true
CLASS=; INSTALLED=; TARGET=; DATA=; PLUGIN_DIR=; PATH_ARG=; PLUGIN_CODE=0
case $SUB in
  gate|post-install|check|migrate-check) ;;
  *) die "usage: $0 gate|post-install|check|migrate-check (see the header)" 64 ;;
esac
while [ $# -gt 0 ]; do
  case $1 in
    --class) CLASS=${2:?--class needs a value}; shift 2 ;;
    --installed) INSTALLED=${2:?--installed needs a directory}; shift 2 ;;
    --target) TARGET=${2:?--target needs a directory}; shift 2 ;;
    --data) DATA=${2:?--data needs the BB data directory}; shift 2 ;;
    --plugin-dir) PLUGIN_DIR=${2:?--plugin-dir needs a directory}; shift 2 ;;
    --path) PATH_ARG=${2:?--path needs a directory}; shift 2 ;;
    --plugin-code) PLUGIN_CODE=1; shift ;;
    *) die "unknown argument: $1" 64 ;;
  esac
done
[ -n "$DATA" ] || die "$SUB needs --data BBDATA" 64
case $SUB in
  gate)          [ -n "$CLASS" ] && [ -n "$INSTALLED" ] && [ -n "$TARGET" ] || die "gate needs --class, --installed and --target" 64 ;;
  check)         [ -n "$INSTALLED" ] && [ -n "$TARGET" ] || die "check needs --installed and --target" 64 ;;
  migrate-check) [ -n "$PATH_ARG" ] || die "migrate-check needs --path DIR" 64 ;;
esac
[ "$PLUGIN_CODE" = 0 ] || [ "$SUB" = check ] || die "--plugin-code applies to check only" 64
DB=$DATA/plugins/autarch/data.db

# ---- copied from scripts/home-ui.sh (only the message prefix and the installed-build source differ) ----------------------------
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
ui_files_only() {   # $1 = installed build, $2 = target; every difference must be under ui/ or __tests__/
  local out
  out=$(diff -rq -x node_modules -x .git -x dist "$1" "$2" 2>&1 | grep -Ev "^(Files .*/(ui|__tests__)/.* differ|Only in [^:]*/(ui|__tests__)(/[^:]*)?: |Only in [^:]*: (ui|__tests__)$)" || true)
  [ -z "$out" ] || die "NOT UI-only: files outside ui/ and __tests__/ differ from the installed build:
$out
This goes to mk as a script (or a reviewed PR + deploy of a build that only touches ui/)." 3
}
plugin_code_only() {   # $1 = installed build, $2 = target; dependencies and the schema must not move (migrations.ts was compared by the caller)
  local f
  [ -f "$1/package.json" ] && [ -f "$2/package.json" ] || die "NOT plugin-code-only: package.json is missing from the installed build or the target" 3
  for f in package.json package-lock.json npm-shrinkwrap.json .npmrc pnpm-lock.yaml yarn.lock; do
    if [ -e "$1/$f" ] || [ -L "$1/$f" ] || [ -e "$2/$f" ] || [ -L "$2/$f" ]; then
      { [ -f "$1/$f" ] && [ ! -L "$1/$f" ] && [ -f "$2/$f" ] && [ ! -L "$2/$f" ] && cmp -s "$1/$f" "$2/$f"; } \
        || die "NOT plugin-code-only: $f differs from the installed build. A dependency change goes to mk as a script." 3
    fi
  done
  # Defence in depth, not proof: lines added to plugin code outside ui/ and __tests__/ must not contain schema statements. The
  # deploy's real guarantee on code behaviour is the cross-lab review of the merged head that the deploy approval requires.
  local f rel old rc list added=''
  # The file list goes through a temp file so a failed traversal (an unreadable subdirectory) is seen and refuses, instead of
  # silently dropping the files it could not list. This is stricter than home-ui.sh, which discards find's exit status.
  list=$(mktemp) || die "cannot create a temp file"
  find "$2" -path "$2/node_modules" -prune -o -type f -print0 >"$list" 2>/dev/null \
    || { rm -f "$list"; die "NOT plugin-code-only: could not list every file under the target (an unreadable directory?); refusing" 3; }
  while IFS= read -r -d '' f; do
    rel=${f#"$2"/}
    case $rel in ui/*|__tests__/*|node_modules/*) continue ;; esac
    old=$1/$rel; [ -f "$old" ] || old=/dev/null
    rc=0; added+=$(diff -a --unchanged-line-format= --old-line-format= --new-line-format='%L ' -- "$old" "$f") || rc=$?
    [ "$rc" -le 1 ] || { rm -f "$list"; die "NOT plugin-code-only: could not diff $rel against the installed build (diff exit $rc)" 3; }
  done <"$list"
  rm -f "$list"
  if printf '%s' "$added" | tr '\n\0' '  ' | grep -Eiq '(create|alter|drop)[[:space:]]+(virtual[[:space:]]+)?(table|index|trigger|view)|pragma[[:space:]]+(user_version|writable_schema)|schema_meta'; then
    die "NOT plugin-code-only: changed plugin code adds a schema statement (CREATE/ALTER/DROP, schema_meta). A schema change goes to mk as a script." 3
  fi
}

# ---- the gate_collect steps: every failed gate is reported, not just the first ----------------------------------------------------
ERR=$(mktemp); trap 'rm -f "$ERR"' EXIT
STEP_NAMES=(); STEP_RCS=(); STEP_TEXT=(); STEP_OUT=; STEP_RC=0
step() {   # $1 = gate name; the rest is a command run in a subshell, so its die ends only that step. Success text is its stdout.
  local name=$1 out msg rc=0; shift
  out=$( ("$@") 2>"$ERR" ) || rc=$?
  msg=$(cat "$ERR")
  if [ "$rc" = 0 ]; then msg=$out; else msg=${msg#home-gates: }; fi
  STEP_NAMES+=("$name"); STEP_RCS+=("$rc"); STEP_TEXT+=("$msg"); STEP_OUT=$out; STEP_RC=$rc
}
target_is_build() { { [ -f "$TARGET/package.json" ] && [ -f "$TARGET/migrations.ts" ]; } || die "$TARGET is not a Home plugin build (package.json or migrations.ts is missing)" 3; echo "package.json and migrations.ts present"; }
target_has_node_modules() { [ -d "$TARGET/node_modules" ] || die "$TARGET has no node_modules (run npm ci there as mk)" 3; echo "node_modules present"; }
installed_present() { [ -n "$INSTALLED" ] && [ -d "$INSTALLED" ] || die "cannot tell the installed Home build ('$INSTALLED' is not a directory); refusing"; echo "$INSTALLED"; }
schema_match() { [ "$1" = "$2" ] || die "NOT UI-only: the build is schema $2, the live database is schema $1. This is a migration; it goes to mk as a script." 3; echo "schema $1"; }
migrations_identical() { cmp -s "$INSTALLED/migrations.ts" "$TARGET/migrations.ts" || die "NOT UI-only: migrations.ts differs from the installed build ($INSTALLED). This goes to mk as a script." 3; echo "migrations.ts identical"; }
scope_check() {   # $1 = ui | plugin-code | either
  case $1 in
    ui) ui_files_only "$INSTALLED" "$TARGET"; echo "only ui/ and __tests__/ differ" ;;
    plugin-code) plugin_code_only "$INSTALLED" "$TARGET"; echo "plugin code may differ; package files and migrations.ts identical, no schema statement added" ;;
    either) if ( ui_files_only "$INSTALLED" "$TARGET" ) 2>/dev/null; then echo "only ui/ and __tests__/ differ"
            else plugin_code_only "$INSTALLED" "$TARGET"; echo "plugin code may differ; package files and migrations.ts identical, no schema statement added"; fi ;;
  esac
}
evaluate() {   # $1 = scope; fills the STEP_ arrays in gate_collect's order
  local live='' code=''
  step build target_is_build
  step node-modules target_has_node_modules
  step live-schema live_schema; [ "$STEP_RC" != 0 ] || live=$STEP_OUT
  step code-version code_version "$TARGET"; [ "$STEP_RC" != 0 ] || code=$STEP_OUT
  [ -z "$live" ] || [ -z "$code" ] || step schema-match schema_match "$live" "$code"
  step installed-build installed_present
  if [ "$STEP_RC" = 0 ]; then
    step migrations-identical migrations_identical
    step scope scope_check "$1"
  fi
  LIVE=$live
}

json_str() {   # JSON-escape $1 (no jq needed)
  local s=$1
  s=${s//\\/\\\\}; s=${s//\"/\\\"}; s=${s//$'\n'/\\n}; s=${s//$'\r'/\\r}; s=${s//$'\t'/\\t}
  printf '"%s"' "$(printf '%s' "$s" | tr -d '\000-\010\013\014\016-\037')"
}
json_line() { printf '{"gate":%s,"ok":%s,"reason":%s}\n' "$(json_str "$1")" "$2" "$(json_str "$3")"; }

failures() {   # prints the count of failed steps
  local i n=0; for i in "${!STEP_RCS[@]}"; do [ "${STEP_RCS[$i]}" = 0 ] || n=$((n+1)); done; echo "$n"
}
all_refusals() {   # true when every failed step is a refusal (die code 3), not an error
  local i; for i in "${!STEP_RCS[@]}"; do case ${STEP_RCS[$i]} in 0|3) ;; *) return 1 ;; esac; done
}

case $SUB in
check)
  scope=ui; [ "$PLUGIN_CODE" = 0 ] || scope=plugin-code
  evaluate "$scope"
  n=$(failures)
  if [ "$n" != 0 ]; then
    for i in "${!STEP_RCS[@]}"; do [ "${STEP_RCS[$i]}" = 0 ] || echo "home-gates: ${STEP_TEXT[$i]}" >&2; done
    echo "home-gates: check found $n failed gate(s)" >&2; exit 3
  fi
  if [ "$PLUGIN_CODE" = 1 ]; then
    echo "gate ok: schema $LIVE, migrations.ts identical, package.json, lockfiles and .npmrc identical; plugin code may differ from the installed build"
  else
    echo "gate ok: schema $LIVE, only ui/ and __tests__/ differ from the installed build"
  fi ;;

gate)
  case $CLASS in
    plugin-deploy) ;;
    additive-schema)
      json_line class false "class additive-schema is not a tool class yet; use the reviewed Home install script (scripts/install-home-your-move.sh)"
      echo "home-gates: class additive-schema is not handled here; use scripts/install-home-your-move.sh" >&2; exit 10 ;;
    *) die "unknown class '$CLASS'" 64 ;;
  esac
  evaluate either
  n=$(failures)
  for i in "${!STEP_RCS[@]}"; do
    reason=${STEP_TEXT[$i]}
    if [ "${STEP_RCS[$i]}" = 0 ]; then json_line "${STEP_NAMES[$i]}" true "$reason"; continue; fi
    echo "home-gates: $reason" >&2
    case ${STEP_NAMES[$i]} in
      schema-match|migrations-identical) reason+=" A schema change is not a plugin-deploy: use the reviewed Home install script (scripts/install-home-your-move.sh)." ;;
    esac
    json_line "${STEP_NAMES[$i]}" false "$reason"
  done
  if [ "$n" = 0 ]; then json_line plugin-deploy true "schema $LIVE, migrations.ts identical, scope ok"; exit 0; fi
  json_line plugin-deploy false "$n gate(s) failed"
  if all_refusals; then exit 10; else exit 1; fi ;;

post-install)
  PD=${PLUGIN_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)/integrations/bb-plugin-autarch}
  rc=0; code=$( (code_version "$PD") 2>"$ERR" ) || rc=$?
  if [ "$rc" != 0 ]; then json_line migration-level false "$(sed 's/^home-gates: //' "$ERR")"; exit 1; fi
  [ -r "$DB" ] || { json_line migration-level false "cannot read the Home database at $DB"; exit 1; }
  rc=0; max=$(sqlite3 -readonly "file:$DB?mode=ro" 'SELECT max(version) FROM migration_log' 2>"$ERR") || rc=$?
  if [ "$rc" != 0 ] || ! [[ $max =~ ^[0-9]+$ ]]; then json_line migration-level false "cannot read max(version) from migration_log (got '${max:-nothing}')"; exit 1; fi
  if [ "$max" != "$code" ]; then
    json_line migration-level false "migration_log max version is $max; the installed build expects $code"
    echo "home-gates: migration_log max version is $max; the installed build expects $code" >&2; exit 10
  fi
  json_line migration-level true "migration_log max version $max equals CODE_VERSION $code" ;;

migrate-check)
  D=$PATH_ARG
  [ -f "$D/migrations.ts" ] && [ -d "$D/node_modules" ] || die "$D is not a Home plugin build with node_modules"
  [ -r "$DB" ] || die "cannot read the Home database at $DB"
  T=$(mktemp -d); trap 'rm -rf "$T"; rm -f "$ERR"' EXIT
  sqlite3 -readonly "file:$DB?mode=ro" ".backup '$T/copy.db'" || die "copy of the live database failed; nothing was changed"
  [ "$(sqlite3 -readonly "$T/copy.db" 'PRAGMA integrity_check')" = ok ] || die "the copy fails its integrity check; nothing was changed"
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
  echo "migration ran on a copy of the live database; live was not touched" ;;
esac
