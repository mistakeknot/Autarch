#!/bin/bash
# CLI half of home-ui.sh's locked deploy transaction. Source only; no live action on load.
CLI_LIVE=$MKHOME/.local/bin/autarch
CLI_STAGE=
CLI_SWAPPED=0
CLI_SYSTEMCTL=${HOME_UI_SYSTEMCTL:-systemctl}
CLI_PROC_ROOT=${HOME_UI_PROC_ROOT:-/proc}
CLI_BEFORE_PID=

cli_revision() {
  "$1" version 2>/dev/null | sed -n 's/^revision \([0-9a-f]\{40\}\) modified=.*/\1/p'
}

cli_verify() {
  local out
  [ -x "$1" ] || return 1
  out=$("$1" version) || return 1
  [[ $out == "revision $2 modified=false sha256 "* ]]
}

cli_prepare() { # build outside the worktree so the output cannot dirty its VCS stamp
  local root=$1 rev=$2
  local head status
  head=$(git -C "$root" rev-parse HEAD) || return 1
  [ "$head" = "$rev" ] || { echo "home-ui: CLI worktree revision differs from --commit; refusing" >&2; return 1; }
  status=$(git -C "$root" status --porcelain) || {
    echo "home-ui: cannot obtain worktree VCS status; nothing installed" >&2; return 1;
  }
  [ -z "$status" ] || { echo "home-ui: CLI worktree is dirty; refusing modified=false stamp" >&2; return 1; }
  CLI_STAGE=$(mktemp) || return 1
  # Verify Git directly: Go may choose a broken enclosing repository for its automatic stamp.
  if ! (cd "$root" && go build -buildvcs=false -ldflags "-X main.buildRevision=$head" -o "$CLI_STAGE" ./cmd/autarch); then
    echo "home-ui: CLI build failed; check the Go toolchain and build output; nothing installed" >&2
    rm -f "$CLI_STAGE"; CLI_STAGE=; return 1
  fi
  if ! cli_verify "$CLI_STAGE" "$rev"; then
    echo "home-ui: CLI build is unstamped, modified or has the wrong revision (expected $rev); nothing installed" >&2
    rm -f "$CLI_STAGE"; CLI_STAGE=; return 1
  fi
  echo "home-ui: CLI build stamps revision $rev modified=false"
}

cli_backup() {
  local bk=$1 rev sum prev
  if [ -e "$CLI_LIVE" ] || [ -L "$CLI_LIVE" ]; then
    [ -f "$CLI_LIVE" ] || return 1
    rev=$(cli_revision "$CLI_LIVE") || rev=
    sum=$(sha256sum "$CLI_LIVE") || return 1; sum=${sum%% *}
    if [[ $rev =~ ^[0-9a-f]{40}$ ]]; then prev=$CLI_LIVE.prev-${rev:0:12}; else prev=$CLI_LIVE.prev-$sum; fi
    # A prior deploy with the same revision may have different bytes; never overwrite its rollback copy.
    if [ -e "$prev" ] && ! cmp -s "$CLI_LIVE" "$prev"; then prev=$CLI_LIVE.prev-$sum; fi
    if [ ! -e "$prev" ]; then cp -p "$CLI_LIVE" "$prev" || return 1; fi
    cmp -s "$CLI_LIVE" "$prev" || return 1
    printf '%s\n' "$prev" >"$bk/previous-cli.txt" || return 1
    printf '%s\n' "$sum" >"$bk/previous-cli.sha256" || return 1
  else
    printf '\n' >"$bk/previous-cli.txt" || return 1
  fi
}

cli_atomic_copy() {
  local tmp
  mkdir -p "${CLI_LIVE%/*}" || return 1
  tmp=$(mktemp "${CLI_LIVE%/*}/.autarch.XXXXXX") || return 1
  if ! cp "$1" "$tmp" || ! chmod 755 "$tmp" || ! mv -f "$tmp" "$CLI_LIVE"; then
    rm -f "$tmp"; return 1
  fi
}

