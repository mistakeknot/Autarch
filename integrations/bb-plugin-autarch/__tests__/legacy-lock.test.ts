// The shared destination lock in the two legacy scripts (scripts/home-ui.sh, scripts/install-home-your-move.sh), against a stub bb and
// temp directories. Never live: the state directory, the data directory and every build are made here.
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir, userInfo } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";

const HUI = resolve(__dirname, "../../../scripts/home-ui.sh");
const INST = resolve(__dirname, "../../../scripts/install-home-your-move.sh");
const FIXTURES = resolve(__dirname, "fixtures/legacy-b05ebaf");
const BASELINE_COMMIT = "b05ebafb";
const dirs: string[] = [];
const procs: ChildProcess[] = [];
afterEach(() => {
  procs.splice(0).forEach((p) => p.kill("SIGKILL"));
  dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true }));
});

const git = (cwd: string, ...args: string[]) =>
  spawnSync("git", ["-c", "commit.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.invalid", ...args], { cwd, encoding: "utf8" });

function rig(opts: { movesFail?: boolean; noState?: boolean } = {}) {
  const root = mkdtempSync(join(tmpdir(), "llock-"));
  dirs.push(root);
  const st = join(root, "state");
  if (!opts.noState) mkdirSync(st);
  const data = join(root, "bbdata");
  mkdirSync(join(data, "plugins", "autarch"), { recursive: true });
  const db = new Database(join(data, "plugins", "autarch", "data.db"));
  db.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE migration_log (version INTEGER)");
  db.prepare("INSERT INTO schema_meta VALUES ('schema_version', '3')").run();
  db.prepare("INSERT INTO migration_log VALUES (3)").run();
  db.close();
  // The previous build W: a checkout of the plugin. The installed build X: an ordinary directory.
  const wrepo = join(root, "w");
  const prev = join(wrepo, "integrations", "bb-plugin-autarch");
  mkdirSync(prev, { recursive: true });
  writeFileSync(join(prev, "migrations.ts"), "export const CODE_VERSION = 3;\n");
  git(wrepo, "init", "-q");
  git(wrepo, "add", "-A");
  git(wrepo, "commit", "-q", "-m", "w");
  const wsha = git(wrepo, "rev-parse", "HEAD").stdout.trim();
  const cur = join(root, "cur");
  mkdirSync(cur);
  const backup = join(root, "backup");
  mkdirSync(backup);
  writeFileSync(join(backup, "data.db"), "x");
  writeFileSync(join(backup, "previous-source.txt"), prev + "\n");
  const calls = join(root, "calls");
  const bb = join(root, "bb");
  writeFileSync(bb, `#!/bin/sh
echo "$@" >> ${calls}
case "$1 $2" in
  "plugin source") echo "autarch"; echo "  resolved: path:${cur}" ;;
  "plugin list") echo '{"plugins":[{"id":"autarch","enabled":true,"status":"running"}]}' ;;
  "home moves") ${opts.movesFail ? "exit 1" : `echo '{"moves":[]}'`} ;;
  *) exit 0 ;;
esac
`);
  chmodSync(bb, 0o755);
  const env = (extra: Record<string, string> = {}): Record<string, string> => ({
    PATH: process.env.PATH!, HOME: root, HOME_BB_BIN: bb, HOME_BB_DATA: data, HOME_UI_WAIT: "2", HOME_UI_RECHECK_WAIT: "1", HOME_UI_RECHECK_INTERVAL: "1", HOME_UI_STATE_DIR: st,
    HOME_YOUR_MOVE_OUT_DIR: root, ...extra,
  });
  const log = () => (existsSync(calls) ? readFileSync(calls, "utf8") : "");
  const sha = () => (existsSync(join(st, "installed.sha")) ? readFileSync(join(st, "installed.sha"), "utf8").trim() : null);
  return { root, st, cur, lock: join(st, "lock"), data, backup, prev, wsha, calls, env, log, sha };
}
type Rig = ReturnType<typeof rig>;

const hui = (r: Rig, args: string[], extra: Record<string, string> = {}) =>
  spawnSync("bash", [HUI, ...args], { env: r.env(extra), encoding: "utf8" });
