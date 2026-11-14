import Database from "better-sqlite3";
import { afterEach, expect, it, vi } from "vitest";
import { Store, createStoreHandle } from "../store.js";
import { Service } from "../service.js";
import { capturePick } from "../pickcapture.js";
import { decision, pickOf } from "./helpers.js";
import { parseStoredCardAsk } from "../model.js";
import { homeCli } from "../cli.js";
import { Asks } from "../asks.js";
import { Catchup } from "../catchup.js";
import { Delegation } from "../delegation.js";
import { homeMethods } from "../contract.js";
import { wireHome, rpcContract } from "../server.js";
import { ImageStore } from "../otherimages.js";
import { FakeSdk } from "./wakes-helpers.js";
import { FakeTasks } from "./tasks-fake.js";
import { TasksClient } from "../tasks.js";
import { CardWriter, pickWrites } from "../cardwrites.js";
import { evaluatePick } from "../pickhandler.js";

const databases: Database.Database[] = [];
afterEach(() => { vi.restoreAllMocks(); for (const db of databases.splice(0)) db.close(); });
function fixture(participating = true, capture = true, optionId = "a", mode: "shadow" | "live" = "shadow") {
  const db = new Database(":memory:"); databases.push(db);
  const now = () => "2026-01-01T00:00:00.000Z";
  const store = new Store(db, { now });
  const svc = new Service({ store, now, projects: async () => [], env: {} });
  db.prepare("INSERT INTO cards(task_id, title, state) VALUES ('task', 'Original card', 'open')").run();
  const on_pick = { schema: 1, target_thread: "thr_target", reversible: true, subject: { kind: "none" }, ruling: { bead: null, under: [] } };
  store.insertDecision(decision({ id: "original", source: "card", task_id: "task", generation: 1, card_fp: "frozen-fingerprint", thread: "asker",
    body_json: JSON.stringify({ v: 1, kind: "decide", project: "autarch", project_root: "/workspace/project", asker: "thread", thread: "asker", question: "Frozen question?", subject: "Choice", recommendation: "a", options: [
      { id: "a", label: "Blue", kind: "instruction", instruction: "Use blue.", ...(participating ? { on_pick } : {}) },
      { id: "b", label: "Wait", kind: "ruling-only" },
    ] }) }));
  store.recordEvent("on-pick-targets", "original", { asker: { project_id: "project" }, targets: { a: { schema: 1, thread_id: "thr_target", title: "Frozen target", project_id: "project", project_name: "Project", archived: false, deleted: false, observed_at: now() } } });
  store.recordPick({ ...pickOf("original", { pick_id: "original-pick", option_id: optionId, reason: "日本語\u0007".repeat(2000) }),
    ...(mode === "live" ? { capture: () => { capturePick(store, "original", "live", now()); } } : {}) }, mode === "live" ? [] : [
    { id: "ordinary", kind: "wake", recipient: "asker", op: "wake:original:original-pick", payload: "ordinary continuation" },
  ]);
  parseStoredCardAsk(JSON.parse(String(store.decision("original")!.body_json)));
  if (capture && participating && mode === "shadow") capturePick(store, "original", "shadow", now());
  return { db, svc, store };
}

it("reads original pick and exact decision generation equally without writes and preserves all words", () => {
  const { db, svc } = fixture();
  const before = db.prepare("SELECT total_changes() AS n").get();
  const a = svc.readPickAction({ pick_id: "original-pick" });
  expect(a, JSON.stringify(a)).toMatchObject({ ok: true, kind: "action", facts: { card_title: null, question: "Frozen question?", target: { title: "Frozen target" } }, observation_only: true });
  expect(a).toEqual(svc.readPickAction({ decision_id: "original", generation: 1 }));
  if (a.kind !== "action") throw new Error("missing action");
  expect(a.facts.owner_words[0]?.text).toBe("日本語\u0007".repeat(2000));
  expect(a.text).toContain("日本語\u0007".repeat(2000));
  expect(a.text).toContain("Observation only");
  expect(a.text).toContain("manual delivery is not observed");
  expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
});

