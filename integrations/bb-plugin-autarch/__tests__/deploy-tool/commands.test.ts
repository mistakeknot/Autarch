import { afterEach, describe, expect, it } from "vitest";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import Database from "better-sqlite3";
import { pendingRequest } from "../../../../tools/bb-plugin-deploy/lib/tick.mjs";
import { PLUGIN_DIR, Rig, TOOL_SRC, gateEntry, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

const CLICK = "2026-01-02T00:00:00.000Z";
const deployArgs = (sha: string) => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", "agent"];
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const rows = (db: string): string[] => {
  const d = new Database(db, { readonly: true });
  try {
    return (d.prepare("SELECT x FROM t ORDER BY rowid").all() as { x: string }[]).map((r) => r.x);
  } finally {
    d.close();
  }
};

describe("usage and version", () => {
  it("version needs no plugin file and names the protocol", () => {
    const r = spawnSync(process.execPath, [join(TOOL_SRC, "cli.mjs"), "version", "--json"], { encoding: "utf8" });
    expect(r.status).toBe(0);
    expect(JSON.parse(r.stdout)).toMatchObject({ ok: true, protocol: expect.stringMatching(/^bb-plugin-deploy/) });
  });

  it("an unknown command, an unknown flag and a missing value are usage errors (exit 2)", () => {
    for (const args of [["frobnicate"], ["record", "--surprise"], ["record", "--plugin-file"], ["record", "stray"], []]) {
      const r = spawnSync(process.execPath, [join(TOOL_SRC, "cli.mjs"), ...args], { encoding: "utf8" });
      expect(r.status, args.join(" ")).toBe(2);
    }
  });

  it("a command that needs a plugin file says so", () => {
    const r = spawnSync(process.execPath, [join(TOOL_SRC, "cli.mjs"), "record"], { encoding: "utf8" });
    expect(r.status).toBe(2);
    expect(r.stderr).toMatch(/--plugin-file is required/);
  });
});

describe("preview", () => {
  const plugin = { previewGate: [gateEntry("preview-gate", "preview")] };

  it("installs the commit for a dev session, then restores the anchored build", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const wSource = rig.source();
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(["preview", "--commit", Y]);
    expect(r.status, msg(r)).toBe(0);
    expect(r.json?.message).toBe("preview ended");
    expect(rig.calls().filter((c) => c.args[1] === "dev")).toHaveLength(1);
    expect(rig.mutations()).toEqual(["disable", "install", "enable", "disable", "install", "enable"]);
    expect(rig.source()).toBe(wSource);
    expect((r.json?.gates as unknown[]).length).toBeGreaterThan(0);
  });

  it("a refusing preview gate installs nothing", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    rig.gate("preview", "refuse");
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(["preview", "--commit", Y]);
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });

  it("a failed restore after the preview is reported as unhealthy", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const wSource = rig.source() ?? "";
    rig.inject({ failInstallMatch: wSource });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(["preview", "--commit", Y]);
    expect(r.status, msg(r)).toBe(31);
    expect(msg(r)).toMatch(/FAILED/);
  });

  it("--path previews an already built directory", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const dir = rig.makeBuild(Y, "stamped");
    const wSource = rig.source();
    const r = rig.cli(["preview", "--path", dir]);
    expect(r.status, msg(r)).toBe(0);
    expect(rig.calls().some((c) => c.args[1] === "install" && c.args.at(-1) === join(dir, PLUGIN_DIR))).toBe(true);
    expect(rig.source()).toBe(wSource);
  });

  it("--path refuses a directory with no stamp", () => {
    rig = new Rig({ installedKind: "stamped", plugin });
    const dir = rig.makeBuild(rig.commit({ "plug/app.txt": "v2\n" }), "git");
    const r = rig.cli(["preview", "--path", dir]);
    expect(r.status, msg(r)).not.toBe(0);
    expect(rig.mutations()).toEqual([]);
  });
});

