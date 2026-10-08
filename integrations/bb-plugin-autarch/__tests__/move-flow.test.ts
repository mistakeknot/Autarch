// Your move, backend (plan Revision 2 and 6): every closure rule, the report parser and its once-only wakes,
// the grouping selector, the card close, the RPC input contract and the rollback CLI. Fakes only: no network,
// no gh, no real clock.
import { afterEach, describe, expect, it } from "vitest";
import { z } from "zod";
import { homeMethods } from "../contract.js";
import { homeCli } from "../cli.js";
import { groupMoves } from "../moveselect.js";
import { PrPoller, PR_URL, type GhView } from "../movepoll.js";
import { parseReport, ReportWatcher } from "../movereport.js";
import { HOME_MOVE_LABEL } from "../cardwrites.js";
import { cleanupEnvs, rig, SHA, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const PR = "https://github.com/o/r/pull/12";
const SCRIPT = "/opt/run.sh";
const fence = (m: object) => "```home-move\n" + JSON.stringify(m) + "\n```\n";
const prMove = (url = PR) => ({ schema: "home-move/v1", kind: "pr", pr: { url } });
const scriptMove = (sha = SHA) => ({ schema: "home-move/v1", kind: "script", script: { path: SCRIPT, sha256: sha } });

async function withMove(r: Rig, move: object, o: Parameters<Rig["card"]>[0] = {}) {
  const t = r.card(o);
  t.description += fence(move);
  await r.poll();
  return t;
}
const poller = (r: Rig, ghView: GhView) => new PrPoller({ svc: r.svc, ghView, clock: () => r.env.clock.t });
const watcher = (r: Rig, extra: Partial<ConstructorParameters<typeof ReportWatcher>[0]> = {}) =>
  new ReportWatcher({ svc: r.svc, listComments: async (id) => r.fake.comments.filter((c) => c.taskId === id) as never, ...extra });
const wakes = (r: Rig) => r.db.prepare("SELECT recipient, op, payload FROM obligations WHERE kind = 'move-report' ORDER BY rowid").all() as { recipient: string; op: string; payload: string }[];

describe("closure rules: only independent evidence or supersession closes a move", () => {
  it("a claim never closes the move, and says reported, not verified", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    const res = r.svc.claimMove(t.id, 1, "merged it");
    expect(res).toMatchObject({ ok: true, replay: false, state: "claimed", claim: "reported, not verified" });
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "claimed", closed_at: null, closed_by: null });
    // a replayed claim changes nothing
    expect(r.svc.claimMove(t.id, 1)).toMatchObject({ ok: true, replay: true, state: "claimed" });
    // and a claimed move stays pollable
    await poller(r, async () => ({ state: "OPEN", mergedAt: null })).pollOnce();
    expect(r.svc.store.move(t.id, 1)!.state).toBe("claimed");
  });

  it("a failed poll never closes, is counted, and backs off", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    let calls = 0;
    const p = poller(r, async () => {
      calls++;
      throw new Error("gh down");
    });
    await p.pollOnce();
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "open", last_error: "gh down" });
    expect(p.stats).toMatchObject({ polls: 1, errors: 1, closed: 0 });
    await p.pollOnce();
    expect(calls).toBe(1); // backing off
    r.advance(10 * 60_000);
    await p.pollOnce();
    expect(calls).toBe(2);
    expect(r.svc.store.move(t.id, 1)!.state).toBe("open");
  });

  it("an unknown state or OPEN never closes", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    const p = poller(r, async () => ({ state: "WEIRD", mergedAt: null }));
    await p.pollOnce();
    expect(p.stats).toMatchObject({ errors: 1, closed: 0 });
    const o = poller(r, async () => ({ state: "OPEN", mergedAt: null }));
    await o.pollOnce();
    expect(o.stats).toMatchObject({ errors: 0, closed: 0 });
    expect(r.svc.store.move(t.id, 1)!.state).toBe("open");
  });

  it("MERGED closes, with evidence of state and mergedAt", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    const p = poller(r, async () => ({ state: "MERGED", mergedAt: "2026-10-02T00:00:00Z" }));
    await p.pollOnce();
    const m = r.svc.store.move(t.id, 1)!;
    expect(m).toMatchObject({ state: "closed", closed_by: "github" });
    expect(JSON.parse(m.evidence!)).toEqual({ url: PR, state: "MERGED", mergedAt: "2026-10-02T00:00:00Z" });
    expect(p.stats.closed).toBe(1);
  });

  it("CLOSED closes too, and a claimed move is closed by GitHub, not by the claim", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    r.svc.claimMove(t.id, 1);
    await poller(r, async () => ({ state: "CLOSED", mergedAt: null })).pollOnce();
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "closed", closed_by: "github" });
  });

  it("supersede closes the old generation", async () => {
    const r = rig();
    const t = await withMove(r, prMove(), { key: "k-sup" });
    r.edit(t, { description: r.desc({ key: "k-sup", question: "Rewritten?" }) + fence(prMove("https://github.com/o/r/pull/13")) });
    await r.poll();
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "closed", closed_by: "home", evidence: "superseded" });
    expect(r.svc.store.move(t.id, 2)!.state).toBe("open");
  });

  it("replay is idempotent: a second poll or close changes nothing", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    const p = poller(r, async () => ({ state: "MERGED", mergedAt: "2026-10-02T00:00:00Z" }));
    await p.pollOnce();
    const before = JSON.stringify(r.svc.store.move(t.id, 1));
    r.advance(10 * 60_000);
    await p.pollOnce(); // closed moves are not polled
    expect(r.svc.closeMoveOn(t.id, 1, "github", "again")).toBe(false);
    expect(JSON.stringify(r.svc.store.move(t.id, 1))).toBe(before);
    expect(r.db.prepare("SELECT COUNT(*) c FROM move_closes WHERE task_id = ?").get(t.id)).toEqual({ c: 1 });
  });

  it("the poller reads only valid pull request URLs", () => {
    expect(PR_URL.test(PR)).toBe(true);
    for (const bad of ["http://github.com/o/r/pull/1", "https://github.com/o/r/issues/1", "https://github.com/o/r/pull/1?x=1", "https://github.com/o/r/pull/0", "https://evil.com/o/r/pull/1", "-h"]) expect(PR_URL.test(bad)).toBe(false);
  });

  it("the run loop polls at once, then per interval, and stops on abort", async () => {
    const r = rig();
    await withMove(r, prMove());
    let n = 0;
    const ac = new AbortController();
    const p = new PrPoller({ svc: r.svc, ghView: async () => ({ state: "OPEN", mergedAt: null }), clock: () => r.env.clock.t });
    await p.run(ac.signal, async () => {
      if (++n >= 3) ac.abort();
    });
    expect(p.stats.polls).toBe(3);
  });
});

