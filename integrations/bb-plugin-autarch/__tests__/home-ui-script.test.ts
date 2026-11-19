// scripts/home-ui.sh against a stub bb and a temp data directory. Never live. The deploy subcommand fetches origin, so it is
// covered below through an extracted transaction body with fake Git, build, plugin and service commands.
import { spawn, spawnSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";
import { homeUiSafety } from "./home-ui-safety";

const SCRIPT = resolve(__dirname, "../../../scripts/home-ui.sh");
const dirs: string[] = [];
const CLI_HELPER = resolve(__dirname, "../../../scripts/home-cli.sh");

// Exercise the real deploy/rollback bodies and CLI helpers with shell stand-ins. No live services or bb.
function cliRig(mode = "ok") {
  const root = mkdtempSync(join(tmpdir(), "hui-cli-"));
  dirs.push(root);
  const sha = "a".repeat(40), old = "b".repeat(40);
  const bin = join(root, ".local/bin/autarch");
  mkdirSync(join(root, ".local/bin"), { recursive: true });
  const binary = (rev: string) => `#!/bin/bash
if [ "$1" = version ]; then echo 'revision ${rev} modified=false sha256 sample'; exit; fi
[ "${rev}" != "${sha}" ] || [ "$MODE" != postfail ] || exit 1
echo '{"counts":0}'
`;
  const filings = join(root, "filer/filings.json");
  mkdirSync(join(root, "filer"));
  writeFileSync(filings, '{"sample":{"held_revisions":[{}],"owner_attention":true,"replaced_by":"sample"}}\n');
  writeFileSync(bin, binary(old)); chmodSync(bin, 0o755);
  writeFileSync(join(root, "new"), binary(mode === "wrong" ? old : mode === "unstamped" ? "" : sha));
  const script = readFileSync(SCRIPT, "utf8");
  const body = script.slice(script.lastIndexOf("case $SUB in\ncheck)"));
  const harness = join(root, "run");
  writeFileSync(harness, `#!/bin/bash
set -euo pipefail
MKHOME=$HOME; REPO=$HOME; BUILDS=$HOME/builds; PLUGIN_REL=plugin; COMMIT=${sha}
BB=true; PLUGIN_CODE=0; PATH_ARG=; RESTORE_DATA=0
ST=$HOME/state; mkdir -p "$ST" "$BUILDS/build-${sha.slice(0, 12)}/plugin" "$HOME/backup" "$HOME/previous/plugin"
die() { echo "$*" >&2; exit 1; }
stamp_warning() { echo "$*"; }
clear_rollback_stamp() { rm -f "$1/installed.sha" "$1/status.json"; }
cp() {
  local src=$1; [ "$1" != -- ] || src=$2
  if [ "$src" = "$AUTARCH_CARD_STATE_DIR/filings.json" ]; then
    if [ "\${REQUIRE_FILER_LOCK:-}" = 1 ]; then
      if flock -n "$AUTARCH_CARD_STATE_DIR/filing.lock" true; then echo "snapshot was not locked" >&2; return 98; fi
    fi
    [ "\${STATE_COPY_FAIL:-}" != 1 ] || return 1
    echo snapshot >> "$HOME/order"
  fi
  case "$1" in *autarch.prev-*) echo restore >> "$HOME/order";; esac
  command cp "$@"
}
git() { case "$*" in *status*) [ "$MODE" != dirty ] || echo "?? stray"; [ "$MODE" != vcsfail ];; *'rev-parse HEAD'*) if [ "$2" = "$HOME/previous/plugin" ]; then echo ${old}; else echo "$COMMIT"; fi;; *--show-toplevel*)
  local top="$BUILDS/build-${sha.slice(0, 12)}"
  [ "$2" != "$HOME/previous/plugin" ] || top="$HOME/previous"
  if [ -n "\${STAMP_GROUP_SIGNAL:-}" ]; then
    bash -c 'kill -s "$1" -- "-$2" || exit; echo "$1" > "$HOME/stamp-child-survived"; echo "$3"' stamp-child "$STAMP_GROUP_SIGNAL" "$$" "$top"
  else echo "$top"; fi;; esac; }
npm() { :; }
mv() {
  if [ "\${STAMP_PHASE:-}" = before ] && [ "\${3:-}" = "$HOME/state/installed.sha" ]; then kill -s "$STAMP_SIGNAL" "$$"; fi
  command mv "$@" || return
  if [ "\${STAMP_PHASE:-after}" = after ] && [ -n "\${STAMP_SIGNAL:-}" ] && [ "\${3:-}" = "$HOME/state/installed.sha" ]; then kill -s "$STAMP_SIGNAL" "$$"; fi
}
go() { printf '%s\\n' "$*" > "$HOME/go-args"; [ "$MODE" != buildfail ] || return 1; cp "$HOME/new" "$6"; chmod +x "$6"; }
fixture_systemctl() { echo "$*" >> "$HOME/unit"; if [ "$MODE" = restorefail ] && grep -q "revision ${old}" "$HOME/.local/bin/autarch"; then return 1; fi; if [ "$MODE" = queryfail ] && grep -q "revision ${sha}" "$HOME/.local/bin/autarch"; then return 1; fi; if [ "$2" = restart ] && [ "$MODE" = restartfail ] && grep -q "revision ${sha}" "$HOME/.local/bin/autarch"; then return 1; fi; [ "$MODE" != missing ] && [ "\${UNIT_MISSING:-}" != 1 ] || { echo not-found; return; }; if [ "$2" = restart ]; then
    if [ "$MODE" != stale ] || ! grep -q "revision ${sha}" "$HOME/.local/bin/autarch"; then
      local pid; pid=$(cat "$HOME/pid"); pid=$((pid+1)); echo "$pid" > "$HOME/pid"
      local target="$HOME/.local/bin/autarch"
      if [ "$MODE" = deleted ] && grep -q "revision ${sha}" "$target"; then target="$target (deleted)"; fi
      mkdir -p "$HOME/proc/$pid"; ln -s "$target" "$HOME/proc/$pid/exe"
    fi
  fi
  if [ "$2" = show ]; then case "$*" in *MainPID*) cat "$HOME/pid";; *) echo loaded;; esac; fi; }
curl() { if [ "$MODE" = healthfail ] && grep -q "revision ${sha}" "$HOME/.local/bin/autarch"; then return 1; fi; echo '{"status":"ok"}'; }
gate_ui_only() { :; }
backup() { echo "$HOME/previous/plugin" > "$HOME/backup/previous-source.txt"; touch "$HOME/backup/data.db"; echo "$HOME/backup"; }
install_build() {
  echo "$1" >> "$HOME/plugin-log"
  if [ "$1" = "$HOME/previous/plugin" ]; then [ "\${RESTORE_DIE:-}" != 1 ] || die "plugin disable failed";
    if [ -n "\${RESTORE_SIGNAL:-}" ]; then kill -s "$RESTORE_SIGNAL" "$$"; fi;
  elif [ -n "\${DEPLOY_SIGNAL:-}" ]; then kill -s "$DEPLOY_SIGNAL" "$$"; fi
}
rollback_cmd() {
  if [ -n "\${REPORT_SIGNAL:-}" ] && [ -f "$HOME/state/installed.sha" ] && [ "$(cat "$HOME/state/installed.sha")" = "$COMMIT" ]; then kill -s "$REPORT_SIGNAL" "$$"; fi
  echo "rollback $1"
}
[ -f "$HOME/pid" ] || echo 10 > "$HOME/pid"
mkdir -p "$HOME/proc/10"
[ -L "$HOME/proc/10/exe" ] || ln -s "$HOME/.local/bin/autarch (deleted)" "$HOME/proc/10/exe"
HOME_UI_PROC_ROOT=$HOME/proc
HOME_UI_SYSTEMCTL=fixture_systemctl
systemctl() { echo "unexpected ambient systemctl" >&2; return 99; }
source '${CLI_HELPER}'
SUB=$1; BACKUP_ARG=$HOME/backup
trap 'if [[ $BASH_COMMAND == "PLUGIN_TOUCHED=0 CLI_SWAPPED=0" ]] && [ "\${STAMP_PHASE:-}" = clear ]; then kill -s "$STAMP_SIGNAL" "$$"; fi' DEBUG
${body}
`);
  // A separate session keeps process-group signals inside this temporary rig.
  const run = (sub = "deploy", extra: Record<string, string> = {}) => spawnSync("setsid", ["bash", harness, sub], { env: { ...process.env, HOME: root, MODE: mode, HOME_UI_WAIT: "1", HOME_UI_STATE_DIR: join(root, "state"), AUTARCH_CARD_STATE_DIR: join(root, "filer"), XDG_STATE_HOME: join(root, "xdg"), ...extra }, encoding: "utf8", timeout: 10000, killSignal: "SIGKILL" });
  return { root, bin, sha, old, filings, run, version: () => spawnSync(bin, ["version"], { encoding: "utf8" }).stdout };
}

describe("Home CLI deploy transaction", () => {
  it("check reports the live revision and build stamp without installing or restarting", () => {
    const r = cliRig(), out = r.run("check");
    expect(out.status, out.stderr).toBe(0);
    expect(out.stdout).toContain(`live CLI revision: ${r.old}`);
    expect(out.stdout).toContain(`CLI build stamps revision ${r.sha} modified=false`);
    expect(r.version()).toContain(r.old);
    expect(existsSync(join(r.root, "unit"))).toBe(false);
    expect(existsSync(join(r.root, "plugin-log"))).toBe(false);
  });
  it("installs the target, saves the previous binary, and rollback restores it", () => {
    const r = cliRig();
    expect(r.run().status).toBe(0);
    expect(r.version()).toContain(r.sha);
    expect(readFileSync(join(r.root, "go-args"), "utf8")).toContain(`-buildvcs=false -ldflags -X main.buildRevision=${r.sha}`);
    expect(readFileSync(join(r.root, "backup/previous-cli.txt"), "utf8")).toContain(`autarch.prev-${r.old.slice(0, 12)}`);
    const rolled = r.run("rollback");
    expect(rolled.status, rolled.stderr).toBe(0);
    expect(r.version()).toContain(r.old);
  });
  for (const mode of ["wrong", "unstamped", "vcsfail", "dirty", "buildfail"]) it(`refuses ${mode} before installing either component`, () => {
    const r = cliRig(mode), out = r.run();
    expect(out.status).not.toBe(0);
    expect(out.stderr).toMatch(/CLI (build|worktree)|VCS status/);
    expect(r.version()).toContain(r.old);
    expect(existsSync(join(r.root, "plugin-log"))).toBe(false);
  });
  for (const mode of ["postfail", "healthfail", "restartfail", "stale", "deleted"]) it(`restores both components and restarts after ${mode}`, () => {
    const r = cliRig(mode), out = r.run();
    expect(out.status).not.toBe(0);
    expect(out.stderr).toContain("restor");
    expect(out.stdout).toContain("rollback");
    expect(r.version()).toContain(r.old);
    expect(readFileSync(join(r.root, "unit"), "utf8").match(/restart/g)?.length).toBe(2);
    expect(readFileSync(join(r.root, "plugin-log"), "utf8")).toContain("previous");
  });
  it("restores the old executable even when service queries fail after the swap", () => {
    const r = cliRig("queryfail"), out = r.run();
    expect(out.status).not.toBe(0);
    expect(r.version()).toContain(r.old);
    expect(out.stderr).toContain("restoring CLI anyway");
    expect(readFileSync(join(r.root, "unit"), "utf8")).toContain("restart");
  });
  for (const signal of ["INT", "TERM"]) it(`restores after ${signal} during plugin installation`, () => {
    const r = cliRig(), out = r.run("deploy", { DEPLOY_SIGNAL: signal });
    expect(out.status, out.stderr).toBe(5);
    expect(r.version()).toContain(r.old);
    expect(readFileSync(join(r.root, "plugin-log"), "utf8")).toContain("previous/plugin");
    expect(out.stdout).toContain(`rollback ${r.root}/backup`);
  });
  it("prints the mismatch warning and rollback command when plugin restore exits internally", () => {
    const r = cliRig("postfail"), out = r.run("deploy", { RESTORE_DIE: "1" });
    expect(out.status).toBe(5);
    expect(out.stderr).toContain("plugin and CLI may differ");
    expect(out.stdout.trim().split("\n").at(-1)).toBe(`rollback ${r.root}/backup`);
  });
  for (const valid of [false, true]) it(`reconciles installed.sha despite a CLI rollback failure (valid=${valid})`, () => {
    const r = cliRig();
    expect(r.run().status).toBe(0);
    const stamp = join(r.root, "state/installed.sha"), status = join(r.root, "state/status.json");
    writeFileSync(stamp, r.sha); writeFileSync(status, "{}");
    if (valid) writeFileSync(join(r.root, "backup/previous-commit.txt"), r.old);
    const out = r.run("rollback", { MODE: "restorefail" });
    expect(out.status, out.stderr).toBe(5);
    if (valid) expect(readFileSync(stamp, "utf8").trim()).toBe(r.old);
    else { expect(existsSync(stamp)).toBe(false); expect(existsSync(status)).toBe(false); }
  });
  it("automatically restores an absent CLI without inventing a previous binary", () => {
    const r = cliRig("postfail"); rmSync(r.bin);
    const out = r.run("deploy", { UNIT_MISSING: "1" });
    expect(out.status).toBe(5);
    expect(existsSync(r.bin)).toBe(false);
    expect(readFileSync(join(r.root, "plugin-log"), "utf8")).toContain("previous/plugin");
    expect(out.stderr).not.toContain("rollback incomplete");
  });
  for (const dangling of [false, true]) it(`refuses a symlinked live CLI (dangling=${dangling})`, () => {
    const r = cliRig(), other = join(r.root, "other");
    if (!dangling) writeFileSync(other, readFileSync(r.bin));
    rmSync(r.bin); symlinkSync(other, r.bin);
    expect(r.run().status).not.toBe(0);
    expect(existsSync(join(r.root, "plugin-log"))).toBe(false);
  });
  for (const automatic of [true, false]) it(`saves identical filer state before CLI restoration (automatic=${automatic})`, () => {
    const r = cliRig(automatic ? "postfail" : "ok");
    let out = r.run();
    if (!automatic) { expect(out.status).toBe(0); out = r.run("rollback"); }
    expect(out.status, out.stderr).toBe(automatic ? 5 : 0);
    const copies = readdirSync(join(r.root, "backup")).filter(p => /^filings-\d{8}T/.test(p));
    expect(copies).toHaveLength(1);
    const saved = join(r.root, "backup", copies[0]);
    expect(readFileSync(saved)).toEqual(readFileSync(r.filings));
    expect(out.stdout).toContain(saved);
    expect(readFileSync(join(r.root, "order"), "utf8").trim().split("\n")).toEqual(["snapshot", "restore"]);
  });
  for (const automatic of [true, false]) it(`refuses CLI rollback when filer backup fails but restores the plugin (automatic=${automatic})`, () => {
    const r = cliRig(automatic ? "postfail" : "ok");
    let out;
    if (automatic) out = r.run("deploy", { STATE_COPY_FAIL: "1" });
    else { expect(r.run().status).toBe(0); out = r.run("rollback", { STATE_COPY_FAIL: "1" }); }
    expect(out.status).toBe(5);
    expect(r.version()).toContain(r.sha);
    expect(out.stderr).toContain("refusing CLI rollback");
    expect(out.stderr).toMatch(/may differ/);
    expect(readFileSync(join(r.root, "plugin-log"), "utf8")).toContain("previous/plugin");
  });
  for (const xdg of [true, false]) it(`preserves state from the CLI fallback path (XDG=${xdg})`, () => {
    const r = cliRig("postfail");
    const stateDir = xdg ? join(r.root, "xdg/autarch/needsmk") : join(r.root, ".local/state/autarch/needsmk");
    mkdirSync(stateDir, { recursive: true });
    const contents = readFileSync(r.filings); writeFileSync(join(stateDir, "filings.json"), contents);
    const out = r.run("deploy", { AUTARCH_CARD_STATE_DIR: "", XDG_STATE_HOME: xdg ? join(r.root, "xdg") : "" });
    expect(out.status).toBe(5);
    const saved = readdirSync(join(r.root, "backup")).find(p => /^filings-\d{8}T/.test(p))!;
    expect(readFileSync(join(r.root, "backup", saved))).toEqual(contents);
  });
  it("keeps protected system commands ahead of appended Go directories", () => {
    const r = cliRig(), goDir = join(r.root, ".local/go/bin");
    mkdirSync(goDir, { recursive: true });
    writeFileSync(join(goDir, "git"), "#!/bin/sh\nexit 99\n"); chmodSync(join(goDir, "git"), 0o755);
    const script = readFileSync(SCRIPT, "utf8");
    const prologue = script.slice(script.indexOf("set -euo pipefail"), script.indexOf("umask 077"));
    const out = spawnSync("bash", ["-c", `${prologue}\ncommand -v git`], { env: { ...process.env, HOME: r.root }, encoding: "utf8" });
    expect(out.status).toBe(0);
    expect(out.stdout).not.toContain(goDir);
  });
  it("attempts CLI rollback after plugin restoration exits and reports both outcomes", () => {
    const r = cliRig(); expect(r.run().status).toBe(0);
    writeFileSync(join(r.root, "state/status.json"), "{}");
    const out = r.run("rollback", { RESTORE_DIE: "1" });
    expect(out.status).toBe(5);
    expect(r.version()).toContain(r.old);
    expect(out.stderr).toContain("plugin=failed; CLI=restored");
    expect(readFileSync(join(r.root, "state/installed.sha"), "utf8").trim()).toBe(r.sha);
    expect(existsSync(join(r.root, "state/status.json"))).toBe(true);
    expect(readdirSync(join(r.root, "backup")).some(p => p.startsWith("filings-"))).toBe(true);
  });
  for (const automatic of [true, false]) it(`takes the filer's own lock for the rollback snapshot (automatic=${automatic})`, () => {
    const r = cliRig(automatic ? "postfail" : "ok");
    let out;
    if (automatic) out = r.run("deploy", { REQUIRE_FILER_LOCK: "1" });
    else { expect(r.run().status).toBe(0); out = r.run("rollback", { REQUIRE_FILER_LOCK: "1" }); }
    expect(out.status, out.stderr).toBe(automatic ? 5 : 0);
    expect(r.version()).toContain(r.old);
    expect(spawnSync("flock", ["-n", join(r.root, "filer/filing.lock"), "true"]).status).toBe(0);
  });
  for (const automatic of [true, false]) it(`bounds the filer lock wait and restores the plugin (automatic=${automatic})`, async () => {
    const r = cliRig(automatic ? "postfail" : "ok");
    if (!automatic) expect(r.run().status).toBe(0);
    const holder = spawn("bash", ["-c", 'exec 8>>"$1"; flock -x 8; echo held; read -r -t 2 || true', "holder", join(r.root, "filer/filing.lock")], { env: { HOME: r.root, PATH: process.env.PATH! } });
    const finished = new Promise<void>(ok => holder.once("exit", () => ok()));
    try {
      await new Promise<void>((ok, no) => {
        holder.stdout.once("data", () => ok());
        holder.once("error", no);
        holder.once("exit", () => no(new Error("lock holder exited before readiness")));
      });
      const out = r.run(automatic ? "deploy" : "rollback", { HOME_UI_FILER_LOCK_WAIT: "0.1" });
      expect(holder.exitCode).toBeNull();
      expect(spawnSync("flock", ["-n", join(r.root, "filer/filing.lock"), "true"]).status).not.toBe(0);
      expect(out.status).toBe(5);
      expect(r.version()).toContain(r.sha);
      expect(out.stderr).toContain("cannot lock filer state; refusing CLI rollback");
      expect(out.stderr).toContain("plugin=restored; CLI=failed");
      expect(out.stderr).toContain("may differ");
      expect(readFileSync(join(r.root, "plugin-log"), "utf8")).toContain("previous/plugin");
      expect(out.stdout.trim().split("\n").at(-1)).toBe(`rollback ${r.root}/backup`);
      expect(readdirSync(join(r.root, "backup")).some(p => p.startsWith("filings-"))).toBe(false);
    } finally {
      holder.kill("SIGKILL");
      await finished;
    }
  });
  for (const signal of ["INT", "TERM"]) for (const phase of ["clear", "before", "after"]) it(`reports a completed deploy on ${signal} at stamp phase ${phase}`, () => {
    const r = cliRig();
    mkdirSync(join(r.root, "state"));
    writeFileSync(join(r.root, "state/installed.sha"), r.old + "\n");
    const out = r.run("deploy", { STAMP_SIGNAL: signal, STAMP_PHASE: phase });
    expect(out.status, out.stderr).toBe(0);
    expect(r.version()).toContain(r.sha);
    expect(readFileSync(join(r.root, "state/installed.sha"), "utf8").trim()).toBe(r.sha);
    expect(readFileSync(join(r.root, "plugin-log"), "utf8")).not.toContain("previous");
    expect(out.stdout).toContain(`deployed ${r.sha}; Home is running`);
    expect(out.stdout.trim().split("\n").at(-1)).toBe(`rollback ${r.root}/backup`);
  });
  for (const signal of ["INT", "TERM"]) it(`protects the stamp child from process-group ${signal}`, () => {
    const r = cliRig();
    mkdirSync(join(r.root, "state"));
    writeFileSync(join(r.root, "state/installed.sha"), r.old + "\n");
    const out = r.run("deploy", { STAMP_GROUP_SIGNAL: signal });
    expect(out.status, out.stderr).toBe(0);
    expect(readFileSync(join(r.root, "stamp-child-survived"), "utf8").trim()).toBe(signal);
    expect(readFileSync(join(r.root, "state/installed.sha"), "utf8").trim()).toBe(r.sha);
    expect(out.stdout).not.toContain("installed.sha was not updated");
    expect(out.stdout).toContain(`deployed ${r.sha}; Home is running`);
    expect(out.stdout).toContain("ignoring INT/TERM");
    expect(out.stdout.trim().split("\n").at(-1)).toBe(`rollback ${r.root}/backup`);
    expect(readFileSync(join(r.root, "plugin-log"), "utf8")).not.toContain("previous");
  });
  for (const [signal, status] of [["INT", 130], ["TERM", 143]] as const) it(`restores the ${signal} handler after stamping (exit ${status})`, () => {
    const r = cliRig(), out = r.run("deploy", { REPORT_SIGNAL: signal });
    expect(out.status, out.stderr).toBe(status);
    expect(readFileSync(join(r.root, "state/installed.sha"), "utf8").trim()).toBe(r.sha);
    expect(out.stdout).toContain(`deployed ${r.sha}; Home is running`);
    expect(readFileSync(join(r.root, "plugin-log"), "utf8")).not.toContain("previous");
  });
  for (const signal of ["INT", "TERM"]) it(`ignores a second ${signal} during automatic restoration`, () => {
    const r = cliRig("postfail"), out = r.run("deploy", { RESTORE_SIGNAL: signal });
    expect(out.status).toBe(5);
    expect(r.version()).toContain(r.old);
    expect(out.stdout.trim().split("\n").at(-1)).toBe(`rollback ${r.root}/backup`);
  });
  it("skips an absent user unit with a message", () => {
    const r = cliRig("missing"), out = r.run();
    expect(out.status).toBe(0);
    expect(out.stdout).toContain("unit does not exist");
    expect(readFileSync(join(r.root, "unit"), "utf8")).not.toContain("restart");
  });
});
afterEach(() => dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true })));

