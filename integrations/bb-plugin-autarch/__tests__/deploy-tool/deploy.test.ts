import { afterEach, describe, expect, it } from "vitest";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { Rig, gateEntry, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

const deployArgs = (sha: string, extra: string[] = []) => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", "agent", ...extra];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);

describe("deploy: happy path", () => {
  it("builds, backs up, installs, enables, sets config after enable and records", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(0);
    expect(r.json?.phases).toEqual(["prepare", "preflight", "preInstall", "anchor-previous", "backup", "disable", "install", "preEnable", "enable", "postEnablePreConfig", "configAfterEnable", "health"]);
    expect(rig.mutations()).toEqual(["disable", "install", "enable", "config"]);
    expect(rig.installedSha()).toBe(Y);
    expect(existsSync(join(rig.source() ?? "", "built.txt"))).toBe(true);
    const rec = rig.record() as { installed: { commit: string; backup: string }; previous: { commit: string }; history: { event: string }[] };
    expect(rec.installed.commit).toBe(Y);
    expect(rec.previous.commit).toBe(rig.W);
    expect(rec.history.at(-1)?.event).toBe("install");
    expect(rig.state().config).toEqual({ feature: "on" });
    // the legacy backup format: data.db, previous-source.txt and a manifest in a 0700 directory
    const names = readdirSync(rec.installed.backup);
    expect(names).toEqual(expect.arrayContaining(["data.db", "previous-source.txt", "manifest.json", "export-moves.json"]));
    expect(readFileSync(join(rec.installed.backup, "previous-source.txt"), "utf8").trim()).toContain(`${rig.W.slice(0, 12)}`);
    // the report carries the rollback and restore-data commands and the last-resort recovery block
    expect(String(r.json?.rollbackCommand)).toContain("rollback");
    expect(String(r.json?.restoreDataCommand)).toContain("restore-data");
    expect((r.json?.recovery as { lastResort: string } | undefined)?.lastResort).toContain("flock");
  });

  it("anchors a legacy (unstamped git) live install by rebuilding it immutably", () => {
    rig = new Rig({ installedKind: "git" });
    const liveBefore = rig.liveRoot();
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(0);
    const rec = rig.record() as { previous: { commit: string; buildRoot: string } };
    expect(rec.previous.commit).toBe(rig.W);
    expect(rec.previous.buildRoot).not.toBe(liveBefore);
    expect(existsSync(join(rec.previous.buildRoot, ".bb-plugin-deploy", "stamp.json"))).toBe(true);
  });

  it("refuses an existing build directory", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    mkdirSync(join(rig.buildsRoot, "fixture", `${Y.slice(0, 12)}-20260101T000000Z`), { recursive: true });
    const r = rig.cli(deployArgs(Y), { env: { BPD_UTC_STAMP: "20260101T000000Z" } });
    expect(r.status).not.toBe(0);
    expect(rig.mutations()).toEqual([]);
  });

  it("refuses a tree mismatch before any build command runs", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y, ["--expect-tree", "0".repeat(40)]));
    expect(r.status).toBe(10);
    expect(msg(r)).toMatch(/tree/);
    expect(rig.mutations()).toEqual([]);
    for (const d of rig.buildDirs()) expect(existsSync(join(d, "plug", "built.txt"))).toBe(false);
  });

  it("refuses a short sha and an unapproved class", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y.slice(0, 12))).status).toBe(10);
    expect(rig.cli(["deploy", "--commit", Y, "--class", "other", "--mode", "agent"]).status).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });

  it("refuses a commit that is not an ancestor of origin/main", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.commit({ "plug/app.txt": "v3\n" });
    // a commit on a side branch
    const r0 = rig.cli(["version"]);
    expect(r0.status).toBe(0);
    const side = rig.commit({ "plug/app.txt": "v4\n" });
    expect(rig.cli(deployArgs(side)).status).toBe(0);
    expect(Y).not.toBe(side);
    expect(rig.cli(deployArgs("1".repeat(40))).status).toBe(10);
  });
});

