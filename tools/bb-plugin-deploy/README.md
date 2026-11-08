# bb-plugin-deploy

One deploy tool for bb plugins. It builds a plugin from a git commit into an
immutable, stamped build directory, backs up the plugin's data, installs the
build through `bb plugin`, waits for the plugin to be healthy, records what it
did, and can roll back. Everything that differs between plugins lives in the
plugin's own `deploy.json` ("plugin file"); the tool itself holds no plugin
specifics.

**Status: this is the tool core only. Nothing calls it yet.** No install
wrapper, timer or root tool uses it, and the plugin folder is unchanged apart
from this tool's tests. Wiring is separate work.

Tracked as bead mk-zq345.3.

## Requirements

- Node 22 or newer. ESM `.mjs` files, no npm dependencies of its own.
- External programs, always started with argument arrays (never a shell):
  `git`, `bb`, `sqlite3`, `flock`, and `npm` (only if the plugin file's build
  steps use it). Children get a fixed `PATH` and a minimal environment.

## Commands

```
cli.mjs <command> --plugin-file PATH [--bb-data DIR] [--state-dir DIR] [--host-file PATH] [--json]
```

| Command | What it does |
| --- | --- |
| `version` | Prints the protocol, node version and the tool's own commit. Needs no plugin file. |
| `validate-plugin-file` | Checks the plugin file against the `bb-plugin-deploy/plugin@1` schema. |
| `record [--reconcile] [--verify-closure]` | Shows the live install (stamped, plain git checkout, or unknown). `--reconcile` writes what it observed into `record.json`, repairs a stale `installed.sha`, and republishes `status.json` (a failure to repair `installed.sha` exits 1). `--verify-closure` checks the control-path closure of the live build. |
| `status [--write]` | Shows the live install and the last result; `--write` publishes `status.json`. |
| `check --commit SHA --class C` | Runs every check that `deploy` would run before it changes anything. |
| `prepare --commit SHA --class C` | Clones, verifies and builds into a build directory. Installs nothing. |
| `deploy --commit SHA --class C --mode agent\|runner\|mk-script` | The full transaction (below). |
| `rollback [--to previous\|--build DIR]` | Reinstalls the previous (or a named) verified build. |
| `restore-data --backup DIR [--check]` | Restores a plugin's sqlite data from a deploy backup and reinstalls the build that was live then. Without `--check` it needs `--mode mk-script`. |
| `preview --commit SHA \| --path DIR` | Installs a commit (or an already built directory) for a dev session, then restores the anchored build. Follows the same trust rule as `deploy` (`--mode`, `--confirmed-first-run`), runs the plugin's `previewGate` first, takes a data backup, and records a `preview` transaction in `record.json` while it runs. It adds no history event and leaves `installed` and `installed.sha` as they were. |
| `update-tick` | Reads the plugin's update request (a click in the plugin's settings), and deploys it in runner mode if it is new and has review evidence. Always publishes status. |
| `gc [--apply]` | Lists, or with `--apply` removes, old builds beyond the plugin's retention. The installed and previous builds are never removed. |

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Done. |
| 1 | Error before anything was changed. |
| 2 | Usage error. |
| 10 | Refused by a rule or a gate. Nothing was changed. |
| 20 | Needs mk (a control path changed, or first-run evidence is missing). Nothing was changed. |
| 30 | The deploy failed and the previous build was restored and is healthy. |
| 31 | The deploy failed and the restore also failed. Use the printed last-resort recovery block. |
| 75 | Another deploy holds the lock. |

## The deploy transaction

Ordered phases, each recorded in `record.json` with a transaction state, so a
crash can be understood afterwards:

1. Take the lock (below), resolve the live install, anchor the previous build.
2. Clone the commit and verify it (branch ancestry, tree hash if given,
   control-path closure).
3. Trust rule: a change under a control path pauses for mk (exit 20); a first
   run needs a recorded decision; runner mode needs review evidence for the
   commit. The control diff and the pause happen right after the clone and
   before any build command runs, and a paused deploy leaves no build
   directory behind.
4. Plugin-defined checks and `preflight` gates; build; stamp the build
   directory with its commit and file digests.
5. Back up the plugin's declared data (sqlite `.backup` plus an integrity
   check) and export what the plugin file lists.
6. `preInstall` gates, `bb plugin disable`, `install --yes`, `preEnable`
   gates, `enable`, `postEnablePreConfig` gates, configuration, then health
   checks (kinds: `running`, `log-line`, `process-count`, `check`,
   `setting-equals`). A health baseline (`log-line` with `since` of `install`,
   `enable` or `config`) is taken immediately before the command it belongs
   to, so a line that command itself writes is seen. For `config` that command
   is the last `configAfterEnable` key's `config set` (or the end of
   `postEnablePreConfig` when there are no keys), so a line written while an
   earlier key was set does not count.
7. On success write `installed.sha`, `record.json`, then `status.json` last.
   If any of those writes fails the deploy counts as failed and is rolled
   back. A rollback (the command) that has already changed what is live tries
   each bookkeeping write on its own; if one fails it exits 1, says bb is
   running the target, and names the `record --reconcile` command that
   repairs the files. On failure reinstall the anchored previous build, verify it is
   healthy, and report 30 or 31.

