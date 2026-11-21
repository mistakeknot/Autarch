// mk-cqnab.1 PR3 stage 1: the versioned pick-parameter digest. A retry under the same pick id must carry
// the same exact words; a legacy pick must match its old digest and its stored words. Nothing historical is rewritten.
import { createHash } from "node:crypto";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { ImageStore, type ImageIn } from "../otherimages.js";
import { pickParamsDigest } from "../service.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";

afterEach(cleanupEnvs);
const roots: string[] = [];
afterEach(() => { while (roots.length) rmSync(roots.pop()!, { recursive: true, force: true }); });

type Rig = ReturnType<typeof rig>;
const rev = (r: Rig, id: string) => (r.svc.store.decision(id) as { revision: string }).revision;
const PNG = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.from("png-body-1")]);
const PNG2 = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.from("png-body-2")]);
const im = (b: Buffer): ImageIn => ({ mime: "image/png", data_base64: b.toString("base64") });
const QUESTIONS = [
  { id: "ui", prompt: "Where does it run?", recommendation: "terminal", options: [{ id: "terminal", label: "Terminal" }, { id: "web", label: "Web" }] },
  { id: "pace", prompt: "How does play move?", recommendation: "turns", options: [{ id: "turns", label: "Turn-based" }, { id: "live", label: "Real-time" }] },
];
const legacy = (base: string) => createHash("sha256").update(base).digest("hex");
const obligations = (r: Rig, id: string) => r.db.prepare("SELECT id, kind, payload, next_try_at FROM obligations WHERE decision_id = ? ORDER BY id").all(id);
const writes = (r: Rig, id: string) => r.db.prepare("SELECT id, payload FROM card_writes WHERE decision_id = ? ORDER BY id").all(id);

/** A pick exactly as the pre-PR3 Service stored it: legacy digest, stored words and a queued wake payload. */
function legacyPick(r: Rig, id: string, opts: { reason: string | null; hash: string; option?: string; payload?: string }) {
  const res = r.svc.store.recordPick(
    { decision_id: id, pick_id: "p-legacy", option_id: opts.option ?? "a", revision: rev(r, id), by: "mk", surface: "home", reason: opts.reason, params_hash: opts.hash },
    [{ id: `ob:${id}:wake:p-legacy`, kind: "wake", recipient: "thr_a", op: `wake:${id}:p-legacy`, payload: opts.payload ?? "legacy queued payload\nmk added (unverified plain text):\nUse blue é́\u0007" }],
  );
  expect(res.ok).toBe(true);
}

describe("versioned pick-parameter digest", () => {
  it("is framed, versioned and distinct from every legacy digest", () => {
    const d = pickParamsDigest({ option_id: "a", revision: "r", by: "mk", reason: "x", answer_form: "option", answers: null, image_digest: null });
    expect(d).toMatch(/^p2:[0-9a-f]{64}$/);
    expect(d).not.toBe(legacy("a|r|mk"));
    // Framing: moving a separator between fields changes the digest.
    expect(pickParamsDigest({ option_id: "a|r", revision: "mk", by: "x", reason: null, answer_form: "option", answers: null, image_digest: null }))
      .not.toBe(pickParamsDigest({ option_id: "a", revision: "r|mk", by: "x", reason: null, answer_form: "option", answers: null, image_digest: null }));
  });
  it.each([
    ["absent vs empty", null, ""],
    ["empty vs space", "", " "],
    ["space vs no-break space", " ", "\u00a0"],
    ["space vs ideographic space", " ", "\u3000"],
    ["tab vs em space", "\t", "\u2003"],
    ["composed vs decomposed", "é", "é"],
  ])("exact reason: %s differ", (_n, a, b) => {
    const f = (reason: string | null) => pickParamsDigest({ option_id: "a", revision: "r", by: "mk", reason, answer_form: "option", answers: null, image_digest: null });
    expect(f(a)).not.toBe(f(b));
  });
  it("answer form, answer values and image digest each change the digest", () => {
    const b = { option_id: "answers", revision: "r", by: "mk", reason: null, answer_form: "answers" as const, answers: [["pace", "turns"], ["ui", "terminal"]] as [string, string][], image_digest: null };
    expect(pickParamsDigest(b)).not.toBe(pickParamsDigest({ ...b, answers: [["pace", "live"], ["ui", "terminal"]] }));
    expect(pickParamsDigest(b)).not.toBe(pickParamsDigest({ ...b, answer_form: "own-words" }));
    expect(pickParamsDigest(b)).not.toBe(pickParamsDigest({ ...b, image_digest: "f".repeat(64) }));
  });
});