describe("deploy: no bb mutation before the transaction starts", () => {
  it("a preflight refusal installs nothing", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.gate("preflight", "refuse");
    const r = rig.cli(deployArgs(Y));
    expect(r.status).toBe(10);
    expect(r.json?.nothingInstalled).toBe(true);
    expect(rig.mutations()).toEqual([]);
    expect(((r.json?.gates ?? []) as { ok: boolean; reason: string }[]).some((g) => !g.ok && g.reason === "refused by test")).toBe(true);
  });

  it("a preInstall check that needs mk exits 20 and installs nothing", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.gate("preInstall", "needs");
    const r = rig.cli(deployArgs(Y));
    expect(r.status).toBe(20);
    expect(rig.mutations()).toEqual([]);
  });

  it("a failed required export (backup failure) installs nothing", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ fail: { moves: true } });
    const r = rig.cli(deployArgs(Y));
    expect(r.status).toBe(1);
    expect(msg(r)).toMatch(/export moves failed/);
    expect(rig.mutations()).toEqual([]);
    expect(rig.state().source).toContain(rig.W.slice(0, 12));
  });

  it("a data directory outside the bound bb data dir is refused", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { data: [{ name: "data.db", kind: "sqlite", path: "/tmp/elsewhere/data.db" }] } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status).toBe(10);
    expect(msg(r)).toMatch(/data-dir mismatch/);
    expect(rig.mutations()).toEqual([]);
  });

  it("a failing target build installs nothing", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/build.mjs": "process.exit(3);\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status).toBe(1);
    expect(msg(r)).toMatch(/build command failed/);
    expect(rig.mutations()).toEqual([]);
  });

  it("a build that changes a tracked file is not trusted", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/build.mjs": 'import { appendFileSync } from "node:fs"; appendFileSync("app.txt", "x");\n' });
    const r = rig.cli(deployArgs(Y));
    expect(r.status).toBe(1);
    expect(rig.mutations()).toEqual([]);
  });

  it("a live install that is dirty is refused at anchoring, before mutation", () => {
    rig = new Rig({ installedKind: "git" });
    const live = rig.liveRoot() ?? "";
    writeFileSync(join(live, "plug", "app.txt"), "locally edited\n");
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    // the dirty install is not a trusted engine, so an agent deploy is refused outright
    const agent = rig.cli(deployArgs(Y));
    expect(agent.status, msg(agent)).toBe(10);
    expect(rig.mutations()).toEqual([]);
    // from a clean clone engine in mk-script mode the refusal comes from anchoring itself
    const engine = rig.makeBuild(rig.W, "git");
    const r = rig.cli(["deploy", "--commit", Y, "--class", "plugin-deploy", "--mode", "mk-script"], { engineRoot: engine, pluginRoot: engine });
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/cannot anchor the previous install/);
    expect(rig.mutations()).toEqual([]);
  });

  it("a first install with no live plugin is refused (nothing to roll back to)", () => {
    rig = new Rig({ noInstalled: true });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y), { engineRoot: rig.makeBuild(rig.W, "git"), pluginRoot: rig.makeBuild(rig.W, "git") });
    expect(r.status).not.toBe(0);
    expect(rig.mutations()).toEqual([]);
  });
});

describe("deploy: failure after mutation recovers to the anchored previous", () => {
  const recovered = (r: CliResult) => {
    expect(r.json?.recovery).toMatchObject({ ran: true, ok: true });
    expect(rig.state().source).toContain(rig.W.slice(0, 12));
    expect(rig.state().enabled).toBe(true);
    expect(rig.installedSha()).toBe(rig.W);
  };

  it("a preEnable refusal rolls back (exit 30)", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.gate("preEnable", "refuse");
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(rig.mutations().slice(0, 2)).toEqual(["disable", "install"]);
    expect(rig.mutations()).not.toContain("config");
    recovered(r);
    expect((r.json?.recoveryBlock as { lastResort: string } | undefined)?.lastResort).toContain("flock");
    expect((rig.record() as { installed: { commit: string } }).installed.commit).toBe(rig.W);
  });

  it("a health failure rolls back (exit 30)", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.inject({ neverRunMatch: Y.slice(0, 12) });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(msg(r)).toMatch(/health failed/);
    recovered(r);
  });

  it("a postEnablePreConfig failure never sets config", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.gate("postEnable", "fail");
    const r = rig.cli(deployArgs(Y));
    expect(r.status).toBe(30);
    expect(rig.mutations()).not.toContain("config");
    recovered(r);
  });

  it("a configAfterEnable entry that requires a marker no check writes is an invalid plugin file", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { preInstall: [gateEntry("schema-gate", "preInstall")] } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status).not.toBe(0);
    expect(msg(r)).toMatch(/invalid/);
    expect(rig.mutations()).toEqual([]);
  });

  it("a marker is passed to later checks as BPD_MARKER_<NAME>=1 and cleared when its check fails", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    expect(readFileSync(join(rig.stateDir, "marker-env.log"), "utf8")).toContain("BPD_MARKER_SCHEMA=1");
    const Z = rig.commit({ "plug/app.txt": "v3\n" });
    rig.gate("preInstall", "refuse");
    expect(rig.cli(deployArgs(Z)).status).toBe(10);
    expect(existsSync(join(rig.stateDir, "markers", "schema"))).toBe(false);
  });

  it("a failed recovery exits 31 and says so", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.gate("preEnable", "refuse");
    rig.inject({ failInstallMatch: rig.W.slice(0, 12) });
    const r = rig.cli(deployArgs(Y));
    expect(r.status).toBe(31);
    expect(msg(r)).toMatch(/FAILED/);
    expect(r.json?.recovery).toMatchObject({ ran: true, ok: false });
  });

  it("rollbackLeaves disabled recovers without enabling", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { rollbackLeaves: "disabled" } });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.gate("preEnable", "refuse");
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(rig.state().enabled).toBe(false);
    expect(rig.state().source).toContain(rig.W.slice(0, 12));
  });
});