it("requires positive immutable evidence for no-action; missing generation is unavailable", () => {
  const { svc } = fixture(false, false);
  expect(svc.readPickAction({ decision_id: "original", generation: 1 })).toMatchObject({ kind: "no-action", ordinary_continuation: true });
  expect(svc.readPickAction({ decision_id: "original", generation: 2 })).toMatchObject({ kind: "unavailable", ordinary_continuation: false });
});

it("missing participating capture is unavailable but ordinary wake proves manual continuation", () => {
  const { svc } = fixture(true, false);
  expect(svc.readPickAction({ decision_id: "original", generation: 1 })).toMatchObject({ kind: "unavailable", ordinary_continuation: true });
});

it.each(["pending", "queued", "uncertain", "delivered"] as const)("malformed immutable ask cannot release %s live routing to a legacy ordinary wake", state => {
  const { svc, db, store } = fixture(true, true, "a", "live");
  db.prepare("INSERT INTO obligations(id,decision_id,kind,op,recipient,next_try_at,updated_at) VALUES ('ordinary','original','wake','wake:original:original-pick','asker','2026-01-01','2026-01-01')").run();
  if (state !== "pending") {
    db.prepare(`INSERT INTO obligations(id,decision_id,kind,op,recipient,state,attempt,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id)
      SELECT 'relay',decision_id,'pick-relay',relay_op,target_thread,?,1,'9999-12-31T23:59:59.999Z',captured_at,0,0,1 FROM pick_actions`).run(state === "delivered" ? "done" : state);
    db.prepare("INSERT INTO attempts(obligation_id,n,state,handle,evidence,at) VALUES ('relay',1,?,'queue-handle',?,'2026-01-01')").run(state, state === "delivered" ? "durable-host-id" : null);
  }
  // Corrupt only the read projection: immutable storage and its triggers stay intact.
  const stored = store.decision("original")!;
  vi.spyOn(store, "decision").mockReturnValue({ ...stored, body_json: "{" });
  const reconcile = vi.spyOn(svc, "reconcile");
  const send = vi.spyOn(FakeSdk.prototype, "send");
  const before = db.prepare("SELECT total_changes() AS n").get();
  const view = svc.readPickAction({ pick_id: "original-pick" });
  expect(view).toEqual(svc.readPickAction({ decision_id: "original", generation: 1 }));
  expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
  expect(reconcile).not.toHaveBeenCalled();
  expect(send).not.toHaveBeenCalled();
  expect(view).toMatchObject({ kind: "unavailable", ordinary_continuation: false });
});

it.each([
  ["pick-relay", "valid"], ["pick-receipt", "valid"],
  ["pick-relay", "malformed"], ["pick-receipt", "malformed"],
  ["pick-relay", "throws"], ["pick-receipt", "throws"],
] as const)("orphan %s contradicts no-action with %s ask and immutable ordinary routing", (kind, read) => {
  const { svc, db, store } = fixture(false, false);
  // Expanded-schema evidence can outlive/miss its action; retain all protection policies.
  db.prepare(`INSERT INTO obligations(id,decision_id,kind,op,recipient,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id)
    VALUES ('protected','original',?,?,'thr_target','9999-12-31T23:59:59.999Z','2026-01-01',0,0,1)`).run(kind, `${kind}:original:original-pick`);
  if (read !== "valid") {
    const projection = { ...store.decision("original")!, body_json: "{" };
    if (read === "throws") Object.defineProperty(projection, "body_json", { get() { throw new Error("stored body projection unavailable"); } });
    vi.spyOn(store, "decision").mockReturnValue(projection);
  }
  const reconcile = vi.spyOn(svc, "reconcile");
  const send = vi.spyOn(FakeSdk.prototype, "send");
  const before = db.prepare("SELECT total_changes() AS n").get();
  const view = svc.readPickAction({ pick_id: "original-pick" });
  expect(view).toEqual(svc.readPickAction({ decision_id: "original", generation: 1 }));
  expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
  expect(reconcile).not.toHaveBeenCalled();
  expect(send).not.toHaveBeenCalled();
  expect(view).toMatchObject({ kind: "unavailable", ordinary_continuation: false });
});