function plugin(root: string, name: string, version: number, extra = "") {
  const d = join(root, name);
  mkdirSync(join(d, "node_modules"), { recursive: true });
  writeFileSync(join(d, "package.json"), "{}");
  writeFileSync(join(d, "migrations.ts"), `export const CODE_VERSION = ${version};\n${extra}`);
  return d;
}

function rig(live = 3, failAfter = "") {
  const root = mkdtempSync(join(tmpdir(), "hui-"));
  dirs.push(root);
  const data = join(root, "bbdata");
  mkdirSync(join(data, "plugins", "autarch"), { recursive: true });
  const db = new Database(join(data, "plugins", "autarch", "data.db"));
  db.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE t (x TEXT)");
  db.prepare("INSERT INTO schema_meta VALUES ('schema_version', ?)").run(String(live));
  db.prepare("INSERT INTO t VALUES ('before')").run();
  db.close();
  const cur = plugin(root, "cur", live);
  const calls = join(root, "calls");
  const bb = join(root, "bb");
  writeFileSync(bb, `#!/bin/sh
echo "$@" >> ${calls}
case "$1 $2" in
  "plugin source") echo "autarch"; echo "  resolved: path:${cur}" ;;
  "plugin install") [ -n "$HUI_FAIL_AFTER" ] && [ "$(grep -c '^plugin install' ${calls})" -gt "$HUI_FAIL_AFTER" ] && exit 1; exit 0 ;;
  "plugin list") echo '{"plugins":[{"id":"autarch","enabled":true,"status":"running"}]}' ;;
  *) exit 0 ;;
esac
`);
  chmodSync(bb, 0o755);
  const run = (args: string[]) => spawnSync("bash", [SCRIPT, ...args], { env: { PATH: process.env.PATH!, HUI_FAIL_AFTER: failAfter, HOME: root, HOME_BB_BIN: bb, HOME_BB_DATA: data, HOME_UI_WAIT: "2", HOME_UI_RECHECK_WAIT: "1", HOME_UI_RECHECK_INTERVAL: "1", ...homeUiSafety(root) }, encoding: "utf8" });
  const log = () => (existsSync(calls) ? readFileSync(calls, "utf8") : "");
  const backups = () => { try { return readdirSync(join(data, "home-backups")); } catch { return []; } };
  return { root, data, cur, run, log, backups };
}