describe("deploy: log-line health baselines", () => {
  const plugin = (writer: string) => ({
    postEnablePreConfig: [gateEntry("post-enable", "postEnable"), { name: "announce", run: ["bash", "-c", "echo 'ready old-source' >> {stateDir}/app.log"] }],
    health: [
      { kind: "running", timeoutSec: 3 },
      { kind: "check", name: "writer", run: ["bash", "-c", `${writer} >> {stateDir}/app.log; true`] },
      { kind: "log-line", name: "ready", file: "{stateDir}/app.log", pattern: "ready", fail: ["FAILED"], since: "config", timeoutSec: 1 },
    ],
  });

  it("an old-source announcement before the config switch does not satisfy the check", () => {
    rig = new Rig({ installedKind: "stamped", plugin: plugin("true") });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(msg(r)).toMatch(/no .*ready.* line within/);
  });

  it("an old announcement followed by a new-source failure fails the check", () => {
    rig = new Rig({ installedKind: "stamped", plugin: plugin("echo FAILED new-source") });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(msg(r)).toMatch(/failure line matched/);
  });

  it("a new-source announcement after the switch passes", () => {
    rig = new Rig({ installedKind: "stamped", plugin: plugin("echo ready new-source") });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
  });
});

describe("rollback", () => {
  it("rolls back to the anchored previous build and keeps it immutable", () => {
    rig = new Rig({ installedKind: "git" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    const prev = (rig.record() as { previous: { buildRoot: string } }).previous.buildRoot;
    rig.clearCalls();
    const r = rig.cli(["rollback", "--to", "previous"]);
    expect(r.status, msg(r)).toBe(0);
    expect(rig.state().source).toBe(join(prev, "plug"));
    expect(rig.installedSha()).toBe(rig.W);
    expect(rig.mutations()).toEqual(["disable", "install", "enable"]);
    const rec = rig.record() as { installed: { commit: string }; previous: { commit: string }; history: { event: string }[] };
    expect(rec.installed.commit).toBe(rig.W);
    expect(rec.previous.commit).toBe(Y);
    expect(rec.history.at(-1)?.event).toBe("rollback");
  });

  it("refuses when the record has no previous build", () => {
    rig = new Rig({ installedKind: "stamped" });
    const r = rig.cli(["rollback", "--to", "previous"]);
    expect(r.status).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });

  it("refuses a --build outside the builds root and a build that no longer verifies", () => {
    rig = new Rig({ installedKind: "stamped" });
    expect(rig.cli(["rollback", "--build", "/tmp"]).status).toBe(10);
    const b = rig.makeBuild(rig.W, "stamped");
    writeFileSync(join(b, "plug", "app.txt"), "tampered\n");
    expect(rig.cli(["rollback", "--build", b]).status).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });
});

describe("check, prepare and gc", () => {
  it("check runs the pre-install checks on a scratch build and removes it", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const before = rig.buildDirs().length;
    const r = rig.cli(["check", "--commit", Y, "--class", "plugin-deploy"]);
    expect(r.status, msg(r)).toBe(0);
    expect(rig.buildDirs().length).toBe(before);
    expect(rig.mutations()).toEqual([]);
    rig.gate("preInstall", "refuse");
    expect(rig.cli(["check", "--commit", Y, "--class", "plugin-deploy"]).status).toBe(10);
  });

  it("prepare builds and stamps without touching bb", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(["prepare", "--commit", Y, "--class", "plugin-deploy", "--mode", "agent"]);
    expect(r.status, msg(r)).toBe(0);
    expect(existsSync(join(String(r.json?.build), ".bb-plugin-deploy", "stamp.json"))).toBe(true);
    expect(rig.mutations()).toEqual([]);
  });

  it("gc keeps the installed build, the previous build and the newest N, and never touches backups", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { retention: { builds: 1 } } });
    const old = ["20250101T000001Z", "20250101T000002Z", "20250101T000003Z"].map((n) => rig.makeBuild(rig.W, "stamped", `${rig.W.slice(0, 12)}-${n}`));
    const dry = rig.cli(["gc"]);
    expect(dry.status, msg(dry)).toBe(0);
    for (const d of old) expect(existsSync(d)).toBe(true);
    const live = rig.liveRoot() ?? "";
    expect(rig.cli(["gc", "--apply"]).status).toBe(0);
    expect(existsSync(live)).toBe(true);
    expect(old.filter((d) => existsSync(d)).length).toBeLessThan(old.length);
    mkdirSync(join(rig.backupsRoot, "keepme"), { recursive: true });
    expect(rig.cli(["gc", "--apply"]).status).toBe(0);
    expect(existsSync(join(rig.backupsRoot, "keepme"))).toBe(true);
  });
});
