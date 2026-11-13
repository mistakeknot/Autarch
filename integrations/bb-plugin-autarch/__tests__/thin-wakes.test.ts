import Database from "better-sqlite3";
import { join } from "node:path";
import { Store } from "../store.js";
import { afterEach, beforeEach, expect, it } from "vitest";
import { decision } from "./helpers.js";
import { parseAsk, normalizedJson, identity, revision, semanticKey } from "../model.js";
import { migrate, MIGRATIONS } from "../migrations.js";
import type { Service } from "../service.js";
import { framed, WakeLoop } from "../wakes.js";
import { ask, makeEnv, OPTIONS, type Env } from "./service-helpers.js";
import { FakeSdk } from "./wakes-helpers.js";

let env: Env;
let svc: Service;
beforeEach(() => { env = makeEnv(); svc = env.open(); });
afterEach(() => env.cleanup());
const question = "q".repeat(299) + "🚀" + "tail".repeat(100);
async function file(over: Record<string, unknown> = {}, card = true) {
  const input = ask(env, { question, recommendation: "project", ...over });
  if (card) {
    const a = parseAsk(input);
    const id = "card-fixture-g1";
    svc.store.insertDecision(decision({ id, request_id: "card:fixture:g1", identity: identity(a), revision: revision(a), semantic_key: semanticKey(a), subject: a.subject!, project: "Autarch", project_root: env.roots.Autarch, thread: "thr-a", body_json: normalizedJson(a), source: "card", task_id: "task-fixture", generation: 1, tasks_project_id: "project-fixture", card_fp: "fp" }));
    svc.store.db.prepare("INSERT INTO cards(task_id, root_state) VALUES ('task-fixture', 'verified')").run();
    return id;
  }
  const r = await svc.file(input, {});
  if (!r.ok) throw new Error(r.error);
  return r.decision_id;
}
function pick(id: string, opt = "project", reason?: string) {
  const d = svc.store.decision(id)!;
  const r = svc.pick(id, opt, String(d.revision), `pick-${id}`, "mk", "home", reason);
  if (!r.ok) throw new Error(r.error);
  return svc.store.obligationsFor(id).find(o => o.kind === "wake" || o.kind === "steps-done")!;
}

