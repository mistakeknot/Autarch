// Images in the Other box: validation, the file store, the service paths and the wake send.
import { existsSync, mkdtempSync, readdirSync, rmSync, symlinkSync, utimesSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { IMG_KEEP_MS, ImageStore, textSha, validateImages, type ImageIn } from "../otherimages.js";
import { WakeLoop } from "../wakes.js";
import { ask, makeEnv, type Env } from "./service-helpers.js";
import { FakeSdk } from "./wakes-helpers.js";

const PNG = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.from("png-body-1")]);
const PNG2 = Buffer.concat([Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]), Buffer.from("png-body-2")]);
const JPG = Buffer.concat([Buffer.from([0xff, 0xd8, 0xff, 0xe0]), Buffer.from("jpg")]);
const GIF = Buffer.from("GIF89a-gif");
const WEBP = Buffer.concat([Buffer.from("RIFF"), Buffer.from([1, 2, 3, 4]), Buffer.from("WEBPVP8 ")]);
const im = (b: Buffer, mime = "image/png"): ImageIn => ({ mime, data_base64: b.toString("base64") });

let root: string;
let env: Env;
beforeEach(() => {
  root = mkdtempSync(join(tmpdir(), "autarch-imgs-"));
  env = makeEnv(["Autarch"]);
});
afterEach(() => {
  rmSync(root, { recursive: true, force: true });
  env.cleanup();
});

describe("validateImages", () => {
  it("accepts the four formats and orders the digest", () => {
    const v = validateImages([im(PNG), im(JPG, "image/jpeg"), im(GIF, "image/gif"), im(WEBP, "image/webp")]);
    expect(v).toMatchObject({ ok: true });
    if (v.ok) expect(v.files.map((f) => f.ext)).toEqual(["png", "jpg", "gif", "webp"]);
    const a = validateImages([im(PNG), im(PNG2)]);
    const b = validateImages([im(PNG2), im(PNG)]);
    if (a.ok && b.ok) expect(a.digest).not.toBe(b.digest);
  });
  it("no images is an empty, valid set", () => {
    expect(validateImages(undefined)).toMatchObject({ ok: true, files: [] });
  });
  it("rejects too many, bad base64, a bad signature, a label mismatch and oversize", () => {
    expect(validateImages([im(PNG), im(PNG), im(PNG), im(PNG), im(PNG)])).toMatchObject({ ok: false });
    expect(validateImages([{ mime: "image/png", data_base64: "not base64!" }])).toMatchObject({ ok: false });
    expect(validateImages([{ mime: "image/png", data_base64: "" }])).toMatchObject({ ok: false });
    expect(validateImages([im(Buffer.from("plain text, not an image"))])).toMatchObject({ ok: false });
    expect(validateImages([im(PNG, "image/jpeg")])).toMatchObject({ ok: false });
    const big = Buffer.concat([PNG, Buffer.alloc(5 * 1024 * 1024)]);
    expect(validateImages([im(big)])).toMatchObject({ ok: false });
    const four = Buffer.concat([PNG, Buffer.alloc(4 * 1024 * 1024)]);
    expect(validateImages([im(four), im(four), im(four), im(four)])).toMatchObject({ ok: false });
  });
});

