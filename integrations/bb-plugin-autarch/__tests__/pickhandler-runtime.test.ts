// mk-cqnab.1 PR3 stage 1: the read-only runtime evidence path. Real temp SQLite through the card rig,
// Tasks comments through the real TasksClient over the fake tasks plugin. Nothing here captures or sends.
import { afterEach, describe, expect, it, vi } from "vitest";
import { collectPickEvidence, evaluatePickRuntime, pickActionOps, type PickEvidenceSources } from "../pickhandler.js";
import { TasksClient } from "../tasks.js";
import type { TargetEvidence } from "../picktarget.js";
import { cleanupEnvs, rig } from "./card-rig.js";

afterEach(cleanupEnvs);
afterEach(() => vi.useRealTimers());

const FILED = "1".repeat(40);
const evidence = (over: Partial<TargetEvidence> = {}) => async (id: string): Promise<TargetEvidence> =>
  ({ schema: 1, thread_id: id, title: "Work", project_id: "bbp", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z", ...over });
const declaration = (subject: unknown = { kind: "none" }) => ({ schema: 1, target_thread: "thr_target", reversible: true, subject, ruling: { bead: null, under: [] } });
const askOf = (subject?: unknown) => ({ recommendation: "a", options: [
  { id: "a", label: "Blue", kind: "instruction", instruction: "Use blue.", reversible: true, on_pick: declaration(subject) },
  { id: "b", label: "Wait", kind: "ruling-only" },
] });

type Rig = ReturnType<typeof rig>;
async function participating(opts: { subject?: unknown; target?: Partial<TargetEvidence> } = {}) {
  let target = evidence(opts.target);
  const r = rig({ svc: { readTargetEvidence: (id: string) => target(id) } });
  r.enableDelegation();
  const t = r.card({ ask: askOf(opts.subject), blocks: "thread:thr_target" });
  await r.poll();
  const g1 = r.gens(t.id)[0];
  expect(g1, JSON.stringify(r.cardRow(t.id))).toBeDefined();
  return { r, t, g1, setTarget: (over: Partial<TargetEvidence>) => { target = evidence(over); } };
}
function sources(r: Rig, over: Partial<PickEvidenceSources> = {}): PickEvidenceSources {
  const tasks = new TasksClient(r.fake, { now: () => r.env.clock.t });
  return {
    store: r.svc.store, listComments: id => tasks.listComments(id), readTargetEvidence: evidence(), readHead: async () => ({ state: "OPEN", headRefOid: FILED }),
    imageCount: () => 0, now: r.env.now, ...over,
  };
}
const mkComment = (r: Rig, taskId: string, createdAt: string, body = "Only after the review") => r.fake.addComment(taskId, { kind: "user", threadId: null, body, createdAt });
const counts = (r: Rig) => r.db.prepare(`SELECT (SELECT COUNT(*) FROM obligations) AS o, (SELECT COUNT(*) FROM events) AS e, (SELECT COUNT(*) FROM pick_actions) AS a,
  (SELECT COUNT(*) FROM pick_note_fences) AS f, (SELECT COUNT(*) FROM card_writes) AS w, (SELECT COUNT(*) FROM picks) AS p, (SELECT group_concat(mode || ':' || epoch) FROM pick_handler_control) AS c`).get();

describe("runtime original evidence deadline", () => {
  it("expired budget issues no external read", async () => {
    const { r, g1 } = await participating({ subject: { kind: "github-pr", repo: "example/repo", number: 7, head: FILED } });
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    vi.useFakeTimers();
    vi.setSystemTime(r.env.clock.t);
    const listComments = vi.fn(async () => []);
    const readTargetEvidence = vi.fn(evidence());
    const readHead = vi.fn(async () => ({ state: "OPEN", headRefOid: FILED }));
    const res = await evaluatePickRuntime(sources(r, { listComments, readTargetEvidence, readHead, now: () => new Date().toISOString() }), g1.id, { deadlineAt: new Date(Date.now() - 1).toISOString() });
    expect(listComments).not.toHaveBeenCalled();
    expect(readTargetEvidence).not.toHaveBeenCalled();
    expect(readHead).not.toHaveBeenCalled();
    expect(res!.evaluation.outcome).toBe("fallback");
    expect(vi.getTimerCount()).toBe(0);
  });
  it("hung comments stop at the original pick deadline and ignore late completion", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    r.advance(119_000);
    vi.useFakeTimers();
    vi.setSystemTime(r.env.clock.t);
    let finish = (_value: unknown) => {};
    const comments = new Promise<unknown>(resolve => { finish = resolve; });
    const readTargetEvidence = vi.fn(evidence());
    const resolved = vi.fn();
    const before = counts(r);
    const pending = evaluatePickRuntime(sources(r, { listComments: () => comments, readTargetEvidence, now: () => new Date().toISOString() }), g1.id).then(resolved);
    await vi.advanceTimersByTimeAsync(999);
    expect(resolved).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(resolved).toHaveBeenCalledTimes(1);
    await pending;
    expect(resolved.mock.calls[0][0]).toMatchObject({ input: { comments: { result: "unknown" } }, evaluation: { outcome: "fallback" } });
    expect(readTargetEvidence).not.toHaveBeenCalled();
    finish([]);
    await vi.advanceTimersByTimeAsync(0);
    expect(resolved).toHaveBeenCalledTimes(1);
    expect(resolved.mock.calls[0][0].evaluation.outcome).toBe("fallback");
    expect(counts(r)).toEqual(before);
    expect(vi.getTimerCount()).toBe(0);
  });
  it("hung target uses only remaining budget after comments and ignores late completion", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    vi.useFakeTimers();
    vi.setSystemTime(r.env.clock.t);
    const deadlineAt = new Date(Date.now() + 1000).toISOString();
    const listComments = () => new Promise<unknown>(resolve => setTimeout(() => resolve([]), 600));
    let finish = (_value: TargetEvidence) => {};
    const target = new Promise<TargetEvidence>(resolve => { finish = resolve; });
    const readTargetEvidence = vi.fn(() => target);
    const resolved = vi.fn();
    const before = counts(r);
    const pending = evaluatePickRuntime(sources(r, { listComments, readTargetEvidence, now: () => new Date().toISOString() }), g1.id, { deadlineAt }).then(resolved);
    await vi.advanceTimersByTimeAsync(600);
    expect(readTargetEvidence).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(399);
    expect(resolved).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(resolved).toHaveBeenCalledTimes(1);
    await pending;
    expect(resolved.mock.calls[0][0]).toMatchObject({ input: { target: { result: "unknown" } }, evaluation: { outcome: "fallback" } });
    finish(await evidence()("thr_target"));
    await vi.advanceTimersByTimeAsync(0);
    expect(resolved).toHaveBeenCalledTimes(1);
    expect(resolved.mock.calls[0][0].evaluation.outcome).toBe("fallback");
    expect(counts(r)).toEqual(before);
    expect(vi.getTimerCount()).toBe(0);
  });
  it("slow comments record completion time and successful reads clear timers", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    vi.useFakeTimers();
    vi.setSystemTime(r.env.clock.t);
    const completedAt = new Date(Date.now() + 600).toISOString();
    const pending = evaluatePickRuntime(sources(r, { listComments: () => new Promise(resolve => setTimeout(() => resolve([]), 600)), now: () => new Date().toISOString() }), g1.id);
    await vi.advanceTimersByTimeAsync(600);
    const res = await pending;
    expect(res!.input.comments.observed_at).toBe(completedAt);
    expect(res!.evaluation.outcome).toBe("automatic");
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("runtime collection reads persisted state and Tasks comments", () => {
  it("a clean recommended none pick proposes automatic with not-applicable head and no head read", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const readHead = vi.fn(async () => ({ state: "OPEN", headRefOid: FILED }));
    const res = await evaluatePickRuntime(sources(r, { readHead }), g1.id);
    expect(res!.evaluation.checks).toEqual({
      recommendation: { result: "pass", reasons: ["recommended"] },
      "no-extra-words": { result: "pass", reasons: ["no-extra-words"] },
      reversible: { result: "pass", reasons: ["declared-reversible"] },
      "subject-head": { result: "not-applicable", reasons: ["subject-none"] },
    });
    expect(res!.evaluation).toMatchObject({ outcome: "automatic", exclusions: [], cell: "instruction:none", activation: { mode: "off", permitted: false, reason: "handler-off" } });
    expect(readHead).not.toHaveBeenCalled();
  });
  it("reads the PR head through the injected bounded reader and the persisted deadline basis", async () => {
    const { r, g1 } = await participating({ subject: { kind: "github-pr", repo: "example/repo", number: 7, head: FILED } });
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const readHead = vi.fn(async () => ({ state: "OPEN", headRefOid: FILED }));
    const res = await evaluatePickRuntime(sources(r, { readHead }), g1.id);
    expect(res!.evaluation.checks["subject-head"]).toEqual({ result: "pass", reasons: ["head-unchanged"] });
    expect(readHead).toHaveBeenCalledWith("example/repo", 7, 20_000);
    r.advance(121_000);
    const late = await evaluatePickRuntime(sources(r, { readHead }), g1.id);
    expect(late!.evaluation.checks["subject-head"]).toEqual({ result: "unknown", reasons: ["evaluation-timeout"] });
  });
  it("collects from the stored generation and pick, not caller-supplied authority", async () => {
    const { r, g1 } = await participating();
    expect(await r.validatedMkPick(g1.id, "p-words", "a", "Only if green")).toMatchObject({ ok: true });
    const input = await collectPickEvidence(sources(r), g1.id);
    expect(input).toMatchObject({
      decision: { id: g1.id, source: "card", task_id: g1.task_id, generation: 1, revision: g1.revision, state: "open", request_registered: true, binding: "confirmed", asker: "thr_a" },
      pick: { pick_id: "p-words", option_id: "a", by: "mk", reason: "Only if green", image_count: 0 },
    });
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.checks["no-extra-words"]).toEqual({ result: "fail", reasons: ["ordinary-reason"] });
  });
  it("is read-only: no rows, events, comments or control changes", async () => {
    const { r, t, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const before = counts(r);
    const comments = r.fake.comments.filter(c => c.taskId === t.id).length;
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.outcome).toBe("automatic");
    expect(await collectPickEvidence(sources(r), g1.id)).not.toBeNull();
    expect(counts(r)).toEqual(before);
    expect(r.fake.comments.filter(c => c.taskId === t.id)).toHaveLength(comments);
  });
  it("returns null for an unknown decision or one without a pick", async () => {
    const { r, g1 } = await participating();
    expect(await collectPickEvidence(sources(r), "card-missing-g1")).toBeNull();
    expect(await collectPickEvidence(sources(r), g1.id)).toBeNull();
  });
  it("Home's own Ruled comment after the pick still passes check 2", async () => {
    const { r, t, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    await r.writer.drain();
    expect(r.fake.comments.some(c => c.taskId === t.id && c.body.startsWith("Ruled: Blue"))).toBe(true);
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.checks["no-extra-words"].result).toBe("pass");
  });
  it("a handler status comment with the expected complete marker still passes", async () => {
    const { r, t, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    mkComment(r, t.id, "2026-12-01T00:00:00.000Z", `Pick handler: pending\n\nhome-pick-status: ${pickActionOps(g1.id, `p-${g1.id}`).status}`);
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.checks["no-extra-words"].result).toBe("pass");
  });
  it("a vizier/agent comment is not an owner condition; a null-thread user comment is", async () => {
    const { r, t, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    r.fake.addComment(t.id, { kind: "agent", threadId: "thr_viz", body: "noted", createdAt: "2026-12-01T00:00:00.000Z" });
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.checks["no-extra-words"].result).toBe("pass");
    mkComment(r, t.id, "2026-12-01T00:00:01.000Z");
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.checks["no-extra-words"]).toEqual({ result: "fail", reasons: ["owner-comment"] });
  });
  it("a Home note comment counts as owner words even with a write marker", async () => {
    const { r, t, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    mkComment(r, t.id, "2026-12-01T00:00:00.000Z", `home-write: cw-${g1.id}-comment\n\nhome-note: note_1`);
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.checks["no-extra-words"]).toEqual({ result: "fail", reasons: ["owner-comment"] });
  });
  it("requires no pagination or edit timestamps and discloses the limitations", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const e = (await evaluatePickRuntime(sources(r), g1.id))!.evaluation;
    expect(e.limitations).toEqual(["comment-edits-before-cutoff-not-detectable", "external-comment-read-race"]);
  });
});

