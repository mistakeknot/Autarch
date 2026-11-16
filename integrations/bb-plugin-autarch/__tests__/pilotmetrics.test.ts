import { describe, it, expect } from "vitest";
import { mkdtempSync, writeFileSync, existsSync } from "node:fs";
import Database from "better-sqlite3";
import { Store } from "../store.js";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { recordPilotPick, pastedErrorV1 } from "../pilotmetrics.js";
describe("pilot pick metrics", () => {
  it("logs facts only, includes needs-context and deduplicates retry", () => {
    const dir = mkdtempSync(join(tmpdir(), "pilot-metrics-"));
    const env = { AUTARCH_CARD_STATE_DIR: dir };
    const store = new Store(new Database(":memory:"));
    writeFileSync(join(dir,"pilots.json"), JSON.stringify({ daycap: {enabled:true,until:"2099-01-01T00:00:00Z"} }));
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
  it("names the pasted error heuristic", () => {
    for (const text of ["Traceback (most recent call last):", "error: missing build", "  at foo (build.js:14)", "exit code: 2"]) expect(pastedErrorV1(text)).toBe(true);
    expect(pastedErrorV1("Please choose blue")).toBe(false);
  });
  it("does not record observations when pilots are absent or disabled", () => {
    const store = new Store(new Database(":memory:"));
    const env = { AUTARCH_CARD_STATE_DIR: mkdtempSync(join(tmpdir(), "pilot-disabled-")) };
    const input = { requestKey: "example", cardKey: "fixture", project: "example", pickId: "pick", time: "2026-11-15T12:00:00Z", optionKind: "ruling-only", other: false };
    recordPilotPick(input, store, env);
    writeFileSync(join(env.AUTARCH_CARD_STATE_DIR, "pilots.json"), JSON.stringify({ daycap: { enabled: true, until: "2099-01-01T00:00:00Z" } }));
    recordPilotPick(input, store, { ...env, AUTARCH_NEEDSMK_PILOTS_OFF: "1" });
    expect(store.pilotMetrics()).toEqual([]); store.db.close();
  });
});

it("does not classify ordinary prose as pasted errors", () => {
  for (const text of ["at 10:30 tomorrow", "We can discuss the stack trace later", "The error: label could be clearer", "Traceback is the title of a book", "exit code 2 is the choice we discussed"]) expect(pastedErrorV1(text)).toBe(false);
});
