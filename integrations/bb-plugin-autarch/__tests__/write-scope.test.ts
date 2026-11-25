import { describe, expect, it } from "vitest";
import { fsWriteUses, atomicJsonUses, isFdOnly, isOpener, parse, parseFile, readOnlyFlags, rel, serverSources } from "./static-scan.js";

// Plan 1.3.8 and Task 2.7: fs writes live only in ruling.ts and export.ts. backup.ts may open O_RDONLY and fsync
// (the backup itself is written by SQLite's VACUUM INTO); rootrun.ts may only open files O_RDONLY.
// ledger.ts joined the writers with the rulings ledger: it only appends one line per pick to that one file.
// otherimages.ts joined with the Other box's images: it writes only under its own images root, with hashed directory
// names, generated file names, exclusive create and lstat checks on every directory (see its header).
// trigger.ts joined with the update trigger: it only touches or creates one named file (TRIGGER_FILE)
// under the update directory it is given, and never writes any content to it (see its header).
// The governor adapter writes policy, throttle state and inboxes only in explicit live mode (bead mk-z0lht.1).
const FULL = new Set(["ruling.ts", "export.ts", "ledger.ts", "otherimages.ts", "trigger.ts"]);
function governorWriteAllowed(use: ReturnType<typeof fsWriteUses>[number], file = "governor-runtime.ts"): boolean {
  const a = use.args?.map(s => s.replace(/\s/g, "")) ?? [];
  if (file === "governor.ts") {
    if (use.api === "mkdir") return a[0] === "dirname(path)";
    if (use.api === "writeFile") return a[0] === "`${path}.tmp`" && a[1] === "JSON.stringify(obj,null,1)";
    if (use.api === "rename") return a[0] === "`${path}.tmp`" && a[1] === "path";
    return false;
  }
  if (use.api === "namespace") {
    if (use.context?.replace(/\s/g, "") === "disk:InboxFS=fs") return true;
    if (use.callee === "applyOperator") return use.position === 4 && a.join(",") === "cfg,level,force,exec,fs";
    if (use.callee === "fileProtectedCard") return use.position === 6 && a.join(",") === "cfg,level,pace,rn,vz,exec,fs";
    if (use.callee === "prepareInboxes") return use.position === 7 && a.join(",") === "cfg,awaitthreads(),old,next,pace,why,Date.now(),fs,deps.roots";
    return use.callee === "atomicJson" && use.position === 2 && ["fs", "disk"].includes(a[2]);
  }
  if (use.api === "atomicJson") return (a[0] === 'join(cfg.state_dir,`card-ask-L${level}.json`)' && a[1] === "ask" && a[2] === "disk") || (a[0] === 'join(cfg.state_dir,"throttle.json")' && a[1] === "throttle" && a[2] === "fs");
  if (use.api === "mkdir") return a[0] === "cfg.state_dir";
  if (use.api === "copyFile") return a[0] === "path" && a[1]?.startsWith('join(cfg.state_dir,`operator.toml.bak-');
  if (use.api === "writeFile") return a[0] === "`${path}.governor.tmp`" && ["old", "next"].includes(a[1]);
  if (use.api === "rename") return a[0] === "`${path}.governor.tmp`" && a[1] === "path";
  if (use.api === "unlink") return a[0] === "join(cfg.state_dir,f)";
  if (use.api === "appendFile") return a[0] === 'join(cfg.storage_root,id,"inbox.md")';
  return false;
}
// Audited reminder writer: only pinned spool descriptors may reach writing opens,
// renames or unlink. Ancestor descriptors are read-only; only the named spool is created.
function reminderWriteAllowed(use: ReturnType<typeof fsWriteUses>[number]): boolean {
  const a = use.args?.map(s => s.replace(/\s/g, "")) ?? [];
  if (use.api === "openSync") {
    if (a[1] === "C.O_RDONLY|C.O_DIRECTORY|C.O_NOFOLLOW") return a[0] === '"/"' || a[0] === "this.path(fd,part)" || a[0] === "this.path(parent,name)" || a[0] === 'this.path(pluginDir,"followthrough")' || a[0] === "this.spoolPath(parent,name)";
    return a[0] === "this.spoolPath(parent,name)" && a[1] === "flags|C.O_NOFOLLOW|C.O_NONBLOCK" && a[2] === "0o600";
  }
  if (use.api === "mkdirSync") return ["this.spoolPath(parent,name)", 'this.path(pluginDir,"followthrough")'].includes(a[0]) && a[1] === "{mode:0o700}";
  if (use.api === "writeSync") return a.join(",") === "fd,bytes,n,bytes.length-n";
  if (use.api === "fsyncSync") return a.length === 1 && ["fd", "parent"].includes(a[0]);
  if (use.api === "renameSync") return a[0] === "this.spoolPath(parent,temp)" && a[1] === "this.spoolPath(parent,name)";
  if (use.api === "unlinkSync") return a[0] === "this.spoolPath(parent,name)";
  if (use.api === "rmdirSync") return a[0] === "this.spoolPath(parent,temp)";
  return false;
}
const RDONLY_PLUS_FSYNC = new Set(["backup.ts"]);
// settled.ts reads one coordinator's settled-for-mk.md O_RDONLY | O_NOFOLLOW and writes nothing.
// docread.ts is the document reader: it opens one allowlisted file O_RDONLY | O_NOFOLLOW and writes nothing.
// evidencestore.ts reads only a content hash beneath the separate evidence root.
const RDONLY_ONLY = new Set(["rootrun.ts", "docread.ts", "settled.ts", "evidencestore.ts"]);