describe("mk-7okdy runtime owner-comment cutoff", () => {
  it("mk-7okdy runtime N1 first generation has no cutoff: every mk comment counts, including pre-filing", async () => {
    const { r, t, g1 } = await participating();
    mkComment(r, t.id, "2020-01-01T00:00:00.000Z");
    expect(Date.parse("2020-01-01T00:00:00.000Z")).toBeLessThan(Date.parse(g1.filed_at));
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const input = await collectPickEvidence(sources(r), g1.id);
    expect(input!.comments).toMatchObject({ result: "fail", reason: "owner-comment", cutoff: null });
  });
  it("mk-7okdy runtime N1 later generation cutoff is the previous generation's Home first observation", async () => {
    const { r, t, g1 } = await participating();
    // Tasks timestamps are deliberately misleading: neither creation nor edit time may be the cutoff.
    Object.assign(t, { createdAt: "2030-01-01T00:00:00.000Z" });
    r.advance(10 * 60_000);
    const between = new Date(r.env.clock.t).toISOString();
    mkComment(r, t.id, new Date(Date.parse(g1.filed_at) - 60_000).toISOString(), "before g1 was observed");
    r.advance(10 * 60_000);
    r.edit(t, { description: r.desc({ key: String(r.cardRow(t.id).request_key), question: "Which colour, really?", ask: askOf(), blocks: "thread:thr_target" }), updatedAt: "2031-01-01T00:00:00.000Z" });
    await r.poll();
    const g2 = r.gens(t.id)[1];
    expect(g2, JSON.stringify(r.cardRow(t.id))).toBeDefined();
    expect(g2.generation).toBe(2);
    expect((await r.validatedMkPick(g2.id)).ok).toBe(true);
    const clean = await collectPickEvidence(sources(r), g2.id);
    expect(clean!.comments).toMatchObject({ result: "pass", cutoff: g1.filed_at });
    expect(clean!.comments.cutoff).not.toBe(g2.filed_at);
    // A comment after g1's observation but before g2's filing counts: the cutoff is not the current generation's filing.
    mkComment(r, t.id, between, "after g1 was observed");
    expect(Date.parse(between)).toBeLessThan(Date.parse(g2.filed_at));
    const dirty = await collectPickEvidence(sources(r), g2.id);
    expect(dirty!.comments).toMatchObject({ result: "fail", reason: "owner-comment", cutoff: g1.filed_at });
  });
  it("mk-7okdy runtime earlier generations' Home write-back comments are known markers", async () => {
    const { r, t, g1 } = await participating();
    expect((await r.validatedVizierPick(g1.id)).ok).toBe(true);
    await r.writer.drain();
    const back = r.dele.bringBack(t.id, g1.id, null, {});
    expect(back.ok).toBe(true); if (!back.ok) return;
    await r.writer.drain(); // Complete the existing relabel write before the fresh Tasks entry read.
    expect((await r.validatedMkPick(back.decision_id)).ok).toBe(true);
    await r.writer.drain();
    const e = (await evaluatePickRuntime(sources(r), back.decision_id))!.evaluation;
    expect(r.fake.comments.filter(c => c.taskId === t.id && c.body.startsWith("Ruled:"))).toHaveLength(2);
    expect(e.checks["no-extra-words"].result).toBe("pass");
  });
  it("mk-7okdy runtime missing previous observation is unknown", async () => {
    const { r, t } = await participating();
    const orphan = orphanGeneration(r, t.id, "2026-09-26T14:03:00.000Z", false);
    const input = await collectPickEvidence(sources(r), orphan);
    expect(input!.comments).toMatchObject({ result: "unknown", reason: "comment-cutoff-unavailable" });
    expect((await evaluatePickRuntime(sources(r), orphan))!.evaluation.outcome).toBe("fallback");
  });
  it("mk-7okdy runtime malformed previous observation is unknown", async () => {
    const { r, t } = await participating();
    const orphan = orphanGeneration(r, t.id, "not-a-timestamp", true);
    const input = await collectPickEvidence(sources(r), orphan);
    expect(input!.comments).toMatchObject({ result: "unknown", reason: "comment-cutoff-unavailable" });
  });
  it("mk-7okdy runtime unavailable comment read is unknown", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const input = await collectPickEvidence(sources(r, { listComments: async () => { throw new Error("tasks offline"); } }), g1.id);
    expect(input!.comments).toMatchObject({ result: "unknown", reason: "comments-unavailable" });
    const malformed = await collectPickEvidence(sources(r, { listComments: async () => [{ id: "c", body: "x", createdAt: "broken", kind: "user", threadId: null }] }), g1.id);
    expect(malformed!.comments).toMatchObject({ result: "unknown", reason: "comments-unavailable" });
  });
  it.each(["pending", "failed", "recorded"])("mk-7okdy runtime persisted %s note fence fails permanently", async state => {
    const { r, t, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    r.db.prepare("INSERT INTO pick_note_fences(decision_id, task_id, generation, note_id, created_at, posting_state) VALUES (?, ?, 1, 'note_1', ?, ?)").run(g1.id, t.id, r.env.now(), state);
    expect((await collectPickEvidence(sources(r), g1.id))!.comments).toMatchObject({ result: "fail", reason: "home-note-fence" });
  });
  it("mk-7okdy runtime unreadable note fence is unknown, never absence", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const store = r.svc.store;
    const broken = { pickEvidenceSnapshot: (id: string) => ({ ...store.pickEvidenceSnapshot(id)!, note_fences: null }) };
    expect((await collectPickEvidence(sources(r, { store: broken }), g1.id))!.comments).toMatchObject({ result: "unknown", reason: "note-fence-unavailable" });
  });
});