describe("claim and skip", () => {
  it("skip is Later: it never closes, replays, and refuses a closed move", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    expect(r.svc.skipMove(t.id, 1)).toMatchObject({ ok: true, replay: false, state: "open" });
    expect(r.svc.skipMove(t.id, 1)).toMatchObject({ ok: true, replay: true });
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "open", closed_at: null });
    expect(r.svc.skipMove("nope", 1)).toMatchObject({ ok: false, status: 404 });
    r.svc.closeMoveOn(t.id, 1, "github", "x");
    expect(r.svc.skipMove(t.id, 1)).toMatchObject({ ok: false, status: 409 });
    expect(r.svc.claimMove(t.id, 1)).toMatchObject({ ok: false, status: 409 });
  });

  it("a script claim sets a 2 hour report deadline; a pr claim sets none", async () => {
    const r = rig();
    const s = await withMove(r, scriptMove());
    const p = await withMove(r, prMove());
    const a = r.svc.claimMove(s.id, 1);
    const b = r.svc.claimMove(p.id, 1);
    expect(a).toMatchObject({ ok: true, report_deadline_at: new Date(r.env.clock.t + 2 * 3600_000).toISOString() });
    expect(b).toMatchObject({ ok: true, report_deadline_at: null });
  });
});

describe("report parser", () => {
  it("reads RESULT, both report-tell shapes, and extracts the failing step and error", () => {
    expect(parseReport("RESULT: OK")).toEqual({ outcome: "succeeded", failing_step: null, error_line: null });
    expect(parseReport("install-publish1c mode=live exit=0 failing_step=none")).toMatchObject({ outcome: "succeeded" });
    expect(parseReport("install-xvfb (install): SUCCESS")).toMatchObject({ outcome: "succeeded" });
    expect(parseReport("install-publish1c mode=live exit=2 failing_step=build\nerror: no space left")).toEqual({ outcome: "failed", failing_step: "build", error_line: "error: no space left" });
    expect(parseReport("RESULT: FAILED\nfailing step: unpack\nERROR: bad archive")).toEqual({ outcome: "failed", failing_step: "unpack", error_line: "ERROR: bad archive" });
  });

  it("a failure signal beats a success line, and free text is not a report", () => {
    expect(parseReport("RESULT: OK\nx mode=a exit=1 failing_step=s")).toMatchObject({ outcome: "failed" });
    expect(parseReport("All done, looks good!")).toBeNull();
    expect(parseReport("")).toBeNull();
    expect(parseReport("result: ok")).toBeNull();
  });

  it("treats the body as untrusted: strips escapes, caps sizes", () => {
    const r = parseReport(`RESULT: FAILED\nfailing step: ${"s".repeat(500)}\nERROR: \u001b[31mboom\u001b[0m\u0007 ${"e".repeat(2000)}`)!;
    expect(r.failing_step!.length).toBeLessThanOrEqual(100);
    expect(r.error_line!.length).toBeLessThanOrEqual(300);
    expect(r.error_line).not.toMatch(/\u001b|\u0007/);
    expect(r.error_line!.startsWith("ERROR: boom")).toBe(true);
    // a huge body is truncated before parsing: a result line past the cap is not seen
    expect(parseReport("x".repeat(20_000) + "\nRESULT: OK")).toBeNull();
  });
});