describe("home-ui.sh gates", () => {
  it("check passes when the schema matches and migrations.ts is identical", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    const out = r.run(["check", "--path", n]);
    expect(out.status).toBe(0);
    expect(out.stdout).toContain("gate ok");
  });

  it("refuses a build whose schema differs from the live one, before any backup or bb change", () => {
    const r = rig(3);
    const n = plugin(r.root, "new", 4);
    const out = r.run(["preview", "--path", n]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("NOT UI-only");
    expect(r.backups()).toEqual([]);
    expect(r.log()).not.toMatch(/disable|install/);
  });

  it("allows a ui/ change but refuses a change to any other file", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    mkdirSync(join(n, "ui"));
    writeFileSync(join(n, "ui", "a.tsx"), "x");
    expect(r.run(["check", "--path", n]).status).toBe(0);
    writeFileSync(join(n, "server.ts"), "writes the db");
    const out = r.run(["check", "--path", n]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("server.ts");
  });

  it("refuses a build whose migrations.ts differs even at the same version", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3, "// changed\n");
    const out = r.run(["check", "--path", n]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("migrations.ts differs");
  });

  it("--plugin-code allows plugin code outside ui/ but still refuses dependency, schema and migrations.ts changes", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    writeFileSync(join(n, "server.ts"), "plugin logic");
    const refused = r.run(["check", "--path", n]);
    expect(refused.status).toBe(3);
    const ok = r.run(["check", "--path", n, "--plugin-code"]);
    expect(ok.status).toBe(0);
    expect(ok.stdout).toContain("plugin code may differ");
    writeFileSync(join(n, "package.json"), '{"dependencies":{"x":"1"}}');
    const dep = r.run(["check", "--path", n, "--plugin-code"]);
    expect(dep.status).toBe(3);
    expect(dep.stderr).toContain("package.json differs");
    const s2 = plugin(r.root, "new2", 4);
    expect(r.run(["check", "--path", s2, "--plugin-code"]).status).toBe(3);
    const m = plugin(r.root, "new3", 3, "// changed\n");
    expect(r.run(["check", "--path", m, "--plugin-code"]).stderr).toContain("migrations.ts differs");
    expect(r.run(["preview", "--plugin-code"]).status).toBe(64);
  });

  it("--plugin-code refuses changed plugin code that contains schema statements", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    writeFileSync(join(n, "store.ts"), 'db.exec("CREATE TABLE gate_bypassed(x TEXT)");');
    const out = r.run(["check", "--path", n, "--plugin-code"]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("schema statement");
    writeFileSync(join(n, "store.ts"), "ok();");
    expect(r.run(["check", "--path", n, "--plugin-code"]).status).toBe(0);
  });

  it("--plugin-code scans added lines, including new directories and multi-line SQL, and ignores unchanged references", () => {
    const r = rig();
    writeFileSync(join(r.cur, "backup.ts"), 'q("SELECT value FROM schema_meta");\n');
    const n = plugin(r.root, "new", 3);
    writeFileSync(join(n, "backup.ts"), 'q("SELECT value FROM schema_meta");\n// comment only\n');
    expect(r.run(["check", "--path", n, "--plugin-code"]).status).toBe(0);
    mkdirSync(join(n, "lib"));
    writeFileSync(join(n, "lib", "x.ts"), "db.exec(`CREATE\n  TABLE t2(x)`);\n");
    const out = r.run(["check", "--path", n, "--plugin-code"]);
    expect(out.status).toBe(3);
    expect(out.stderr).toContain("schema statement");
  });

  it("--plugin-code scans binary-looking files, lines starting with ++ and odd file names", () => {
    const r = rig();
    const bad = 'db.exec("CREATE TABLE bypass(x)");';
    for (const [name, body] of [["bin.js", `\0${bad}`], ["pp.js", `++c; ${bad}`], ["a b ui", bad]] as const) {
      const n = plugin(r.root, `n-${name.replace(/\W/g, "_")}`, 3);
      mkdirSync(join(n, "lib"), { recursive: true });
      writeFileSync(join(n, "lib", name), body);
      expect(r.run(["check", "--path", n, "--plugin-code"]).status, name).toBe(3);
    }
    const n2 = plugin(r.root, "n-sp", 3);
    mkdirSync(join(n2, "lib x"), { recursive: true });
    mkdirSync(join(n2, "lib x", "ui"));
    writeFileSync(join(n2, "lib x", "ui", "p.js"), bad);
    expect(r.run(["check", "--path", n2, "--plugin-code"]).status).toBe(3);
  });

  it("--plugin-code refuses any added, changed or dangling lockfile and .npmrc", () => {
    const r = rig();
    for (const f of ["package-lock.json", "npm-shrinkwrap.json", ".npmrc", "pnpm-lock.yaml", "yarn.lock"]) {
      const n = plugin(r.root, `n-${f}`, 3);
      writeFileSync(join(n, f), "new");
      const out = r.run(["check", "--path", n, "--plugin-code"]);
      expect(out.status, f).toBe(3);
      expect(out.stderr).toContain(f);
    }
    const d = plugin(r.root, "dangling", 3);
    symlinkSync("/nonexistent-lock", join(d, "package-lock.json"));
    expect(r.run(["check", "--path", d, "--plugin-code"]).status).toBe(3);
  });

  it("refuses to pick when no Home database or more than one is found", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    const home = join(r.root, "h");
    mkdirSync(home);
    const env = { PATH: process.env.PATH!, HOME: home, HOME_BB_BIN: join(r.root, "bb"), HOME_UI_WAIT: "2", ...homeUiSafety(r.root) };
    expect(spawnSync("bash", [SCRIPT, "check", "--path", n], { env, encoding: "utf8" }).stderr).toContain("found 0");
    for (const x of ["x", "y"]) { mkdirSync(join(home, x, "plugins", "autarch"), { recursive: true }); writeFileSync(join(home, x, "plugins", "autarch", "data.db"), ""); }
    expect(spawnSync("bash", [SCRIPT, "check", "--path", n], { env, encoding: "utf8" }).stderr).toContain("found 2");
  });

  it("finds the Home database in a data directory five levels below the home directory", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    const deep = join(r.root, "a", "b", "c", "data");
    mkdirSync(join(deep, "plugins", "autarch"), { recursive: true });
    writeFileSync(join(deep, "plugins", "autarch", "data.db"), "");
    const env = { PATH: process.env.PATH!, HOME: join(r.root, "a"), HOME_BB_BIN: join(r.root, "bb"), HOME_UI_WAIT: "2", ...homeUiSafety(r.root) };
    const out = spawnSync("bash", [SCRIPT, "check", "--path", n], { env, encoding: "utf8" });
    expect(out.stderr).not.toContain("cannot pick one Home database");
  });

  it("fails closed on an unreadable database, a missing node_modules and a bad commit", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    rmSync(join(n, "node_modules"), { recursive: true });
    expect(r.run(["check", "--path", n]).stderr).toContain("node_modules");
    rmSync(join(r.data, "plugins", "autarch", "data.db"));
    const m = plugin(r.root, "new2", 3);
    expect(r.run(["check", "--path", m]).status).not.toBe(0);
    expect(r.run(["deploy", "--commit", "abc"]).status).toBe(64);
    expect(r.run(["deploy"]).status).toBe(64);
  });
});