const inst = (r: Rig, args: string[], extra: Record<string, string> = {}) =>
  spawnSync("bash", [INST, ...args], { env: r.env(extra), encoding: "utf8" });
const SHA40 = "a".repeat(40);

/** Another process (its own open file description) holding the lock until killed or `secs` pass. */
function holder(lock: string, secs = 60): Promise<ChildProcess> {
  const p = spawn("bash", ["-c", 'exec 8>>"$1"; flock -n 8 && echo held; sleep "$2"', "holder", lock, String(secs)], { stdio: ["ignore", "pipe", "ignore"] });
  procs.push(p);
  return new Promise((ok, no) => {
    p.stdout!.once("data", () => ok(p));
    p.once("exit", () => no(new Error("holder exited before it held the lock")));
  });
}
/** Run `inner` (a bash snippet using $S, $ST, $BK, $PREV) as a child of a parent that holds the lock the way `how` says. */
function underHolder(r: Rig, how: "runner" | "fd17", script: string, inner: string, extra: Record<string, string> = {}) {
  const pre =
    how === "runner"
      ? 'exec 9>"$ST/lock"; flock -n 9 || exit 0'                            // the old runner's exact pattern
      : 'exec 17>>"$ST/lock"; flock -n 17 || exit 0; export BPD_LOCK_FD=17';
  return spawnSync("bash", ["-c", `${pre}\n${inner}`], { env: r.env({ S: script, ST: r.st, BK: r.backup, PREV: r.prev, ...extra }), encoding: "utf8" });
}
/** Make the stub bb report whether the descriptor named by BPD_LOCK_FD is the lock's own, re-lockable one (what a lock-aware child needs). */
function nestedProbe(r: Rig) {
  const bb = readFileSync(join(r.root, "bb"), "utf8").replace(
    'echo "$@" >>',
    `if [ "$1 $2" = "plugin install" ]; then if [ -n "$BPD_LOCK_FD" ] && [ "$(stat -L -c %d:%i /proc/self/fd/$BPD_LOCK_FD 2>/dev/null)" = "$(stat -c %d:%i ${r.lock})" ] && flock -n "$BPD_LOCK_FD"; then echo NESTED-OK >> ${r.calls}; else echo NESTED-NO >> ${r.calls}; fi; fi\necho "$@" >>`,
  );
  writeFileSync(join(r.root, "bb"), bb);
}
const NOWAIT = { HOME_UI_LOCK_WAIT: "1" };
const HOLDS = /another deploy holds the lock/;
const isMk = userInfo().username === "mk";