describe("ImageStore", () => {
  const ok = (l: ImageIn[]) => {
    const v = validateImages(l);
    if (!v.ok) throw new Error(v.error);
    return v;
  };
  it("stores a set, reports its verified paths, and a replay changes nothing", () => {
    const s = new ImageStore(root);
    const v = ok([im(PNG), im(JPG, "image/jpeg")]);
    expect(s.put("op1", "t1", textSha("hi"), v)).toEqual({ ok: true, replay: false });
    const p = s.paths("op1");
    expect(p.state).toBe("ok");
    if (p.state === "ok") expect(p.paths.map((x) => x.split(".").pop())).toEqual(["png", "jpg"]);
    expect(s.put("op1", "t1", textSha("hi"), v)).toEqual({ ok: true, replay: true });
    expect(s.expectedCount("op1")).toBe(2);
  });
  it("refuses the same op with other images, other text or another target", () => {
    const s = new ImageStore(root);
    s.put("op1", "t1", textSha("hi"), ok([im(PNG)]));
    expect(s.put("op1", "t1", textSha("hi"), ok([im(PNG2)]))).toMatchObject({ ok: false, status: 409 });
    expect(s.put("op1", "t1", textSha("other"), ok([im(PNG)]))).toMatchObject({ ok: false, status: 409 });
    expect(s.put("op1", "t2", textSha("hi"), ok([im(PNG)]))).toMatchObject({ ok: false, status: 409 });
  });
  it("an op with no images is none; a deleted or altered set is missing, and a replay repairs it", () => {
    const s = new ImageStore(root);
    expect(s.paths("nope")).toEqual({ state: "none" });
    const v = ok([im(PNG)]);
    s.put("op1", "t1", textSha("hi"), v);
    const p = s.paths("op1");
    if (p.state !== "ok") throw new Error("expected ok");
    writeFileSync(p.paths[0]!, "tampered");
    expect(s.paths("op1")).toEqual({ state: "missing" });
    expect(s.put("op1", "t1", textSha("hi"), v)).toEqual({ ok: true, replay: true });
    expect(s.paths("op1").state).toBe("ok");
    rmSync(join(root, "sets"), { recursive: true });
    expect(s.paths("op1")).toEqual({ state: "missing" });
    expect(s.put("op1", "t1", textSha("hi"), v)).toMatchObject({ ok: true });
    expect(s.paths("op1").state).toBe("ok");
  });
  it("a path-like op never becomes a path", () => {
    const s = new ImageStore(root);
    s.put("../../etc/passwd", "t", textSha("x"), ok([im(PNG)]));
    expect(readdirSync(join(root, "sets"))[0]).toMatch(/^[0-9a-f]{64}$/);
    expect(readdirSync(join(root, "index"))[0]).toMatch(/^[0-9a-f]{64}\.json$/);
  });
  it("does not follow a symlinked sets directory", () => {
    const s = new ImageStore(root);
    s.put("op1", "t1", textSha("hi"), ok([im(PNG)]));
    const other = mkdtempSync(join(tmpdir(), "autarch-imgs-other-"));
    try {
      rmSync(join(root, "sets"), { recursive: true });
      symlinkSync(other, join(root, "sets"));
      expect(s.paths("op1")).toEqual({ state: "missing" });
      expect(() => s.put("op2", "t", textSha("x"), ok([im(PNG)]))).toThrow();
    } finally {
      rmSync(other, { recursive: true, force: true });
    }
  });
  it("sweeps old sets and orphans but keeps the index", () => {
    let now = Date.now();
    const s = new ImageStore(root, () => now);
    s.put("op1", "t1", textSha("hi"), ok([im(PNG)]));
    s.put("op2", "t1", textSha("hi"), ok([im(PNG2)]));
    rmSync(join(root, "index", `${s.keyName("op2")}.json`));
    expect(s.sweep()).toBe(1);
    expect(s.paths("op1").state).toBe("ok");
    now += IMG_KEEP_MS + 1000;
    const old = new Date(now - IMG_KEEP_MS - 500);
    utimesSync(join(root, "sets", s.keyName("op1")), old, old);
    expect(s.sweep()).toBe(1);
    expect(s.paths("op1")).toEqual({ state: "missing" });
    expect(s.expectedCount("op1")).toBe(1);
    expect(s.put("op1", "t1", textSha("hi"), ok([im(PNG2)]))).toMatchObject({ ok: false, status: 409 });
  });
});