/** A synthetic later generation whose predecessor is absent or carries a malformed observation (real SQLite rows). */
function orphanGeneration(r: Rig, taskId: string, previousAt: string, withPrevious: boolean): string {
  const g1 = r.gens(taskId)[0];
  const task = `${taskId}-orphan`;
  const row = (n: number, at?: string) => ({
    id: `card-${task}-g${n}`, request_id: `card:${task}:g${n}`, identity: g1.identity, revision: g1.revision, semantic_key: `${g1.semantic_key}-${n}`, subject: g1.subject,
    kind: "decide", project: g1.project, project_root: g1.project_root, root_dev: g1.root_dev, root_ino: g1.root_ino, asker: g1.asker, thread: g1.thread,
    body_json: g1.body_json, source: "card" as const, task_id: task, generation: n, tasks_project_id: g1.tasks_project_id, card_fp: g1.card_fp, ...(at ? { at } : {}),
  });
  if (withPrevious) expect(r.svc.store.insertDecision(row(1, previousAt)).inserted).toBe(true);
  const g2 = row(2);
  expect(r.svc.store.insertDecision(g2).inserted).toBe(true);
  expect(r.svc.store.recordPick({ decision_id: g2.id, pick_id: `p-${g2.id}`, option_id: "a", revision: g1.revision, by: "mk", surface: "home", reason: null, params_hash: null }, []).ok).toBe(true);
  return g2.id;
}