describe("restore-data", () => {
  const backupOf = (): string => (rig.record() as { installed: { backup: string } }).installed.backup;

  it("--check validates the backup and changes nothing", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    rig.clearCalls();
    const r = rig.cli(["restore-data", "--backup", backupOf(), "--check"]);
    expect(r.status, msg(r)).toBe(0);
    expect(r.json?.message).toBe("restore-data check passed");
    expect(rig.calls().filter((c) => c.args[0] === "plugin" && c.args[1] !== "list" && c.args[1] !== "source")).toEqual([]);
  });

  it("replacing live data is mk-only: agent mode is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    rig.clearCalls();
    const r = rig.cli(["restore-data", "--backup", backupOf()]);
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/mk-only/);
    expect(rig.mutations()).toEqual([]);
  });

  it("with --mode mk-script it restores the sqlite data and reinstalls the previous source", () => {
    rig = new Rig({ installedKind: "stamped" });
    const seeded = rows(rig.dataDb);
    const wSource = rig.source();
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    const d = new Database(rig.dataDb);
    d.prepare("INSERT INTO t VALUES ('after-deploy')").run();
    d.close();
    expect(rows(rig.dataDb)).toEqual([...seeded, "after-deploy"]);
    const r = rig.cli(["restore-data", "--backup", backupOf(), "--mode", "mk-script"]);
    expect(r.status, msg(r)).toBe(0);
    expect(rows(rig.dataDb)).toEqual(seeded);
    expect(rig.source()).toBe(wSource);
    expect(rig.installedSha()).toBe(rig.W);
  });

  it("refuses a missing backup, a corrupt backup and a backup with no previous source", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y)).status).toBe(0);
    expect(rig.cli(["restore-data", "--backup", join(rig.root, "nope"), "--check"]).status).toBe(10);
    const r0 = rig.cli(["restore-data", "--check"]);
    expect(r0.status, msg(r0)).toBe(10);
    spawnSync("bash", ["-c", 'printf garbage > "$1/data.db"', "x", backupOf()]);
    const bad = rig.cli(["restore-data", "--backup", backupOf(), "--check"]);
    expect(bad.status, msg(bad)).toBe(10);
    spawnSync("rm", ["-f", join(backupOf(), "previous-source.txt")]);
    expect(rig.cli(["restore-data", "--backup", backupOf(), "--check"]).status).toBe(10);
  });
});

describe("update-tick", () => {
  it("with nothing pending it publishes status and does nothing else", () => {
    rig = new Rig({ installedKind: "stamped" });
    const r = rig.cli(["update-tick"]);
    expect(r.status, msg(r)).toBe(0);
    expect(r.json?.message).toBe("nothing to do");
    expect(rig.mutations()).toEqual([]);
    expect(rig.installedSha()).toBe(rig.W);
  });

  it("a pending click for a reviewed commit is deployed in runner mode and a result is written", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.approve(Y);
    rig.setRequest(Y, CLICK);
    const r = rig.cli(["update-tick"]);
    expect(r.status, msg(r)).toBe(0);
    expect(r.json?.command).toBe("update-tick");
    expect(rig.installedSha()).toBe(Y);
    expect(rig.readJson("result.json")).toMatchObject({ sha: Y, ok: true });
    // the click is now satisfied
    rig.clearCalls();
    expect(rig.cli(["update-tick"]).json?.message).toBe("nothing to do");
    expect(rig.mutations()).toEqual([]);
  });

  it("a click with no review evidence is refused, recorded as a failed result, and nothing is installed", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    rig.setRequest(Y, CLICK);
    const r = rig.cli(["update-tick"]);
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
    expect(rig.readJson("result.json")).toMatchObject({ sha: Y, ok: false });
  });

  it("a plugin file with no updateRequest cannot tick", () => {
    rig = new Rig({ installedKind: "stamped", plugin: { updateRequest: undefined } });
    const r = rig.cli(["update-tick"]);
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/no updateRequest/);
  });

  it("a request that is not a full sha is ignored", () => {
    rig = new Rig({ installedKind: "stamped" });
    rig.setRequest("abc123", CLICK);
    const r = rig.cli(["update-tick"]);
    expect(r.status, msg(r)).toBe(0);
    expect(rig.mutations()).toEqual([]);
  });
});

describe("pendingRequest", () => {
  const live = (commit: string) => ({ live: { state: "stamped", commit } });
  const A = "a".repeat(40);
  const B = "b".repeat(40);

  it("is null with no request or when the request is already installed", () => {
    expect(pendingRequest(live(A), null, null)).toBeNull();
    expect(pendingRequest(live(A), { sha: A, clicked_at: CLICK }, null)).toBeNull();
  });

  it("is the request when another commit is installed and no result covers it", () => {
    const req = { sha: B, clicked_at: CLICK };
    expect(pendingRequest(live(A), req, null)).toBe(req);
    expect(pendingRequest(live(A), req, { sha: "c".repeat(40), finished_at: "2026-02-01T00:00:00.000Z" })).toBe(req);
  });

  it("a result at least as new as the click satisfies it; an older one does not; a re-click is pending again", () => {
    const req = { sha: B, clicked_at: CLICK };
    expect(pendingRequest(live(A), req, { sha: B, finished_at: "2026-01-02T00:00:01.000Z" })).toBeNull();
    expect(pendingRequest(live(A), req, { sha: B, finished_at: "2026-01-01T00:00:00.000Z" })).toBe(req);
    const again = { sha: B, clicked_at: "2026-01-03T00:00:00.000Z" };
    expect(pendingRequest(live(A), again, { sha: B, finished_at: "2026-01-02T00:00:01.000Z" })).toBe(again);
  });

  it("an unknown live install counts as nothing installed", () => {
    const req = { sha: B, clicked_at: CLICK };
    expect(pendingRequest({ live: { state: "unknown", reason: "dirty" } }, req, null)).toBe(req);
  });
});
