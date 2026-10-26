// The Other box: "Answer with this" is a virtual pick option `other`; "Ask / note" is a comment plus
// an owner wake that never rules or closes.
import { freeOwnWordsId, OWN_WORDS_LABEL } from "../ownwords.js";
import { commentBody } from "../cardwrites.js";
import { afterEach, describe, expect, it } from "vitest";
import { cleanupEnvs, opened, rig } from "./card-rig.js";
import { hasNoteMarker } from "../service.js";

afterEach(cleanupEnvs);

const rev = (r: ReturnType<typeof rig>, id: string) => (r.svc.store.decision(id) as any).revision as string;
const answer = (r: ReturnType<typeof rig>, id: string, text: string | undefined, pickId = "p-other", by = "mk") => r.svc.pick(id, "other", rev(r, id), pickId, by, "home", text);

describe("Answer with this (option other)", () => {
  it("records mk's words in picks.reason, queues the ruling file and an owner wake", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    const res = answer(r, g1.id, "  Do the day split, but only for last week.  ");
    expect(res).toMatchObject({ ok: true, status: 201 });
    const p = r.svc.store.pick(g1.id)!;
    expect(p).toMatchObject({ option_id: "other", by: "mk", reason: "Do the day split, but only for last week." });
    const kinds = (r.db.prepare("SELECT kind, recipient, payload FROM obligations WHERE decision_id = ?").all(g1.id) as any[]);
    expect(kinds.map((k) => k.kind)).toContain("ruling-file");
    const wake = kinds.find((k) => k.kind === "wake");
    expect(wake.recipient).toBe(g1.thread);
    expect(wake.payload).toContain("Do the day split, but only for last week.");
    expect(wake.payload).toContain("not an instruction");
  });

  it("enforces the 20000-character cap and rejects empty text", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    expect(answer(r, g1.id, "x".repeat(20001))).toMatchObject({ ok: false, status: 400 });
    expect(answer(r, g1.id, "   ")).toMatchObject({ ok: false, status: 400 });
    expect(answer(r, g1.id, undefined)).toMatchObject({ ok: false, status: 400 });
    expect(r.svc.store.pick(g1.id)).toBeUndefined();
    expect(answer(r, g1.id, "y".repeat(20000), "p-ok")).toMatchObject({ ok: true });
  });

  it("delivers a full-length answer to the owner whole", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    const words = "z".repeat(20000);
    expect(answer(r, g1.id, words, "p-long")).toMatchObject({ ok: true });
    const wake = (r.db.prepare("SELECT payload FROM obligations WHERE decision_id = ? AND kind = 'wake'").all(g1.id) as any[])[0];
    expect(wake.payload).toContain(words);
  });

  it("is refused on an unbound card (root not verified)", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    r.db.prepare("UPDATE cards SET root_state = 'unbound' WHERE task_id = ?").run(t.id);
    expect(answer(r, g1.id, "hello")).toMatchObject({ ok: false, status: 403 });
    expect(r.svc.store.pick(g1.id)).toBeUndefined();
  });

  it("is mk's alone: the vizier cannot rule other, and delegation never sees it", async () => {
    const r = rig();
    r.enableDelegation();
    const { g1 } = await opened(r);
    expect(answer(r, g1.id, "hello", "p-v", "vizier")).toMatchObject({ ok: false, status: 403 });
    const res = r.dele.rule(g1.id, "other", "reversible and routine", { threadId: "thr_viz" });
    expect(res).toMatchObject({ ok: false, status: 400 });
    expect(r.svc.store.pick(g1.id)).toBeUndefined();
  });

  it("replays idempotently and refuses a changed answer", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    expect(answer(r, g1.id, "same words")).toMatchObject({ ok: true, status: 201 });
    expect(answer(r, g1.id, "same words")).toMatchObject({ ok: true, status: 200 });
    expect(answer(r, g1.id, "other words")).toMatchObject({ ok: false, status: 409 });
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM picks WHERE decision_id = ?").get(g1.id)).toEqual({ n: 1 });
    expect(r.db.prepare("SELECT COUNT(*) AS n FROM obligations WHERE decision_id = ? AND kind = 'wake'").get(g1.id)).toEqual({ n: 1 });
  });

  it("keeps markup and escape codes as text, and mirrors the answer to the card comment", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const text = "<img src=x onerror=alert(1)> \u001b[31mred";
    expect(answer(r, g1.id, text)).toMatchObject({ ok: true });
    expect(r.svc.store.pick(g1.id)!.reason).toBe(text);
    const w = r.db.prepare("SELECT payload FROM card_writes WHERE task_id = ? AND kind = 'comment'").get(t.id) as { payload: string };
    expect(JSON.parse(w.payload)).toMatchObject({ option_id: "other", reason: text });
  });
});

