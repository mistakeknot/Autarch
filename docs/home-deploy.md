# Home deploy

`scripts/home-ui.sh deploy --commit <full SHA>` always installs both the Home
plugin and the autarch CLI from the verified merged commit. Use the default
legacy engine: tool-engine deploy refuses because its installed tool cannot
restore the CLI. Tool checks and previews retain their existing behavior.

Other plugin-only routes, including the update runner and direct calls to
`tools/bb-plugin-deploy`, do not move the CLI. Use `scripts/home-ui.sh deploy`
to keep the plugin and CLI in step.

Before backing up or changing either live component, deploy verifies the build
worktree's HEAD and completely clean Git status, builds `./cmd/autarch` with
`-buildvcs=false -ldflags "-X main.buildRevision=<verified HEAD>"`, and checks
that `version` reports the target revision with `modified=false`. Explicit
stamping avoids Go selecting a broken enclosing Git repository. Ordinary Go
builds continue to use their BuildInfo stamp.

The existing database/plugin backup also records the previous CLI path and
digest. CLI copies remain beside the installed binary as
`autarch.prev-<revision12>` (or its SHA-256 if unstamped or the name is already
occupied by different bytes). A temporary file beside the live executable is
renamed atomically over it. Deploy restarts `autarch-serve.service`, verifies
its active state, a changed MainPID and a procfs executable resolving to the
installed path without a deleted suffix, and unauthenticated loopback `/health`. It checks the installed
revision, and runs `needs-mk budget --project Autarch` and parses its JSON.
An absent unit is explicitly skipped; failure to query systemd is an error.

A failure after the swap automatically restores both CLI and plugin, restarts
and checks the restored service, exits nonzero, and prints the rollback command.
An incomplete restoration explicitly warns that the components may differ.
Once both components pass verification, the command announces that it is
ignoring INT/TERM. The stamp and outcome are written with that protection,
inherited by children. INT/TERM stay ignored for the rest of the run except a
brief window just before exit, after stamping, when INT can end the command
with 130 and TERM with 143. Signals received while ignored are discarded.
`rollback BACKUP_DIR` restores both components; old backups without CLI records
leave the CLI alone and say so. Data restoration remains opt-in.

Before every CLI rollback, the command copies the current filer state to
`filings-<timestamp>-<unique suffix>.json` in that deploy's backup directory and
prints its path. This happens before restoring or restarting the old binary.
The copy holds the filer's existing exclusive `filing.lock` through verification.
Lock acquisition waits at most 30 seconds (`HOME_UI_FILER_LOCK_WAIT` overrides
this bound for tests); a timeout refuses CLI rollback while plugin rollback
still proceeds and reports both outcomes with the rollback command.
Keep filers quiet during rollback to avoid new entries after that lock is released.
If the copy or its verification fails, CLI rollback is refused; plugin rollback
still proceeds, and the command warns that the components may differ. If no
filer state exists, the command reports that there is nothing to preserve.

An older CLI may reinstate filing limits. On its next save it can also drop
`held_revisions`, `replaced_by` and `owner_attention` from `filings.json`, which
could bring held or replaced cards back. The snapshot protects those fields;
rollback does not migrate or restore filer state automatically.

To recover the fields:

1. Pause all card-filing, release and sweep callers. Run as the account that
   owns filer state, with the same state-directory environment as those callers.
2. Redeploy a newer build that understands these fields through
   `scripts/home-ui.sh deploy --commit <newer full SHA>`.
3. Compare the printed saved copy with the current `filings.json`. If the old
   binary wrote new entries or changed existing ones, merge by hand into a
   separate JSON file: retain the new entries and reconcile each existing
   filing's status and revision history, restoring the three fields from the
   saved copy where appropriate. Blindly replacing state would lose intervening
   filings or updates. Use that merged file as `saved_copy` below. If there
   were no intervening writes, use the saved snapshot directly.
4. Set `saved_copy` to the full path of that snapshot or merged file and run
   this atomic restore, which takes the filer's own lock and preserves the
   current file as another backup. Resume callers only after verifying the
   restored file with the newer CLI.

```bash
saved_copy=/path/to/backup/filings-TIMESTAMP-SUFFIX.json
state_dir=${AUTARCH_CARD_STATE_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/autarch/needsmk}
(
  set -eu
  flock -x 9
  jq -e 'type == "object"' "$saved_copy" >/dev/null
  if [ -f "$state_dir/filings.json" ]; then
    before=$(mktemp "$state_dir/filings-before-recovery-XXXXXX.json")
    cp -- "$state_dir/filings.json" "$before"
    printf 'Current state saved: %s\n' "$before"
  fi
  staged=$(mktemp "$state_dir/.filings-restore-XXXXXX")
  trap 'rm -f "$staged"' EXIT
  cp -- "$saved_copy" "$staged"
  chmod 600 "$staged"
  mv -f -- "$staged" "$state_dir/filings.json"
) 9>>"$state_dir/filing.lock"
```

`check` reports the installed CLI revision and target. For an existing target
worktree it builds and verifies the CLI without installing it; otherwise it
reports the stamp as unknown until deploy creates the worktree. Neither check
nor build validation restarts services or installs a binary.

Service commands and procfs reads can be redirected for isolated fixtures with
`HOME_UI_SYSTEMCTL` (executable name or path) and `HOME_UI_PROC_ROOT`.
