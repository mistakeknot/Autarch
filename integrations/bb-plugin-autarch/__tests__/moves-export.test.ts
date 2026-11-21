// The moves rollback export (`bb home moves`): stored rows are read back unchanged and grouped as before.
// Rows are inserted with plain SQL because nothing in the plugin opens a move any more (bead mk-j5ipa.10).
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { homeCli } from "../cli.js";
import type { Delegation } from "../delegation.js";
import type { Service } from "../service.js";
import { makeEnv, type Env, verifiedDelegation } from "./service-helpers.js";

let env: Env;
let svc: Service;
let dele: Delegation;
beforeEach(() => {
  env = makeEnv(["Autarch"]);
  svc = env.open();
  dele = verifiedDelegation(svc);
});
afterEach(() => env.cleanup());

function seed(): void {
  const ins = svc.store.db.prepare(
    `INSERT INTO moves(task_id, generation, kind, payload_json, state, opened_by, opened_at, claimed_at, claimed_by, closed_at, closed_by, evidence, report_state, report_json, report_at, skipped_at)
     VALUES (@task_id, @generation, @kind, @payload_json, @state, @opened_by, @opened_at, @claimed_at, @claimed_by, @closed_at, @closed_by, @evidence, @report_state, @report_json, @report_at, @skipped_at)`,
  );
  const base = { claimed_at: null, claimed_by: null, closed_at: null, closed_by: null, evidence: null, report_state: null, report_json: null, report_at: null, skipped_at: null };
  ins.run({ ...base, task_id: "T1", generation: 1, kind: "pr", payload_json: '{"kind":"pr"}', state: "closed", opened_by: "card", opened_at: "2026-10-12T03:00:00.000Z", closed_at: "2026-10-12T04:00:00.000Z", closed_by: "github", evidence: "MERGED" });
  ins.run({ ...base, task_id: "T2", generation: 1, kind: "script", payload_json: '{"kind":"script"}', state: "closed", opened_by: "card", opened_at: "2026-10-12T03:10:00.000Z", closed_at: "2026-10-12T04:30:00.000Z", closed_by: "home", evidence: "moves retired", report_state: "succeeded", report_json: '{"outcome":"succeeded"}', report_at: "2026-10-12T03:20:00.000Z", skipped_at: "2026-10-12T03:15:00.000Z" });
  ins.run({ ...base, task_id: "T3", generation: 2, kind: "read", payload_json: '{"kind":"read"}', state: "open", opened_by: "pick", opened_at: "2026-10-12T03:20:00.000Z" });
}

describe("bb home moves (rollback export)", () => {
  it("reads every stored move back unchanged, newest first, with the closed ones in the audit list", async () => {
    seed();
    const cli = homeCli({
      svc,
      asks: new Asks(svc),
      catchup: new Catchup(svc, dele),
      rule: (id, option, reason, ctx) => dele.rule(id, option, reason, ctx),
      isVizier: () => false,
    });
    const r = await Promise.resolve(cli.run(["moves", "--json"], {}));
    expect(r.exitCode).toBe(0);
    const out = JSON.parse(r.stdout!) as { moves: { task_id: string; generation: number; state: string }[]; groups: Record<string, unknown[]> };
    expect(out.moves.map((m) => `${m.task_id}/${m.generation}/${m.state}`)).toEqual(["T3/2/open", "T2/1/closed", "T1/1/closed"]);
    expect(out.moves[1]).toMatchObject({ kind: "script", closed_by: "home", evidence: "moves retired", report_state: "succeeded", skipped_at: "2026-10-12T03:15:00.000Z" });
    expect(Object.keys(out.groups).sort()).toEqual(["audit", "hidden", "later", "reported", "yourMove"]);
    expect(out.groups.yourMove).toHaveLength(1);
    expect(out.groups.audit).toEqual([
      { task_id: "T2", generation: 1, kind: "script", closed_at: "2026-10-12T04:30:00.000Z", closed_by: "home", evidence: "moves retired" },
      { task_id: "T1", generation: 1, kind: "pr", closed_at: "2026-10-12T04:00:00.000Z", closed_by: "github", evidence: "MERGED" },
    ]);
  });
});
