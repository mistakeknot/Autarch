# Home deploy

`scripts/home-ui.sh deploy --commit <full SHA>` always installs both the Home
plugin and the autarch CLI from the verified merged commit. Use the default
legacy engine: tool-engine deploy refuses because its installed tool cannot
restore the CLI. Tool checks and previews retain their existing behavior.

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
`rollback BACKUP_DIR` restores both components; old backups without CLI records
leave the CLI alone and say so. Data restoration remains opt-in.

`check` reports the installed CLI revision and target. For an existing target
worktree it builds and verifies the CLI without installing it; otherwise it
reports the stamp as unknown until deploy creates the worktree. Neither check
nor build validation restarts services or installs a binary.

Service commands and procfs reads can be redirected for isolated fixtures with
`HOME_UI_SYSTEMCTL` (executable name or path) and `HOME_UI_PROC_ROOT`.
