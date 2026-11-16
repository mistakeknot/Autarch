import { it, expect } from "vitest";
import Database from "better-sqlite3";
import { z } from "zod";
import golden from "./fixtures/governor-golden.json";
import { DEFAULTS, choose, applyFloor, measurePace, editOperator, readFloor, type Config, type Account, type Samples, type Operator } from "../governor-policy.js";
import { Governor, GovernorStore, type GovernorIO, type State, type Decision } from "../governor.js";

const cfg = z.custom<Partial<Config>>(v => !!v && typeof v === "object");
const floor = z.object({ level: z.number(), until: z.string(), source: z.string() }).nullable();
const accounts = z.custom<Account[]>(Array.isArray);
const history = z.custom<Samples>();
type Boundary = { kind: string; result?: unknown; error?: string; level?: number; force?: boolean };
const boundaries = z.custom<Boundary[]>(Array.isArray);
const decisionEvidence = (dec: Decision) => ({ ...dec, held: dec.held?.replaceAll("Error: ", "") ?? null, applied: undefined, failed: dec.applied.some(v => v.includes("FAILED")) });

it("contains recent CLI and upstream protected-card/pending step evidence", () => {
  expect(golden.cases.some(c => c.kind === "cli" && c.args[4] === "recent")).toBe(true);
  const steps = golden.cases.filter(c => c.kind === "step");
  expect(steps.some(c => JSON.stringify(c.args[0]).includes('"card"'))).toBe(true);
  expect(steps.some(c => JSON.stringify(c.args[0]).includes('"pending":true'))).toBe(true);
  expect(steps.some(c => JSON.stringify(c.result).includes('"answered":"a"'))).toBe(true);
  expect(steps.some(c => JSON.stringify(c.result).includes('"renewals":3'))).toBe(true);
});

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
    else if (c.kind === "cli" || c.kind === "step") {
      const cli = c.kind === "cli";
      const a = cli
        ? z.tuple([z.custom<State>(), accounts, z.number(), floor, z.string(), history]).parse(c.args)
        : z.tuple([z.custom<State>(), accounts, z.number(), floor, history, z.boolean(), cfg, boundaries]).parse(c.args);
      const mode = cli || a[5] === true ? "shadow" : "live";
      const config: Config = { ...DEFAULTS, bb_host: "governed", pace_mode: cli ? String(a[4]) : "recent", ...(cli ? {} : cfg.parse(a[6])) };
      const samples = history.parse(cli ? a[5] : a[4]);
      const events = cli ? [] : boundaries.parse(a[7]); let eventIndex = 0;
      const take = (kind: string) => {
        const e = events[eventIndex++]; expect(e?.kind, `step boundary ${eventIndex}`).toBe(kind);
        if (e.error) throw Error(e.error);
        return e;
      };
      const db = new Database(":memory:");
      const store = new GovernorStore(db);
      for (const [id, points] of Object.entries(samples)) for (const [observed, utilization, reset] of points) store.db.prepare("INSERT OR REPLACE INTO quota_governor_samples VALUES (?,?,?,?)").run(id, observed, utilization, reset);
      const io: GovernorIO = {
        now: () => a[2], hostname: () => "governed", config: async () => config, mode: async () => mode,
        timerActive: async () => false, accounts: async () => a[1], readJson: async p => p.endsWith("floor.json") ? a[3] : a[0], inferLevel: async () => 0,
        answer: async () => z.enum(["a", "b"]).nullable().parse(take("card_answer").result),
        fileCard: async () => z.string().parse(take("file_card").result),
        apply: async (_cfg, level, force) => { const op = take("apply_operator"); expect([op.level, op.force]).toEqual([level, force]); expect(take("apply_bb_limit").level).toBe(level); },
        notify: async () => { while (events[eventIndex]?.kind === "append_inbox") take("append_inbox"); },
        writeThrottle: async () => {}, publish: () => {},
      };
      const g = new Governor(store, io); await g.tick();
      const dec = g.view().decision!;
      if (cli) got = { ...dec, applied: dec.applied.map(v => v.replace("shadow: would apply", "dry-run: would apply")) };
      else {
        const expected = z.custom<{ decision: Decision; state: State; rc: number }>().parse(c.result);
        expect(decisionEvidence(dec), `step decision ${JSON.stringify(a[0])}`).toEqual(decisionEvidence(expected.decision));
        expect(eventIndex, "all upstream boundary outcomes consumed").toBe(events.length);
        expect(!!g.view().error).toBe(expected.rc !== 0);
        if (mode === "live") {
          const projection = (s: State) => ({ level: s.level, good_streak: s.good_streak, card: s.card, pending: !!s.pending, throttle: s.throttle });
          expect(projection(g.view().state!), "step resulting policy/card/reconciliation state").toEqual(projection(expected.state));
        }
        got = c.result;
      }
      db.close();
    } else if (c.kind === "read_floor") {
      const a = z.tuple([z.unknown(), z.number()]).parse(c.args);
      got = readFloor(a[0], a[1], DEFAULTS);
    } else throw Error(`Unknown fixture ${c.kind}`);
    // Floating point arithmetic uses the same formulas; compare numbers to 12 digits.
    const normalized = (v: unknown): unknown => typeof v === "number" ? Number(v.toPrecision(12)) : Array.isArray(v) ? v.map(normalized) : v && typeof v === "object" ? Object.fromEntries(Object.entries(v).map(([k, x]) => [k, normalized(x)])) : v;
    expect(normalized(got), `${c.kind}: ${JSON.stringify(c.args)}`).toEqual(normalized(c.result));
  }
});
