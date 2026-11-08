import { afterEach, describe, expect, it } from "vitest";
import { appendFileSync, existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { checkRequest, updateInfo } from "../../updates";
import { Rig, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

const CLICK = "2026-01-02T00:00:00.000Z";
const deployArgs = (sha: string, mode = "agent") => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", mode];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
/** The plugin's own reader, over the files the tool wrote (no systemd timer, no /proc/locks lookups beyond the lock file). */
const info = () => updateInfo(rig.stateDir, rig.requestRaw(), { now: new Date().toISOString(), timerFile: join(rig.root, "no-timer"), procLocks: "/dev/null" });
const withheld = () => readdirSync(rig.stateDir).filter((n) => n.startsWith("status.json.withheld-"));

describe("publication through the plugin's own updateInfo / checkRequest", () => {
  it("an agent deploy with no pending click leaves an older successful result alone and shows the new installed commit", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y1 = rig.commit({ "plug/app.txt": "v2\n" });
    rig.approve(Y1);
    expect(rig.cli(deployArgs(Y1, "runner")).status).toBe(0);
    const firstResult = rig.readJson("result.json");
    expect(firstResult?.sha).toBe(Y1);
    const Y2 = rig.commit({ "plug/app.txt": "v3\n" });
    expect(rig.cli(deployArgs(Y2)).status).toBe(0);
    expect(rig.readJson("result.json")).toEqual(firstResult);
    const i = info();
    expect(i.status?.installed).toBe(Y2);
    expect(i.status?.latest).toBe(Y2);
    expect(i.available).toBe(false);
    expect(i.pending).toBe(false);
  });

  it("an agent deploy of the sha a pending click names writes result.json and settles the click", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.setRequest(Y, CLICK);
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(0);
    const res = rig.readJson("result.json") as { sha: string; ok: boolean };
    expect(res.sha).toBe(Y);
    expect(res.ok).toBe(true);
    const i = info();
    expect(i.pending).toBe(false);
    expect(i.status?.installed).toBe(Y);
  });

  it("a pending click for a different sha than the one deployed gets no result and stays pending", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const Z = rig.commit({ "plug/app.txt": "v3\n" });
    rig.setRequest(Z, CLICK);
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    expect(existsSync(join(rig.stateDir, "result.json"))).toBe(false);
    const i = info();
    expect(i.status?.installed).toBe(Y);
    expect(i.pending).toBe(true);
  });

  it("a refused runner deploy installs nothing", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.approve(Y);
    rig.gate("preflight", "refuse");
    const r = rig.cli(deployArgs(Y, "runner"));
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });

  it("a rollback after a successful click result leaves result.json alone, reports the older installed commit and re-offers the click", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.setRequest(Y, CLICK);
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    const before = readFileSync(join(rig.stateDir, "result.json"), "utf8");
    const rb = rig.cli(["rollback", "--to", "previous"]);
    expect(rb.status, msg(rb)).toBe(0);
    expect(readFileSync(join(rig.stateDir, "result.json"), "utf8")).toBe(before);
    const i = info();
    expect(i.status?.installed).toBe(rig.W);
    expect(i.status?.latest).toBe(Y);
    expect(i.available).toBe(true);
    expect(i.pending).toBe(false);
    expect(checkRequest(i, Y, "2026-01-03T00:00:00.000Z").ok).toBe(true);
  });

  it("status.json is written last: it is never older than the result it follows", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.approve(Y);
    expect(rig.cli(deployArgs(Y, "runner")).status).toBe(0);
    const res = rig.readJson("result.json") as { finished_at: string };
    const st = rig.readJson("status.json") as { checked_at: string; installed: string };
    expect(Date.parse(st.checked_at)).toBeGreaterThan(Date.parse(res.finished_at));
    expect(st.installed).toBe(Y);
  });

  it("an unknown live install withholds an old positive status instead of leaving it up", () => {
    rig = new Rig({ installedKind: "git" });
    expect(rig.cli(["status", "--write"]).status).toBe(0);
    expect(existsSync(join(rig.stateDir, "status.json"))).toBe(true);
    appendFileSync(join(rig.liveRoot() ?? "", "plug", "app.txt"), "dirty\n");
    const r = rig.cli(["status", "--write"]);
    expect(r.status, msg(r)).toBe(0);
    expect(existsSync(join(rig.stateDir, "status.json"))).toBe(false);
    expect(withheld().length).toBe(1);
    // and installed.sha is not touched by an unknown state
    expect(rig.installedSha()).toBe(rig.W);
  });

  it("an unreachable repository (latest cannot be computed) withholds status", () => {
    rig = new Rig({ installedKind: "stamped" });
    expect(rig.cli(["status", "--write"]).status).toBe(0);
    expect(existsSync(join(rig.stateDir, "status.json"))).toBe(true);
    rig.breakUpstream();
    const r = rig.cli(["status", "--write"]);
    expect(r.status, msg(r)).toBe(0);
    expect(existsSync(join(rig.stateDir, "status.json"))).toBe(false);
  });

  it("an observed external install is recorded, published and writes installed.sha", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    // what the old runner does: a plain checkout, no stamp, installed by bb directly
    rig.install(rig.makeBuild(Y, "git"));
    const r = rig.cli(["record", "--reconcile"], { engineRoot: rig.buildDirs()[0] ?? "", pluginRoot: rig.buildDirs()[0] ?? "" });
    expect(r.status, msg(r)).toBe(0);
    const rec = rig.record() as { installed: { commit: string }; history: { event: string }[] };
    expect(rec.installed.commit).toBe(Y);
    expect(rec.history.some((h) => h.event === "observed-external-install")).toBe(true);
    expect(rig.installedSha()).toBe(Y);
  });
});