describe("fs write scope (plan Task 2.7)", () => {
  it("only ruling.ts, export.ts, ledger.ts, otherimages.ts and trigger.ts write; backup.ts only opens read-only and fsyncs; rootrun.ts and docread.ts only open read-only", () => {
    const problems: string[] = [];
    for (const f of serverSources()) {
      const name = rel(f);
      if (FULL.has(name)) continue;
      const governor = ["governor-runtime.ts", "governor.ts"].includes(name);
      const sf = parseFile(f);
      for (const u of [...fsWriteUses(sf, name === "governor-runtime.ts" ? ["disk"] : name === "governor.ts" ? ["fs"] : [], governor), ...atomicJsonUses(sf)]) {
        if (governor && governorWriteAllowed(u, name)) continue;
        if (name === "followthrough-reminder.ts" && reminderWriteAllowed(u)) continue;
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
    expect(uses("rootrun.ts")).toEqual(["openSync"]);
    expect(uses("docread.ts")).toEqual(["openSync"]);
    expect(uses("settled.ts")).toEqual(["openSync"]);
    expect(uses("evidencestore.ts")).toEqual(["openSync"]);
    expect(uses("trigger.ts")).toEqual(["openSync"]);
    expect(uses("followthrough-reminder.ts")).toEqual(expect.arrayContaining(["openSync", "mkdirSync", "writeSync", "renameSync", "unlinkSync", "fsyncSync"]));
    // Pilot configuration is shared with the filer, not plugin-owned. Metrics use
    // Store's SQLite events; this module stays a filesystem reader (bead mk-bzi8s.12).
    expect(uses("pilotmetrics.ts")).toEqual([]);
  });
  it("allows only governor policy/state/inbox calls and detects injected filesystem writes", () => {
    const sf = parseFile(serverSources().find(f => rel(f) === "governor-runtime.ts")!);
    const allowed = [...fsWriteUses(sf, ["disk"], true), ...atomicJsonUses(sf)];
    expect(allowed.length).toBeGreaterThan(5); expect(allowed.every(u => governorWriteAllowed(u))).toBe(true);
    const core = fsWriteUses(parseFile(serverSources().find(f => rel(f) === "governor.ts")!), ["fs"], true);
    expect(core.map(u => u.api)).toEqual(["mkdir", "writeFile", "rename"]); expect(core.every(u => governorWriteAllowed(u, "governor.ts"))).toBe(true);
    for (const src of ['disk.writeFile("/unexpected", next)', 'fs.appendFile(join(cfg.storage_root,id,"other.md"), text)', 'disk.rename(path, "/unexpected")', 'disk.rm(cfg.state_dir)', 'disk.unlink(join(other,f))']) {
      const uses = fsWriteUses(parse(`import * as fs from "node:fs/promises"; ${src}`), ["disk"]);
      expect(uses.length).toBe(1); expect(uses.every(u => governorWriteAllowed(u)), src).toBe(false);
    }
  });
  it("audits reminder writes narrowly and catches injected destinations and unsafe opens", () => {
    const sf = parseFile(serverSources().find(f => rel(f) === "followthrough-reminder.ts")!);
    const uses = fsWriteUses(sf);
    expect(uses.length).toBeGreaterThan(6); expect(uses.every(reminderWriteAllowed)).toBe(true);
    expect(sf.text).toContain("if (!this.spoolDirs.has(fd)) throw");
    for (const code of ['openSync(path, C.O_WRONLY)', 'mkdirSync(dataDir)', 'unlinkSync(path)', 'renameSync(this.spoolPath(parent,temp), path)', 'writeFileSync(path, bytes)']) {
      const bad = fsWriteUses(parse(`import { openSync, mkdirSync, unlinkSync, renameSync, writeFileSync } from "node:fs"; ${code}`));
      expect(bad.length).toBeGreaterThan(0); expect(bad.every(reminderWriteAllowed)).toBe(false);
    }
  });
  it("detects namespace values, aliases and arbitrary atomicJson destinations", () => {
    for (const text of ['import * as fs from "node:fs/promises"; unknownWriter(fs)', 'import * as fs from "node:fs/promises"; const alias = fs', 'import * as fs from "node:fs/promises"; const writer = fs.writeFile']) {
      const uses = fsWriteUses(parse(text), [], true);
      expect(uses.length, text).toBeGreaterThan(0); expect(uses.every(u => governorWriteAllowed(u)), text).toBe(false);
    }
    for (const text of ['atomicJson("/unexpected", x, fs)', 'import { atomicJson as save } from "./governor.js"; save("/unexpected", x, disk)']) {
      const uses = atomicJsonUses(parse(text)); expect(uses).toHaveLength(1); expect(uses.every(u => governorWriteAllowed(u))).toBe(false);
    }
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