describe("Service.pick retry under the same pick id", () => {
  it("new ordinary-reason change under the same pick id conflicts", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-1", "mk", "home", "Use blue")).toMatchObject({ ok: true, status: 201 });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-1", "mk", "home", "Use blue")).toMatchObject({ ok: true, status: 200 });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-1", "mk", "home", "Use red")).toMatchObject({ ok: false, status: 409, error: "pick id reused" });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-1", "mk", "home")).toMatchObject({ ok: false, status: 409, error: "pick id reused" });
    expect(r.svc.store.pick(g1.id)!.reason).toBe("Use blue");
  });
  it.each([
    ["absent vs empty", undefined, ""],
    ["empty vs no-break space", "", "\u00a0"],
    ["space vs ideographic space", " ", "\u3000"],
    ["LF vs line separator", "\n", "\u2028"],
  ])("empty vs Unicode whitespace retry: %s conflicts", async (_n, first, second) => {
    const r = rig();
    const { g1 } = await opened(r);
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-w", "mk", "home", first)).toMatchObject({ ok: true, status: 201 });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-w", "mk", "home", first)).toMatchObject({ ok: true, status: 200 });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-w", "mk", "home", second)).toMatchObject({ ok: false, status: 409 });
    expect(r.svc.store.pick(g1.id)!.reason).toBe(first ?? null);
  });
  it("own-words collision: a real option named other retried with different words conflicts", async () => {
    const r = rig();
    const ask = { options: [{ id: "other", label: "Other plan", kind: "instruction", instruction: "Do the other plan.", reversible: true }, { id: "b", label: "B", kind: "ruling-only" }] };
    const { g1 } = await opened(r, { ask });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-o", "mk", "cli", "first words")).toMatchObject({ ok: true, status: 201 });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-o", "mk", "cli", "first words")).toMatchObject({ ok: true, status: 200 });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-o", "mk", "cli", "second words")).toMatchObject({ ok: false, status: 409 });
  });
  it("own words: exact raw words are part of the identity", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-ow", "mk", "home", "same words")).toMatchObject({ ok: true, status: 201 });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-ow", "mk", "home", "same words")).toMatchObject({ ok: true, status: 200 });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-ow", "mk", "home", "same words ")).toMatchObject({ ok: false, status: 409 });
  });
  it("multi-answers: changed answers or a changed note conflict", async () => {
    const r = rig();
    const { g1 } = await opened(r, { ask: { questions: QUESTIONS, options: [] } });
    const pick = (answers: Record<string, string>, note?: string) => r.svc.pick(g1.id, "answers", rev(r, g1.id), "p-m", "mk", "home", note, undefined, undefined, answers);
    expect(pick({ ui: "terminal", pace: "turns" }, "note one")).toMatchObject({ ok: true, status: 201 });
    expect(pick({ pace: "turns", ui: "terminal" }, "note one")).toMatchObject({ ok: true, status: 200 });
    expect(pick({ ui: "terminal", pace: "turns" }, "note  one")).toMatchObject({ ok: false, status: 409 });
    expect(pick({ ui: "web", pace: "turns" }, "note one")).toMatchObject({ ok: false, status: 409 });
  });
  it("images: the same words with other images conflict; the same images replay", async () => {
    const root = mkdtempSync(join(tmpdir(), "autarch-digest-imgs-")); roots.push(root);
    const r = rig({ svc: { images: new ImageStore(root) } });
    const { g1 } = await opened(r);
    const pick = (img: Buffer) => r.svc.pick(g1.id, "other", rev(r, g1.id), "p-i", "mk", "home", "see this", undefined, [im(img)]);
    expect(pick(PNG)).toMatchObject({ ok: true, status: 201 });
    expect(pick(PNG)).toMatchObject({ ok: true, status: 200 });
    expect(pick(PNG2)).toMatchObject({ ok: false, status: 409 });
  });
});

