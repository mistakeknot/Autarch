// mk-cqnab.1 PR3 stage 2b: guarded relay preparation, transactional claim, ruling-file first failure, the
// original-deadline watchdog and fallback facts, through the real Service, Store and WakeLoop over SQLite with a
// fake host SDK. The injected before-send guard is an isolated test seam, not the full external check.
import { afterEach, expect, it, vi } from "vitest";
import { chmodSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { cleanupEnvs, rig } from "./card-rig.js";
import { tmpDir } from "./helpers.js";
import { TasksClient } from "../tasks.js";
import type { ServiceDeps } from "../service.js";
import { appendRuling } from "../ledger.js";
import { claimPickRelay, pickRelayPayload, type SendGuardResult } from "../pickcapture.js";
import { RECOVERY_WARNING } from "../pickfacts.js";
import { ImageStore } from "../otherimages.js";
import { commentBody, pickWrites } from "../cardwrites.js";
import { pickActionOps } from "../pickhandler.js";
import type { ObligationRow, PickActionRow } from "../store.js";
import { framed, WakeLoop } from "../wakes.js";
import { FakeSdk, netError } from "./wakes-helpers.js";

const SENTINEL = "9999-12-31T23:59:59.999Z";
const dirs: ReturnType<typeof tmpDir>[] = [];
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  cleanupEnvs();
  while (dirs.length) dirs.pop()!.cleanup();
});

const permitted = async (): Promise<SendGuardResult> => ({ result: "permitted", reason: "isolated test guard" });
type Guard = ((row: ObligationRow) => Promise<SendGuardResult>) | null;

async function live(o: { guard?: Guard; ledger?: "file" | "off"; target?: string; blocks?: string; deps?: Partial<ServiceDeps>; mode?: "live" | "shadow"; subject?: { kind: "github-pr"; repo: string; number: number; head: string } } = {}) {
  const dir = tmpDir(); dirs.push(dir);
  const ledger = join(dir.dir, "rulings.jsonl");
  const guard = o.guard === undefined ? permitted : o.guard;
  const target = o.target ?? "thr_target";
  const r = rig({ svc: {
    readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Work", project_id: "project", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }),
    ...(o.mode === "shadow" ? { pickHandlerMode: "shadow" } : { pickCaptureRuntime: { mode: () => "live", ...(guard ? { beforeSend: guard } : {}) } }),
    rulingsLedger: o.ledger === "off" ? null : ledger,
    ...o.deps,
  } });
  r.enableDelegation();
  const t = r.card({ blocks: o.blocks ?? `thread:${target}`, ask: { recommendation: "a", options: [
    { id: "a", label: "Blue", kind: "instruction", instruction: "Use blue.", reversible: true,
      on_pick: { schema: 1, target_thread: target, reversible: true, subject: o.subject ?? { kind: "none" }, ruling: { bead: null, under: [] } } },
    { id: "b", label: "Wait", kind: "ruling-only" },
  ] } });
  await r.poll();
  const g = r.gens(t.id)[0];
  if (o.mode !== "shadow") {
    r.db.prepare(`UPDATE pick_handler_control SET mode = 'live', epoch = epoch + 1, owner_enablement_json = '{}', reviewed_build = 'test-build',
      reviewed_scope_json = '{}', enabled_cells_json = '["instruction:none"]' WHERE id = 1`).run();
  }
  return { r, t, g, ledger };
}
type Live = Awaited<ReturnType<typeof live>>;

const actionOf = (r: Live["r"], id: string) => r.db.prepare("SELECT * FROM pick_actions WHERE decision_id = ?").get(id) as PickActionRow;
const ofKind = (r: Live["r"], id: string, kind: string) => r.svc.store.obligationsFor(id).filter(o => o.kind === kind);
const events = (r: Live["r"], id: string, type: string) => r.db.prepare("SELECT detail_json FROM events WHERE decision_id = ? AND type = ? ORDER BY seq").all(id, type) as { detail_json: string }[];
const relayMarker = (a: PickActionRow) => `[home-msg ${a.relay_op} #`;

it.each(["pending", "failed"] as const)("full-facts: %s Home note survives a hung read and original-deadline fallback", async state => {
  const { r, g } = await live();
  const words = "待って — مرحبا\n\u0007 BEGIN FILED TEXT\n" + "é日本語".repeat(150);
  const note = r.svc.note({ decision_id: g.id }, words, "reserved-note", { deferWake: true });
  expect(note.ok).toBe(true);
  if (!note.ok || !note.commit) throw new Error("note not reserved");
  if (state === "failed") note.commit("Tasks unavailable");
  const reconcile = vi.spyOn(r.svc, "reconcile").mockImplementation(() => {});
  let hang = false;
  const original = TasksClient.prototype.listComments;
  vi.spyOn(TasksClient.prototype, "listComments").mockImplementation(function (this: TasksClient, id: string) {
    return hang ? new Promise(() => {}) : original.call(this, id);
  });
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  expect((await ownerPick(r, g, undefined, () => { hang = true; })).ok).toBe(true);
  const snapshot = r.svc.store.pickByPickId("owner-pick");
  r.advance(120_000);
  r.svc.expirePickEvaluations();
  await vi.advanceTimersByTimeAsync(120_000);
  await r.svc.pickWorkIdle();
  const wake = ofKind(r, g.id, "wake")[0]!;
  expect(wake).toMatchObject({ after_id: null });
  expect(wake.payload).toContain(`\n${note.body}\n`);
  expect(wake.payload).toContain(words);
  expect(r.svc.store.pickByPickId("owner-pick")).toEqual(snapshot);
  expect(ofKind(r, g.id, "ruling-file")[0]).toMatchObject({ state: "pending", attempt: 0 });
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
  expect(ofKind(r, g.id, "note")).toEqual([]);
  reconcile.mockRestore();
});