describe("report watcher", () => {
  const comment = (r: Rig, id: string, body: string, over: object = {}) => r.fake.addComment(id, { threadId: "thr_a", body, createdAt: r.env.now(), ...over });

  it("a failed report is stored, never closes or claims, and wakes the owner once", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(60_000);
    comment(r, t.id, `${SCRIPT} sha256:${SHA}\nrun mode=live exit=3 failing_step=build\nerror: disk full`);
    const w = watcher(r);
    expect(await w.sweep()).toMatchObject({ reports: 1, wakes: 1 });
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "open", report_state: "failed", closed_at: null });
    expect(JSON.parse(r.svc.store.move(t.id, 1)!.report_json!)).toMatchObject({ failing_step: "build", error_line: "error: disk full", source: "comment" });
    expect(wakes(r)).toHaveLength(1);
    expect(wakes(r)[0]).toMatchObject({ recipient: "thr_a", op: expect.stringMatching(new RegExp(`^move-report:${t.id}:1:failed:`)) });
    // replay: sweeping again, even from a fresh watcher, adds nothing
    expect(await w.sweep()).toMatchObject({ reports: 0, wakes: 0 });
    expect(await watcher(r).sweep()).toMatchObject({ reports: 0, wakes: 0 });
    expect(wakes(r)).toHaveLength(1);
  });

  it("a success report is stored for display and does not close, wake or claim", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: OK`);
    expect(await watcher(r).sweep()).toMatchObject({ reports: 1, wakes: 0 });
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "open", report_state: "succeeded" });
    expect(wakes(r)).toHaveLength(0);
  });

  it("a report the CLI recorded is not overwritten by an older card comment", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: OK`);
    const w = watcher(r);
    expect(await w.sweep()).toMatchObject({ reports: 1 });
    // The CLI failure lands with a clock behind the card's: order is by what the sweep read, not by time.
    r.svc.store.setMoveReport(t.id, 1, "failed", { outcome: "failed", failing_step: "build", error_line: null, comment_id: "cli:x:1", author: "report-tell", thread_id: null, report_link: null, reported_at: new Date(r.env.clock.t - 3_600_000).toISOString(), source: "cli", cli_ids: ["cli:x:1"], seen: JSON.parse(r.svc.store.move(t.id, 1)!.report_json!).seen } as never);
    expect(await w.sweep()).toMatchObject({ reports: 0, wakes: 1 });
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "open", report_state: "failed" });
    expect(JSON.parse(r.svc.store.move(t.id, 1)!.report_json!)).toMatchObject({ source: "cli", failing_step: "build" });
    // A newer card comment then does replace it, and the CLI failure's wake is not lost.
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: OK`);
    expect(await w.sweep()).toMatchObject({ reports: 1, wakes: 0 });
    expect(r.svc.store.move(t.id, 1)!.report_state).toBe("succeeded");
    expect(wakes(r)).toHaveLength(1);
  });

  it("ignores reports for another script, older than the move, or written by Home", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    comment(r, t.id, `${SCRIPT} sha256:${"b".repeat(64)}\nRESULT: OK`, { createdAt: new Date(r.env.clock.t + 1000).toISOString() });
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: OK`, { createdAt: new Date(r.env.clock.t - 60_000).toISOString() });
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: OK`, { authorName: "Home", createdAt: new Date(r.env.clock.t + 1000).toISOString() });
    comment(r, t.id, `RESULT: OK`, { createdAt: new Date(r.env.clock.t + 1000).toISOString() });
    expect(await watcher(r).sweep()).toMatchObject({ reports: 0, wakes: 0 });
    expect(r.svc.store.move(t.id, 1)!.report_state).toBeNull();
  });

  it("the deadline passing with no report wakes the owner once, and a claim is needed first", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    const w = watcher(r);
    r.advance(3 * 3600_000);
    expect(await w.sweep()).toMatchObject({ wakes: 0 }); // never claimed: no deadline
    r.svc.claimMove(t.id, 1);
    r.advance(2 * 3600_000 - 1000);
    expect(await w.sweep()).toMatchObject({ wakes: 0 });
    r.advance(2000);
    expect(await w.sweep()).toMatchObject({ wakes: 1 });
    expect(r.svc.store.move(t.id, 1)).toMatchObject({ state: "claimed", report_state: "no-report" });
    r.advance(3600_000);
    expect(await w.sweep()).toMatchObject({ wakes: 0 });
    expect(await watcher(r).sweep()).toMatchObject({ wakes: 0 });
    expect(wakes(r).map((x) => x.op)).toEqual([`move-report:${t.id}:1:no-report`]);
  });

  it("a late report replaces no-report, and a failed late report wakes once more under its own key", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.svc.claimMove(t.id, 1);
    r.advance(3 * 3600_000);
    const w = watcher(r);
    await w.sweep();
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: FAILED\nfailing step: x`);
    await w.sweep();
    expect(r.svc.store.move(t.id, 1)!.report_state).toBe("failed");
    expect(wakes(r).map((x) => x.op.replace(/:failed:.*/, ":failed"))).toEqual([`move-report:${t.id}:1:no-report`, `move-report:${t.id}:1:failed`]);
  });

  it("only the owner thread or its live title-successor can author a report; anyone else stays a comment", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: OK`, { threadId: "thr_stranger" });
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: FAILED\nfailing step: x`, { threadId: null });
    expect(await watcher(r).sweep()).toMatchObject({ reports: 0, wakes: 0 });
    expect(r.svc.store.move(t.id, 1)!.report_state).toBeNull();
    expect(wakes(r)).toHaveLength(0);
    // the successor, as the server decides it, is accepted
    const w = watcher(r, { isReporter: async (m, th) => th === m.owner || th === "thr_succ" });
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: OK`, { threadId: "thr_succ" });
    expect(await w.sweep()).toMatchObject({ reports: 1 });
  });

  it("two distinct failed reports wake twice; the same report replayed wakes once", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(1000);
    const w = watcher(r);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: FAILED\nfailing step: one`, { id: "R1" });
    expect(await w.sweep()).toMatchObject({ wakes: 1 });
    expect(await watcher(r).sweep()).toMatchObject({ wakes: 0 });
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: FAILED\nfailing step: two`, { id: "R2" });
    expect(await w.sweep()).toMatchObject({ wakes: 1 });
    expect(wakes(r)).toHaveLength(2);
    expect(new Set(wakes(r).map((x) => x.op)).size).toBe(2);
  });

  it("two distinct failed reports in one sweep wake twice, in order; display uses the latest", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: FAILED\nfailing step: one`, { id: "R1" });
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: FAILED\nfailing step: two`, { id: "R2" });
    expect(await watcher(r).sweep()).toMatchObject({ wakes: 2 });
    expect(wakes(r).map((x) => x.op.split(":").pop())).toEqual(["R1", "R2"]);
    expect(JSON.parse(r.svc.store.move(t.id, 1)!.report_json!)).toMatchObject({ comment_id: "R2", failing_step: "two" });
    expect(await watcher(r).sweep()).toMatchObject({ wakes: 0 });
  });

  it("a failed report followed by an OK report in one sweep wakes once for the failure; display state is succeeded", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: FAILED\nfailing step: one`, { id: "R1" });
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: OK`, { id: "R2" });
    expect(await watcher(r).sweep()).toMatchObject({ wakes: 1 });
    expect(wakes(r).map((x) => x.op.split(":").pop())).toEqual(["R1"]);
    expect(r.svc.store.move(t.id, 1)!.report_state).toBe("succeeded");
    expect(await watcher(r).sweep()).toMatchObject({ wakes: 0 });
  });

  it("routes through resolveTarget, and an unrouted wake is left pending and retried", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(1000);
    comment(r, t.id, `${SCRIPT} ${SHA}\nRESULT: FAILED`);
    let target: string | null = null;
    const logs: string[] = [];
    const w = watcher(r, { resolveTarget: async () => target, log: (m) => logs.push(m) });
    expect(await w.sweep()).toMatchObject({ reports: 1, wakes: 0 });
    await w.sweep();
    expect(logs.filter((l) => l.includes("no thread to wake"))).toHaveLength(1);
    expect(wakes(r)).toHaveLength(0);
    target = "thr_viz";
    expect(await w.sweep()).toMatchObject({ wakes: 1 });
    expect(wakes(r)[0]!.recipient).toBe("thr_viz");
  });

  it("a comment read failure changes nothing and is counted", async () => {
    const r = rig();
    await withMove(r, scriptMove());
    const w = new ReportWatcher({ svc: r.svc, listComments: async () => { throw new Error("tasks down"); } });
    expect(await w.sweep()).toMatchObject({ errors: 1, reports: 0, wakes: 0 });
  });

  it("reads a report file only from an allowed root", async () => {
    const r = rig();
    const t = await withMove(r, scriptMove());
    r.advance(1000);
    comment(r, t.id, `Report: ${r.env.dir}/report.md`);
    const files = new Map([[`${r.env.dir}/report.md`, `${SCRIPT} ${SHA}\nRESULT: FAILED\nfailing step: z`]]);
    const { writeFileSync } = await import("node:fs");
    writeFileSync(`${r.env.dir}/report.md`, files.get(`${r.env.dir}/report.md`)!);
    expect(await watcher(r).sweep()).toMatchObject({ reports: 0 }); // no roots: files are not read
    expect(await watcher(r, { reportRoots: ["/nonexistent-root"] }).sweep()).toMatchObject({ reports: 0 });
    expect(await watcher(r, { reportRoots: [r.env.dir] }).sweep()).toMatchObject({ reports: 1, wakes: 1 });
    expect(JSON.parse(r.svc.store.move(t.id, 1)!.report_json!)).toMatchObject({ source: "file", failing_step: "z" });
  });
});

describe("grouping selector", () => {
  it("groups live moves by precedence and lists closed ones as an audit, newest first", async () => {
    const r = rig();
    const a = await withMove(r, prMove("https://github.com/o/r/pull/1"));
    const b = await withMove(r, prMove("https://github.com/o/r/pull/2"));
    const c = await withMove(r, prMove("https://github.com/o/r/pull/3"));
    const d = await withMove(r, scriptMove());
    const e = await withMove(r, prMove("https://github.com/o/r/pull/5"));
    const f = await withMove(r, prMove("https://github.com/o/r/pull/6"));
    r.svc.claimMove(b.id, 1);
    r.svc.skipMove(c.id, 1);
    r.svc.store.setMoveReport(d.id, 1, "succeeded", { outcome: "succeeded" });
    r.db.prepare("UPDATE moves SET hidden_at = ?, hidden_by = 'mk' WHERE task_id = ?").run(r.env.now(), e.id);
    r.svc.closeMoveOn(f.id, 1, "github", "merged");
    r.advance(1000);
    r.svc.closeMoveOn(a.id, 1, "github", "merged");
    const g = groupMoves(r.svc.store.moves());
    expect(g.yourMove.map((m) => m.task_id)).toEqual([]);
    expect(g.reported.map((m) => m.task_id).sort()).toEqual([b.id, d.id].sort());
    expect(g.later.map((m) => m.task_id)).toEqual([c.id]);
    expect(g.hidden.map((m) => m.task_id)).toEqual([e.id]);
    expect(g.audit.map((x) => [x.task_id, x.closed_by])).toEqual([[a.id, "github"], [f.id, "github"]]);
  });

  it("is pure: it does not modify its input", async () => {
    const r = rig();
    await withMove(r, prMove());
    const rows = r.svc.store.moves();
    const snap = JSON.stringify(rows);
    groupMoves(rows);
    expect(JSON.stringify(rows)).toBe(snap);
    expect(groupMoves([]).yourMove).toEqual([]);
  });
});

describe("card close", () => {
  const closeAndDrain = async (r: Rig, id: string) => {
    r.svc.closeMoveOn(id, 1, "github", "merged");
    return r.writer.drain();
  };

  it("closes the tasks card when Home filed it (mk-move label)", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    const l = r.fake.addLabel(r.tp.id, HOME_MOVE_LABEL);
    t.labelIds = [...t.labelIds, l.id];
    await closeAndDrain(r, t.id);
    expect((r.fake.tasks.find((x) => x.id === t.id) as { status?: string }).status).toBe("done");
    expect(r.db.prepare("SELECT state FROM move_closes WHERE task_id = ?").get(t.id)).toEqual({ state: "done" });
    await r.writer.drain(); // replay: no second close
    expect(r.fake.calls.filter((c) => c.method === "updateTask" && (c.input as { status?: string }).status === "done")).toHaveLength(1);
  });

  it("leaves a card Home did not file alone", async () => {
    const r = rig();
    const t = await withMove(r, prMove());
    await closeAndDrain(r, t.id);
    expect((r.fake.tasks.find((x) => x.id === t.id) as { status?: string }).status).not.toBe("done");
    expect(r.db.prepare("SELECT state FROM move_closes WHERE task_id = ?").get(t.id)).toEqual({ state: "skipped" });
  });

  it("does not close the card while another move on it is live", async () => {
    const r = rig();
    const t = await withMove(r, prMove(), { key: "k-two" });
    const l = r.fake.addLabel(r.tp.id, HOME_MOVE_LABEL);
    t.labelIds = [...t.labelIds, l.id];
    r.edit(t, { description: r.desc({ key: "k-two", question: "Again?" }) + fence(prMove("https://github.com/o/r/pull/20")) });
    await r.poll(); // gen 1 superseded, gen 2 open
    await r.writer.drain();
    expect((r.fake.tasks.find((x) => x.id === t.id) as { status?: string }).status).not.toBe("done");
  });
});

describe("rpc contract and CLI", () => {
  const parse = (k: keyof typeof homeMethods, v: unknown) => (homeMethods[k].input as z.ZodTypeAny).safeParse(v).success;

  it("validates claimMove and skipMove input strictly", () => {
    expect(parse("claimMove", { task_id: "T1", generation: 1 })).toBe(true);
    expect(parse("claimMove", { task_id: "T1", generation: 1, note: "did it" })).toBe(true);
    for (const bad of [{}, { task_id: "", generation: 1 }, { task_id: "T1", generation: 0 }, { task_id: "T1", generation: 1.5 }, { task_id: "T1", generation: "1" }, { task_id: "T1", generation: 1, note: "x".repeat(501) }, { task_id: "T1", generation: 1, by: "vizier" }, { task_id: "x".repeat(65), generation: 1 }]) expect(parse("claimMove", bad)).toBe(false);
    expect(parse("skipMove", { task_id: "T1", generation: 2 })).toBe(true);
    expect(parse("skipMove", { task_id: "T1" })).toBe(false);
    expect(parse("skipMove", { task_id: "T1", generation: 1, extra: 1 })).toBe(false);
    expect(parse("moves", null)).toBe(true);
    expect(parse("moves", {})).toBe(false);
  });

  it("bb home moves --json exports every move with its groups", async () => {
    const r = rig();
    const a = await withMove(r, prMove());
    const b = await withMove(r, prMove("https://github.com/o/r/pull/9"));
    r.svc.claimMove(b.id, 1);
    r.svc.closeMoveOn(a.id, 1, "github", "merged");
    const cli = homeCli({ svc: r.svc, asks: {} as never, catchup: {} as never, rule: (() => { throw new Error("no"); }) as never, isVizier: () => false });
    const res = await cli.run(["moves", "--json"], {});
    expect(res.exitCode).toBe(0);
    const out = JSON.parse(res.stdout!);
    expect(out.moves.map((m: { task_id: string; state: string }) => [m.task_id, m.state]).sort()).toEqual([[a.id, "closed"], [b.id, "claimed"]].sort());
    expect(out.groups.audit).toHaveLength(1);
    expect(out.groups.reported.map((m: { task_id: string }) => m.task_id)).toEqual([b.id]);
  });
});