describe("legacy retries keep their old digest and must match stored words", () => {
  it("legacy ordinary-reason retry with the same words replays the old queued payload byte for byte", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    const words = "Use blue é́\u0007";
    legacyPick(r, g1.id, { reason: words, hash: legacy(`a|${rev(r, g1.id)}|mk`) });
    const before = { pick: r.svc.store.pick(g1.id), obligations: obligations(r, g1.id), writes: writes(r, g1.id) };
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-legacy", "mk", "home", words)).toMatchObject({ ok: true, status: 200 });
    expect({ pick: r.svc.store.pick(g1.id), obligations: obligations(r, g1.id), writes: writes(r, g1.id) }).toEqual(before);
    expect(r.svc.store.pick(g1.id)!.params_hash).toBe(legacy(`a|${rev(r, g1.id)}|mk`));
  });
  it("legacy ordinary-reason retry with changed words conflicts and changes nothing", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    legacyPick(r, g1.id, { reason: "Use blue", hash: legacy(`a|${rev(r, g1.id)}|mk`) });
    const before = { pick: r.svc.store.pick(g1.id), obligations: obligations(r, g1.id) };
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-legacy", "mk", "home", "Use red")).toMatchObject({ ok: false, status: 409, error: "pick id reused" });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-legacy", "mk", "home")).toMatchObject({ ok: false, status: 409 });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-legacy", "mk", "home", "Use blue\u00a0")).toMatchObject({ ok: false, status: 409 });
    expect({ pick: r.svc.store.pick(g1.id), obligations: obligations(r, g1.id) }).toEqual(before);
  });
  it("legacy pick without words conflicts with a retry that adds words", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    legacyPick(r, g1.id, { reason: null, hash: legacy(`a|${rev(r, g1.id)}|mk`) });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-legacy", "mk", "home")).toMatchObject({ ok: true, status: 200 });
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-legacy", "mk", "home", "added later")).toMatchObject({ ok: false, status: 409 });
  });
  it("legacy own-words retry replays only with its stored words", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    legacyPick(r, g1.id, { option: "other", reason: "my words", hash: legacy(`other|${rev(r, g1.id)}|mk|my words`) });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-legacy", "mk", "home", "  my words  ")).toMatchObject({ ok: true, status: 200 });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-legacy", "mk", "home", "your words")).toMatchObject({ ok: false, status: 409 });
  });
  it("legacy image retry keeps its img1 digest", async () => {
    const root = mkdtempSync(join(tmpdir(), "autarch-digest-legacy-")); roots.push(root);
    const store = new ImageStore(root);
    const r = rig({ svc: { images: store } });
    const { g1 } = await opened(r);
    const { validateImages } = await import("../otherimages.js");
    const v = validateImages([im(PNG)]);
    if (!v.ok) throw new Error(v.error);
    const base = `other|${rev(r, g1.id)}|mk|see this`;
    legacyPick(r, g1.id, { option: "other", reason: "see this", hash: `img1:${createHash("sha256").update(JSON.stringify([base, v.digest])).digest("hex")}` });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-legacy", "mk", "home", "see this", undefined, [im(PNG)])).toMatchObject({ ok: true, status: 200 });
    expect(r.svc.pick(g1.id, "other", rev(r, g1.id), "p-legacy", "mk", "home", "see this", undefined, [im(PNG2)])).toMatchObject({ ok: false, status: 409 });
  });
  it("a pick with no stored digest still conflicts", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    const res = r.svc.store.recordPick({ decision_id: g1.id, pick_id: "p-null", option_id: "a", revision: rev(r, g1.id), by: "mk", surface: "home", reason: null, params_hash: null }, []);
    expect(res.ok).toBe(true);
    expect(r.svc.pick(g1.id, "a", rev(r, g1.id), "p-null", "mk", "home")).toMatchObject({ ok: false, status: 409 });
  });
});
