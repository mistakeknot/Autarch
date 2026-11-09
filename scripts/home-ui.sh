#!/bin/bash
# Home UI deploy and live preview, for changes that do not touch the database schema.
#   scripts/home-ui.sh check    [--commit SHA | --path DIR] [--plugin-code]   gates only; changes nothing
#   scripts/home-ui.sh deploy   --commit <40-hex sha> [--plugin-code]
#                                                             backup, install that merged-main commit, verify, print the rollback.
#                                                             --plugin-code also allows plugin code changes outside ui/ (not package.json,
#                                                             package-lock.json or migrations.ts, and never a schema change)
#   scripts/home-ui.sh preview  [--path DIR]                  backup, run DIR (default: this checkout) as Home with hot reload on the
#                                                             live data; Ctrl+C restores the previous build
#   scripts/home-ui.sh rollback BACKUP_DIR [--restore-data]   reinstall the build the backup recorded (data restore only if asked)
#   scripts/home-ui.sh migrate-check [--path DIR]             run DIR's migrations on a COPY of a fresh backup; never touches live
# Run as mk (refused as root). Not for schema changes: those still go to mk as scripts (install-home-your-move.sh).
# --engine tool: begin (mk-zq345.7)
# --engine legacy|tool (check, deploy and preview only; default legacy, which is everything below). --engine tool hands the work to the
# deploy tool of the INSTALLED build (tools/bb-plugin-deploy), after verifying that build's control closure, and fails closed with
# "use --engine legacy" otherwise. It needs --commit (deploy and check; preview takes --commit or --path) and takes --expect-tree TREE.
# It refuses --plugin-code (the tool gates by class) and --restore-data. rollback and migrate-check never call the tool.
# --engine tool: end
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
  mapfile -t found < <(find "$MKHOME" -maxdepth 6 -path '*/plugins/autarch/data.db' -type f 2>/dev/null)
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
COMMIT=; PATH_ARG=; RESTORE_DATA=0; BACKUP_ARG=; PLUGIN_CODE=0
# --engine tool: begin (mk-zq345.7)
ENGINE=legacy; ENGINE_SET=; EXPECT_TREE=
# --engine tool: end
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
    --plugin-code) PLUGIN_CODE=1; shift ;;
    # --engine tool: begin (mk-zq345.7)
    --engine) ENGINE=${2:?--engine needs tool or legacy}; ENGINE_SET=1; shift 2 ;;
    --expect-tree) EXPECT_TREE=${2:?--expect-tree needs a tree sha}; shift 2 ;;
    # --engine tool: end
    *) die "unknown argument: $1" 64 ;;
  esac
done
[[ -z $COMMIT || $COMMIT =~ ^[0-9a-f]{40}$ ]] || die "--commit must be a full 40-hex sha" 64
[ "$PLUGIN_CODE" = 0 ] || [ "$SUB" = deploy ] || [ "$SUB" = check ] || die "--plugin-code applies to check and deploy only" 64
[ "$SUB" != deploy ] || [ -n "$COMMIT" ] || die "deploy needs --commit <40-hex sha>" 64
# --engine tool: begin (mk-zq345.7)
# --engine: validated before anything is read or locked. Without the flag none of this changes the legacy path.
case $ENGINE in tool|legacy) ;; *) die "--engine must be tool or legacy, not '$ENGINE'" 64 ;; esac
[ -z "$ENGINE_SET" ] || case $SUB in check|deploy|preview) ;; *) die "--engine applies to check, deploy and preview only; $SUB never calls the tool" 64 ;; esac
[ -z "$EXPECT_TREE" ] || [ "$ENGINE" = tool ] || die "--expect-tree applies to --engine tool only" 64
if [ "$ENGINE" = tool ]; then
  [ "$PLUGIN_CODE" = 0 ] || die "--plugin-code is a legacy gate option; --engine tool gates by class (use --engine legacy for it)" 64
  [ "$RESTORE_DATA" = 0 ] || die "--restore-data belongs to rollback, which never calls the tool" 64
  [[ -z $EXPECT_TREE || $EXPECT_TREE =~ ^[0-9a-f]{40}$ ]] || die "--expect-tree must be a full 40-hex tree sha" 64
  case $SUB in
    check|deploy) [ -n "$COMMIT" ] || die "$SUB --engine tool needs --commit <40-hex sha>" 64
                  [ -z "$PATH_ARG" ] || die "$SUB --engine tool takes --commit, not --path" 64 ;;
    preview)      { [ -n "$COMMIT" ] || [ -n "$PATH_ARG" ]; } && { [ -z "$COMMIT" ] || [ -z "$PATH_ARG" ]; } || die "preview --engine tool needs exactly one of --commit and --path" 64 ;;
  esac
