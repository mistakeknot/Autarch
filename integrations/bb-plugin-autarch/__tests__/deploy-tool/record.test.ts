import { afterEach, describe, expect, it } from "vitest";
import { appendFileSync, chmodSync, existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import { checkRequest, updateInfo } from "../../updates";
import { Rig, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

const CLICK = "2026-01-02T00:00:00.000Z";
const deployArgs = (sha: string, mode = "agent") => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", mode];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
interface Rec { installed: { commit: string; buildRoot: string } | null; previous: { commit: string; buildRoot: string } | null; history: { event: string; detail: string; commit: string }[] }
const rec = () => rig.record() as unknown as Rec;
const info = () => updateInfo(rig.stateDir, rig.requestRaw(), { now: new Date().toISOString(), timerFile: join(rig.root, "no-timer"), procLocks: "/dev/null" });

describe("record --reconcile observes what is live", () => {
  it("a stamped live build is recorded once and a second reconcile changes nothing", () => {
    rig = new Rig({ installedKind: "stamped" });
    const first = rig.cli(["record", "--reconcile"]);
    expect(first.status, msg(first)).toBe(0);
    expect(rec().installed?.commit).toBe(rig.W);
    expect(rec().history.map((h) => h.event)).toEqual(["observed-external-install"]);
    const again = rig.cli(["record", "--reconcile"]);
    expect(again.status, msg(again)).toBe(0);
    expect(rec().history).toHaveLength(1);
  });

  it("a legacy git checkout is recorded from its HEAD", () => {
    rig = new Rig({ installedKind: "git" });
    expect(rig.cli(["record", "--reconcile"]).status).toBe(0);
    expect(rec().installed?.commit).toBe(rig.W);
  });

  it("an unknown live install (dirty checkout) changes nothing and writes no installed.sha", () => {
    rig = new Rig({ installedKind: "git" });
    appendFileSync(join(rig.liveRoot() ?? "", "plug", "app.txt"), "local edit\n");
    const r = rig.cli(["record", "--reconcile"]);
    expect(r.status, msg(r)).toBe(0);
    expect(existsSync(join(rig.stateDir, "record.json"))).toBe(false);
    expect(rig.installedSha()).toBeNull();
  });

  it("a different commit moves installed to previous and records who was there", () => {
    rig = new Rig({ installedKind: "stamped" });
    expect(rig.cli(["record", "--reconcile"]).status).toBe(0);
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.install(rig.makeBuild(Y, "git"));
    expect(rig.cli(["record", "--reconcile"]).status).toBe(0);
    const r = rec();
    expect(r.installed?.commit).toBe(Y);
    expect(r.previous?.commit).toBe(rig.W);
    expect(r.history.at(-1)).toMatchObject({ event: "observed-external-install", commit: Y, detail: `was ${rig.W.slice(0, 12)}` });
    expect(rig.installedSha()).toBe(Y);
  });

  it("the same commit in a different build directory is recorded as observed", () => {
    rig = new Rig({ installedKind: "stamped" });
    expect(rig.cli(["record", "--reconcile"]).status).toBe(0);
    rig.install(rig.makeBuild(rig.W, "git"));
    expect(rig.cli(["record", "--reconcile"]).status).toBe(0);
    expect(rec().history.at(-1)?.detail).toBe("same commit, different build");
    expect(rec().previous).toBeNull();
  });

  it("the history is capped at 200 entries", () => {
    rig = new Rig({ installedKind: "stamped" });
    expect(rig.cli(["record", "--reconcile"]).status).toBe(0);
    const p = join(rig.stateDir, "record.json");
    const r = JSON.parse(readFileSync(p, "utf8")) as Rec;
    r.history = Array.from({ length: 250 }, (_, i) => ({ event: "pause", detail: `n${i}`, commit: rig.W }));
    writeFileSync(p, JSON.stringify(r));
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    expect(rec().history.length).toBeLessThanOrEqual(200);
    expect(rec().history.at(-1)?.event).toBe("install");
  });
});

describe("previous anchoring", () => {
  it("an old-runner install between two tool deploys: the next deploy anchors what is really live", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    // an old runner (no tool) installs Z as a plain checkout
    const Z = rig.commit({ "plug/app.txt": "v3\n" });
    const zBuild = rig.makeBuild(Z, "git");
    rig.install(zBuild);
    const V = rig.commit({ "plug/app.txt": "v4\n" });
    const r = rig.cli(deployArgs(V));
    expect(r.status, msg(r)).toBe(0);
    const after = rec();
    expect(after.installed?.commit).toBe(V);
    expect(after.previous?.commit).toBe(Z);
    // the plain checkout was rebuilt immutably into a new tool build, not reused in place
    expect(after.previous?.buildRoot).not.toBe(zBuild);
    expect(existsSync(join(after.previous?.buildRoot ?? "", ".bb-plugin-deploy", "stamp.json"))).toBe(true);
    expect(after.history.some((h) => h.event === "observed-external-install" && h.detail === `was ${Y.slice(0, 12)}` && h.commit === Z)).toBe(true);
  });

  it("a stamped live build that no longer verifies is refused as the previous", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    appendFileSync(join(rig.liveRoot() ?? "", "plug", "app.txt"), "tampered\n");
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });
});

describe("legacy rollback with the tool absent", () => {
  /** Today's runner pattern: take fd 9 with a non-blocking flock, do nothing else when there is no pending work. */
  const fakeRunnerTick = (): number => {
    const sh = join(rig.root, "old-runner.sh");
    writeFileSync(sh, `#!/usr/bin/env bash\nexec 9>"${join(rig.stateDir, "lock")}"\nflock -n 9 || exit 0\necho tick >> "${join(rig.root, "runner.log")}"\n`);
    chmodSync(sh, 0o755);
    return spawnSync("bash", [sh], { encoding: "utf8" }).status ?? -1;
  };

  it("after X is installed and then legacy-rolled back to W, the next tool look shows W and re-offers X", () => {
    rig = new Rig({ installedKind: "stamped" });
    const X = rig.commit({ "plug/app.txt": "v2\n" });
    rig.setRequest(X, CLICK);
    expect(rig.cli(deployArgs(X)).status).toBe(0);
    expect(rig.installedSha()).toBe(X);
    // the legacy rollback: a plain checkout of W goes live; nothing updates installed.sha
    rig.install(rig.makeBuild(rig.W, "git"));
    expect(fakeRunnerTick()).toBe(0);
    expect(rig.installedSha()).toBe(X);
    // the next tool look reconciles with what bb really runs
    const st = rig.cli(["status", "--write"]);
    expect(st.status, msg(st)).toBe(0);
    expect(rig.installedSha()).toBe(rig.W);
    const i = info();
    expect(i.status?.installed).toBe(rig.W);
    expect(i.available).toBe(true);
    expect(checkRequest(i, X, "2026-01-03T00:00:00.000Z").ok).toBe(true);
    expect(rec().history.some((h) => h.event === "observed-external-install" && h.detail === `was ${X.slice(0, 12)}`)).toBe(true);
  });
});
