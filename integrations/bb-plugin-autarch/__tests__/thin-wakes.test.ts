import { afterEach, beforeEach, expect, it } from "vitest";
import { decision } from "./helpers.js";
import { parseAsk, normalizedJson, identity, revision, semanticKey } from "../model.js";
import type { Service } from "../service.js";
import { framed, WakeLoop } from "../wakes.js";
import { ask, makeEnv, OPTIONS, type Env } from "./service-helpers.js";
import { FakeSdk } from "./wakes-helpers.js";
import { homeMethods } from "../contract.js";

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
  expect(w.payload).toBe(`Ruling on ${id} (autarch/catch-up: collapse order): mk picked "Collapse per project".\nQuestion: ${"q".repeat(299)}🚀…\nLook the card up with \`bb home get --id ${id}\`.\nInstruction: ${OPTIONS[0]!.instruction}`);
});
it("exactly 300 code points needs no ellipsis", async () => {
  const id = await file({ question: "q".repeat(299) + "🚀" });
  expect(pick(id).payload).toContain(`Question: ${"q".repeat(299)}🚀\nInstruction:`);
});
it("matched pick retains mk's added free text in full", async () => {
  const words = "Keep this instruction. ".repeat(300);
  const id = await file();
  expect(pick(id, "project", words).payload).toContain(`\nmk added (unverified plain text):\n${words}`);
});
it("a cut question tells the asker how to fetch the complete card", async () => {
  const id = await file();
  expect(pick(id).payload).toContain(`Look the card up with \`bb home get --id ${id}\`.`);
});
it("one finished step uses singular wording", async () => {
  const id = await file({ kind: "steps", options: undefined, recommendation: undefined, steps: ["one"] }, false);
  const payload = pick(id, "done").payload;
  expect(payload).toContain("1 step.");
  expect(payload).not.toContain("1 steps");
});
it.each(["project", "day"])("a blank note adds nothing to a %s pick", async (option) => {
  const id = await file();
  expect(pick(id, option, "").payload).not.toContain("mk added");
});
it.each(["project", "day"])("a whitespace-only note adds nothing to a %s pick", async (option) => {
  const id = await file();
  expect(pick(id, option, " \t\n ").payload).not.toContain("mk added");
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
it("mismatched pick forwards mk's note to the asking thread as unverified plain text", async () => {
  const id = await file();
  const words = "Per day, because the project grouping hides the urgent work.";
  const w = pick(id, "day", words);
  expect(w.recipient).toBe("thr-a");
  expect(w.payload).toBe(`Ruling on ${id} (autarch/catch-up: collapse order): mk picked "Collapse per day".\nQuestion: ${question}\nmk added (unverified plain text):\n${words}`);
  const sdk = new FakeSdk();
  await new WakeLoop(svc, sdk).drain();
  expect(sdk.sent.find(m => m.threadId === "thr-a")!.input).toBe(framed(w.op!, 1, w.payload!));
});
it.each(["project", "day"])("a %s pick retains a maximum-length note with control characters byte for byte", async (option) => {
  const id = await file();
  const words = " \tline\r\n\u0000\u001b[31m".padEnd(20000, "x");
  const input = { decision_id: id, option_id: option, revision: String(svc.store.decision(id)!.revision), pick_id: `pick-${id}`, reason: words };
  expect(homeMethods.pick.input.safeParse(input).success).toBe(true);
  expect(homeMethods.pick.input.safeParse({ ...input, reason: words + "x" }).success).toBe(false);
  expect(pick(id, option, words).payload).toContain(`\nmk added (unverified plain text):\n${words}`);
});
it("non-card picks keep their full question even when recommended", async () => {
  const id = await file({}, false);
  expect(pick(id).payload).toContain(`Question: ${question}`);
});
it("reopening and retrying the same pick replays the identical saved payload and identity", async () => {
  const id = await file();
  const w = pick(id);
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
it("old full and new compact obligations both replay their stored payload byte for byte", async () => {
  const id = await file();
  // Seed the text and columns an older build recorded, before making a new pick.
  const oldPayload = `Ruling on ${id}: mk picked "Collapse per project".\nQuestion: ${question}\nInstruction: ${OPTIONS[0]!.instruction}`;
  svc.store.insertObligations(id, [{ id: "legacy-wake", kind: "wake", recipient: "thr-old", op: "legacy-op", payload: oldPayload }]);
  pick(id);
  const saved = svc.store.obligationsFor(id).find(o => o.op === `wake:${id}:pick-${id}`)!;
  expect(saved.payload).toContain("🚀…");
  expect(saved.payload).not.toContain(question);
  svc = env.open();
  const sdk = new FakeSdk();
  await new WakeLoop(svc, sdk).drain();
  expect(sdk.sent.find(m => m.threadId === "thr-old")!.input).toBe(framed("legacy-op", 1, oldPayload));
  expect(sdk.sent.find(m => m.threadId === "thr-a")!.input).toBe(framed(saved.op!, 1, saved.payload!));
  expect(svc.store.obligation("legacy-wake")!.payload).toBe(oldPayload);
  expect(svc.store.obligation(saved.id)!.payload).toBe(saved.payload);
});
