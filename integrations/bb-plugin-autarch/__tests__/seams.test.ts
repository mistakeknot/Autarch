import { describe, expect, it } from "vitest";
import { parse, parseFile, rel, seamViolations, serverSources } from "./static-scan.js";

// Test seams (MigrateOptions.test and BackupTestSeams.{hook,skipHold}, StoreOptions.hook) may be defined in
// production types but production code must never pass them. This is an import-graph and syntax check, not a text grep:
// a call to RegExp#test is not a seam.
describe("production code never passes the test seams (Task 2.3 ledger c)", () => {
  it("no server source assigns a seam property", () => {
    const bad = serverSources().flatMap((f) => seamViolations(parseFile(f)));
    expect(bad).toEqual([]);
  });

  it("the seams still exist where tests use them, so the names being checked are real", () => {
    const names = (n: string) => serverSources().find((f) => rel(f) === n)!;
    expect(parseFile(names("migrations.ts")).getText()).toContain("test?: BackupTestSeams");
    expect(parseFile(names("backup.ts")).getText()).toContain("skipHold?: boolean");
    expect(parseFile(names("store.ts")).getText()).toContain("hook?: StoreHook");
  });

  describe("positive fixtures", () => {
    it("flags every way of passing a seam", () => {
      expect(seamViolations(parse("migrate(db, { test: { skipHold: true } });")).length).toBe(2);
      expect(seamViolations(parse("const test = {}; migrate(db, { test });")).length).toBe(1);
      expect(seamViolations(parse("new Store(db, { hook: (s) => {} });")).length).toBe(1);
      expect(seamViolations(parse("opts.test = seams;")).length).toBe(1);
      expect(seamViolations(parse('migrate(db, { "test": {} });')).length).toBe(1);
    });
    it("does not flag RegExp#test, reads, or unrelated properties", () => {
      expect(seamViolations(parse("if (/x/.test(s)) {} const h = opts.test?.hook; migrate(db, { log, now });"))).toEqual([]);
    });
    it("allows relaying the caller's own options through the layers", () => {
      expect(seamViolations(parse("migrate(db, { test: opts.test, log }); this.hook = opts.hook ?? (() => {});"))).toEqual([]);
    });
  });
});