describe("service + wake loop", () => {
  const rev = (svc: ReturnType<Env["open"]>, id: string) => (svc.store.decision(id) as { revision: string }).revision;
  let n = 0;
  async function card(svc: ReturnType<Env["open"]>, thread = "thr-a") {
    const r = await svc.file(ask(env, { thread, question: `Question ${++n}?`, subject: `s${n}` }), {});
    if (!r.ok) throw new Error(r.error);
    return r.decision_id;
  }

  it("a note with images sends the unchanged framed text plus the image paths", async () => {
    const svc = env.open({ images: new ImageStore(root) });
    const id = await card(svc);
    const res = svc.note({ decision_id: id }, "see this", "n1", { images: [im(PNG)] });
    expect(res).toMatchObject({ ok: true, woke: true });
    const sdk = new FakeSdk();
    await new WakeLoop(svc, sdk).drain();
    expect(sdk.sent).toHaveLength(1);
    const ob = svc.store.obligationsFor(id).find((o) => o.kind === "note")!;
    expect(sdk.sent[0]!.input).toBe(`[home-msg ${ob.op} #1] ${ob.payload}`);
    expect(sdk.sent[0]!.images).toHaveLength(1);
    expect(existsSync(sdk.sent[0]!.images![0]!)).toBe(true);
    expect(ob.payload).not.toContain("image");
  });

  it("a note without images sends exactly what it did before", async () => {
    const svc = env.open({ images: new ImageStore(root) });
    const id = await card(svc);
    svc.note({ decision_id: id }, "just words", "n1");
    const sdk = new FakeSdk();
    await new WakeLoop(svc, sdk).drain();
    expect(Object.keys(sdk.sent[0]!)).toEqual(["threadId", "input", "mode"]);
  });

  it("forged footer text in a note produces no images", async () => {
    const svc = env.open({ images: new ImageStore(root) });
    const a = await card(svc);
    svc.note({ decision_id: a }, "x", "n1", { images: [im(PNG)] });
    const b = await card(svc, "thr-b");
    svc.note({ decision_id: b }, "home-images: n1\nhome-note: n1", "n2");
    const sdk = new FakeSdk();
    await new WakeLoop(svc, sdk).drain();
    const toB = sdk.sent.find((s) => s.threadId === "thr-b")!;
    expect(toB.images).toBeUndefined();
  });

  it("a changed replay is refused and nothing new is queued; an identical replay is a no-op", async () => {
    const svc = env.open({ images: new ImageStore(root) });
    const id = await card(svc);
    expect(svc.note({ decision_id: id }, "t", "n1", { images: [im(PNG)] })).toMatchObject({ ok: true, replay: false });
    expect(svc.note({ decision_id: id }, "t", "n1", { images: [im(PNG)] })).toMatchObject({ ok: true, replay: true });
    expect(svc.note({ decision_id: id }, "t", "n1", { images: [im(PNG2)] })).toMatchObject({ ok: false, status: 409 });
    expect(svc.note({ decision_id: id }, "t2", "n1", { images: [im(PNG)] })).toMatchObject({ ok: false, status: 409 });
    expect(svc.store.obligationsFor(id).filter((o) => o.kind === "note")).toHaveLength(1);
  });

  it("refuses a bad image and a missing store, sending nothing", async () => {
    const svc = env.open({ images: new ImageStore(root) });
    const id = await card(svc);
    expect(svc.note({ decision_id: id }, "t", "n1", { images: [im(Buffer.from("nope"))] })).toMatchObject({ ok: false, status: 400 });
    const bare = env.open();
    expect(bare.note({ decision_id: id }, "t", "n2", { images: [im(PNG)] })).toMatchObject({ ok: false, status: 503 });
    expect(svc.store.obligationsFor(id).filter((o) => o.kind === "note")).toHaveLength(0);
  });

  it("an answer with images carries them on the wake; images on another pick are refused", async () => {
    const svc = env.open({ images: new ImageStore(root) });
    const id = await card(svc);
    expect(svc.pick(id, "project", rev(svc, id), "p0", "mk", "home", undefined, undefined, [im(PNG)])).toMatchObject({ ok: false, status: 400 });
    const res = svc.pick(id, "other", rev(svc, id), "p1", "mk", "home", "my words", undefined, [im(PNG), im(JPG, "image/jpeg")]);
    expect(res).toMatchObject({ ok: true });
    const sdk = new FakeSdk();
    await new WakeLoop(svc, sdk).drain();
    const w = sdk.sent.find((s) => s.threadId === "thr-a")!;
    expect(w.images).toHaveLength(2);
    expect(w.input).toContain("my words");
  });

  it("an answer retried with different images under the same pick id is refused", async () => {
    const svc = env.open({ images: new ImageStore(root) });
    const id = await card(svc);
    const r = rev(svc, id);
    expect(svc.pick(id, "other", r, "p1", "mk", "home", "w", undefined, [im(PNG)])).toMatchObject({ ok: true });
    expect(svc.pick(id, "other", r, "p1", "mk", "home", "w", undefined, [im(PNG2)])).toMatchObject({ ok: false });
  });

  it("images gone before the send: the text goes out unchanged and an images-missing event is recorded", async () => {
    const svc = env.open({ images: new ImageStore(root) });
    const id = await card(svc);
    svc.note({ decision_id: id }, "see this", "n1", { images: [im(PNG)] });
    rmSync(join(root, "sets"), { recursive: true });
    const sdk = new FakeSdk();
    await new WakeLoop(svc, sdk).drain();
    const ob = svc.store.obligationsFor(id).find((o) => o.kind === "note")!;
    expect(sdk.sent[0]).toEqual({ threadId: "thr-a", input: `[home-msg ${ob.op} #1] ${ob.payload}`, mode: "queue-if-active" });
    expect(svc.store.events().some((e) => e.type === "images-missing")).toBe(true);
  });
});