describe("home-ui.sh: lock, rollback record", () => {
  it("with no lock held, rollback behaves as before and records the reinstalled build", () => {
    const r = rig();
    const out = hui(r, ["rollback", r.backup]);
    expect(out.status).toBe(0);
    expect(out.stdout).toContain("rolled back to " + r.prev);
    expect(r.log()).toContain("plugin install --yes " + r.prev);
    expect(r.sha()).toBe(r.wsha);
    expect(readFileSync(r.lock, "utf8")).toBe("");   // the lock file is only a lock
  });

  it("with no state directory it warns and still rolls back, writing nothing", () => {
    const r = rig({ noState: true });
    const out = hui(r, ["rollback", r.backup]);
    expect(out.status).toBe(0);
    expect(out.stderr).toContain("no state dir");
    expect(r.log()).toContain("plugin install --yes " + r.prev);
    expect(existsSync(r.st)).toBe(false);
  });

  it("works when `bb home moves` fails: it reads nothing from the tool or the moves export", () => {
    const r = rig({ movesFail: true });
    const out = hui(r, ["rollback", r.backup]);
    expect(out.status).toBe(0);
    expect(r.log()).not.toContain("home moves");
    expect(r.sha()).toBe(r.wsha);
  });

  it("leaves installed.sha alone when the reinstall fails, or the previous build is not a plugin checkout", () => {
    const r = rig();
    writeFileSync(join(r.st, "installed.sha"), "x".repeat(40) + "\n");
    writeFileSync(join(r.root, "bb"), "#!/bin/sh\ncase \"$1 $2\" in \"plugin install\") exit 1;; esac\nexit 0\n");
    const failed = hui(r, ["rollback", r.backup]);
    expect(failed.status).toBe(5);
    expect(r.sha()).toBe("x".repeat(40));

    const r2 = rig();
    writeFileSync(join(r2.st, "installed.sha"), "y".repeat(40) + "\n");
    writeFileSync(join(r2.backup, "previous-source.txt"), r2.root + "\n");   // a directory that is not <checkout>/integrations/bb-plugin-autarch
    const out = hui(r2, ["rollback", r2.backup]);
    expect(out.status).toBe(0);
    expect(out.stderr).toContain("installed.sha was not updated");
    expect(r2.sha()).toBe("y".repeat(40));
  });

  for (const [name, args] of [
    ["rollback", (r: Rig) => ["rollback", r.backup]],
    ["preview", (r: Rig) => ["preview", "--path", r.cur]],
    ["deploy", () => ["deploy", "--commit", SHA40]],
  ] as const) {
    it(`${name} waits on a held lock for the cap, then stops without touching bb`, async () => {
      const r = rig();
      writeFileSync(join(r.cur, "migrations.ts"), "export const CODE_VERSION = 3;\n");
      await holder(r.lock);
      const t0 = Date.now();
      const out = hui(r, args(r), NOWAIT);
      expect(out.status).toBe(75);
      expect(out.stderr).toMatch(HOLDS);
      expect(out.stderr).toMatch(/pid \d+/);
      expect(Date.now() - t0).toBeGreaterThanOrEqual(900);
      expect(r.log()).toBe("");
      expect(r.sha()).toBeNull();
    });
  }

  it("proceeds once the holder lets go inside the cap", async () => {
    const r = rig();
    await holder(r.lock, 2);
    const out = hui(r, ["rollback", r.backup], { HOME_UI_LOCK_WAIT: "30" });
    expect(out.status).toBe(0);
    expect(out.stderr).toContain("waiting for the destination lock");
    expect(r.sha()).toBe(r.wsha);
  });

  it("check and migrate-check take no lock", async () => {
    const r = rig();
    await holder(r.lock);
    writeFileSync(join(r.cur, "migrations.ts"), "export const CODE_VERSION = 3;\n");
    writeFileSync(join(r.cur, "package.json"), "{}");
    mkdirSync(join(r.cur, "node_modules"), { recursive: true });
    const out = hui(r, ["check", "--path", r.cur], NOWAIT);
    expect(out.status).toBe(0);
    expect(out.stderr).not.toMatch(HOLDS);
    const mig = hui(r, ["migrate-check", "--path", r.cur], NOWAIT);
    expect(mig.stderr).not.toMatch(HOLDS);
    expect(mig.stderr).not.toContain("waiting for the destination lock");
  });

  it("as a child of the old runner (fd 9), rollback runs inherited with no wait and no deadlock", () => {
    const r = rig();
    const out = underHolder(r, "runner", HUI, 'bash "$S" rollback "$BK"', NOWAIT);
    expect(out.status).toBe(0);
    expect(out.stderr).not.toContain("waiting for the destination lock");
    expect(r.sha()).toBe(r.wsha);
  });

  it("as a child of a holder that exports BPD_LOCK_FD=17, rollback runs inherited", () => {
    const r = rig();
    const out = underHolder(r, "fd17", HUI, 'bash "$S" rollback "$BK"', NOWAIT);
    expect(out.status).toBe(0);
    expect(out.stderr).not.toContain("waiting for the destination lock");
  });

  it("an independently opened lock stays excluded while the runner's child works, and after it exits", () => {
    const r = rig();
    // Slow the reinstall so the independent attempt lands while the child is mid-rollback.
    const bb = readFileSync(join(r.root, "bb"), "utf8").replace("echo \"$@\" >>", "[ \"$1 $2\" = \"plugin install\" ] && sleep 1\necho \"$@\" >>");
    writeFileSync(join(r.root, "bb"), bb);
    const inner = [
      'bash "$S" rollback "$BK" & c=$!',
      "sleep 0.4",
      '(exec 8>>"$ST/lock"; flock -n 8) && echo INDEPENDENT-DURING || echo excluded-during',
      "wait $c; echo CHILD=$?",
      '(exec 8>>"$ST/lock"; flock -n 8) && echo INDEPENDENT-AFTER || echo excluded-after',
    ].join("\n");
    const out = underHolder(r, "runner", HUI, inner);
    expect(out.stdout).toContain("excluded-during");
    expect(out.stdout).toContain("CHILD=0");
    expect(out.stdout).toContain("excluded-after");
    expect(out.stdout).not.toMatch(/INDEPENDENT/);
    expect(r.log()).toContain("plugin install --yes " + r.prev);
    expect(r.sha()).toBe(r.wsha);
  });

  it("an inherited descriptor on the lock's inode that does not hold it: waits on that same descriptor, then proceeds", async () => {
    const r = rig();
    await holder(r.lock, 2);
    const out = spawnSync("bash", ["-c", 'exec 17>>"$ST/lock"; bash "$S" rollback "$BK"'], { env: r.env({ S: HUI, ST: r.st, BK: r.backup, BPD_LOCK_FD: "17", HOME_UI_LOCK_WAIT: "30" }), encoding: "utf8" });
    expect(out.status).toBe(0);
    expect(out.stderr).toContain("waiting for the destination lock");
    expect(r.sha()).toBe(r.wsha);
  });

  it("does not trust the environment alone", async () => {
    // BPD_LOCK_FD names a closed descriptor: the script opens its own and waits on the real holder.
    const r = rig();
    await holder(r.lock);
    const closed = hui(r, ["rollback", r.backup], { ...NOWAIT, BPD_LOCK_FD: "17" });
    expect(closed.status).toBe(75);
    // ... a descriptor on another file: no inode match, same result.
    const other = spawnSync("bash", ["-c", 'exec 17>>"$ST/other"; bash "$S" rollback "$BK"'], { env: r.env({ S: HUI, ST: r.root, BK: r.backup, BPD_LOCK_FD: "17", ...NOWAIT }), encoding: "utf8" });
    expect(other.status).toBe(75);
    // ... the lock file's own inode on that descriptor, but not locked by us: flock -n fails and the wait runs out.
    const unlocked = spawnSync("bash", ["-c", 'exec 17>>"$ST/lock"; bash "$S" rollback "$BK"'], { env: r.env({ S: HUI, ST: r.st, BK: r.backup, BPD_LOCK_FD: "17", ...NOWAIT }), encoding: "utf8" });
    expect(unlocked.status).toBe(75);
    expect(r.log()).toBe("");
  });

  it("exports the descriptor it holds, so a lock-aware child reuses it instead of waiting on its parent", () => {
    const standalone = rig();
    nestedProbe(standalone);
    expect(hui(standalone, ["rollback", standalone.backup], NOWAIT).status).toBe(0);
    expect(standalone.log()).toContain("NESTED-OK");
    const runner = rig();
    nestedProbe(runner);
    const out = underHolder(runner, "runner", HUI, 'bash "$S" rollback "$BK"', NOWAIT);
    expect(out.status).toBe(0);
    expect(runner.log()).toContain("NESTED-OK");
  });

  it("a non-numeric BPD_LOCK_FD falls back to fd 9", () => {
    const r = rig();
    const out = underHolder(r, "runner", HUI, 'bash "$S" rollback "$BK"', { ...NOWAIT, BPD_LOCK_FD: "../x" });
    expect(out.status).toBe(0);
  });

  it("HOME_UI_LOCK=skip lets rollback run on a held lock, with the race risk printed; other commands still lock", async () => {
    const r = rig();
    await holder(r.lock);
    const out = hui(r, ["rollback", r.backup], { ...NOWAIT, HOME_UI_LOCK: "skip" });
    expect(out.status).toBe(0);
    expect(out.stderr).toMatch(/RACE RISK/);
    expect(r.sha()).toBe(r.wsha);
    const dep = hui(r, ["deploy", "--commit", SHA40], { ...NOWAIT, HOME_UI_LOCK: "skip" });
    expect(dep.status).toBe(75);
  });
});

