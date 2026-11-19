import { describe, it, expect, vi } from "vitest";
import { mkdtempSync, writeFileSync, existsSync } from "node:fs";
import Database from "better-sqlite3";
import { Store } from "../store.js";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { recordPilotPick, pastedErrorV1 } from "../pilotmetrics.js";
import { openStore, tmpDir } from "./helpers.js";
describe("pilot pick metrics", () => {
  it.each([
    { name: "coldread only", pilots: { coldread: { enabled: true, until: "2099-01-01T00:00:00Z" } } },
    { name: "linkcheck only", pilots: { linkcheck: { enabled: true, until: "2099-01-01T00:00:00Z" } } },
    { name: "coldread with stale daycap", pilots: {
      daycap: { enabled: true, until: "2000-01-01T00:00:00Z" },
      coldread: { enabled: true, until: "2099-01-01T00:00:00Z" },
    } },
  ])("$name logs facts only, includes needs-context and deduplicates retry", ({ pilots }) => {
    const dir = mkdtempSync(join(tmpdir(), "pilot-metrics-"));
    const env = { AUTARCH_CARD_STATE_DIR: dir };
    const store = new Store(new Database(":memory:"));
    writeFileSync(join(dir,"pilots.json"), JSON.stringify(pilots));
    writeFileSync(join(dir,"filings.json"), JSON.stringify({ example:{request:{Type:"design",Urgent:true}} }));
    const input = { requestKey:"example", cardKey:"TEST-9", project:"example", pickId:"pick-1", time:"2026-11-15T12:00:00Z", optionKind:"needs-context", other:true, reason:"error: secret-value" };
    recordPilotPick(input,store,env);recordPilotPick(input,store,env);
    expect(existsSync(join(dir,"pilots-metrics.jsonl"))).toBe(false);
    const rows = store.pilotMetrics(); expect(rows).toHaveLength(1);
    const raw=JSON.stringify(rows[0]);
    expect(raw).not.toContain("secret-value");
    expect(raw.trim().split("\n")).toHaveLength(1);
    expect(JSON.parse(raw)).toMatchObject({card_type:"design",exempt_flags:["urgent"],other:true,pasted_error:true,free_text_length:19,option_kind:"needs-context"});
    expect(store.events().filter(e => e.type === "pilot-pick-metric")).toHaveLength(1);
    store.db.close();
  });
  it("treats daycap as retired while preserving historical observations", () => {
    const dir = tmpDir();
    const store = new Store(new Database(":memory:"));
    try {
      writeFileSync(join(dir.dir, "pilots.json"), JSON.stringify({ daycap: { enabled: true, until: "2099-01-01T00:00:00Z" } }));
      const historical = { time: "2026-11-15T12:00:00Z", event_id: "pick:historical", event: "picked" };
      store.recordPilotMetric(historical);
      recordPilotPick({ requestKey: "example", cardKey: "fixture", project: "example", pickId: "new", time: historical.time, optionKind: "ruling-only", other: false }, store, { AUTARCH_CARD_STATE_DIR: dir.dir });
      expect(store.pilotMetrics()).toEqual([historical]);
    } finally { store.close(); dir.cleanup(); }
  });
  it("names the pasted error heuristic", () => {
    for (const text of ["Traceback (most recent call last):", "error: missing build", "  at foo (build.js:14)", "exit code: 2"]) expect(pastedErrorV1(text)).toBe(true);
    expect(pastedErrorV1("Please choose blue")).toBe(false);
  });
  it("does not record observations when pilots are absent or disabled", () => {
    const store = new Store(new Database(":memory:"));
    const env = { AUTARCH_CARD_STATE_DIR: mkdtempSync(join(tmpdir(), "pilot-disabled-")) };
    const input = { requestKey: "example", cardKey: "fixture", project: "example", pickId: "pick", time: "2026-11-15T12:00:00Z", optionKind: "ruling-only", other: false };
    recordPilotPick(input, store, env);
    writeFileSync(join(env.AUTARCH_CARD_STATE_DIR, "pilots.json"), JSON.stringify({ coldread: { enabled: true, until: "2099-01-01T00:00:00Z" } }));
    recordPilotPick(input, store, { ...env, AUTARCH_NEEDSMK_PILOTS_OFF: "1" });
    expect(store.pilotMetrics()).toEqual([]); store.db.close();
  });
});

