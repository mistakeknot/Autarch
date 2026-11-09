import { afterEach, describe, expect, it } from "vitest";
import { appendFileSync, copyFileSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { Rig, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

const deployArgs = (sha: string, mode = "agent", extra: string[] = []) => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", mode, ...extra];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const history = () => (rig.record() as { history: { event: string; detail: string }[] }).history;

describe("trust rule: the installed control closure must match its commit", () => {
  const refused = (r: CliResult, re?: RegExp) => {
    expect(r.status, msg(r)).toBe(10);
    if (re) expect(msg(r)).toMatch(re);
    expect(rig.mutations()).toEqual([]);
  };

  it("a modified tool file in a stamped build is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    appendFileSync(join(rig.liveRoot() ?? "", "tools", "bb-plugin-deploy", "lib", "gc.mjs"), "\n// tampered\n");
    refused(rig.cli(deployArgs(Y)), /installed deploy tooling does not match commit/);
  });

  it("a modified home-gates.sh in a stamped build is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    appendFileSync(join(rig.liveRoot() ?? "", "scripts", "home-gates.sh"), "\necho tampered\n");
    refused(rig.cli(deployArgs(Y)), /installed deploy tooling does not match commit/);
  });

  it("a symlink swapped into the closure is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const g = join(rig.liveRoot() ?? "", "scripts", "home-gates.sh");
    copyFileSync(g, join(rig.root, "gates-copy.sh"));
    rmSync(g);
    symlinkSync(join(rig.root, "gates-copy.sh"), g);
    refused(rig.cli(deployArgs(Y)), /symlink|does not match/);
  });

  it("an extra file under a closure directory is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    writeFileSync(join(rig.liveRoot() ?? "", "tools", "bb-plugin-deploy", "lib", "extra.mjs"), "export {};\n");
    refused(rig.cli(deployArgs(Y)), /installed deploy tooling does not match commit/);
  });

  it("a missing closure file is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rmSync(join(rig.liveRoot() ?? "", "scripts", "home-gates.sh"));
    refused(rig.cli(deployArgs(Y)));
  });

  it("a stamp whose commit is not the build's HEAD is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const p = join(rig.liveRoot() ?? "", ".bb-plugin-deploy", "stamp.json");
    const stamp = JSON.parse(readFileSync(p, "utf8")) as Record<string, unknown>;
    writeFileSync(p, JSON.stringify({ ...stamp, commit: Y }));
    refused(rig.cli(deployArgs(Y)));
  });

  it("a modified home-gates.sh in an unstamped (legacy) build is refused too", () => {
    rig = new Rig({ installedKind: "git" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    appendFileSync(join(rig.liveRoot() ?? "", "scripts", "home-gates.sh"), "\necho tampered\n");
    refused(rig.cli(deployArgs(Y)));
  });

  it("record --verify-closure reports a clean closure and refuses a dirty one", () => {
    rig = new Rig({ installedKind: "stamped" });
    const ok = rig.cli(["record", "--verify-closure"]);
    expect(ok.status, msg(ok)).toBe(0);
    expect(ok.json?.closureVerified).toBe(true);
    appendFileSync(join(rig.liveRoot() ?? "", "scripts", "home-gates.sh"), "\necho tampered\n");
    expect(rig.cli(["record", "--verify-closure"]).status).toBe(10);
  });

  it("the installed plugin file is the one used: a plugin file outside the installed build is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const other = rig.makeBuild(rig.W, "stamped");
    const r = rig.cli(deployArgs(Y), { pluginRoot: other });
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });
});

