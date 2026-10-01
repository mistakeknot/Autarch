import { describe, expect, it } from "vitest";
import { fsWriteUses, isFdOnly, isOpener, parse, parseFile, readOnlyFlags, rel, serverSources } from "./static-scan.js";

// Plan 1.3.8 and Task 2.7: fs writes live only in ruling.ts and export.ts. backup.ts may open O_RDONLY and fsync
// (the backup itself is written by SQLite's VACUUM INTO); rootrun.ts may only open files O_RDONLY.
const FULL = new Set(["ruling.ts", "export.ts"]);
const RDONLY_PLUS_FSYNC = new Set(["backup.ts"]);
const RDONLY_ONLY = new Set(["rootrun.ts"]);

describe("fs write scope (plan Task 2.7)", () => {
  it("only ruling.ts and export.ts write; backup.ts only opens read-only and fsyncs; rootrun.ts only opens read-only", () => {
    const problems: string[] = [];
    for (const f of serverSources()) {
      const name = rel(f);
      if (FULL.has(name)) continue;
      for (const u of fsWriteUses(parseFile(f))) {
        const okRead = isOpener(u.api) && readOnlyFlags(u.flags);
        if (RDONLY_PLUS_FSYNC.has(name) && (okRead || isFdOnly(u.api))) continue;
        if (RDONLY_ONLY.has(name) && okRead) continue;
        problems.push(`${name}: ${u.api} at ${u.at}${u.flags ? ` flags ${u.flags}` : ""}`);
      }
    }
    expect(problems).toEqual([]);
  });

  it("the scan sees the allowed writers, so it is not vacuous", () => {
    const uses = (n: string) => fsWriteUses(parseFile(serverSources().find((f) => rel(f) === n)!)).map((u) => u.api);
    expect(uses("ruling.ts")).toEqual(expect.arrayContaining(["openSync", "fsyncSync"]));
    expect(uses("export.ts")).toEqual(expect.arrayContaining(["renameSync", "writeSync"]));
    expect(uses("backup.ts")).toEqual(expect.arrayContaining(["openSync", "fsyncSync"]));
  });

  describe("positive fixtures", () => {
    const apis = (s: string) => fsWriteUses(parse(s)).map((u) => u.api);
    it("flags writes by name, namespace and default import, and by reference", () => {
      expect(apis('import { writeFileSync } from "node:fs"; writeFileSync(p, d);')).toEqual(["writeFileSync"]);
      expect(apis('import * as fs from "fs"; fs.unlinkSync(p);')).toEqual(["unlinkSync"]);
      expect(apis('import fs from "node:fs"; fs.mkdirSync(p);')).toEqual(["mkdirSync"]);
      expect(apis('import { rename } from "node:fs/promises"; await rename(a, b);')).toEqual(["rename"]);
      expect(apis('import { rmSync } from "node:fs"; [p].forEach(rmSync);')).toEqual(["rmSync"]);
    });
    it("tells a read-only open from a writing one", () => {
      const f = (s: string) => fsWriteUses(parse(s))[0]!;
      expect(readOnlyFlags(f('import { openSync } from "node:fs"; openSync(p, "r");').flags)).toBe(true);
      expect(readOnlyFlags(f('import { openSync, constants } from "node:fs"; openSync(p, constants.O_RDONLY);').flags)).toBe(true);
      expect(readOnlyFlags(f('import { openSync, constants } from "node:fs"; openSync(p, constants.O_RDONLY | constants.O_CREAT);').flags)).toBe(false);
      expect(readOnlyFlags(f('import { openSync } from "node:fs"; openSync(p, "w");').flags)).toBe(false);
      expect(readOnlyFlags(f('import { openSync } from "node:fs"; openSync(p, "wx", 0o600);').flags)).toBe(false);
    });
    it("reads are not writes", () => {
      expect(apis('import { readFileSync, existsSync, readdirSync, statSync } from "node:fs"; readFileSync(p); existsSync(p);')).toEqual([]);
    });
  });
});