describe("pilot metric keyed dedup (mk-bzi8s.13)", () => {
  const legacy = { time: "2026-11-15T12:00:00Z", event_id: "pick:legacy", event: "picked" };
  const watermarkKey = "pilot-metric-dedup/v1/seq";
  const eventSeq = (store: Store) => (store.db.prepare("SELECT COALESCE(MAX(seq), 0) AS seq FROM events").get() as { seq: number }).seq;
  it("seeds legacy IDs once, then a fresh pick after restart reads only above the watermark without changing the schema", () => {
    const dir = tmpDir(); const file = join(dir.dir, "data.db"); let store = openStore(file);
    try {
      const schema = store.db.prepare("SELECT type, name, sql FROM sqlite_master ORDER BY name").all();
      store.recordEvent("pilot-pick-metric", null, legacy);
      for (let i = 0; i < 32; i++) store.recordEvent("unrelated", null, { i });
      store.recordPilotMetric(legacy); // first use must retain the existing observation, without a duplicate
      const seededSeq = eventSeq(store);
      store.close();
      const traced: string[] = [];
      store = new Store(new Database(file, { verbose: sql => traced.push(String(sql)) }));
      const watermark = Number(store.setting(watermarkKey) ?? 0);
      for (let i = 0; i < 32; i++) store.recordEvent("unrelated", null, { i });
      traced.length = 0;
      const fresh = { ...legacy, event_id: "pick:fresh" };
      // The first fresh pick belongs inside the trace: retries alone cannot prove the cost of a new pick.
      store.recordPilotMetric(legacy); store.recordPilotMetric(fresh); store.recordPilotMetric(fresh);
      const reads = traced.map(sql => sql.replace(/\s+/g, " ")).filter(sql => /^\s*SELECT.*\bFROM\s+events\b/i.test(sql));
      expect(reads).toHaveLength(2);
      const rangeRead = reads.find(sql => /WHERE seq > /i.test(sql));
      expect(rangeRead).toMatch(new RegExp(`\\bFROM events WHERE seq > ${watermark}(?:\\.0)? AND type = 'pilot-pick-metric'.*ORDER BY seq`, "i"));
      const maxRead = reads.find(sql => /SELECT COALESCE\(MAX\(seq\), 0\) AS seq FROM events/i.test(sql));
      expect(maxRead).toBeDefined();
      const maxPlan = store.db.prepare(`EXPLAIN QUERY PLAN ${maxRead}`).all() as { detail: string }[];
      expect(maxPlan.some(row => /SEARCH events/i.test(row.detail))).toBe(true);
      expect(maxPlan.some(row => /SCAN events/i.test(row.detail))).toBe(false);
      expect(watermark).toBe(seededSeq);
      const plan = store.db.prepare(`EXPLAIN QUERY PLAN ${rangeRead}`).all() as { detail: string }[];
      expect(plan.some(row => /SEARCH events USING INTEGER PRIMARY KEY/i.test(row.detail))).toBe(true);
      expect(plan.some(row => /SCAN events/i.test(row.detail))).toBe(false);
      expect(Number(store.setting(watermarkKey))).toBe(eventSeq(store));
      expect(store.pilotMetrics().map(m => m.event_id)).toEqual([legacy.event_id, "pick:fresh"]);
      expect(store.db.prepare("SELECT type, name, sql FROM sqlite_master ORDER BY name").all()).toEqual(schema);
    } finally { store.close(); dir.cleanup(); }
  });
  it("repairs an unkeyed metric written during rollback before recording a retried pick", () => {
    const dir = tmpDir(); const file = join(dir.dir, "data.db"); let store = openStore(file);
    const duringRollback = { ...legacy, event_id: "pick:during-rollback" };
    try {
      store.recordPilotMetric(legacy); // sets the ready flag
      store.close(); store = openStore(file);
      // The old writer records the event but knows nothing about the new dedup keys.
      store.recordEvent("pilot-pick-metric", null, duringRollback);
      store.close(); store = openStore(file);
      store.recordPilotMetric(duringRollback);
      expect(store.pilotMetrics()).toEqual([legacy, duringRollback]);
      const prepare = vi.spyOn(store.db, "prepare");
      try {
        store.recordPilotMetric(duringRollback);
        expect(prepare.mock.calls.some(([sql]) => /^\s*SELECT.*\bFROM\s+events\b/i.test(sql.replace(/\s+/g, " ")))).toBe(false);
      } finally { prepare.mockRestore(); }
    } finally { store.close(); dir.cleanup(); }
  });
  it("indexes every rollback event before advancing, including events from a second rollback", () => {
    const dir = tmpDir(); const file = join(dir.dir, "data.db"); let store = openStore(file);
    const a = { ...legacy, event_id: "pick:rollback-a" }, b = { ...legacy, event_id: "pick:rollback-b" };
    const fresh = { ...legacy, event_id: "pick:fresh" }, later = { ...legacy, event_id: "pick:later-rollback" };
    try {
      store.recordPilotMetric(legacy);
      const before = Number(store.setting(watermarkKey));
      store.close(); store = openStore(file);
      store.recordEvent("pilot-pick-metric", null, a); store.recordEvent("unrelated", null, {});
      store.recordEvent("pilot-pick-metric", null, b);
      store.close(); store = openStore(file);
      store.recordPilotMetric(fresh); // must index a and b too, before moving past them
      expect(Number(store.setting(watermarkKey))).toBeGreaterThan(before);
      expect(Number(store.setting(watermarkKey))).toBe(eventSeq(store));
      store.recordPilotMetric(a); store.recordPilotMetric(b);
      store.close(); store = openStore(file);
      store.recordEvent("pilot-pick-metric", null, later);
      store.close(); store = openStore(file);
      store.recordPilotMetric(later); store.recordPilotMetric(later);
      expect(store.pilotMetrics()).toEqual([legacy, a, b, fresh, later]);
      expect(Number(store.setting(watermarkKey))).toBe(eventSeq(store));
    } finally { store.close(); dir.cleanup(); }
  });
  it("advances past an unrelated tail while repairing rollback metrics without copying unrelated IDs", () => {
    const traced: string[] = [];
    const store = new Store(new Database(":memory:", { verbose: sql => traced.push(String(sql)) }));
    const repaired = { ...legacy, event_id: "pick:repair" }, fresh = { ...legacy, event_id: "pick:fresh" };
    try {
      store.recordPilotMetric(legacy);
      store.recordEvent("pilot-pick-metric", null, repaired);
      for (let i = 0; i < 64; i++) store.recordEvent("unrelated", null, { event_id: fresh.event_id, i });
      const tail = eventSeq(store);
      traced.length = 0;
      store.recordPilotMetric(repaired); // repair only: no new event to advance the watermark
      const rangeRead = traced.find(sql => /FROM events WHERE seq > /i.test(sql.replace(/\s+/g, " ")));
      const rows = store.db.prepare(rangeRead!).all() as { id: string }[];
      expect(rows.map(row => row.id)).toEqual([repaired.event_id]);
      expect(Number(store.setting(watermarkKey))).toBe(tail);
      expect(store.setting(`pilot-metric-dedup/v1/id/${fresh.event_id}`)).toBeUndefined();
      store.recordPilotMetric(fresh); store.recordPilotMetric(repaired);
      expect(store.pilotMetrics()).toEqual([legacy, repaired, fresh]);
      expect(Number(store.setting(watermarkKey))).toBe(eventSeq(store));
    } finally { store.close(); }
  });
  it("upgrades an existing ready flag with no watermark without skipping unkeyed events", () => {
    const store = new Store(new Database(":memory:"));
    const missed = { ...legacy, event_id: "pick:old-writer" };
    try {
      store.recordEvent("pilot-pick-metric", null, legacy); store.recordEvent("pilot-pick-metric", null, missed);
      store.setSetting("pilot-metric-dedup/v1/ready", "1");
      store.setSetting(`pilot-metric-dedup/v1/id/${legacy.event_id}`, "1");
      store.recordPilotMetric(legacy);
      expect(Number(store.setting(watermarkKey))).toBe(eventSeq(store));
      store.recordPilotMetric(missed);
      expect(store.pilotMetrics()).toEqual([legacy, missed]);
      expect(Number(store.setting(watermarkKey))).toBe(eventSeq(store));
    } finally { store.close(); }
  });
  it("rolls the observation and dedup state back together", () => {
    const store = new Store(new Database(":memory:"));
    try {
      expect(() => store.atomically(() => { store.recordPilotMetric(legacy); throw new Error("interrupted"); })).toThrow("interrupted");
      expect(store.pilotMetrics()).toEqual([]);
      expect(store.setting(watermarkKey)).toBeUndefined();
      store.recordPilotMetric(legacy); store.recordPilotMetric(legacy);
      expect(store.pilotMetrics()).toEqual([legacy]);
      expect(Number(store.setting(watermarkKey))).toBe(eventSeq(store));
    } finally { store.close(); }
  });
});

it("does not classify ordinary prose as pasted errors", () => {
  for (const text of ["at 10:30 tomorrow", "We can discuss the stack trace later", "The error: label could be clearer", "Traceback is the title of a book", "exit code 2 is the choice we discussed"]) expect(pastedErrorV1(text)).toBe(false);
});