cli_capture_pid() {
  local state
  state=$("$CLI_SYSTEMCTL" --user show autarch-serve.service --property=LoadState --value) || return 1
  CLI_BEFORE_PID=
  [ "$state" != not-found ] || return 0
  [ "$state" = loaded ] || return 1
  CLI_BEFORE_PID=$("$CLI_SYSTEMCTL" --user show autarch-serve.service -p MainPID --value) || return 1
  [[ $CLI_BEFORE_PID =~ ^[0-9]+$ ]]
}

cli_unit() {
  local state
  state=$("$CLI_SYSTEMCTL" --user show autarch-serve.service --property=LoadState --value) || return 1
  if [ "$state" = not-found ]; then
    echo "home-ui: autarch-serve.service unit does not exist; skipping restart and health check"
    return 0
  fi
  [ "$state" = loaded ] || return 1
  "$CLI_SYSTEMCTL" --user restart autarch-serve.service || return 1
  local attempt pid exe installed
  installed=$(readlink -e "$CLI_LIVE") || return 1
  for attempt in $(seq 1 "${HOME_UI_WAIT:-30}"); do
    pid=$("$CLI_SYSTEMCTL" --user show autarch-serve.service -p MainPID --value) || return 1
    exe=
    if [[ $pid =~ ^[1-9][0-9]*$ ]]; then exe=$(readlink "$CLI_PROC_ROOT/$pid/exe" 2>/dev/null) || exe=; fi
    if [[ $pid =~ ^[1-9][0-9]*$ && $pid != "$CLI_BEFORE_PID" && $exe != *' (deleted)' ]] &&
      [ "$(readlink -e "$CLI_PROC_ROOT/$pid/exe" 2>/dev/null)" = "$installed" ] &&
      "$CLI_SYSTEMCTL" --user is-active --quiet autarch-serve.service &&
      curl --noproxy '*' -fsS --max-time 2 http://127.0.0.1:8110/health | jq -e 'type == "object" and .status == "ok"' >/dev/null; then return 0; fi
    [ "$attempt" = "${HOME_UI_WAIT:-30}" ] || sleep 1
  done
  echo "home-ui: serve restart verification failed (PID, executable, active state or health): before=$CLI_BEFORE_PID current=$pid executable=${exe:-unreadable}" >&2
  return 1
}

cli_postcheck() {
  cli_unit || return 1
  cli_verify "$CLI_LIVE" "$COMMIT" || return 1
  # budget always emits JSON; generic check also supports commits before card limits were removed.
  local out
  out=$("$CLI_LIVE" needs-mk budget --project Autarch) || return 1
  printf '%s\n' "$out" | jq -e 'type == "object"' >/dev/null || return 1
  return 0
}

cli_restore() {
  local bk=$1 prev sum
  [ -f "$bk/previous-cli.txt" ] || { echo "home-ui: old backup has no CLI record; CLI unchanged"; return 0; }
  prev=$(cat "$bk/previous-cli.txt") || return 1
  if [ -z "$prev" ]; then
    rm -f "$CLI_LIVE" || return 1
    echo "home-ui: previous CLI was absent; removed installed CLI; service cannot be restored" >&2
    return 1
  fi
  sum=$(sha256sum "$prev") || return 1
  [ "${sum%% *}" = "$(cat "$bk/previous-cli.sha256")" ] || return 1
  # A failed service query must never prevent putting the old executable back.
  cli_capture_pid || { echo "home-ui: cannot capture serve PID during rollback; restoring CLI anyway" >&2; CLI_BEFORE_PID=unknown; }
  cli_atomic_copy "$prev" || return 1
  cmp -s "$prev" "$CLI_LIVE" || return 1
  "$CLI_LIVE" version || echo "home-ui: restored CLI has no working version command; backup digest was verified" >&2
  cli_unit
}

cli_deploy_exit() {
  local rc=$? failed=0
  trap - EXIT INT TERM
  if [ "$CLI_SWAPPED" = 1 ]; then
    echo "home-ui: deploy failed after CLI swap; restoring previous CLI and plugin" >&2
    cli_restore "$BK" || failed=1
    install_build "$(cat "$BK/previous-source.txt")" || failed=1
    [ "$failed" = 0 ] || echo "home-ui: automatic rollback incomplete; plugin and CLI may differ; run rollback" >&2
    rollback_cmd "$BK"
    rc=5
  fi
  [ -z "$CLI_STAGE" ] || rm -f "$CLI_STAGE"
  exit "$rc"
}