describe("install-home-your-move.sh: lock", () => {
  for (const mode of ["--go", "--rollback"]) {
    it(`${mode} waits on a held lock for the cap, then stops before resolving anything`, async () => {
      const r = rig();
      await holder(r.lock);
      const t0 = Date.now();
      const args = mode === "--go" ? ["--go", "--commit", SHA40] : ["--rollback", "--previous-plugin", r.prev];
      const out = inst(r, args, NOWAIT);
      expect(out.status).toBe(75);
      expect(out.stderr).toMatch(HOLDS);
      expect(out.stderr).toMatch(/pid \d+/);
      expect(Date.now() - t0).toBeGreaterThanOrEqual(900);
      expect(r.log()).toBe("");
    });
  }

  it("--rollback with HOME_UI_LOCK=skip does not wait; --go still does", async () => {
    const r = rig();
    await holder(r.lock);
    const rb = inst(r, ["--rollback"], { ...NOWAIT, HOME_UI_LOCK: "skip" });
    expect(rb.stderr).toMatch(/RACE RISK/);
    expect(rb.status).not.toBe(75);
    const go = inst(r, ["--go", "--commit", SHA40], { ...NOWAIT, HOME_UI_LOCK: "skip" });
    expect(go.status).toBe(75);
  });

  it("--check takes no lock", async () => {
    const r = rig();
    await holder(r.lock);
    const out = inst(r, ["--check"], { ...NOWAIT, HOME_YOUR_MOVE_TEST_DATA_DIR: join(r.data, "plugins", "autarch"), HOME_YOUR_MOVE_TEST_BB: join(r.root, "bb") });
    expect(out.stderr).not.toMatch(HOLDS);
    expect(out.status).toBe(0);
  });

  it("a closed or non-matching BPD_LOCK_FD makes it open its own descriptor and wait", async () => {
    const r = rig();
    await holder(r.lock);
    expect(inst(r, ["--rollback"], { ...NOWAIT, BPD_LOCK_FD: "17" }).status).toBe(75);
    const other = spawnSync("bash", ["-c", 'exec 17>>"$ST/other"; bash "$S" --rollback'], { env: r.env({ S: INST, ST: r.root, BPD_LOCK_FD: "17", ...NOWAIT }), encoding: "utf8" });
    expect(other.status).toBe(75);
  });

  // Past the lock, --go and --rollback insist on running as the service user (or root); these need that account.
  it("the installer's lock block exports the descriptor it holds, standalone and under the runner", () => {
    // Run the block's own text (the real script's children sit behind `env -i`, which would hide the variable).
    const lines = readFileSync(INST, "utf8").split("\n");
    const from = lines.findIndex((l) => l.startsWith("# shared destination lock: begin"));
    const to = lines.findIndex((l) => l.startsWith("# shared destination lock: end"));
    const probe = 'bash -c \'if [ -n "$BPD_LOCK_FD" ] && [ "$(stat -L -c %d:%i /proc/self/fd/$BPD_LOCK_FD 2>/dev/null)" = "$(stat -c %d:%i "$HOME_UI_STATE_DIR/lock")" ] && flock -n "$BPD_LOCK_FD"; then echo NESTED-OK; else echo NESTED-NO; fi\'';
    const run = (r: Rig, pre: string) => {
      const h = join(r.root, "harness.sh");
      writeFileSync(h, ["set -e", "MKHOME=/nonexistent", "MODE=rollback", ...lines.slice(from, to + 1), probe, ""].join("\n"));
      return spawnSync("bash", ["-c", `${pre}\nbash "$H"`], { env: r.env({ H: h, ST: r.st, ...NOWAIT }), encoding: "utf8" });
    };
    const alone = run(rig(), "true");
    expect(alone.stdout).toContain("NESTED-OK");
    const under = run(rig(), 'exec 9>"$ST/lock"; flock -n 9 || exit 0');
    expect(under.stdout).toContain("NESTED-OK");
    expect(under.stderr).not.toContain("waiting for the destination lock");
  });

  describe.skipIf(!isMk)("past the lock (as the service user)", () => {
    const rollback = (r: Rig) => ["--rollback", "--previous-plugin", r.prev];

    it("--rollback with no lock held behaves as before", { timeout: 60_000 }, () => {
      const r = rig();
      const out = inst(r, rollback(r));
      expect(out.status).toBe(0);
      expect(r.log()).toContain("plugin install --yes " + r.prev);
    });

    it("--go with no lock held reaches its own checks (a wrong commit stops it at exit 2)", () => {
      const r = rig();
      const out = inst(r, ["--go", "--commit", SHA40]);
      expect(out.status).toBe(2);
      expect(r.log()).toBe("");
    });

    it("as children of the old runner's fd 9, --go and then --rollback run inherited, with no deadlock", { timeout: 60_000 }, () => {
      const r = rig();
      // The runner's exact pattern: take fd 9, then run the installer --go, then --rollback.
      const out = underHolder(r, "runner", INST, `bash "$S" --go --commit ${SHA40}; echo go-rc=$?; bash "$S" --rollback --previous-plugin "$PREV"; echo rb-rc=$?`, NOWAIT);
      expect(out.stdout).toContain("go-rc=2");
      expect(out.stdout).toContain("rb-rc=0");
      expect(out.stderr).not.toContain("waiting for the destination lock");
      expect(r.log()).toContain("plugin install --yes " + r.prev);
    });

    it("as a child of a holder that exports BPD_LOCK_FD=17, --rollback runs inherited", { timeout: 60_000 }, () => {
      const r = rig();
      const out = underHolder(r, "fd17", INST, 'bash "$S" --rollback --previous-plugin "$PREV"; echo rb-rc=$?', NOWAIT);
      expect(out.stdout).toContain("rb-rc=0");
      expect(out.stderr).not.toContain("waiting for the destination lock");
    });
  });
});