it("steps-done tells the asker the count and how to fetch, without echoing steps", async () => {
  const steps = ["a".repeat(8000), "b".repeat(8000)];
  const id = await file({ kind: "steps", options: undefined, recommendation: undefined, steps }, false);
  const w = pick(id, "done");
  expect(w.payload).toBe(`mk finished your steps ${id} (autarch/catch-up: collapse order): 2 steps. Look the card up with \`bb home get --id ${id}\`.`);
  expect(w).toMatchObject({ thin: 1 });
});
it("steps-done to a mentioning thread keeps the original full steps", async () => {
  const over = { kind: "steps", options: undefined, recommendation: undefined, steps: ["first", "second"] };
  const id = await file(over, false);
  await svc.file(ask(env, { question, ...over, thread: "thr-b" }), {});
  pick(id, "done");
  expect(svc.store.obligationsFor(id).find(o => o.recipient === "thr-b")!.payload).toBe(`mk finished your steps ${id} (autarch/catch-up: collapse order).\n1. first\n2. second`);
});
it("matched pick cuts at 300 code points, keeping the boundary astral character and instruction", async () => {
  const id = await file();
  const w = pick(id);
  expect(w.payload).toBe(`Ruling on ${id} (autarch/catch-up: collapse order): mk picked "Collapse per project".\nQuestion: ${"q".repeat(299)}🚀…\nInstruction: ${OPTIONS[0]!.instruction}`);
  expect(w).toMatchObject({ thin: 1 });
});
it("exactly 300 code points needs no ellipsis", async () => {
  const id = await file({ question: "q".repeat(299) + "🚀" });
  expect(pick(id).payload).toContain(`Question: ${"q".repeat(299)}🚀\nInstruction:`);
});
it("matched pick retains mk's added free text in full", async () => {
  const words = "Keep this instruction. ".repeat(300);
  const id = await file();
  expect(pick(id, "project", words).payload).toContain(words);
});
it("own-words answers stay full", async () => {
  const words = "My answer. ".repeat(300).trim();
  const id = await file();
  const w = pick(id, "other", words);
  expect(w.payload).toContain(`Question: ${question}`);
  expect(w.payload).toContain(words);
});
it("mismatched pick keeps the full original payload", async () => {
  const id = await file();
  expect(pick(id, "day").payload).toBe(`Ruling on ${id} (autarch/catch-up: collapse order): mk picked "Collapse per day".\nQuestion: ${question}`);
});
it("non-card picks keep their full question even when recommended", async () => {
  const id = await file({}, false);
  expect(pick(id).payload).toContain(`Question: ${question}`);
});
it("reopening and retrying the same pick replays the identical saved payload and identity", async () => {
  const id = await file();
  const w = pick(id);
  expect(w).toMatchObject({ thin: 1 });
  const oldPick = svc.store.pick(id);

  svc = env.open();
  expect(pick(id).payload).toBe(w.payload);
  // Replay the pending obligation through the actual wake transport after restart.
  const sdk = new FakeSdk();
  await new WakeLoop(svc, sdk).drain();
  expect(sdk.sent[0]!.input).toBe(framed(w.op!, 1, w.payload!));
  expect(svc.store.obligation(w.id)!.payload).toBe(w.payload);
  expect(svc.store.pick(id)).toEqual(oldPick);
});
it("legacy records with no flag replay their full saved text, and an older reader accepts the new schema", async () => {
  const id = await file();
  const payload = `Question: ${question}\n1. legacy step`;
  svc.store.insertObligations(id, [{ id: "legacy-wake", kind: "wake", recipient: "thr-a", op: "legacy-op", payload }]);
  svc = env.open();
  const sdk = new FakeSdk();
  await new WakeLoop(svc, sdk).drain();
  expect(sdk.sent[0]!.input).toBe(framed("legacy-op", 1, payload));
  expect(svc.store.obligation("legacy-wake")).toMatchObject({ thin: null });
  const db = new Database(env.file);
  expect(() => migrate(db, { codeVersion: 6, migrations: MIGRATIONS.filter(m => m.version <= 6) })).not.toThrow();
  db.close();
});

it("upgrades a real v6 obligation without changing its bytes, and v6 can still read new records", async () => {
  const path = join(env.dir, "v6.db");
  const old = new Store(new Database(path), { now: env.now, migrate: { codeVersion: 6, migrations: MIGRATIONS.filter(m => m.version <= 6) } });
  const d = decision();
  old.insertDecision(d);
  const payload = `Question: ${question}\n1. complete legacy steps`;
  old.db.prepare("INSERT INTO obligations(id, decision_id, kind, recipient, payload, op, next_try_at, updated_at) VALUES (?, ?, 'wake', 'thr-a', ?, 'old-op', ?, ?)").run("v6-wake", d.id, payload, env.now(), env.now());
  old.db.close();
  const current = new Store(new Database(path), { now: env.now });
  expect(current.obligation("v6-wake")).toMatchObject({ thin: null, payload });
  const restarted = env.open({ store: current });
  const sdk = new FakeSdk();
  await new WakeLoop(restarted, sdk).drain();
  expect(sdk.sent[0]!.input).toBe(framed("old-op", 1, payload));
  current.insertObligations(d.id, [{ id: "new-thin", kind: "wake", recipient: "thr-a", payload: "thin snapshot", thin: 1 }]);
  current.db.close();
  const rolledBack = new Store(new Database(path), { now: env.now, migrate: { codeVersion: 6, migrations: MIGRATIONS.filter(m => m.version <= 6) } });
  expect(rolledBack.obligation("new-thin")!.payload).toBe("thin snapshot");
  expect(rolledBack.obligation("v6-wake")!.payload).toBe(payload);
  rolledBack.db.close();
});
