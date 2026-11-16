import { it, expect } from "vitest";
import Database from "better-sqlite3";
import { z } from "zod";
import golden from "./fixtures/governor-golden.json";
import { DEFAULTS, choose, applyFloor, measurePace, editOperator, type Config, type Account, type Samples, type Operator } from "../governor-policy.js";
import { Governor, GovernorStore, type GovernorIO, type State } from "../governor.js";

const cfg = z.custom<Partial<Config>>(v => !!v && typeof v === "object");
const floor = z.object({ level: z.number(), until: z.string(), source: z.string() }).nullable();
const accounts = z.custom<Account[]>(Array.isArray);

it(`matches ${golden.cases.length} Python policy and CLI dry-run observations`, async () => {
  for (const c of golden.cases) {
    let got: unknown;
    if (c.kind === "choose") {
      const a = z.tuple([z.number(), z.number().nullable(), z.number(), cfg]).parse(c.args);
      got = choose(a[0], a[1], a[2], { ...DEFAULTS, ...a[3] });
    } else if (c.kind === "apply_floor") {
      const a = z.tuple([z.number(), z.number(), z.number(), z.string(), floor]).parse(c.args);
      got = applyFloor(...a);
    } else if (c.kind === "pooled_pace" || c.kind === "recent_pace") {
      const a = z.tuple([accounts, z.number(), cfg, z.custom<Samples>().optional()]).parse(c.args);
      got = measurePace(a[0], a[1], { ...DEFAULTS, ...a[2], pace_mode: c.kind === "pooled_pace" ? "cumulative" : "recent" }, a[3] ?? {});
    } else if (c.kind === "edit_operator") {
      const a = z.tuple([z.string(), z.custom<Operator>()]).parse(c.args);
      got = editOperator(...a);
    }
    else if (c.kind === "cli") {
      const a = z.tuple([z.custom<State>(), accounts, z.number(), floor]).parse(c.args);
      const db = new Database(":memory:");
      const store = new GovernorStore(db);
      const io: GovernorIO = {
        now: () => a[2], hostname: () => "governed", config: async () => ({ ...DEFAULTS, bb_host: "governed", pace_mode: "cumulative" }), mode: async () => "shadow",
        timerActive: async () => { throw Error("unexpected live operation"); }, accounts: async () => a[1], readJson: async p => p.endsWith("floor.json") ? a[3] : a[0], inferLevel: async () => 0,
        answer: async () => null, fileCard: async () => { throw Error("unexpected live operation"); }, apply: async () => { throw Error("unexpected live operation"); }, notify: async () => { throw Error("unexpected live operation"); }, writeThrottle: async () => { throw Error("unexpected live operation"); }, publish: () => {},
      };
      const g = new Governor(store, io); await g.tick();
      const dec = g.view().decision!;
      got = { ...dec, applied: dec.applied.map(v => v.replace("shadow: would apply", "dry-run: would apply")) };
      db.close();
    } else throw Error(`Unknown fixture ${c.kind}`);
    // Floating point arithmetic uses the same formulas; compare numbers to 12 digits.
    const normalized = (v: unknown): unknown => typeof v === "number" ? Number(v.toPrecision(12)) : Array.isArray(v) ? v.map(normalized) : v && typeof v === "object" ? Object.fromEntries(Object.entries(v).map(([k, x]) => [k, normalized(x)])) : v;
    expect(normalized(got), `${c.kind}: ${JSON.stringify(c.args)}`).toEqual(normalized(c.result));
  }
});
