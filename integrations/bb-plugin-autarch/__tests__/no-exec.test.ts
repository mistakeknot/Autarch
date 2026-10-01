import { describe, expect, it } from "vitest";
import { execViolations, parse, parseFile, rel, serverSources } from "./static-scan.js";

describe("no process execution in server code (plan Task 2.7)", () => {
  it("scans a real set of server sources, and none of them spawns or forks", () => {
    const files = serverSources();
    expect(files.map(rel)).toEqual(expect.arrayContaining(["server.ts", "serve.ts", "service.ts", "store.ts", "cli.ts"]));
    expect(files.map(rel).filter((f) => /^(ui|__tests__|e2e|scripts)\//.test(f))).toEqual([]);
    const bad = files.flatMap((f) => execViolations(parseFile(f)));
    expect(bad).toEqual([]);
  });

  describe("positive fixtures prove the check fires", () => {
    const hits = (src: string) => execViolations(parse(src));
    it.each([
      ['import { spawn } from "node:child_process";'],
      ['import cp from "child_process";'],
      ['import { execa } from "execa";'],
      ['import { Worker } from "node:worker_threads";'],
      ['import cluster from "node:cluster";'],
      ['export * from "node:child_process";'],
      ['const cp = require("child_process");'],
      ['const cp = require("node:child_process");'],
      ['const m = await import("node:child_process");'],
      ['const m = await import(name);'],
      ['const r = require(name);'],
      ['import cp = require("node:child_process");'],
      ['Bun.spawn(["ls"]);'],
      ['Bun.spawnSync(["ls"]);'],
      ['Bun["spawn"](["ls"]);'],
      ['process.binding("spawn_sync");'],
      ['process["binding"]("spawn_sync");'],
    ])("flags %s", (src) => {
      expect(hits(src).length).toBeGreaterThan(0);
    });
  });

  describe("legitimate .exec() calls do not trigger it", () => {
    const hits = (src: string) => execViolations(parse(src));
    it("regexp exec, SQLite exec and unrelated requires pass", () => {
      expect(hits("const m = /a(b)/.exec(s); const m2 = re.exec(s);")).toEqual([]);
      expect(hits("db.exec('CREATE TABLE t(x)'); this.db.exec(sql);")).toEqual([]);
      expect(hits('import { readFileSync } from "node:fs"; const x = require("node:path"); await import("./cards.js");')).toEqual([]);
      expect(hits('const text = "child_process"; // spawn exec import("node:child_process")')).toEqual([]);
    });
  });
});