describe("note marker and an ask with a real option named other", () => {
  it("the idempotence marker is matched as the whole last line, so n1 never matches n10", () => {
    const body = (id: string) => `mk's note (Home, advisory; plain text):\nhi\n\nhome-note: ${id}`;
    expect(hasNoteMarker(body("n10"), "n1")).toBe(false);
    expect(hasNoteMarker(body("n1"), "n10")).toBe(false);
    expect(hasNoteMarker(body("n1"), "n1")).toBe(true);
    expect(hasNoteMarker(body("n1") + "\n", "n1")).toBe(true);
    expect(hasNoteMarker(" home-note: n1 ", "n1")).toBe(false);
    expect(hasNoteMarker("hi\nhome-note: n1 ", "n1")).toBe(false);
    expect(hasNoteMarker("hi\r\nhome-note: n1\r", "n1")).toBe(true);
    expect(hasNoteMarker("hi\nhome-note: n1\r\n", "n1")).toBe(true);
    expect(hasNoteMarker("a home-note: n1 in the middle\nmore", "n1")).toBe(false);
  });

  it("an ask with a real option id other still takes mk's own words, recorded under mk-own-words", async () => {
    const r = rig();
    const { g1 } = await opened(r, { ask: { options: [{ id: "other", label: "Something else", kind: "ruling-only", reversible: true }, { id: "a", label: "A", kind: "ruling-only", reversible: true }] } });
    // the bare id other is the ask's own option and takes no reason from the free-text box
    expect(answer(r, g1.id, "my own words", "p-x")).toMatchObject({ ok: false, status: 400 });
    expect(r.svc.store.pick(g1.id)).toBeUndefined();
    const words = r.svc.pick(g1.id, "mk-own-words", rev(r, g1.id), "p-words", "mk", "home", "my own words");
    expect(words).toMatchObject({ ok: true });
    expect(r.svc.store.pick(g1.id)).toMatchObject({ option_id: "mk-own-words", reason: "my own words" });
  });

  it("free text never selects a real option: with a real mk-own-words option the box uses the next free id", async () => {
    const r = rig();
    const opts = [{ id: "other", label: "Something else", kind: "ruling-only" as const, reversible: true }, { id: "mk-own-words", label: "Real", kind: "ruling-only" as const, reversible: true }];
    expect(freeOwnWordsId(opts)).toBe("mk-own-words-2");
    expect(freeOwnWordsId([...opts, { id: "mk-own-words-2" }])).toBe("mk-own-words-3");
    expect(freeOwnWordsId(undefined)).toBe("mk-own-words");
    const { g1 } = await opened(r, { ask: { options: opts } });
    const words = r.svc.pick(g1.id, freeOwnWordsId(opts), rev(r, g1.id), "p-w2", "mk", "home", "Do not run anything");
    expect(words).toMatchObject({ ok: true });
    expect(r.svc.store.pick(g1.id)).toMatchObject({ option_id: "mk-own-words-2", reason: "Do not run anything" });
  });

  it("the card comment calls a pick own words only for the virtual option, never a real option with a reserved-looking id", () => {
    const virtual = commentBody("c1", { option_id: "mk-own-words", option_label: OWN_WORDS_LABEL, reason: "my words", by: "mk" });
    expect(virtual).toContain("mk's answer (plain text):");
    const real = commentBody("c2", { option_id: "mk-own-words", option_label: "Real", reason: "a reason", by: "vizier" });
    expect(real).not.toContain("mk's answer (plain text):");
  });

  it("picking the real option other still works on such an ask", async () => {
    const r = rig();
    const { g1 } = await opened(r, { ask: { options: [{ id: "other", label: "Something else", kind: "ruling-only", reversible: true }, { id: "a", label: "A", kind: "ruling-only", reversible: true }] } });
    const real = r.svc.pick(g1.id, "other", rev(r, g1.id), "p-real", "mk", "home");
    expect(real).toMatchObject({ ok: true });
    expect(r.svc.store.pick(g1.id)).toMatchObject({ option_id: "other", reason: null });
  });
});