describe("runtime target and binding evidence", () => {
  it("N2: a Tasks linkedBbProjectId conflict plays no role in evidence or eligibility", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const plain = (await evaluatePickRuntime(sources(r), g1.id))!;
    expect(plain.evaluation.outcome).toBe("automatic");
    r.tp.linkedBbProjectId = "bbp_conflicting";
    const linked = (await evaluatePickRuntime(sources(r), g1.id))!;
    expect(linked).toEqual(plain);
  });
  it.each([
    ["project changed", { project_id: "bbp_other" }, "target-project-changed"],
    ["archived", { archived: true }, "target-not-live"],
  ] as const)("fresh host evidence: %s", async (_n, over, code) => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const e = (await evaluatePickRuntime(sources(r, { readTargetEvidence: evidence(over) }), g1.id))!.evaluation;
    expect(e.exclusions.map(x => x.code)).toEqual([code]);
    expect(e.outcome).toBe("fallback");
  });
  it("unavailable host evidence is excluded, not trusted", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const e = (await evaluatePickRuntime(sources(r, { readTargetEvidence: async () => { throw new Error("offline"); } }), g1.id))!.evaluation;
    expect(e.exclusions.map(x => x.code)).toEqual(["target-unavailable"]);
    const none = (await evaluatePickRuntime(sources(r, { readTargetEvidence: undefined }), g1.id))!.evaluation;
    expect(none.exclusions.map(x => x.code)).toEqual(["target-unavailable"]);
  });
  it("an unconfirmed binding is excluded", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    r.db.prepare("UPDATE project_bindings SET state = 'suggested'").run();
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.exclusions.map(x => x.code)).toEqual(["binding-unconfirmed"]);
  });
  it("a vizier ruling is excluded as not an owner pick", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedVizierPick(g1.id)).ok).toBe(true);
    const e = (await evaluatePickRuntime(sources(r), g1.id))!.evaluation;
    expect(e.exclusions.map(x => x.code)).toContain("not-owner-pick");
    expect(e.outcome).toBe("fallback");
  });
  it("a superseded generation is excluded", async () => {
    const { r, t, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const back = r.dele.bringBack(t.id, g1.id, null, {});
    expect(back.ok).toBe(true);
    expect((await evaluatePickRuntime(sources(r), g1.id))!.evaluation.exclusions.map(x => x.code)).toContain("generation-superseded");
  });
  it("persisted handler control feeds activation; shadow is observation only", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    r.db.prepare("UPDATE pick_handler_control SET mode = 'shadow', epoch = epoch + 1").run();
    const e = (await evaluatePickRuntime(sources(r), g1.id))!.evaluation;
    expect(e.activation).toEqual({ mode: "shadow", permitted: false, reason: "shadow-observation-only" });
  });
  it("the Service wiring uses the stored vizier, host target reader and Tasks client", async () => {
    const { r, g1 } = await participating();
    expect((await r.validatedMkPick(g1.id)).ok).toBe(true);
    const tasks = new TasksClient(r.fake, { now: () => r.env.clock.t });
    const res = await r.svc.pickEvaluation(g1.id, () => tasks, { readHead: async () => { throw new Error("must not read a head for none"); } });
    expect(res!.evaluation).toMatchObject({ outcome: "automatic", cell: "instruction:none" });
    expect(res!.input.comments.observed_at).toBe(r.env.now());
  });
});