it.each([false, true])("unavailable immutable ask preserves unconflicted ordinary routing with shadow capture %s", capture => {
  const { svc, db, store } = fixture(true, capture);
  const stored = store.decision("original")!;
  vi.spyOn(store, "decision").mockReturnValue({ ...stored, body_json: "{" });
  const before = db.prepare("SELECT total_changes() AS n").get();
  const view = svc.readPickAction({ pick_id: "original-pick" });
  expect(view).toMatchObject({ kind: "unavailable", ordinary_continuation: true });
  expect(view).toEqual(svc.readPickAction({ decision_id: "original", generation: 1 }));
  expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
});

it("actual CLI serializes exact queries, exposes text/full JSON and rejects ambiguous or invalid args", async () => {
  const { svc, db } = fixture();
  const cli = homeCli({ svc, asks: new Asks(svc), catchup: new Catchup(svc, new Delegation(svc)), isVizier: () => false,
    rule: () => ({ ok: false, status: 400, error: "unused" }) });
  const before = db.prepare("SELECT total_changes() AS n").get();
  const result = await cli.run(["pick-action", "--pick", "original-pick", "--json"], {});
  expect(result.exitCode).toBe(0);
  expect(JSON.parse(result.stdout!)).toEqual(svc.readPickAction({ pick_id: "original-pick" }));
  expect(await cli.run(["pick-action", "--decision", "original", "--generation", "1", "--json"], {})).toEqual(result);
  for (const args of [[], ["--pick", "original-pick", "--generation", "1"], ["--pick", "original-pick", "--decision", "original"],
    ["--decision", "original"], ["--generation", "1"], ...["0", "-1", "1.5", "NaN", "1e0", "9007199254740992"].map(n => ["--decision", "original", "--generation", n])]) {
    expect((await cli.run(["pick-action", ...args], {})).exitCode, JSON.stringify(args)).toBe(2);
  }
  expect((await cli.run(["pick-action", "--pick", "missing", "--json"], {})).exitCode).toBe(1);
  expect((await cli.run(["pick-action", "--pick", "original-pick"], {})).stdout).toContain("日本語\u0007".repeat(2000));
  expect(JSON.parse((await cli.run(["capabilities", "--json"], {})).stdout!)).toMatchObject({ pick_action: [1], live_delivery: false });
  expect((await cli.run(["--help"], {})).stdout).toContain("bb home pick-action");
  expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
});

it("RPC contract validates exact generation queries before handler access", () => {
  expect(homeMethods.pickAction.input.safeParse({ decision_id: "original", generation: 1 }).success).toBe(true);
  for (const query of [{}, { pick_id: "p", generation: 1 }, { decision_id: "d" }, { decision_id: "d", generation: 0 }, { pick_id: "p", decision_id: "d", generation: 1 }]) {
    expect(homeMethods.pickAction.input.safeParse(query).success).toBe(false);
  }
});

it("current card title and closure cannot replace frozen target presentation or remove readback", () => {
  const { svc, db } = fixture();
  db.prepare("UPDATE cards SET title = 'Changed current title', state = 'ruled' WHERE task_id = 'task'").run();
  const before = db.prepare("SELECT total_changes() AS n").get();
  expect(svc.readPickAction({ decision_id: "original", generation: 1 })).toMatchObject({ kind: "action", facts: { card_title: null, target: { title: "Frozen target" } } });
  expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
});

