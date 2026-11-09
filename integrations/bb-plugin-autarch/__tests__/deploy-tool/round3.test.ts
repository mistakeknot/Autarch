import { afterEach, describe, expect, it } from "vitest";
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { Rig, gateEntry, waitFor, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

const deployArgs = (sha: string, mode = "agent", extra: string[] = []) => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", mode, ...extra];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const exited = (p: { once: (e: "exit", f: (c: number | null) => void) => unknown }): Promise<number | null> => new Promise((res) => p.once("exit", (c) => res(c)));
const sabotageSha = () => {
  rmSync(join(rig.stateDir, "installed.sha"), { force: true, recursive: true });
  mkdirSync(join(rig.stateDir, "installed.sha"));
};

describe("a build directory that already exists is never touched", () => {
  it("the refusal leaves the existing directory and what is in it", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const dir = join(rig.buildsRoot, "fixture", `${Y.slice(0, 12)}-20260101T000000Z`);
    mkdirSync(dir, { recursive: true });
    writeFileSync(join(dir, "keep.txt"), "mine\n");
    const r = rig.cli(deployArgs(Y), { env: { BPD_UTC_STAMP: "20260101T000000Z" } });
    expect(r.status).not.toBe(0);
    expect(readFileSync(join(dir, "keep.txt"), "utf8")).toBe("mine\n");
    expect(rig.mutations()).toEqual([]);
  });
});

describe("the config baseline is taken just before the LAST config key", () => {
  const plugin = {
    configAfterEnable: [
      { key: "first", value: "1" },
      { key: "second", value: "2" },
    ],
    health: [{ kind: "log-line", name: "ready", file: "{stateDir}/app.log", pattern: "ready", since: "config", timeoutSec: 1 }],
  };
  const log = (): string => join(rig.stateDir, "app.log");

  it("a line written while the first key was set does not satisfy the check", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    rig.inject({ appendOn: { "config:first": [{ file: log(), text: "ready from first key\n" }] } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(msg(r)).toMatch(/no .*ready.* line within/);
  });

  it("a line written when the last key was set satisfies it", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    rig.inject({ appendOn: { "config:second": [{ file: log(), text: "ready from second key\n" }] } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
  });
});

describe("a confirmed first run from the installed engine", () => {
  const changeControl = (): string => rig.commit({ "scripts/home-gates.sh": `${readFileSync(join(rig.upstream, "scripts", "home-gates.sh"), "utf8")}\n# reviewed change\n` });

  it("is paused without the decision and goes through with mk's pick naming the commit", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = changeControl();
    const paused = rig.cli(deployArgs(Y));
    expect(paused.status, msg(paused)).toBe(20);
    expect(rig.mutations()).toEqual([]);
    rig.addDecision("d-good", `first run of ${Y}`, "confirm-first-run", "mk");
    const ok = rig.cli(deployArgs(Y, "agent", ["--confirmed-first-run", "d-good"]));
    expect(ok.status, msg(ok)).toBe(0);
    expect(rig.installedSha()).toBe(Y);
  });

  it("a decision that names another commit does not lift the pause", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = changeControl();
    rig.addDecision("d-wrong", `first run of ${"9".repeat(40)}`, "confirm-first-run", "mk");
    const r = rig.cli(deployArgs(Y, "agent", ["--confirmed-first-run", "d-wrong"]));
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });
});

describe("children find the bb the tool uses", () => {
  it("the directory of BB_CLI leads the PATH of a build command", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { build: [["bash", "-c", 'echo "$PATH" > "$1/path.log"', "x", "{stateDir}"]] } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y), { env: { BB_PLUGIN_DEPLOY_BB: undefined, BB_CLI: rig.stub } });
    expect(r.status, msg(r)).toBe(0);
    expect(readFileSync(join(rig.stateDir, "path.log"), "utf8").trim().split(":")[0]).toBe(dirname(rig.stub));
  });
});

describe("rollback bookkeeping", () => {
  it("a compatibility-file failure after the live rollback exits 1 and names the repair command", () => {
    rig = new Rig({ installedKind: "git" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    const prev = (rig.record() as { previous: { buildRoot: string; commit: string } }).previous;
    sabotageSha();
    const r = rig.cli(["rollback", "--to", "previous"]);
    expect(r.status, msg(r)).toBe(1);
    expect(rig.state().source).toBe(join(prev.buildRoot, "plug"));
    expect(msg(r)).toMatch(/bookkeeping write failed/);
    expect(msg(r)).toMatch(/record --reconcile/);
    expect((rig.record() as { installed: { commit: string } }).installed.commit).toBe(prev.commit);
  });
});

describe("record --reconcile", () => {
  it("brings a stale installed.sha in line with what is live", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    writeFileSync(join(rig.stateDir, "installed.sha"), `${rig.W}\n`);
    const r = rig.cli(["record", "--reconcile"]);
    expect(r.status, msg(r)).toBe(0);
    expect(rig.installedSha()).toBe(Y);
    expect(r.json?.status).toBeTruthy();
  });

  it("reports a failure to repair installed.sha instead of crashing or claiming success", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    sabotageSha();
    const r = rig.cli(["record", "--reconcile"]);
    expect(r.status, msg(r)).toBe(1);
    expect(msg(r)).toMatch(/installed\.sha/);
  });
});

describe("preview", () => {
  const plugin = { previewGate: [gateEntry("preview-gate", "preview")] };

  it("a preview from a clone engine in agent mode is refused before bb is touched", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const engine = rig.makeBuild(Y, "git");
    const r = rig.cli(["preview", "--commit", Y], { engineRoot: engine, pluginRoot: engine });
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });

  it("takes a data backup and records a preview transaction while running, none afterwards", async () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    rig.seedData(rig.bbData);
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ pauseAfter: "install" });
    const p = rig.spawnCli(["preview", "--commit", Y]);
    const done = exited(p);
    const paused = join(rig.instances[0].stateDir, "paused");
    expect(waitFor(() => existsSync(paused), 20_000)).toBe(true);
    const mid = rig.record() as { transaction?: { kind: string; state: string; backup: string | null } };
    expect(mid.transaction).toMatchObject({ kind: "preview", state: "mutating" });
    writeFileSync(join(rig.instances[0].stateDir, "release"), "");
    expect(await done).toBe(0);
    const after = rig.record() as { transaction?: unknown; installed: { commit: string }; history: { event: string }[] };
    expect(after.transaction).toBeUndefined();
    expect(after.installed.commit).toBe(rig.W);
    expect(rig.installedSha()).toBe(rig.W);
    expect(after.history.map((h) => h.event)).not.toContain("preview");
  }, 60_000);
});