describe("control-path change pauses agent and runner deploys", () => {
  const withControlChange = () => {
    rig = new Rig({ installedKind: "stamped" });
    return rig.commit({ "scripts/home-gates.sh": `${readFileSync(join(rig.upstream, "scripts", "home-gates.sh"), "utf8")}\n# reviewed change\n` });
  };

  it("deploy --mode agent exits 20 with the exact mk-script command and installs nothing", () => {
    const Y = withControlChange();
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(20);
    expect(msg(r)).toMatch(/deploy tooling changed between/);
    expect(msg(r)).toContain(`--commit ${Y} --class plugin-deploy --mode mk-script`);
    expect(rig.mutations()).toEqual([]);
    expect(history().some((h) => h.event === "pause")).toBe(true);
  });

  it("a content-only change in the same commit range does not pause", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
  });

  it("the pause lifts with --mode mk-script", () => {
    const Y = withControlChange();
    const r = rig.cli(deployArgs(Y, "mk-script"));
    expect(r.status, msg(r)).toBe(0);
    expect(rig.installedSha()).toBe(Y);
    expect(r.json?.warnings).toEqual(expect.arrayContaining([expect.stringMatching(/ancestry/)]));
  });

  it("update-tick records a failed result naming the tooling change", () => {
    const Y = withControlChange();
    rig.approve(Y);
    rig.setRequest(Y, "2026-01-02T00:00:00.000Z");
    const r = rig.cli(["update-tick"]);
    expect(r.status, msg(r)).toBe(20);
    const result = rig.readJson("result.json") as { sha: string; ok: boolean; message: string };
    expect(result.ok).toBe(false);
    expect(result.sha).toBe(Y);
    expect(result.message).toMatch(/deploy tooling changed between/);
    expect(rig.mutations()).toEqual([]);
  });

  it("runner mode needs review evidence for the exact sha", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y, "runner"));
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toBe(`no review evidence for ${Y}; ask the coordinator to review it or install by mk script`);
    expect(rig.mutations()).toEqual([]);
    rig.approve(Y);
    const ok = rig.cli(deployArgs(Y, "runner"));
    expect(ok.status, msg(ok)).toBe(0);
  });

  it("review evidence for another sha or with another verdict does not count", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.approve("2".repeat(40));
    expect(rig.cli(deployArgs(Y, "runner")).status).toBe(10);
    writeFileSync(join(rig.stateDir, "review-evidence", `${Y}.json`), JSON.stringify({ format: "bb-plugin-deploy/review-evidence@1", commit: Y, verdict: "revise" }));
    expect(rig.cli(deployArgs(Y, "runner")).status).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });
});

describe("clone engine and confirmed first run", () => {
  const setup = () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "scripts/home-gates.sh": `${readFileSync(join(rig.upstream, "scripts", "home-gates.sh"), "utf8")}\n# reviewed change\n` });
    const engine = rig.makeBuild(Y, "git");
    return { Y, engine };
  };

  it("a clone engine is refused in agent mode without evidence", () => {
    const { Y, engine } = setup();
    const r = rig.cli(deployArgs(Y), { engineRoot: engine, pluginRoot: engine });
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/not running from the installed build/);
    expect(rig.mutations()).toEqual([]);
  });

  it("mk-script mode allows a clean clone engine (it runs with node built-ins only)", () => {
    const { Y, engine } = setup();
    const r = rig.cli(deployArgs(Y, "mk-script"), { engineRoot: engine, pluginRoot: engine });
    expect(r.status, msg(r)).toBe(0);
    expect(rig.installedSha()).toBe(Y);
  });

  it("a dirty clone engine is refused even in mk-script mode", () => {
    const { Y, engine } = setup();
    appendFileSync(join(engine, "scripts", "home-gates.sh"), "\necho tampered\n");
    const r = rig.cli(deployArgs(Y, "mk-script"), { engineRoot: engine, pluginRoot: engine });
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });

  it("a confirmed first run needs mk's pick naming the exact commit, and is recorded in the history", () => {
    const { Y, engine } = setup();
    rig.addDecision("d-wrong", `first run of ${"9".repeat(40)}`, "confirm-first-run", "mk");
    rig.addDecision("d-notmk", `first run of ${Y}`, "confirm-first-run", "agent");
    rig.addDecision("d-option", `first run of ${Y}`, "something-else", "mk");
    for (const d of ["d-wrong", "d-notmk", "d-option", "d-missing"]) {
      const r = rig.cli(deployArgs(Y, "agent", ["--confirmed-first-run", d]), { engineRoot: engine, pluginRoot: engine });
      expect(r.status, `${d}: ${msg(r)}`).toBe(10);
    }
    expect(rig.mutations()).toEqual([]);
    rig.addDecision("d-good", `first run of ${Y}`, "confirm-first-run", "mk");
    const ok = rig.cli(deployArgs(Y, "agent", ["--confirmed-first-run", "d-good"]), { engineRoot: engine, pluginRoot: engine });
    expect(ok.status, msg(ok)).toBe(0);
    expect(rig.installedSha()).toBe(Y);
    expect(JSON.stringify(rig.record())).toContain("d-good");
  });

  it("a first run from an engine that is not the target commit is refused", () => {
    const { Y } = setup();
    rig.addDecision("d-good", `first run of ${Y}`, "confirm-first-run", "mk");
    const wrongEngine = rig.makeBuild(rig.W, "git");
    const r = rig.cli(deployArgs(Y, "agent", ["--confirmed-first-run", "d-good"]), { engineRoot: wrongEngine, pluginRoot: wrongEngine });
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/engine of the target commit/);
    expect(rig.mutations()).toEqual([]);
  });
});