it.each(["queued", "uncertain", "sending", "delivered", "not-delivered"] as const)("derives %s from attempts, without sending or writing status", state => {
  const { svc, db } = fixture();
  db.prepare(`INSERT INTO obligations(id, decision_id, kind, op, recipient, state, attempt, next_try_at, updated_at, allow_reroute, allow_batch, require_message_id)
    SELECT 'relay', decision_id, 'pick-relay', relay_op, target_thread, ?, 1, '9999-12-31T23:59:59.999Z', captured_at, 0, 0, 1 FROM pick_actions`).run(state === "delivered" ? "done" : state === "not-delivered" ? "pending" : state);
  db.prepare("INSERT INTO attempts(obligation_id,n,state,handle,evidence,at) VALUES ('relay',1,?,'queue-handle',?, '2026-01-01T00:00:01Z')").run(state, state === "delivered" ? "host-message" : null);
  const before = db.prepare("SELECT total_changes() AS n").get();
  const view = svc.readPickAction({ pick_id: "original-pick" });
  expect(view).toMatchObject({ kind: "action", facts: { delivery: { state: state === "delivered" ? "confirmed" : state === "sending" ? "pending" : state === "not-delivered" ? "none-confirmed" : state } } });
  expect(view.ordinary_continuation).toBe(state === "not-delivered");
  expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
  expect(db.prepare("SELECT COUNT(*) AS n FROM card_writes").get()).toEqual({ n: 0 });
});

it.each([null, "sent", "queue-handle"])("done with non-durable message evidence %s remains uncertain", evidence => {
  const { svc, db } = fixture();
  db.prepare(`INSERT INTO obligations(id, decision_id, kind, op, recipient, state, attempt, next_try_at, updated_at, allow_reroute, allow_batch, require_message_id)
    SELECT 'relay', decision_id, 'pick-relay', relay_op, target_thread, 'done', 1, '9999-12-31T23:59:59.999Z', captured_at, 0, 0, 1 FROM pick_actions`).run();
  db.prepare("INSERT INTO attempts(obligation_id,n,state,handle,evidence,at) VALUES ('relay',1,'delivered','queue-handle',?, '2026-01-01T00:00:01Z')").run(evidence);
  expect(svc.readPickAction({ pick_id: "original-pick" })).toMatchObject({ handler_status: "delivery uncertain", facts: { delivery: { state: "uncertain", host_message_id: null } } });
});

it("notes decode exact framing, retain malformed source, and failed history survives later ruling success", () => {
  const { svc, db, store } = fixture();
  const text = "  Wait\n\u0007日本語  ";
  store.recordEvent("home-note-reserved", "original", { note_id: "n", body: `mk's note (Home, advisory; plain text):\n${text}\n\nhome-note: n` });
  store.recordEvent("home-note-reserved", "original", { note_id: "bad", body: "unrecognized framing\nFULL WORDS" });
  db.prepare("INSERT INTO obligations(id,decision_id,kind,op,state,attempt,next_try_at,updated_at) VALUES ('ruling','original','ruling-file','ruling-file:original','done',1,'2026-01-01','2026-01-01')").run();
  store.recordEvent("pick-ruling-file-failure", "original", { pick_id: "original-pick" });
  const result = svc.readPickAction({ pick_id: "original-pick" });
  expect(result).toMatchObject({ kind: "action", ruling_first_failure: { observed: true }, facts: { ruling_file: { state: "written" } }, notes: [{ text, framing: "decoded" }, { text: "unrecognized framing\nFULL WORDS", framing: "unknown" }] });
});

it("malformed evaluation and SQL failure are explicit unavailable, never empty action/no-action", () => {
  const { svc, db } = fixture();
  db.prepare("UPDATE pick_actions SET evaluation_json = '{}' WHERE pick_id = 'original-pick'").run();
  expect(svc.readPickAction({ decision_id: "original", generation: 1 })).toMatchObject({ kind: "unavailable", ordinary_continuation: true });
  db.close(); databases.pop();
  expect(svc.readPickAction({ pick_id: "original-pick" })).toMatchObject({ kind: "unavailable", ordinary_continuation: false });
});