describe("home-ui.sh preview and rollback", () => {
  it("backs up first, swaps to the preview build, runs dev, then restores the previous build", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    const out = r.run(["preview", "--path", n]);
    expect(out.status).toBe(0);
    const [b] = r.backups();
    expect(b).toBeDefined();
    const dir = join(r.data, "home-backups", b);
    expect(readFileSync(join(dir, "previous-source.txt"), "utf8").trim()).toBe(r.cur);
    expect(new Database(join(dir, "data.db"), { readonly: true }).prepare("SELECT x FROM t").get()).toEqual({ x: "before" });
    const calls = r.log().trim().split("\n");
    const installs = calls.filter((c) => c.startsWith("plugin install"));
    expect(installs).toEqual([`plugin install --yes ${n}`, `plugin install --yes ${r.cur}`]);
    expect(calls).toContain(`plugin dev ${n}`);
    expect(calls.indexOf(`plugin dev ${n}`)).toBeLessThan(calls.lastIndexOf(`plugin install --yes ${r.cur}`));
    expect(out.stdout).toContain("rollback: bash");
    expect(out.stdout).toContain("live data");
  });

  it("exits nonzero when the preview cannot restore the previous build", () => {
    const r = rig(3, "1");
    const n = plugin(r.root, "new", 3);
    const out = r.run(["preview", "--path", n]);
    expect(out.status).toBe(7);
    expect(out.stderr).toContain("restore failed");
  });

  it("rollback reinstalls the recorded build and restores data only when asked", () => {
    const r = rig();
    const n = plugin(r.root, "new", 3);
    r.run(["preview", "--path", n]);
    const dir = join(r.data, "home-backups", r.backups()[0]);
    const live = join(r.data, "plugins", "autarch", "data.db");
    const w = new Database(live); w.prepare("INSERT INTO t VALUES ('after')").run(); w.close();
    expect(r.run(["rollback", dir]).status).toBe(0);
    const count = () => { const d = new Database(live, { readonly: true }); try { return d.prepare("SELECT count(*) AS c FROM t").get(); } finally { d.close(); } };
    expect(count()).toEqual({ c: 2 });
    expect(r.run(["rollback", dir, "--restore-data"]).status).toBe(0);
    expect(count()).toEqual({ c: 1 });
    expect(r.run(["rollback", r.root]).status).not.toBe(0);
  });

  it("refuses to run as root", () => {
    if (process.getuid?.() !== 0) return;
    const r = rig();
    expect(spawnSync("bash", [SCRIPT, "check"], { env: { HOME: r.root, PATH: process.env.PATH!, ...homeUiSafety(r.root) }, encoding: "utf8" }).status).toBe(64);
  });
  // R5: check lists every failed gate in one run; deploy and preview stay fail-fast.
  describe("accumulation", () => {
    it("every subset of {no node_modules, other schema, file outside ui/} is reported whole by check", () => {
      const names = ["no node_modules", "the build is schema", "files outside ui/"];
      for (let mask = 1; mask < 8; mask++) {
        const r = rig();
        const n = plugin(r.root, "new", mask & 2 ? 4 : 3);
        if (mask & 1) rmSync(join(n, "node_modules"), { recursive: true });
        if (mask & 4) writeFileSync(join(n, "server.ts"), "export {};\n");
        const out = r.run(["check", "--path", n]);
        expect(out.status, `mask ${mask}`).toBe(3);
        names.forEach((say, i) => { if (mask & (1 << i)) expect(out.stderr, `mask ${mask}: ${say}`).toContain(say); });
        expect(out.stderr).toContain("failed gate(s)");
        expect(r.log()).not.toContain("plugin disable");
      }
    });
    it("check names a build that has no package.json", () => {
      const r = rig();
      const n = plugin(r.root, "new", 3);
      rmSync(join(n, "package.json"));
      const out = r.run(["check", "--path", n]);
      expect(out.status).toBe(3);
      expect(out.stderr).toContain("is not a Home plugin build");
    });
    it("preview stops at the first failed gate", () => {
      const r = rig();
      const n = plugin(r.root, "new", 4);
      rmSync(join(n, "node_modules"), { recursive: true });
      const out = r.run(["preview", "--path", n]);
      expect(out.status).not.toBe(0);
      expect(out.stderr).toContain("no node_modules");
      expect(out.stderr).not.toContain("the build is schema");
    });
  });

});