describe("Ask / note ordering", () => {
  it("with deferWake no obligation exists until the comment is posted and commit runs", async () => {
    const r = rig();
    const { t } = await opened(r);
    const a = r.svc.note({ task_id: t.id }, "question", "o1", { deferWake: true });
    if (!a.ok) throw new Error("expected ok");
    // the comment write failed here: commit is never called, so nobody is woken
    expect(r.db.prepare("SELECT COUNT(*) n FROM obligations WHERE kind = 'note'").get()).toEqual({ n: 0 });
    expect(a.commit()).toEqual({ replay: false, woke: true });
    expect(a.commit()).toEqual({ replay: true, woke: true });
    expect(r.db.prepare("SELECT COUNT(*) n FROM obligations WHERE kind = 'note'").get()).toEqual({ n: 1 });
  });
});

describe("Ask / note", () => {
  it("wakes the owner once, never rules, and is idempotent per note id", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const a = r.svc.note({ task_id: t.id }, " Which week do you mean? ", "n1");
    expect(a).toMatchObject({ ok: true, replay: false, woke: true });
    if (a.ok) expect(a.body).toContain("Which week do you mean?");
    expect(r.svc.note({ task_id: t.id }, "Which week do you mean?", "n1")).toMatchObject({ ok: true, replay: true });
    const obs = r.db.prepare("SELECT kind, recipient, payload FROM obligations WHERE decision_id = ? AND kind = 'note'").all(g1.id) as any[];
    expect(obs).toHaveLength(1);
    expect(obs[0].recipient).toBe(g1.thread);
    expect(r.svc.store.pick(g1.id)).toBeUndefined();
    expect(r.svc.store.decision(g1.id)).toMatchObject({ withdrawn_at: null });
  });

  it("takes a decision id too, and exactly one target", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    expect(r.svc.note({ decision_id: g1.id }, "hi", "n5")).toMatchObject({ ok: true, woke: true, task_id: t.id });
    expect(r.svc.note({ decision_id: g1.id, task_id: t.id }, "hi", "n6")).toMatchObject({ ok: false, status: 400 });
    expect(r.svc.note({}, "hi", "n7")).toMatchObject({ ok: false, status: 400 });
  });

  it("delivers a full-length note to the owner whole", async () => {
    const r = rig();
    const { t, g1 } = await opened(r);
    const words = "q".repeat(20000);
    expect(r.svc.note({ task_id: t.id }, words, "n-long")).toMatchObject({ ok: true, woke: true });
    const ob = (r.db.prepare("SELECT payload FROM obligations WHERE decision_id = ? AND kind = 'note'").all(g1.id) as any[])[0];
    expect(ob.payload).toContain(words);
  });

  it("enforces the 20000 cap and knows unknown cards", async () => {
    const r = rig();
    const { t } = await opened(r);
    expect(r.svc.note({ task_id: t.id }, "x".repeat(20001), "n2")).toMatchObject({ ok: false, status: 400 });
    expect(r.svc.note({ task_id: t.id }, "", "n3")).toMatchObject({ ok: false, status: 400 });
    expect(r.svc.note({ task_id: "no-such" }, "hi", "n4")).toMatchObject({ ok: false, status: 404 });
  });
});