it("actual registered server RPC and CLI read the Store without writes, sends, cancellation or reconciliation", async () => {
  const { db } = fixture();
  // Isolate startup's independent recovery/sweep lifecycle; the actual read handlers are unmocked.
  vi.spyOn(Service.prototype, "start").mockImplementation(() => {});
  vi.spyOn(ImageStore.prototype, "sweep").mockReturnValue(0);
  vi.spyOn(ImageStore.prototype, "expectedCount").mockReturnValue(0);
  const reconcile = vi.spyOn(Service.prototype, "reconcile");
  const sdk = new FakeSdk();
  const disposers: (() => void)[] = [];
  let cli: ReturnType<typeof homeCli> | undefined;
  const publish = vi.fn();
  const bb = { log: { info() {}, warn() {}, error() {} }, onDispose: (fn: () => void) => disposers.push(fn),
    background: { service() {} }, events: { on() {} }, agents: { configure() {} },
    cli: { register: (c: ReturnType<typeof homeCli>) => { cli = c; } }, realtime: { publish }, sdk: { threads: { list: async () => [] } } };
  const handle = createStoreHandle(() => db);
  const home = wireHome(bb as never, handle, { serveAddr: "127.0.0.1:1", serveTokenFile: "unused", serveProjectDirs: [], autarchBin: "unused", rulingsLedger: null }, { sdk });
  try {
    const before = db.prepare("SELECT total_changes() AS n").get();
    const view = await home.handlers.pickAction({ pick_id: "original-pick" });
    expect(view).toMatchObject({ kind: "action", facts: { target: { title: "Frozen target" } }, live_delivery_available: false });
    expect(await home.handlers.pickAction({ decision_id: "original", generation: 1 })).toEqual(view);
    expect(rpcContract).toHaveProperty("pickAction");
    const result = await cli!.run(["pick-action", "--pick", "original-pick", "--json"], {});
    expect(result.exitCode).toBe(0);
    expect(JSON.parse(result.stdout!)).toEqual(view);
    expect(await home.handlers.listAsks(null)).toMatchObject({ pick_actions: [{ decision_id: "original", generation: 1, observation_only: true, status: "manual handling required" }] });
    expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
    expect(sdk.sent).toEqual([]);
    expect(reconcile).not.toHaveBeenCalled();
    expect(publish).not.toHaveBeenCalled();
  } finally { disposers.forEach(fn => fn()); handle.dispose(); }
});

it.each([["other", "own-words"], ["answers", "answers"], ["b", "ordinary-reason"]])("preserves accepted %s words exactly with no selected declaration", (option, kind) => {
  const { svc } = fixture(true, true, option);
  expect(svc.readPickAction({ pick_id: "original-pick" })).toMatchObject({ kind: "action", facts: { declaration: null, owner_words: [{ kind, text: "日本語\u0007".repeat(2000) }] } });
});