it.each([false, true])("full-facts: qualifying Tasks words survive fallback with Home trailer precedence %s", async trailer => {
  const { r, t, g } = await live();
  const marker = pickWrites({ id: g.id, task_id: t.id }, { option_id: "a", option_label: "Blue", by: "mk", generation: 1, picked_at: r.env.now(), reason: null }).find(w => w.kind === "comment")!.id;
  const words = "  日本語\n\u0007 مرحبا\n" + "exact words ".repeat(1000) + (trailer ? `\nhome-write: ${marker}\n\nhome-note: external-note` : "  ");
  const ruledBody = commentBody(marker, { option_label: "Blue", by: "mk", generation: 1 });
  const statusBody = `Pick handler: pending\n\nhome-pick-status: ${pickActionOps(g.id, "owner-pick").status}`;
  const addComments = () => {
    r.fake.addComment(t.id, { kind: "user", threadId: null, body: words, createdAt: "2020-01-01T00:00:00Z" });
  for (const [kind, threadId, body] of [["agent", null, "agent excluded"], ["system", null, "system excluded"], ["user", "thr_a", "owner thread excluded"], ["user", "thr_viz", "vizier excluded"], ["user", null, ruledBody], ["user", null, statusBody]] as const) {
    r.fake.addComment(t.id, { kind, threadId, body, createdAt: r.env.now() });
  }
  };
  expect((await ownerPick(r, g, undefined, addComments)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const wake = ofKind(r, g.id, "wake")[0]!;
  expect(actionOf(r, g.id).disposition).toBe("fallback");
  const saved = r.db.prepare("SELECT payload FROM card_writes WHERE id = ?").get(marker) as { payload: string };
  expect(commentBody(marker, JSON.parse(saved.payload))).toBe(ruledBody);
  expect(wake.payload).toContain(`\n${words}\n`);
  for (const excluded of ["agent excluded", "system excluded", "owner thread excluded", "vizier excluded", ruledBody, statusBody]) expect(wake.payload).not.toContain(excluded);
  expect(ruledBody).not.toBe("");
  expect(r.fake.comments.some(c => c.body === ruledBody)).toBe(true);
  expect(r.fake.comments.some(c => c.body === statusBody)).toBe(true);
  expect(wake.after_id).toBeNull();
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
});

it.each(["changed", "unavailable"] as const)("full-facts: PR head is %s in the fallback", async state => {
  const filed = "a".repeat(40), current = "b".repeat(40);
  const { r, g } = await live({ subject: { kind: "github-pr", repo: "example/repo", number: 7, head: filed } });
  const evaluate = r.svc.pickEvaluation.bind(r.svc);
  vi.spyOn(r.svc, "pickEvaluation").mockImplementation((id, tasks, opts) => evaluate(id, tasks, { ...opts,
    readHead: async () => { if (state === "unavailable") throw new Error("head unavailable"); return { state: "OPEN", headRefOid: current }; },
  }));
  expect((await ownerPick(r, g)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const payload = ofKind(r, g.id, "wake")[0]!.payload!;
  expect(payload).toContain(`filed head ${filed}; current head ${state === "changed" ? current : "unknown"}; state ${state === "changed" ? "OPEN" : "unknown"}`);
  expect(payload).toContain(state === "changed" ? "head-changed" : "head-unavailable");
});

it("full-facts: later generation uses previous Home observation cutoff and keeps the positive control", async () => {
  const { r, t, g } = await live();
  r.advance(60_000);
  r.edit(t, { description: t.description.replace("Collapse per project or per day?", "Which colour now?") });
  await r.poll();
  const next = r.gens(t.id)[1];
  expect(next.generation).toBe(2);
  const words = "between observations 日本語\n\u0007  ";
  expect((await ownerPick(r, next, undefined, () => {
    r.fake.addComment(t.id, { kind: "user", threadId: null, body: "cutoff excluded", createdAt: g.filed_at });
    r.fake.addComment(t.id, { kind: "user", threadId: null, body: words, createdAt: new Date(Date.parse(g.filed_at) + 1).toISOString() });
  })).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const payload = ofKind(r, next.id, "wake")[0]!.payload!;
  expect(payload).toContain(`\n${words}\n`);
  expect(payload).not.toContain("cutoff excluded");
});

it.each(["deadline", "ruling-failure"] as const)("full-facts: known comments and head survive a hung target at %s", async trigger => {
  let hang = false;
  let entered!: () => void;
  const started = new Promise<void>(resolve => { entered = resolve; });
  let finish!: (v: Awaited<ReturnType<NonNullable<ServiceDeps["readTargetEvidence"]>>>) => void;
  const filed = "a".repeat(40), current = "b".repeat(40);
  const target = (id: string) => ({ schema: 1 as const, thread_id: id, title: "Work", project_id: "project", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" });
  const { r, t, g } = await live({ subject: { kind: "github-pr", repo: "example/repo", number: 7, head: filed }, deps: {
    readTargetEvidence: async id => {
      if (!hang) return target(id);
      entered();
      return new Promise(resolve => { finish = resolve; });
    },
  } });
  vi.spyOn(r.svc, "reconcile").mockImplementation(() => {});
  const evaluate = r.svc.pickEvaluation.bind(r.svc);
  vi.spyOn(r.svc, "pickEvaluation").mockImplementation((id, tasks, opts) => evaluate(id, tasks, { ...opts, readHead: async () => ({ state: "OPEN", headRefOid: current }) }));
  const words = "known before hang 日本語\n\u0007  ";
  expect((await ownerPick(r, g, undefined, () => {
    hang = true;
    r.fake.addComment(t.id, { kind: "user", threadId: null, body: words });
  })).ok).toBe(true);
  await started;
  const note = r.svc.note({ decision_id: g.id }, "accepted while waiting", "during-wait", { deferWake: true });
  expect(note.ok).toBe(true);
  if (trigger === "deadline") r.advance(120_000);
  else r.db.prepare("UPDATE obligations SET attempt = 1, last_error = 'write failed' WHERE decision_id = ? AND kind = 'ruling-file'").run(g.id);
  const reads = r.fake.calls.length;
  r.svc.expirePickEvaluations();
  expect(r.fake.calls).toHaveLength(reads);
  const payload = ofKind(r, g.id, "wake")[0]!.payload!;
  expect(payload).toContain(`\n${words}\n`);
  expect(payload).toContain("accepted while waiting");
  expect(payload).toContain(`filed head ${filed}; current head ${current}; state OPEN`);
  expect(ofKind(r, g.id, "wake")[0]!.after_id).toBeNull();
  const settled = actionOf(r, g.id);
  finish(target("thr_target"));
  await r.svc.pickWorkIdle();
  expect(actionOf(r, g.id)).toEqual(settled);
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
});

function ownerPick(r: Live["r"], g: { id: string; revision: string }, reason?: string, before?: () => void) {
  return r.svc.withValidatedPick(g.id, guard => {
    before?.();
    return r.svc.pick(g.id, "a", g.revision, "owner-pick", "mk", "home", reason, undefined, undefined, undefined, guard);
  }, () => new TasksClient(r.fake));
}

/** A relay row from the normal flow: written ruling file, matching ledger, post-commit preparation. */
async function enqueued(o: Parameters<typeof live>[0] = {}) {
  const l = await live(o);
  expect((await ownerPick(l.r, l.g)).ok).toBe(true);
  await l.r.svc.pickWorkIdle();
  const relay = ofKind(l.r, l.g.id, "pick-relay");
  expect(relay, JSON.stringify(events(l.r, l.g.id, "pick-relay-preparation"))).toHaveLength(1);
  return { ...l, relay: relay[0]!, action: actionOf(l.r, l.g.id) };
}

/** Crash after the ledger append, before the ruling-file acknowledgement: relay enqueued while the file is pending. */
async function enqueuedPending() {
  const l = await live();
  const reconcile = vi.spyOn(l.r.svc, "reconcile").mockImplementation(() => {});
  expect((await ownerPick(l.r, l.g)).ok).toBe(true);
  await l.r.svc.pickWorkIdle();
  appendRuling(l.ledger, l.r.svc.rulingLedgerPick(l.g.id)!);
  reconcile.mockRestore();
  expect(await l.r.svc.prepareRelay(actionOf(l.r, l.g.id))).toMatchObject({ result: "enqueued" });
  return { ...l, relay: ofKind(l.r, l.g.id, "pick-relay")[0]!, action: actionOf(l.r, l.g.id) };
}

it("matching ledger plus a pending never-failed ruling file enqueues one guarded relay the real WakeLoop sends to the stored target", async () => {
  const { r, g, ledger } = await live();
  const reconcile = vi.spyOn(r.svc, "reconcile").mockImplementation(() => {});
  expect((await ownerPick(r, g)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  // No ledger row yet: no relay, and the action stays owned until evidence or the deadline settles it.
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
  expect(actionOf(r, g.id).disposition).toBe("pending");
  // Crash after the ledger append, before the ruling-file acknowledgement: the file is pending and never failed.
  appendRuling(ledger, r.svc.rulingLedgerPick(g.id)!);
  reconcile.mockRestore();
  expect(ofKind(r, g.id, "ruling-file")[0]).toMatchObject({ state: "pending", attempt: 0 });
  expect(await r.svc.prepareRelay(actionOf(r, g.id))).toMatchObject({ result: "enqueued" });
  const relay = ofKind(r, g.id, "pick-relay")[0]!;
  expect(relay).toMatchObject({ recipient: "thr_target", payload: pickRelayPayload(r.svc.store, "owner-pick"), next_try_at: SENTINEL, after_id: null, op: actionOf(r, g.id).relay_op });
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_target"]);
  expect(sdk.sent[0]!.input).toBe(framed(relay.op!, 1, relay.payload!));
  expect(r.svc.store.obligation(relay.id)).toMatchObject({ state: "done", next_try_at: SENTINEL });
  expect(r.svc.store.attempts(relay.id)).toMatchObject([{ state: "delivered", evidence: "m1" }]);
  expect(actionOf(r, g.id).disposition).toBe("automatic");
  // The ruling file was never gated on: it is still pending and its ordinary retry still owns it.
  expect(ofKind(r, g.id, "ruling-file")[0]).toMatchObject({ state: "pending", attempt: 0 });
});

it("a written never-failed ruling file proceeds through post-commit preparation and Service.reconcile stays the only ledger writer", async () => {
  const { r, g, relay, ledger } = await enqueued();
  expect(ofKind(r, g.id, "ruling-file")[0]).toMatchObject({ state: "done", attempt: 0 });
  expect(ofKind(r, g.id, "wake")).toEqual([]);
  const { readFileSync } = await import("node:fs");
  expect(readFileSync(ledger, "utf8").trim().split("\n")).toHaveLength(1);
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.map(s => [s.threadId, s.input])).toEqual([["thr_target", framed(relay.op!, 1, relay.payload!)]]);
});

it.each([
  ["absent", null],
  ["unknown", async (): Promise<SendGuardResult> => ({ result: "unknown", reason: "before-send evidence unavailable" })],
  ["refused", async (): Promise<SendGuardResult> => ({ result: "refused", reason: "target changed" })],
] as const)("an %s before-send guard keeps a due relay unsent and unclaimed", async (_name, guard) => {
  const { r, relay } = await enqueued({ guard });
  expect(r.svc.store.dueMessages().map(o => o.id)).toContain(relay.id);
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent).toEqual([]);
  expect(r.svc.store.obligation(relay.id)).toMatchObject({ state: "pending", attempt: 0, next_try_at: SENTINEL });
  expect(r.svc.store.attempts(relay.id)).toEqual([]);
});

it.each(["disabled", "conflicting"] as const)("a %s ledger falls back before any relay, with a prompt asker fact sheet", async state => {
  const l = await live({ ledger: state === "disabled" ? "off" : "file" });
  if (state === "conflicting") writeFileSync(l.ledger, `${JSON.stringify({ id: "home-owner-pick", ts: "2020-01-01T00:00:00Z" })}\n`);
  expect((await ownerPick(l.r, l.g)).ok).toBe(true);
  await l.r.svc.pickWorkIdle();
  const a = actionOf(l.r, l.g.id);
  expect(a.disposition).toBe("fallback");
  expect(ofKind(l.r, l.g.id, "pick-relay")).toEqual([]);
  const wake = ofKind(l.r, l.g.id, "wake");
  expect(wake).toHaveLength(1);
  expect(wake[0]).toMatchObject({ recipient: "thr_a", after_id: null, op: a.fallback_op });
  expect(wake[0]!.payload).toContain(`Ledger: ${state}`);
  expect(wake[0]!.payload).toContain("No relay was sent");
  const sdk = new FakeSdk();
  await new WakeLoop(l.r.svc, sdk).drain();
  expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_a"]);
});

it("an unreadable selected ledger row never relays and the original deadline falls back", async () => {
  const l = await live();
  writeFileSync(l.ledger, '{"id":"home-owner-pick", broken\n');
  expect((await ownerPick(l.r, l.g)).ok).toBe(true);
  await l.r.svc.pickWorkIdle();
  expect(actionOf(l.r, l.g.id).disposition).toBe("pending");
  expect(ofKind(l.r, l.g.id, "pick-relay")).toEqual([]);
  l.r.advance(120_000);
  l.r.svc.expirePickEvaluations();
  expect(actionOf(l.r, l.g.id).disposition).toBe("fallback");
  const wake = ofKind(l.r, l.g.id, "wake")[0]!;
  expect(wake.payload).toContain("Ledger: unknown");
  expect(wake.payload).toContain("(expired)");
  expect(JSON.parse(events(l.r, l.g.id, "pick-fallback")[0]!.detail_json)).toMatchObject({ reason: "evaluation-timeout", delivery: "none-confirmed" });
});

it("first ruling-file failure falls back at once while the ordinary retry stays pending; later success never revives", async () => {
  const { r, g } = await live();
  chmodSync(r.env.roots.Autarch!, 0o555);
  expect((await ownerPick(r, g)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const ruling = ofKind(r, g.id, "ruling-file")[0]!;
  expect(ruling).toMatchObject({ state: "pending", attempt: 1 });
  expect(ruling.last_error).toBeTruthy();
  expect(actionOf(r, g.id).disposition).toBe("fallback");
  expect(events(r, g.id, "pick-ruling-file-failure")).toHaveLength(1);
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
  const wake = ofKind(r, g.id, "wake")[0]!;
  expect(wake).toMatchObject({ recipient: "thr_a", after_id: null });
  expect(wake.payload).toContain("Ruling file: first failure at");
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_a"]);
  chmodSync(r.env.roots.Autarch!, 0o755);
  r.advance(31_000);
  r.svc.reconcileAll();
  expect(ofKind(r, g.id, "ruling-file")[0]).toMatchObject({ state: "done", attempt: 1 });
  expect(await r.svc.prepareRelay(actionOf(r, g.id))).toMatchObject({ result: "not-applied" });
  expect(actionOf(r, g.id).disposition).toBe("fallback");
  expect(events(r, g.id, "pick-ruling-file-failure")).toHaveLength(1);
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
});

it("a ruling-file failure after enqueue and before claim voids the unattempted relay", async () => {
  const { r, g, relay } = await enqueuedPending();
  chmodSync(r.env.roots.Autarch!, 0o555);
  r.svc.reconcile(g.id);
  expect(actionOf(r, g.id).disposition).toBe("fallback");
  expect(r.svc.store.obligation(relay.id)!.voided_at).not.toBeNull();
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_a"]);
  expect(sdk.sent[0]!.input).toContain("No relay was sent");
  expect(r.svc.store.attempts(relay.id)).toEqual([]);
});

it("a ruling-file failure after possible acceptance preserves evidence, stops attempts and warns truthfully", async () => {
  const { r, g, relay, action } = await enqueuedPending();
  const sdk = new FakeSdk();
  sdk.script = [{ kind: "throw", err: netError() }];
  await new WakeLoop(r.svc, sdk).drain();
  expect(r.svc.store.attempts(relay.id)).toMatchObject([{ state: "uncertain" }]);
  chmodSync(r.env.roots.Autarch!, 0o555);
  r.svc.reconcile(g.id);
  expect(actionOf(r, g.id).disposition).toBe("fallback");
  expect(r.svc.store.obligation(relay.id)!.voided_at).toBeNull();
  const wake = ofKind(r, g.id, "wake")[0]!;
  expect(wake.payload).toContain(RECOVERY_WARNING);
  expect(wake.payload).not.toContain("No relay was sent");
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.filter(s => s.input.startsWith(relayMarker(action)))).toHaveLength(1);
  expect(r.svc.store.attempts(relay.id)).toMatchObject([{ state: "uncertain" }]);
});

it("the running Service watchdog expires at the original deadline within a second, independent of ruling backoff, and stops cleanly", async () => {
  const { r, g } = await live();
  const reconcile = vi.spyOn(r.svc, "reconcile").mockImplementation(() => {});
  expect((await ownerPick(r, g)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  reconcile.mockRestore();
  const ruling = ofKind(r, g.id, "ruling-file")[0]!;
  r.db.prepare("UPDATE obligations SET next_try_at = ? WHERE id = ?").run("2099-01-01T00:00:00.000Z", ruling.id);
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
  const base = vi.getTimerCount();
  r.svc.start();
  vi.advanceTimersByTime(1_000);
  expect(actionOf(r, g.id).disposition).toBe("pending");
  r.advance(120_000);
  vi.advanceTimersByTime(1_000);
  expect(actionOf(r, g.id).disposition).toBe("fallback");
  expect(ofKind(r, g.id, "wake")).toHaveLength(1);
  expect(r.svc.store.obligation(ruling.id)).toMatchObject({ state: "pending", attempt: 0, next_try_at: "2099-01-01T00:00:00.000Z" });
  r.svc.stop();
  expect(vi.getTimerCount()).toBe(base);
});

it("a WakeLoop pass expires an overdue action before any protected claim", async () => {
  const { r, g, relay, action } = await enqueued();
  r.advance(120_001);
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_a"]);
  expect(sdk.sent.some(s => s.input.startsWith(relayMarker(action)))).toBe(false);
  expect(r.svc.store.obligation(relay.id)!.voided_at).not.toBeNull();
  expect(r.svc.store.attempts(relay.id)).toEqual([]);
  expect(JSON.parse(events(r, g.id, "pick-fallback")[0]!.detail_json)).toMatchObject({ reason: "evaluation-timeout", delivery: "none-confirmed" });
});

it.each([[60_000, "enqueued"], [120_000, "fallback"]] as const)("capture resumption at service start %ims after capture uses the original deadline", async (elapsed, outcome) => {
  const { r, g, ledger } = await live();
  const reconcile = vi.spyOn(r.svc, "reconcile").mockImplementation(() => {});
  expect((await ownerPick(r, g)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  appendRuling(ledger, r.svc.rulingLedgerPick(g.id)!);
  reconcile.mockRestore();
  expect(actionOf(r, g.id).disposition).toBe("pending");
  r.advance(elapsed);
  r.svc.start();
  try {
    await r.svc.pickWorkIdle();
    expect(ofKind(r, g.id, "pick-relay")).toHaveLength(outcome === "enqueued" ? 1 : 0);
    expect(actionOf(r, g.id).disposition).toBe(outcome === "enqueued" ? "automatic" : "fallback");
  } finally {
    r.svc.stop();
  }
});

it("resuming a nonexpired live capture twice prepares one relay and one send under the unchanged original deadline", async () => {
  const { r, g, ledger } = await live();
  const reconcile = vi.spyOn(r.svc, "reconcile").mockImplementation(() => {});
  expect((await ownerPick(r, g)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  appendRuling(ledger, r.svc.rulingLedgerPick(g.id)!);
  reconcile.mockRestore();
  const deadline = actionOf(r, g.id).evaluation_deadline_at;
  r.advance(60_000);
  r.svc.resumePickActions();
  r.svc.resumePickActions();
  await r.svc.pickWorkIdle();
  r.svc.resumePickActions();
  await r.svc.pickWorkIdle();
  const relay = ofKind(r, g.id, "pick-relay");
  expect(relay).toHaveLength(1);
  expect(actionOf(r, g.id)).toMatchObject({ disposition: "automatic", evaluation_deadline_at: deadline });
  expect(events(r, g.id, "pick-relay-preparation").map(e => JSON.parse(e.detail_json).result)).toEqual(["no-relay", "enqueued"]);
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  r.svc.resumePickActions();
  await r.svc.pickWorkIdle();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.map(s => s.threadId)).toEqual(["thr_target", "thr_a"]);
  expect(sdk.sent[1].input).toContain("already relayed; do not act");
  expect(ofKind(r, g.id, "pick-receipt")).toHaveLength(1);
  expect(r.svc.store.attempts(relay[0]!.id)).toHaveLength(1);
});

it("capture resumption never prepares a shadow capture", async () => {
  const { r, g } = await live({ mode: "shadow" });
  expect((await ownerPick(r, g)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const before = r.svc.store.obligationsFor(g.id);
  r.svc.resumePickActions();
  await r.svc.pickWorkIdle();
  expect(r.svc.store.obligationsFor(g.id)).toEqual(before);
  expect(actionOf(r, g.id).disposition).toBe("pending");
  expect(events(r, g.id, "pick-relay-preparation")).toEqual([]);
});

it("restart keeps the persisted deadline and settles the invalidated capture epoch at startup before claims", async () => {
  const { r, g, relay, action } = await enqueued();
  r.advance(60_000);
  r.restart();
  expect(actionOf(r, g.id).evaluation_deadline_at).toBe(action.evaluation_deadline_at);
  r.svc.start();
  try {
    expect(actionOf(r, g.id).disposition).toBe("fallback");
    expect(r.svc.store.obligation(relay.id)!.voided_at).not.toBeNull();
    const sdk = new FakeSdk();
    await new WakeLoop(r.svc, sdk).drain();
    expect(sdk.sent.some(s => s.input.startsWith(relayMarker(action)))).toBe(false);
    expect(JSON.parse(events(r, g.id, "pick-fallback")[0]!.detail_json)).toMatchObject({ reason: "mode-epoch-changed" });
  } finally {
    r.svc.stop();
  }
});

it("a hung evidence read ends at the original capture deadline and its late completion cannot enqueue", async () => {
  const { r, g } = await live();
  let hang = false;
  const original = TasksClient.prototype.listComments;
  let finish!: (v: Awaited<ReturnType<typeof original>>) => void;
  vi.spyOn(TasksClient.prototype, "listComments").mockImplementation(function (this: TasksClient, id: string) {
    if (!hang) return original.call(this, id);
    return new Promise<Awaited<ReturnType<typeof original>>>(resolve => { finish = resolve; });
  });
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  expect((await ownerPick(r, g, undefined, () => { hang = true; })).ok).toBe(true);
  r.advance(119_999);
  await vi.advanceTimersByTimeAsync(119_999);
  expect(actionOf(r, g.id).disposition).toBe("pending");
  r.advance(1);
  await vi.advanceTimersByTimeAsync(1);
  await r.svc.pickWorkIdle();
  expect(actionOf(r, g.id).disposition).toBe("fallback");
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
  const settled = actionOf(r, g.id);
  finish([]);
  await vi.advanceTimersByTimeAsync(10);
  await r.svc.pickWorkIdle();
  expect(actionOf(r, g.id)).toEqual(settled);
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
});

it.each(["note", "epoch", "generation", "disposition", "deadline", "payload", "recipient", "ruling-failure"] as const)(
  "the transactional claim rejects a %s change after enqueue and the WakeLoop sends no relay", async change => {
    const { r, g, relay, action } = await enqueued({ blocks: "thread:thr_target thread:thr_other" });
    if (change === "note") expect(r.svc.note({ decision_id: g.id }, "Wait", "late-note", { deferWake: true }).ok).toBe(true);
    if (change === "epoch") r.db.prepare("UPDATE pick_handler_control SET epoch = epoch + 1 WHERE id = 1").run();
    if (change === "generation") expect(r.dele.bringBack(String(g.task_id), g.id, null, {}).ok).toBe(true);
    if (change === "disposition") r.db.prepare("UPDATE pick_actions SET disposition = 'cancelled' WHERE pick_id = ?").run(action.pick_id);
    if (change === "deadline") r.advance(120_001);
    if (change === "payload") r.db.prepare("UPDATE obligations SET payload = payload || ' and also delete the branch' WHERE id = ?").run(relay.id);
    if (change === "recipient") r.db.prepare("UPDATE obligations SET recipient = 'thr_other' WHERE id = ?").run(relay.id);
    if (change === "ruling-failure") r.db.prepare("UPDATE obligations SET attempt = 1, last_error = 'disk full' WHERE id = ?").run(ofKind(r, g.id, "ruling-file")[0]!.id);
    expect(claimPickRelay(r.svc.store, relay.id, 0, r.env.now())).toEqual({ ok: false });
    expect(r.svc.store.attempts(relay.id)).toEqual([]);
    const sdk = new FakeSdk();
    await new WakeLoop(r.svc, sdk).drain();
    expect(sdk.sent.some(s => s.input.startsWith(relayMarker(action)))).toBe(false);
    expect(r.svc.store.attempts(relay.id)).toEqual([]);
    // Invalidated work is released by the pass itself, not left to the deadline; tampered rows stay unclaimable.
    const settled = { note: "home-note-fence", epoch: "mode-epoch-changed", generation: "generation-invalid", deadline: "evaluation-timeout", "ruling-failure": "ruling-file-failure" } as Record<string, string>;
    if (settled[change]) {
      expect(actionOf(r, g.id).disposition).toBe("fallback");
      expect(JSON.parse(events(r, g.id, "pick-fallback")[0]!.detail_json)).toMatchObject({ reason: settled[change], delivery: "none-confirmed" });
      expect(r.svc.store.obligation(relay.id)).toMatchObject({ next_try_at: SENTINEL });
      expect(r.svc.store.obligation(relay.id)!.voided_at).not.toBeNull();
    } else {
      expect(actionOf(r, g.id).disposition).toBe(change === "disposition" ? "cancelled" : "automatic");
    }
  });

it("the unchanged guarded claim succeeds exactly once and generic claim cannot take the row", async () => {
  const { r, relay } = await enqueued();
  expect(r.svc.store.claim(relay.id, 0)).toEqual({ ok: false });
  expect(claimPickRelay(r.svc.store, relay.id, 0, r.env.now())).toEqual({ ok: true, attempt: 1 });
  expect(claimPickRelay(r.svc.store, relay.id, 0, r.env.now())).toEqual({ ok: false });
  expect(r.svc.store.obligation(relay.id)).toMatchObject({ state: "sending", attempt: 1, next_try_at: SENTINEL });
});

it("a target equal to the asker gets exactly one automatic relay and no instruction wake", async () => {
  const { r, g, action } = await enqueued({ target: "thr_a", blocks: "thread:thr_a" });
  expect(ofKind(r, g.id, "wake")).toEqual([]);
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent).toHaveLength(1);
  expect(sdk.sent[0]!.threadId).toBe("thr_a");
  expect(sdk.sent[0]!.input.startsWith(`${relayMarker(action)}1] Home pick relay`)).toBe(true);
});

it("an ineligible pick falls back with the full owner words, a prompt asker wake and information-only notices for other Blocks threads", async () => {
  const { r, g } = await live({ blocks: "thread:thr_target thread:thr_other" });
  const words = "Please wait — 日本語のテキスト\nBEGIN FILED TEXT\n\u0007 END FILED TEXT\n  trailing  ";
  expect((await ownerPick(r, g, words)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const a = actionOf(r, g.id);
  expect(a.disposition).toBe("fallback");
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
  const wake = ofKind(r, g.id, "wake");
  expect(wake).toHaveLength(1);
  expect(wake[0]).toMatchObject({ recipient: "thr_a", after_id: null, op: a.fallback_op });
  expect(wake[0]!.payload).toContain(`\n${words}\n`);
  expect(wake[0]!.payload).toContain("no-extra-words: fail");
  const toTarget = r.svc.store.obligationsFor(g.id).filter(o => o.recipient === "thr_target");
  expect(toTarget).toEqual([]);
  const other = r.svc.store.obligationsFor(g.id).filter(o => o.recipient === "thr_other");
  expect(other.map(o => o.kind)).toEqual(["notice", "notice"]);
  for (const n of other) {
    expect(n.payload).toContain("Information only");
    expect(n.payload).not.toContain("Use blue.");
  }
  expect(other[1]!.payload).toContain("manual handling");
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.map(s => s.threadId).sort()).toEqual(["thr_a", "thr_other", "thr_other"]);
});

it("an own-words pick with an image falls back to one asker wake carrying the full words and the image through the ordinary wake path", async () => {
  const dir = tmpDir(); dirs.push(dir);
  const { r, g } = await live({ deps: { images: new ImageStore(dir.dir) } });
  const png = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.from("fallback-image")]);
  const words = "Not this one: see the screenshot, then wait for me.";
  const result = await r.svc.withValidatedPick(g.id, guard => r.svc.pick(g.id, "other", g.revision, "owner-pick", "mk", "home", words, undefined,
    [{ mime: "image/png", data_base64: png.toString("base64") }], undefined, guard), () => new TasksClient(r.fake));
  expect(result.ok).toBe(true);
  await r.svc.pickWorkIdle();
  const a = actionOf(r, g.id);
  expect(a.disposition).toBe("fallback");
  expect(ofKind(r, g.id, "pick-relay")).toEqual([]);
  const sdk = new FakeSdk();
  await new WakeLoop(r.svc, sdk).drain();
  // The virtual option declares no target, so every Blocks thread is ordinary information released at fallback.
  expect(sdk.sent.map(s => s.threadId).sort()).toEqual(["thr_a", "thr_target"]);
  const wake = sdk.sent.find(s => s.threadId === "thr_a")!;
  expect(wake.input.startsWith(`[home-msg ${a.fallback_op} #1]`)).toBe(true);
  expect(wake.input).toContain(`\n${words}\n`);
  expect(wake.input).toContain("1 image(s) attached through the ordinary wake image path");
  expect(wake.images).toHaveLength(1);
  const info = sdk.sent.find(s => s.threadId === "thr_target")!;
  expect(info.input).toContain("Information only; do not relay or act");
  expect(info.input).not.toContain(words);
  expect(info.images).toBeUndefined();
});

it("fallback image routing uses the stored action's ordinary wake op, never notice ops, and keeps missing images visible", async () => {
  const dir = tmpDir(); dirs.push(dir);
  const images = new ImageStore(dir.dir);
  const { r, g } = await live({ deps: { images } });
  const png = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.from("route-image")]);
  const result = await r.svc.withValidatedPick(g.id, guard => r.svc.pick(g.id, "other", g.revision, "owner-pick", "mk", "home", "See the image.", undefined,
    [{ mime: "image/png", data_base64: png.toString("base64") }], undefined, guard), () => new TasksClient(r.fake));
  expect(result.ok).toBe(true);
  await r.svc.pickWorkIdle();
  const a = actionOf(r, g.id);
  const ordinary = `wake:${a.decision_id}:${a.pick_id}`;
  const accepted = images.paths(ordinary);
  expect(accepted).toMatchObject({ state: "ok" });
  expect(r.svc.imagesFor(a.fallback_op!)).toEqual(accepted);
  expect(r.svc.imagesFor(ordinary)).toEqual(accepted);
  expect(r.svc.imagesFor(`${a.fallback_op}:notice:thr_target`)).toEqual({ state: "none" });
  const { rmSync } = await import("node:fs");
  for (const p of (accepted as { paths: string[] }).paths) rmSync(p);
  expect(r.svc.imagesFor(a.fallback_op!)).toEqual(images.paths(ordinary));
  expect(r.svc.imagesFor(a.fallback_op!).state).toBe("missing");
});

it("a send without a durable host message id stays uncertain, never confirmed, and the deadline then warns without replay", async () => {
  const { r, g, relay, action } = await enqueued();
  const sdk = new FakeSdk();
  sdk.send = async a => { sdk.sent.push(a); return { delivery: "sent" }; };
  await new WakeLoop(r.svc, sdk).drain();
  expect(r.svc.store.attempts(relay.id)).toMatchObject([{ state: "uncertain" }]);
  expect(r.svc.store.obligation(relay.id)!.state).toBe("uncertain");
  r.advance(120_001);
  r.svc.expirePickEvaluations();
  expect(actionOf(r, g.id).disposition).toBe("fallback");
  expect(JSON.parse(events(r, g.id, "pick-fallback")[0]!.detail_json)).toMatchObject({ reason: "evaluation-timeout", delivery: "uncertain" });
  expect(ofKind(r, g.id, "wake")[0]!.payload).toContain(RECOVERY_WARNING);
  await new WakeLoop(r.svc, sdk).drain();
  expect(sdk.sent.filter(s => s.input.startsWith(relayMarker(action)))).toHaveLength(1);
});

it("a legacy wake sent without a host message id keeps its existing delivered outcome", async () => {
  const { r, g } = await live({ mode: "shadow" });
  expect((await ownerPick(r, g)).ok).toBe(true);
  const sdk = new FakeSdk();
  sdk.send = async a => { sdk.sent.push(a); return { delivery: "sent" }; };
  await new WakeLoop(r.svc, sdk).drain();
  const wake = ofKind(r, g.id, "wake")[0]!;
  expect(r.svc.store.obligation(wake.id)!.state).toBe("done");
  expect(r.svc.store.attempts(wake.id)).toMatchObject([{ state: "delivered", evidence: "sent" }]);
});

it("shadow capture is never expired into fallback work or extra messages", async () => {
  const { r, g } = await live({ mode: "shadow" });
  expect((await ownerPick(r, g)).ok).toBe(true);
  await r.svc.pickWorkIdle();
  const before = r.svc.store.obligationsFor(g.id);
  r.advance(120_001);
  expect(r.svc.expirePickEvaluations()).toBe(0);
  expect(r.svc.store.obligationsFor(g.id)).toEqual(before);
  expect(actionOf(r, g.id).disposition).toBe("pending");
  expect(events(r, g.id, "pick-fallback")).toEqual([]);
});