fi
# --engine tool: end

# shared destination lock: begin (mk-zq345.5)
# deploy, preview and rollback take <state dir>/lock, the file the update runner already locks, before they read anything about the
# installed build, and hold it to the end of the process. A caller that already holds it (the runner's fd 9, or BPD_LOCK_FD) is
# recognised by descriptor, never by environment alone: the fd must be open, name the same device:inode as the lock file, and pass
# flock -n. Otherwise this opens its own descriptor and waits, at most HOME_UI_LOCK_WAIT seconds (default 2100, 35 minutes).
# HOME_UI_LOCK=skip lets rollback run without the lock (mk only, emergency).
lock_holder() {   # $1 = lock file; prints the pid /proc/locks names, or nothing
  local ino; ino=$(stat -c %i "$1" 2>/dev/null) || return 0
  awk -v i="$ino" '$2 != "->" { n = split($6, a, ":"); if (a[n] == i) { print $5; exit } }' /proc/locks 2>/dev/null || true
}
take_lock() {   # $1 = the subcommand
  local st=${HOME_UI_STATE_DIR:-$MKHOME/.local/state/autarch-home-update} l f a b w=${HOME_UI_LOCK_WAIT:-2100}
  l=$st/lock
  if [ "${HOME_UI_LOCK:-}" = skip ]; then
    if [ "$1" = rollback ]; then
      echo "home-ui: HOME_UI_LOCK=skip: running WITHOUT the destination lock. RACE RISK: the update runner or a tool deploy can install a build between this command's steps. Emergency use only." >&2
      return 0
    fi
    echo "home-ui: HOME_UI_LOCK=skip applies to rollback only; taking the lock" >&2
  fi
  [ -d "$st" ] || { echo "home-ui: WARNING: no state dir $st; running without the destination lock" >&2; return 0; }
  f=${BPD_LOCK_FD:-9}; case $f in ''|*[!0-9]*) f=9 ;; esac
  a=$(stat -L -c %d:%i "/proc/self/fd/$f" 2>/dev/null || true); b=$(stat -c %d:%i "$l" 2>/dev/null || true)
  if [ -z "$a" ] || [ "$a" != "$b" ]; then exec {f}>>"$l" || die "cannot open the destination lock $l"; fi
  if ! flock -n "$f"; then
    echo "home-ui: waiting for the destination lock (held by pid $(lock_holder "$l")); giving up after ${w}s" >&2
    flock -w "$w" "$f" || die "another deploy holds the lock (pid $(lock_holder "$l"))" 75
  fi
  export BPD_LOCK_FD="$f"   # children reuse this descriptor (2.1: a holder exports it and keeps it open)
}
case $SUB in deploy|preview|rollback) take_lock "$SUB" ;; esac
# shared destination lock: end
# --engine tool: begin (mk-zq345.7)
[ "$ENGINE" != tool ] || [ "$SUB" != check ] || take_lock "$SUB"   # check takes the lock on the tool path only; the legacy check takes none
# --engine tool: end
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
  if [ "$PLUGIN_CODE" = 1 ]; then
    plugin_code_only "$cur" "$new"
    echo "gate ok: schema $live, migrations.ts identical, package.json, lockfiles and .npmrc identical; plugin code may differ from the installed build"
  else
    ui_files_only "$cur" "$new"
    echo "gate ok: schema $live, only ui/ and __tests__/ differ from the installed build"
  fi
}
# check only: run every gate and report all failures before exiting 3, where gate_ui_only (deploy, preview) stops at the first.
gate_collect() {   # $1 = plugin dir to run
  local new=$1 n=0 live= code= cur= out err
  err=$(mktemp)
  bad() { echo "home-ui: $*" >&2; n=$((n+1)); }
  tryv() {   # $1 = variable to fill with stdout; the rest is a command run in a subshell, so its die only ends that step
    local var=$1; shift
    if out=$( ("$@") 2>"$err" ); then printf -v "$var" '%s' "$out"; else cat "$err" >&2; n=$((n+1)); printf -v "$var" '%s' ''; return 1; fi
  }
  { [ -f "$new/package.json" ] && [ -f "$new/migrations.ts" ]; } || bad "$new is not a Home plugin build (package.json or migrations.ts is missing)"
  [ -d "$new/node_modules" ] || bad "$new has no node_modules (run npm ci there as mk)"
  tryv live live_schema || true
  tryv code code_version "$new" || true
  [ -z "$live" ] || [ -z "$code" ] || [ "$live" = "$code" ] || bad "NOT UI-only: the build is schema $code, the live database is schema $live. This is a migration; it goes to mk as a script."
  if tryv cur installed_dir; then
    cmp -s "$cur/migrations.ts" "$new/migrations.ts" || bad "NOT UI-only: migrations.ts differs from the installed build ($cur). This goes to mk as a script."
    if [ "$PLUGIN_CODE" = 1 ]; then tryv out plugin_code_only "$cur" "$new" || true; else tryv out ui_files_only "$cur" "$new" || true; fi
  fi
  rm -f "$err"
  [ "$n" -eq 0 ] || { echo "home-ui: check found $n failed gate(s); deploy would stop at the first" >&2; exit 3; }
  if [ "$PLUGIN_CODE" = 1 ]; then
    echo "gate ok: schema $live, migrations.ts identical, package.json, lockfiles and .npmrc identical; plugin code may differ from the installed build"
  else
    echo "gate ok: schema $live, only ui/ and __tests__/ differ from the installed build"
  fi
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
  local f rel old rc added=
  while IFS= read -r -d '' f; do
    rel=${f#"$2"/}
    case $rel in ui/*|__tests__/*|node_modules/*) continue ;; esac
    old=$1/$rel; [ -f "$old" ] || old=/dev/null
    rc=0; added+=$(diff -a --unchanged-line-format= --old-line-format= --new-line-format='%L ' -- "$old" "$f") || rc=$?
    [ "$rc" -le 1 ] || die "NOT plugin-code-only: could not diff $rel against the installed build (diff exit $rc)" 3
  done < <(find "$2" -path "$2/node_modules" -prune -o -type f -print0)
  if printf '%s' "$added" | tr '\n\0' '  ' | grep -Eiq '(create|alter|drop)[[:space:]]+(virtual[[:space:]]+)?(table|index|trigger|view)|pragma[[:space:]]+(user_version|writable_schema)|schema_meta'; then
    die "NOT plugin-code-only: changed plugin code adds a schema statement (CREATE/ALTER/DROP, schema_meta). A schema change goes to mk as a script." 3
  fi
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

# --engine tool: begin (mk-zq345.7)
# The INSTALLED build's own deploy tool does the work, never the target's. Before it runs, this verifies the installed build's control
# closure the way the tool does (plan 2.2): every tracked file under the plugin file's controlPaths (plus the tool and the plugin file
# themselves) must exist on disk byte-for-byte as in the build's commit, with no symlink and no extra file. The commit is the stamp's
# (cross-checked against HEAD and its tree) or, for an unstamped build, HEAD with a clean status over the closure. The control paths come
# from the plugin file as COMMITTED, never from the disk copy. Any failure is a refusal (exit 10) that says to use --engine legacy.
# Git reads here ignore replace refs (GIT_NO_REPLACE_OBJECTS), so the baseline is the real commit. The destination lock must be held on a
# descriptor that names <state dir>/lock; an inherited BPD_LOCK_FD alone is not proof. Boundary: the installed build is not copied; the
# tool is exec'd from it right after this check, and re-verifies the same closure in-process (plan 2.2). A same-user writer that swaps and
# restores a closure file in that gap is outside this check; the lock stops the update runner and every other deploy, not a hostile editor.
tdie() { die "--engine tool refused: $*; use --engine legacy (or the reviewed install script)" 10; }
GLOBRE_JQ='def globre:
  gsub("(?<c>[.+^${}()|\\[\\]\\\\])"; "\\" + .c) | gsub("\\*\\*/"; "\u0001") | gsub("\\*\\*"; "\u0002") | gsub("\\*"; "[^/]*") | gsub("\\?"; "[^/]")
  | gsub("\u0001"; "(?:.*/)?") | gsub("\u0002"; ".*") | "^" + . + "$";'
CLOSURE_JQ="$GLOBRE_JQ"'
($ctrl | map(globre)) as $res
| def m($p): any($res[]; . as $r | $p | test($r));
($tree | split("\n") | map(select(length > 0) | capture("^(?<mode>[0-7]+) (?<type>[a-z]+) (?<sha>[0-9a-f]+)\t(?<path>.*)$"))) as $T
| ($disk | split("\n") | map(select(length > 0) | capture("^(?<t>.)\t(?<path>.*)$"))) as $D
| [$T[] | select(m(.path))] as $want
| (($want | map(select(.mode == "120000")) | first) // null) as $wl
| if $wl != null then "FAIL \($wl.path) is a symlink in the commit"
  elif ($want | length) == 0 then "FAIL the closure is empty"
  else
    ([$want[].path | . as $p | split("/") | range(1; length) as $i | .[:$i] | join("/")] | unique) as $anc
    | ($anc | map({key: ., value: true}) | from_entries) as $ancset
    | ($D | map({key: .path, value: .t}) | from_entries) as $dt
    | (($D | map(select(.t == "l" and (m(.path) or ($ancset[.path] // false) or m(.path + "/x")))) | first) // null) as $sl
    | (($anc | map(select(($dt[.] // "") != "d")) | first) // null) as $am
    | (($D | map(select(.t != "d" and .t != "l" and m(.path)) | .path) - ($want | map(.path))) | first // null) as $ex
    | (($want | map(select(($dt[.path] // "") != "f") | .path) | first) // null) as $mi
    | if $sl != null then "FAIL \($sl.path) is a symlink"
      elif $am != null then "FAIL \($am) is missing or not a directory"
      elif $ex != null then "FAIL extra file \($ex)"
      elif $mi != null then "FAIL \($mi) is missing or not a regular file"
      else ("OK", ($want[] | "\(.sha)\t\(.path)")) end
  end'
CLOSURE_REASON=
verify_closure() {   # $1 = build root; sets CLOSURE_REASON and returns 1 on any failure; prints nothing
  local root=$1 T ref head tree sc st n out bad
  T=$(mktemp -d) || { CLOSURE_REASON="cannot make a temp dir"; return 1; }
  local g=(env GIT_NO_REPLACE_OBJECTS=1 GIT_OPTIONAL_LOCKS=0 GIT_TERMINAL_PROMPT=0 git -C "$root")
  head=$("${g[@]}" rev-parse HEAD 2>/dev/null) && [[ $head =~ ^[0-9a-f]{40}$ ]] || { CLOSURE_REASON="the installed build is not a git checkout"; rm -rf "$T"; return 1; }
  tree=$("${g[@]}" rev-parse 'HEAD^{tree}' 2>/dev/null) || tree=
  sc=$root/.bb-plugin-deploy/stamp.json
  if [ -e "$sc" ] || [ -L "$sc" ]; then
    if [ -L "$sc" ] || [ ! -f "$sc" ]; then CLOSURE_REASON="the build stamp is not a regular file"; rm -rf "$T"; return 1; fi
    st=$(jq -r 'select(type == "object") | "\(.commit // "") \(.tree // "")"' "$sc" 2>/dev/null) || st=
    if [ "${st%% *}" != "$head" ] || [ "${st#* }" != "$tree" ] || [ -z "$tree" ]; then CLOSURE_REASON="the build stamp disagrees with the build's HEAD or tree"; rm -rf "$T"; return 1; fi
  fi
  ref=$head
  "${g[@]}" cat-file blob "$ref:$PLUGIN_REL/deploy.json" >"$T/pf.json" 2>/dev/null || { CLOSURE_REASON="the plugin file is not tracked at commit ${ref:0:12}"; rm -rf "$T"; return 1; }
  if ! jq -e --arg pf "$PLUGIN_REL/deploy.json" '(.controlPaths | type == "array" and length > 0 and all(type == "string" and length > 0 and (startswith("/") | not) and (contains("..") | not))) and ((.controlPaths + ["tools/bb-plugin-deploy/**", $pf]) | unique | length > 0)' "$T/pf.json" >/dev/null 2>&1; then
    CLOSURE_REASON="the committed plugin file has no valid controlPaths"; rm -rf "$T"; return 1
  fi
  jq -c --arg pf "$PLUGIN_REL/deploy.json" '(.controlPaths + ["tools/bb-plugin-deploy/**", $pf]) | unique' "$T/pf.json" >"$T/ctrl.json"
  "${g[@]}" ls-tree -r -z --full-tree "$ref" >"$T/tree.z" 2>/dev/null || { CLOSURE_REASON="cannot list commit ${ref:0:12}"; rm -rf "$T"; return 1; }
  n=$(tr -cd '\0' <"$T/tree.z" | wc -c); tr '\0' '\n' <"$T/tree.z" >"$T/tree.txt"
  [ "$(wc -l <"$T/tree.txt")" -eq "$n" ] || { CLOSURE_REASON="a tracked path contains a newline"; rm -rf "$T"; return 1; }
  ( cd "$root" && find . -mindepth 1 \( -path ./.git -o -path ./.bb-plugin-deploy -o -name node_modules \) -prune -o -printf '%y\t%P\0' ) >"$T/disk.z" 2>/dev/null || { CLOSURE_REASON="cannot walk the installed build"; rm -rf "$T"; return 1; }
  n=$(tr -cd '\0' <"$T/disk.z" | wc -c); tr '\0' '\n' <"$T/disk.z" >"$T/disk.txt"
  [ "$(wc -l <"$T/disk.txt")" -eq "$n" ] || { CLOSURE_REASON="a path in the installed build contains a newline"; rm -rf "$T"; return 1; }
  out=$(jq -rn --slurpfile ctrl "$T/ctrl.json" --rawfile tree "$T/tree.txt" --rawfile disk "$T/disk.txt" '$ctrl[0] as $ctrl | '"$CLOSURE_JQ" 2>"$T/jq.err") || { CLOSURE_REASON="closure check failed to run: $(head -c 200 "$T/jq.err")"; rm -rf "$T"; return 1; }
  if [ "${out%%$'\n'*}" != OK ]; then CLOSURE_REASON=${out#FAIL }; CLOSURE_REASON=${CLOSURE_REASON%%$'\n'*}; rm -rf "$T"; return 1; fi
  printf '%s\n' "${out#OK}" | sed '1d' >"$T/want.txt"
  cut -f1 "$T/want.txt" >"$T/want.sha"; cut -f2- "$T/want.txt" >"$T/want.paths"
  ( cd "$root" && env GIT_NO_REPLACE_OBJECTS=1 git hash-object --no-filters --stdin-paths <"$T/want.paths" ) >"$T/got.sha" 2>"$T/hash.err" || { CLOSURE_REASON="git hash-object failed: $(head -c 200 "$T/hash.err")"; rm -rf "$T"; return 1; }
  if ! cmp -s "$T/want.sha" "$T/got.sha"; then
    bad=$(paste -d' ' "$T/want.sha" "$T/got.sha" "$T/want.paths" | awk '$1 != $2 { sub(/^[^ ]+ [^ ]+ /, ""); print; exit }')
    CLOSURE_REASON="${bad:-a closure file} differs from commit ${ref:0:12}"; rm -rf "$T"; return 1
  fi
  if [ ! -e "$sc" ]; then   # unstamped: the closure paths must also be clean in git's own status (catches mode and index changes)
    "${g[@]}" status --porcelain --untracked-files=all -z >"$T/status.z" 2>/dev/null || { CLOSURE_REASON="git status failed in the installed build"; rm -rf "$T"; return 1; }
    tr '\0' '\n' <"$T/status.z" | sed -n 's/^...//p' >"$T/dirty.all"
    if [ -s "$T/dirty.all" ]; then
      bad=$(jq -Rrn --slurpfile ctrl "$T/ctrl.json" --rawfile p "$T/dirty.all" '$ctrl[0] as $ctrl | '"$GLOBRE_JQ"' ($ctrl | map(globre)) as $res | $p | split("\n") | map(select(length > 0) | . as $q | select(any($res[]; . as $r | $q | test($r)))) | first // empty')
      if [ -n "$bad" ]; then CLOSURE_REASON="uncommitted change in the closure: $bad"; rm -rf "$T"; return 1; fi
    fi
  fi
  rm -rf "$T"
}

lock_proved() {   # $1 = lock file, $2 = descriptor: the descriptor names that file and the lock is held (a fresh open cannot take it)
  local a b
  a=$(stat -L -c %d:%i "/proc/self/fd/$2" 2>/dev/null || true); b=$(stat -c %d:%i "$1" 2>/dev/null || true)
  [ -n "$a" ] && [ "$a" = "$b" ] || return 1
  ( flock -n 8 ) 8>>"$1" 2>/dev/null && return 1
  return 0
}

tool_engine() {   # check, deploy or preview through the installed build's tool; never returns on success (exec)
  local src root cli pf st args=() bbenv=() lf
  st=${HOME_UI_STATE_DIR:-$MKHOME/.local/state/autarch-home-update}
  [ -d "$st" ] || tdie "no state dir $st, so no destination lock could be taken"
  lf=${BPD_LOCK_FD:-}; case $lf in ''|*[!0-9]*) tdie "the destination lock was not taken (BPD_LOCK_FD is not a descriptor)" ;; esac
  lock_proved "$st/lock" "$lf" || tdie "the destination lock is not held on descriptor $lf"
  src=$("$BB" plugin source autarch 2>/dev/null | sed -n 's/^ *resolved: path://p' | head -n1) || src=
  [ -n "$src" ] && [ -d "$src" ] || tdie "cannot tell the installed Home build (bb plugin source autarch)"
  src=$(cd "$src" && pwd -P)
  [[ $src == */"$PLUGIN_REL" ]] || tdie "the installed source $src does not end in $PLUGIN_REL"
  root=${src%/"$PLUGIN_REL"}
  [ -n "$root" ] && [ -d "$root" ] || tdie "cannot find the installed build root"
  cli=$root/tools/bb-plugin-deploy/cli.mjs
  { [ -f "$cli" ] && [ ! -L "$cli" ]; } || tdie "the installed build has no deploy tool ($cli)"
  pf=$root/$PLUGIN_REL/deploy.json
  { [ -f "$pf" ] && [ ! -L "$pf" ]; } || tdie "the installed build's plugin file is missing ($pf)"
  jq -e '.format == "bb-plugin-deploy/plugin@1" and (.pluginId | type == "string" and length > 0) and (.controlPaths | type == "array" and length > 0)' "$pf" >/dev/null 2>&1 || tdie "the installed build's plugin file is invalid ($pf)"
  command -v node >/dev/null 2>&1 || tdie "node is not on the PATH"
  command -v git >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 || tdie "git and jq are needed to verify the installed tool"
  verify_closure "$root" || tdie "installed deploy tooling does not match its commit ($CLOSURE_REASON)"
  args=("$cli" "$SUB" --plugin-file "$pf" --bb-data "$BBDATA" --state-dir "$st")
  case $SUB in
    check)   args+=(--commit "$COMMIT" --class plugin-deploy) ;;
    deploy)  args+=(--commit "$COMMIT" --class plugin-deploy --mode agent) ;;
    preview) if [ -n "$COMMIT" ]; then args+=(--commit "$COMMIT"); else args+=(--path "$PATH_ARG"); fi; args+=(--mode agent) ;;
  esac
  [ -z "$EXPECT_TREE" ] || args+=(--expect-tree "$EXPECT_TREE")
  case $BB in /*) bbenv=(BB_PLUGIN_DEPLOY_BB="$BB") ;; esac   # the tool talks to the same bb that told us which build is installed
  echo "home-ui: --engine tool: $SUB via the installed tool $cli (closure verified, lock held)" >&2
  exec env "${bbenv[@]}" node "${args[@]}"
}
[ "$ENGINE" != tool ] || tool_engine
# --engine tool: end
case $SUB in
check)
  if [ -n "$COMMIT" ]; then
    git -C "$REPO" fetch -q origin main || die "fetch failed"
    git -C "$REPO" merge-base --is-ancestor "$COMMIT" origin/main || die "$COMMIT is not on origin/main"
    echo "commit is on origin/main; the schema gate runs after the build exists (deploy)"
  else
    gate_collect "${PATH_ARG:-$REPO/$PLUGIN_REL}"
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
  # installed.sha write: begin (mk-zq345.5)
  # Record the build now installed where the update runner reads it, atomically, only when the state dir exists and the previous
  # build is a checkout of this plugin; anything else only warns, and the rollback still succeeds. Reads nothing from the tool.
  ST=${HOME_UI_STATE_DIR:-$MKHOME/.local/state/autarch-home-update}
  if [ -d "$ST" ]; then
    TOP=$(git -C "$PREV" rev-parse --show-toplevel 2>/dev/null || true)
    SHA=$(git -C "$PREV" rev-parse HEAD 2>/dev/null || true)
    if [ -n "$TOP" ] && [ "$(cd "$PREV" && pwd -P)" = "$TOP/$PLUGIN_REL" ] && [[ $SHA =~ ^[0-9a-f]{40}$ ]] && printf '%s\n' "$SHA" >"$ST/installed.sha.tmp.$$"; then
      mv -f "$ST/installed.sha.tmp.$$" "$ST/installed.sha" || echo "home-ui: WARNING: could not write $ST/installed.sha" >&2
    else
      rm -f "$ST/installed.sha.tmp.$$"
      echo "home-ui: WARNING: $PREV is not a checkout of the plugin; installed.sha was not updated" >&2
    fi
  else
    echo "home-ui: WARNING: no state dir $ST; installed.sha was not updated" >&2
  fi
  # installed.sha write: end
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