it("full four-check evaluation, exclusions, read times, declaration and digests survive readback without turning a shadow proposal into ownership", () => {
  const { svc, db, store } = fixture();
  const ask = parseStoredCardAsk(JSON.parse(String(store.decision("original")!.body_json)));
  const evaluated_at = "2026-01-01T00:00:01.000Z";
  const evaluation = evaluatePick({ policy_version: 1, evaluated_at,
    decision: { id: "original", source: "card", task_id: "task", generation: 1, revision: "rev-1", state: "open", request_registered: true, binding: "confirmed", asker: "asker", ask, exclusions: { root_run: false, move: false, artifact: false } },
    pick: { ...store.pick("original")!, image_count: 0 },
    comments: { result: "pass", reason: "no-owner-comments", observed_at: evaluated_at, cutoff: null, comments: [], limitations: [] },
    head: { result: "not-applicable", reason: "subject-none", observed_at: evaluated_at, head: null, state: null },
    target: { result: "pass", reason: "target-live-in-scope", observed_at: evaluated_at },
    control: { mode: "shadow", epoch: 0, capture_epoch: 0, enabled_cells: [], owner_enablement: false },
  });
  db.prepare("UPDATE pick_actions SET evaluation_json = ?, evaluated_at = ?").run(JSON.stringify(evaluation), evaluated_at);
  const view = svc.readPickAction({ decision_id: "original", generation: 1 });
  expect(view).toMatchObject({ kind: "action", observation_only: true, ordinary_continuation: true, facts: { evaluation, deadline: { outcome: "met" }, delivery: { state: "none-confirmed" } } });
  if (view.kind !== "action") throw new Error("missing action");
  expect(view.snapshot.relay_text).toBe("Use blue.");
  expect(view.snapshot.relay_sha256).toMatch(/^[0-9a-f]{64}$/);
  expect(view.facts.checks).toEqual(evaluation.checks);
  expect(view.image_contract.op).toBe("wake:original:original-pick");
  expect(view.events.map(e => e.type)).toContain("picked");
  expect(view.event_history_complete).toBe(true);
  expect(view.host_history_complete).toBe(false);
});

it("normal Ruled writer unlabels/completes despite a pending relay, and bring-back cannot redirect old action queries", async () => {
  const { svc, db, store } = fixture();
  const fake = new FakeTasks();
  const project = fake.addProject("Example");
  const label = fake.addLabel(project.id, "needs-mk");
  const task = fake.addTask(project.id, { id: "task", labelIds: [label.id] });
  db.prepare("UPDATE cards SET title = 'Current card', state = 'ruled' WHERE task_id = 'task'").run();
  store.insertCardWrites("original", pickWrites({ id: "original", task_id: "task" }, { option_id: "a", option_label: "Blue", by: "mk", generation: 1, picked_at: "2026-01-01", reason: null }));
  db.prepare(`INSERT INTO obligations(id, decision_id, kind, op, recipient, next_try_at, updated_at, allow_reroute, allow_batch, require_message_id)
    SELECT 'relay', decision_id, 'pick-relay', relay_op, target_thread, '9999-12-31T23:59:59.999Z', captured_at, 0, 0, 1 FROM pick_actions`).run();
  const writer = new CardWriter(db, new TasksClient(fake), () => "2026-01-01T00:00:01.000Z");
  expect(await writer.drain()).toEqual({ done: 2, failed: 0 });
  expect(task.status).toBe("done");
  expect(task.labelIds).toEqual([]);
  expect(fake.comments).toHaveLength(1);
  expect(fake.comments[0]!.body).toContain("Ruled:");
  expect(fake.comments[0]!.body).not.toContain("home-pick-status:");
  const frozen = svc.readPickAction({ pick_id: "original-pick" });
  const back = new Delegation(svc).bringBack("task", "original", null, {});
  expect(back.ok).toBe(true);
  expect(svc.readPickAction({ decision_id: "original", generation: 1 })).toEqual(frozen);
  expect(svc.readPickAction({ decision_id: "original", generation: 2 })).toMatchObject({ kind: "unavailable" });
  expect(db.prepare("SELECT kind FROM obligations WHERE kind NOT IN ('wake','pick-relay','void-notice')").all()).toEqual([]);
});

it("later-written ruling with counted failures but no failure timestamp never says not yet failed", () => {
  const { svc, db } = fixture();
  db.prepare("INSERT INTO obligations(id,decision_id,kind,op,state,attempt,next_try_at,updated_at) VALUES ('ruling','original','ruling-file','ruling-file:original','done',1,'2026-01-01','2026-01-01')").run();
  const view = svc.readPickAction({ pick_id: "original-pick" });
  expect(view.kind).toBe("action");
  if (view.kind !== "action") throw new Error("missing action");
  expect(view.text).not.toContain("not yet failed");
  expect(view.facts.ruling_file).toMatchObject({ state: "written", failed_before: true });
});

