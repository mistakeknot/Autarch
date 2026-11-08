import { afterEach, describe, expect, it } from "vitest";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { Rig, gateEntry, waitFor, type CliResult } from "./rig";
import { listBuildDirs } from "../../../../tools/bb-plugin-deploy/lib/context.mjs";
import { mkScriptCommand, shellQuote } from "../../../../tools/bb-plugin-deploy/lib/session.mjs";

let rig: Rig;
afterEach(() => rig?.cleanup());

const deployArgs = (sha: string, mode = "agent", extra: string[] = []) => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", mode, ...extra];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const exited = (p: { once: (e: "exit", f: (c: number | null) => void) => unknown }): Promise<number | null> => new Promise((res) => p.once("exit", (c) => res(c)));
const changeControl = (): string => rig.commit({ "scripts/home-gates.sh": `${readFileSync(join(rig.upstream, "scripts", "home-gates.sh"), "utf8")}\n# changed\n` });

describe("rollback applies the engine trust rule", () => {
  it("a clone engine in agent mode is refused before bb is touched", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    const clone = rig.makeBuild(Y, "git");
    rig.clearCalls();
    const r = rig.cli(["rollback", "--to", "previous"], { engineRoot: clone, pluginRoot: clone });
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/not running from the installed build/);
    expect(rig.mutations()).toEqual([]);
  });

  it("a modified installed control closure is refused, and nothing is changed", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    const live = rig.liveRoot() ?? "";
    writeFileSync(join(live, "scripts", "home-gates.sh"), `${readFileSync(join(live, "scripts", "home-gates.sh"), "utf8")}\n# tampered\n`);
    rig.clearCalls();
    const r = rig.cli(["rollback", "--to", "previous"]);
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });

  it("the installed engine still rolls back", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    expect(rig.cli(["rollback", "--to", "previous"]).status).toBe(0);
    expect(rig.installedSha()).toBe(rig.W);
  });
});

describe("preview", () => {
  const plugin = { previewGate: [gateEntry("preview-gate", "preview")] };

  it("a termination signal during the dev session restores the previous build before exit", async () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const before = rig.state().source;
    rig.inject({ pauseAfter: "dev" });
    const p = rig.spawnCli(["preview", "--commit", Y]);
    const done = exited(p);
    expect(waitFor(() => existsSync(join(rig.instances[0].stateDir, "paused")), 20_000)).toBe(true);
    expect(rig.state().source).not.toBe(before);
    expect(rig.record()).toMatchObject({ transaction: { kind: "preview" } });
    p.kill("SIGTERM");
    expect(await done).toBe(0);
    expect(rig.state().source).toBe(before);
    const after = rig.record() as { transaction?: unknown; installed: { commit: string } };
    expect(after.transaction).toBeUndefined();
    expect(after.installed.commit).toBe(rig.W);
    expect(rig.installedSha()).toBe(rig.W);
  }, 60_000);

  it("keeps the record's rollback target: a preview does not turn the installed build into 'previous'", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    const before = (rig.record() as { previous: { commit: string } }).previous;
    expect(before.commit).toBe(rig.W);
    const Z = rig.commit({ "plug/app.txt": "v3\n" });
    const r = rig.cli(["preview", "--commit", Z]);
    expect(r.status, msg(r)).toBe(0);
    const after = rig.record() as { previous: unknown; installed: { commit: string } };
    expect(after.previous).toEqual(before);
    expect(after.installed.commit).toBe(Y);
    expect(rig.cli(["rollback", "--to", "previous"]).status).toBe(0);
    expect(rig.installedSha()).toBe(rig.W);
  }, 60_000);

  it("a target that changes deploy tooling is paused, for a commit and for a --path build, before any build or install", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const C = changeControl();
    rig.clearCalls();
    const r = rig.cli(["preview", "--commit", C]);
    expect(r.status, msg(r)).toBe(20);
    expect(msg(r)).toMatch(/needs mk's reviewed install or a confirmed first run/);
    expect(msg(r)).toContain("preview --commit");
    expect(rig.mutations()).toEqual([]);
    expect(existsSync(join(rig.buildsRoot, "fixture")) ? readdirSync(join(rig.buildsRoot, "fixture")).filter((n) => n.startsWith(C.slice(0, 12))) : []).toEqual([]);
    const b = rig.makeBuild(C, "stamped");
    const r2 = rig.cli(["preview", "--path", b]);
    expect(r2.status, msg(r2)).toBe(20);
    expect(rig.mutations()).toEqual([]);
  });

  it("mk-script lifts the pause", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const C = changeControl();
    const r = rig.cli(["preview", "--commit", C, "--mode", "mk-script"]);
    expect(r.status, msg(r)).toBe(0);
    expect(rig.installedSha()).toBe(rig.W);
  });
});

describe("generated recovery commands", () => {
  it("name the resolved state dir and host file, and quote every word", () => {
    const s = { cliPath: "/a b/cli.mjs", pluginFile: "/p/it's.json", bbData: "/d", stateDir: "/s s", dirs: { hostFile: "/h/x.json" } };
    const c = mkScriptCommand(s, ["rollback", "--to", "previous"]);
    expect(c).toBe("node '/a b/cli.mjs' rollback --to previous --plugin-file '/p/it'\\''s.json' --bb-data /d --state-dir '/s s' --host-file /h/x.json");
    expect(shellQuote("plain-1.2/x")).toBe("plain-1.2/x");
    expect(shellQuote("$(x)")).toBe("'$(x)'");
  });

  it("a deploy prints a rollback command that opens the same state dir", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(0);
    expect(String(r.json?.rollbackCommand)).toContain(`--state-dir ${rig.stateDir}`);
    expect(String(r.json?.rollbackCommand)).toContain("--host-file ");
  });
});

describe("status publication is part of the protected writes", () => {
  it("a status.json that cannot be written rolls the deploy back", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const liveBefore = rig.liveRoot();
    mkdirSync(join(rig.stateDir, "status.json"), { recursive: true });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(30);
    expect(rig.installedSha()).toBe(rig.W);
    expect(rig.liveRoot()).toBe(liveBefore);
  });
});

describe("build retention follows recency, not the commit prefix", () => {
  it("lists oldest first by the UTC stamp, whatever the commit prefix", () => {
    rig = new Rig({ installedKind: "stamped" });
    const a = rig.makeBuild(rig.W, "stamped", "ffffffffffff-20260101T000000Z");
    const b = rig.makeBuild(rig.W, "stamped", "000000000000-20261101T000000Z");
    const c = rig.makeBuild(rig.W, "stamped", "000000000000-20261101T000001Z");
    const list = (listBuildDirs(rig.buildsRoot, "fixture") as string[]).filter((d) => [a, b, c].includes(d));
    expect(list).toEqual([a, b, c]);
  });

  it("gc with retention 1 keeps the newest build, not the highest hash", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { retention: { builds: 1 } } });
    const jan = rig.makeBuild(rig.W, "stamped", "ffffffffffff-20260101T000000Z");
    const nov = rig.makeBuild(rig.W, "stamped", "000000000000-20261101T000000Z");
    expect(rig.cli(["gc", "--apply"]).status).toBe(0);
    expect(existsSync(nov)).toBe(true);
    expect(existsSync(jan)).toBe(false);
  });
});
