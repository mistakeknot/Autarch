import { afterEach, describe, expect, it } from "vitest";
import { mkdirSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import Database from "better-sqlite3";
import { PLUGIN_DIR, Rig, type CliResult } from "./rig";

let rig: Rig;
afterEach(() => rig?.cleanup());

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

describe("host and instance binding", () => {
  it("two live instances with identical source paths: only the bound one is touched and backed up", () => {
    rig = new Rig({ installedKind: "stamped" });
    const bbDataB = join(rig.root, "bbdata-b");
    const b = rig.addInstance("b", bbDataB);
    const dbB = rig.seedData(bbDataB);
    const conn = new Database(dbB);
    conn.prepare("INSERT INTO t VALUES ('only-in-b')").run();
    conn.close();
    rig.install(rig.liveRoot() ?? "", b);
    const a = new Database(rig.dataDb);
    a.prepare("INSERT INTO t VALUES ('only-in-a')").run();
    a.close();

    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(0);
    expect(rig.mutations()).toEqual(["disable", "install", "enable", "config"]);
    expect(rig.calls(b)).toEqual([]);
    expect(rig.mutations(b)).toEqual([]);
    // every bb call carried instance A's URL
    for (const c of rig.calls()) expect(c.url).toBe(rig.instances[0].serverUrl);
    // the backup is of A's data
    const backup = (rig.record() as { installed: { backup: string } }).installed.backup;
    expect(rows(join(backup, "data.db"))).toEqual(["before", "only-in-a"]);
  });

  it("targeting instance B backs up B's data and leaves A alone", () => {
    rig = new Rig({ installedKind: "stamped" });
    const bbDataB = join(rig.root, "bbdata-b");
    const b = rig.addInstance("b", bbDataB);
    rig.seedData(bbDataB);
    rig.install(rig.liveRoot() ?? "", b);
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y), { inst: b, engineRoot: rig.liveRoot() ?? "", pluginRoot: rig.liveRoot() ?? "" });
    expect(r.status, msg(r)).toBe(0);
    expect(rig.mutations(b)).toEqual(["disable", "install", "enable", "config"]);
    expect(rig.calls()).toEqual([]);
  });

  it("an inherited BB_SERVER_URL that names another instance is refused before any bb call", () => {
    rig = new Rig({ installedKind: "stamped" });
    const b = rig.addInstance("b", join(rig.root, "bbdata-b"));
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y), { env: { BB_SERVER_URL: b.serverUrl } });
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/differs from the runtime file/);
    expect(rig.calls()).toEqual([]);
    expect(rig.calls(b)).toEqual([]);
  });

  it("a matching inherited BB_SERVER_URL is fine and BB_HOST_DAEMON_PORT is cleared for every bb call", () => {
    rig = new Rig({ installedKind: "stamped" });
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y), { env: { BB_SERVER_URL: rig.instances[0].serverUrl, BB_HOST_DAEMON_PORT: "4999" } });
    expect(r.status, msg(r)).toBe(0);
    expect(rig.calls().length).toBeGreaterThan(3);
    for (const c of rig.calls()) {
      expect(c.hostPort).toBeNull();
      expect(c.url).toBe(rig.instances[0].serverUrl);
    }
  });

  it("a runtime file without serverUrl is refused: the tool never guesses an instance", () => {
    rig = new Rig({ installedKind: "stamped" });
    rig.rewriteRuntime(rig.instances[0], { serverUrl: undefined });
    const r = rig.cli(["record"]);
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/no serverUrl/);
    expect(rig.calls()).toEqual([]);
  });

  it("a stale runtime file (dead pid) and a missing runtime file are refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    rig.rewriteRuntime(rig.instances[0], { pid: 2 ** 22 - 7 });
    const stale = rig.cli(["record"]);
    expect(stale.status, msg(stale)).toBe(10);
    expect(msg(stale)).toMatch(/stale|not a live process/);
    const other = join(rig.root, "no-instance");
    mkdirSync(other, { recursive: true });
    const eng = rig.liveRoot() ?? "";
    const missing = rig.cli(["record", "--plugin-file", join(eng, PLUGIN_DIR, "deploy.json"), "--bb-data", other, "--state-dir", rig.stateDir], { noCommon: true });
    expect(missing.status, msg(missing)).toBe(10);
    expect(rig.calls()).toEqual([]);
  });

  it("a bb data directory with no plugin directory is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const bbDataB = join(rig.root, "bbdata-b");
    const b = rig.addInstance("b", bbDataB);
    rmSync(join(bbDataB, "plugins"), { recursive: true, force: true });
    const r = rig.cli(["record"], { inst: b, engineRoot: rig.liveRoot() ?? "", pluginRoot: rig.liveRoot() ?? "" });
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/has no data in this instance/);
  });

  it("two different data directories (flag and environment) are refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const b = rig.addInstance("b", join(rig.root, "bbdata-b"));
    const r = rig.cli(["record"], { env: { HOME_BB_DATA: b.bbData } });
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/two different bb data directories/);
    const r2 = rig.cli(["record"], { env: { BB_PLUGIN_DEPLOY_BB_DATA: b.bbData } });
    expect(r2.status).toBe(10);
    // the same directory from two sources is fine
    expect(rig.cli(["record"], { env: { HOME_BB_DATA: rig.instances[0].bbData } }).status).toBe(0);
  });

  it("with no flag, environment or host file there is no discovery", () => {
    rig = new Rig({ installedKind: "stamped" });
    const eng = rig.liveRoot() ?? "";
    const r = rig.cli(["record", "--plugin-file", join(eng, PLUGIN_DIR, "deploy.json"), "--state-dir", rig.stateDir], { noCommon: true });
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/no bb data directory/);
  });

  it("the host file supplies the data dir, state dir and builds root, and a differing flag is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    const eng = rig.liveRoot() ?? "";
    const hostFile = join(rig.root, "host.json");
    writeFileSync(hostFile, JSON.stringify({ bbData: rig.bbData, stateDir: rig.stateDir, buildsRoot: rig.buildsRoot, backupsRoot: rig.backupsRoot }));
    const args = ["record", "--plugin-file", join(eng, PLUGIN_DIR, "deploy.json"), "--host-file", hostFile];
    const ok = rig.cli(args, { noCommon: true });
    expect(ok.status, msg(ok)).toBe(0);
    const b = rig.addInstance("b", join(rig.root, "bbdata-b"));
    const bad = rig.cli([...args, "--bb-data", b.bbData], { noCommon: true });
    expect(bad.status).toBe(10);
  });

  it("a host file with an unknown key or a relative path is rejected", () => {
    rig = new Rig({ installedKind: "stamped" });
    const eng = rig.liveRoot() ?? "";
    const hostFile = join(rig.root, "host.json");
    const args = ["record", "--plugin-file", join(eng, PLUGIN_DIR, "deploy.json"), "--host-file", hostFile];
    writeFileSync(hostFile, JSON.stringify({ bbData: rig.bbData, surprise: 1 }));
    expect(rig.cli(args, { noCommon: true }).status).not.toBe(0);
    writeFileSync(hostFile, JSON.stringify({ bbData: "relative/dir" }));
    expect(rig.cli(args, { noCommon: true }).status).not.toBe(0);
  });

  it("bb is bound before the backup: a refused binding leaves the backups root empty", () => {
    rig = new Rig({ installedKind: "stamped" });
    const b = rig.addInstance("b", join(rig.root, "bbdata-b"));
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    expect(rig.cli(deployArgs(Y), { env: { BB_SERVER_URL: b.serverUrl } }).status).toBe(10);
    let names: string[] = [];
    try {
      names = readdirSync(rig.backupsRoot);
    } catch {
      names = [];
    }
    expect(names).toEqual([]);
  });
});