it("dormant live pending remains owned despite current off mode; expiry read cannot mutate the original deadline", () => {
  const { svc, db, store } = fixture(true, true, "a", "live");
  expect(svc.readPickAction({ pick_id: "original-pick" })).toMatchObject({ kind: "action", observation_only: false, ordinary_continuation: false,
    handler_status: "pending", current_control: { mode: "off" }, live_delivery_available: false, facts: { delivery: { state: "not-attempted" } } });
  const later = new Service({ store, now: () => "2026-01-01T00:03:00.000Z", projects: async () => [], env: {} });
  const before = db.prepare("SELECT total_changes() AS n").get();
  expect(later.readPickAction({ decision_id: "original", generation: 1 })).toMatchObject({ handler_status: "pending", ordinary_continuation: false,
    facts: { deadline: { at: "2026-01-01T00:02:00.000Z", outcome: "expired" } } });
  expect(db.prepare("SELECT total_changes() AS n").get()).toEqual(before);
});

it("a participating missing capture without immutable ordinary routing is unavailable under every current mode", () => {
  const { svc, db } = fixture(true, false);
  db.prepare("DELETE FROM obligations WHERE id = 'ordinary'").run();
  expect(svc.readPickAction({ pick_id: "original-pick" })).toMatchObject({ kind: "unavailable", ordinary_continuation: false });
  db.prepare("UPDATE pick_handler_control SET mode = 'shadow', epoch = 1").run();
  expect(svc.readPickAction({ decision_id: "original", generation: 1 })).toMatchObject({ kind: "unavailable", ordinary_continuation: false });
});

it("failed informational receipt never changes confirmed target delivery or authorizes another relay", () => {
  const { svc, db } = fixture(true, true, "a", "live");
  db.prepare(`INSERT INTO obligations(id,decision_id,kind,op,recipient,state,attempt,next_try_at,updated_at,allow_reroute,allow_batch,require_message_id)
    SELECT 'relay',decision_id,'pick-relay',relay_op,target_thread,'done',1,'9999-12-31T23:59:59.999Z',captured_at,0,0,1 FROM pick_actions`).run();
  db.prepare("INSERT INTO attempts(obligation_id,n,state,evidence,at) VALUES ('relay',1,'delivered','durable-host-id','2026-01-01T00:00:01Z')").run();
  db.prepare(`INSERT INTO obligations(id,decision_id,kind,op,recipient,state,next_try_at,updated_at,allow_batch,last_error)
    SELECT 'receipt',decision_id,'pick-receipt',receipt_op,asker,'undeliverable','9999-12-31T23:59:59.999Z',captured_at,0,'asker unavailable' FROM pick_actions`).run();
  const result = svc.readPickAction({ pick_id: "original-pick" });
  expect(result).toMatchObject({ kind: "action", handler_status: "already relayed", ordinary_continuation: false, facts: { delivery: { state: "confirmed", host_message_id: "durable-host-id" } } });
  if (result.kind !== "action") throw new Error("missing action");
  expect(result.operations[3]!.rows[0]!.row.state).toBe("undeliverable");
});

it("image readback retains the existing ordinary-wake operation and never re-encodes or sends attachments", () => {
  const { store } = fixture();
  const count = vi.spyOn(ImageStore.prototype, "expectedCount").mockReturnValue(2);
  const svc = new Service({ store, projects: async () => [], env: {}, images: new ImageStore("unused") });
  expect(svc.readPickAction({ pick_id: "original-pick" })).toMatchObject({ kind: "action", facts: { images: { count: 2, op: "wake:original:original-pick" } } });
  expect(count).toHaveBeenCalledWith("wake:original:original-pick");
});