While the live plugin is being changed, `record.json` carries a `transaction`
with its state (`mutating`), the current phase, the anchored `previous` build
and the backup directory. A successful deploy and a successful rollback clear
it; a rollback whose restore also fails leaves it as `rollback-unhealthy`. If
the tool is killed mid-deploy, the next `record --reconcile` keeps the
anchored build as `previous` (unless it is what is live now) and notes the
interrupted phase in the history.

Gate commands are the plugin's own scripts. They receive `BPD_PHASE_CHECK` (the gate's name) and one `BPD_MARKER_<NAME>` variable per marker, and may create or require named markers (`requiresMarker` must
name a declared marker; a failing check clears its marker).

## Lock

`<stateDir>/lock` is taken before the installed build is resolved and held to
the end.

- Wait is capped at 2100 s, then the tool stops with "another deploy holds the
  lock" (exit 75).
- Inheritance is by file descriptor. The candidate is `BPD_LOCK_FD` if set
  (digits only, greater than 2), otherwise fd 9. It counts only if that fd is
  open on the same device and inode as the lock file, and `flock -n` on it
  succeeds. Otherwise the tool opens its own fd and waits with `flock -w`. The
  environment variable alone is never trusted.
- The held fd is passed to children at the same fd number, with `BPD_LOCK_FD`
  set, so a wrapper that already holds the lock can run the tool inside it.
- `BPD_LOCK_WAIT_SEC` (digits only) is a test hook; it can only lower the cap.

## Last-resort recovery

Every successful or rolled-back deploy prints (in text mode under
"Last-resort recovery") a self-contained `bash -s`
block that restores the anchored build using only `bb` and `flock`. It takes
the same lock first (same inheritance rules), runs disable, `install --yes`,
and enable (enable only if the plugin file's `rollbackLeaves` says enabled),
checks that the installed source is the expected path, polls `bb plugin list`
until the plugin is running, and rewrites `installed.sha`. It exits 75 if the
lock is busy and 76 if the verification fails. If the state directory is
missing it warns and recovers without the lock or the record write. A bare
variant with no lock is also provided in the JSON report.

## Plugin file

`bb-plugin-deploy/plugin@1`. Top-level keys are fixed; an unknown key fails
closed. Keys: `format`, `pluginId`, `repo`, `branch`, `pluginDir`, `build`,
`data`, `export`, `preflight`, `preInstall`, `preEnable`,
`postEnablePreConfig`, `configAfterEnable`, `health`, `classes`,
`dataRestore`, `previewGate`, `rollbackLeaves`, `controlPaths`, `retention`,
`updateRequest`. Run `validate-plugin-file` for exact problems.

`updateRequest` is optional. Without it `update-tick` refuses.

Names under `data` and `export` may not be `.` or `..`. A `data` name may not
be `manifest.json`, `previous-source.txt` or `export-*.json`, which the backup
directory uses for its own files. The plugin file itself must be a tracked,
unmodified file of the installed engine build; an edited or untracked copy is
refused.

## Environment hooks

| Variable | Use |
| --- | --- |
| `BPD_LOCK_FD`, `BPD_LOCK_WAIT_SEC` | Lock inheritance and a test-only shorter wait (above). |
| `BPD_POLL_MS` | Health polling interval in milliseconds (tests set it small). |
| `BPD_UTC_STAMP` | Fixed timestamp for reproducible tests. |
| `BB_PLUGIN_DEPLOY_BB` | Path to the `bb` binary to use (tests point it at a stub). Its directory is also put on the children's `PATH`. |

Which `bb` is used: `BB_PLUGIN_DEPLOY_BB`, else `BB_CLI` (an absolute path to
an executable), else `$HOME/.local/bin/bb`, else the first executable `bb` on
the caller's `PATH`, else the bare name on the tool's fixed `PATH`.

## Tests

The tests live in the plugin folder and run with the plugin's normal suite:

```
cd integrations/bb-plugin-autarch
npx vitest run __tests__/deploy-tool
```

They use temporary directories and a fake `bb` stub; they never touch a live
install or the live updater state.

## Design choices (not from an outside requirement)

- A first install with no live plugin is refused at anchoring: there is nothing
  to roll back to.
- An unknown live install (for example a dirty checkout) changes nothing and
  does not write `installed.sha`.
- `restore-data` restores sqlite data only, and replacing live data needs
  `--mode mk-script`. Before restoring it checks the backup's manifest: the
  plugin id, the recorded previous source, and a sha256 digest for every
  sqlite file. A backup that reuses an earlier backup of the same commit is
  followed only if that earlier backup sits in the same directory and has its
  own valid manifest.
- An existing build directory is never written to or removed: a name collision
  (the stamp has one-second resolution) refuses the deploy and leaves it alone.
- Review evidence (`review-evidence/<sha>.json`, format
  `bb-plugin-deploy/review-evidence@1`, verdict `approve`) is the runner-mode
  condition from the plan (section 2.2 and the open question on how strong it
  must be). The tool checks its form, not who wrote it; an agent-mode deploy
  needs none because the agent is not unattended. Strengthening the evidence
  (for example a signed receipt) is left to the runner wiring in a later slice.
- The ancestry check is skipped for `mk-script` mode, with a warning.
- `node_modules` is excluded from the control-closure walk.