describe("the legacy scripts differ from the b05ebaf baseline only by the lock wrapper, the one installed.sha write, the --engine tool opt-in, the install-error recheck and the redundant-enable skip", () => {
  const strip = (text: string, tag: string) => {
    const out: string[] = [];
    let inside = false;
    for (const raw of text.split("\n")) {
      const line = raw.trimStart();
      if (line.startsWith(`# ${tag}: begin`)) { expect(inside).toBe(false); inside = true; continue; }
      if (line.startsWith(`# ${tag}: end`)) { expect(inside).toBe(true); inside = false; continue; }
      if (!inside) out.push(raw);
    }
    expect(inside).toBe(false);
    return out.join("\n");
  };
  const count = (text: string, tag: string) => text.split("\n").filter((l) => l.trimStart().startsWith(`# ${tag}: begin`)).length;

  for (const [file, shaWrites, engineBlocks, recheckBlocks, skipBlocks] of [["home-ui.sh", 1, 6, 3, 2], ["install-home-your-move.sh", 0, 0, 0, 0]] as const) {
    it(`${file}: removing the marked blocks gives the baseline byte for byte`, () => {
      const now = readFileSync(resolve(__dirname, "../../../scripts", file), "utf8");
      expect(count(now, "shared destination lock")).toBe(1);
      expect(count(now, "installed.sha write")).toBe(shaWrites);
      expect(count(now, "--engine tool")).toBe(engineBlocks);   // the opt-in is additive: every line of it sits in a marked block
      expect(count(now, "install-error recheck")).toBe(recheckBlocks);   // additive: the install runs in a marked block, the unchanged line follows
      expect(count(now, "redundant-enable skip")).toBe(skipBlocks);   // additive: a stand-in bb answers the unchanged enable line when the plugin already runs
      const rest = strip(strip(strip(strip(strip(now, "shared destination lock"), "installed.sha write"), "--engine tool"), "install-error recheck"), "redundant-enable skip");
      expect(rest).toBe(readFileSync(join(FIXTURES, file), "utf8"));
    });

    it(`${file}: the fixture is the b05ebaf script (when the history is present)`, () => {
      const probe = spawnSync("git", ["cat-file", "-e", `${BASELINE_COMMIT}^{commit}`], { cwd: __dirname });
      if (probe.status !== 0) return;   // a shallow checkout has no such commit; the fixture is then the only baseline
      const shown = spawnSync("git", ["show", `${BASELINE_COMMIT}:scripts/${file}`], { cwd: __dirname, encoding: "utf8" });
      expect(readFileSync(join(FIXTURES, file), "utf8")).toBe(shown.stdout);
    });
  }
});
