// The Idea box and weekly digest (bead mk-2zojo): an idea is a tracker card with mk's words verbatim; filing is
// idempotent and tells one recipient; Pursue / Park / Drop never close work the coordinator owns. No schema change.
import { afterEach, describe, expect, it } from "vitest";
import { Ideas, IDEA_MAX, cleanIdea, hasIdeaMarker, ideaDescription, ideaTitle, ideaWords } from "../ideas.js";
import { TasksClient } from "../tasks.js";
import { cleanupEnvs, rig, type Rig } from "./card-rig.js";

afterEach(cleanupEnvs);

function desk(r: Rig) {
  r.svc.store.setSetting("vizierThreadId", "thr_viz");
  return new Ideas({ tasks: new TasksClient(r.fake, { now: () => r.env.clock.t }), store: r.svc.store, now: r.env.now });
}
const obligations = (r: Rig) => r.db.prepare("SELECT * FROM obligations WHERE kind = 'idea' ORDER BY rowid").all() as { recipient: string; payload: string; op: string; decision_id: string }[];
const labelNames = (r: Rig, t: { labelIds: string[] }) => t.labelIds.map((id) => r.fake.labels.find((l) => l.id === id)!.name).sort();

describe("idea text", () => {
  it("keeps mk's words, trims, and bounds them", () => {
    expect(cleanIdea("  fork a jawntology  ")).toBe("fork a jawntology");
    expect(cleanIdea("   ")).toBeNull();
    expect(cleanIdea("x".repeat(IDEA_MAX + 1))).toBeNull();
    expect(cleanIdea("x".repeat(IDEA_MAX))).not.toBeNull();
  });
  it("round-trips the words through the description and finds only its own marker", () => {
    const d = ideaDescription("it would be cool if...\nsecond line", "idea-1");
    expect(ideaWords(d)).toBe("it would be cool if...\nsecond line");
    expect(hasIdeaMarker(d, "idea-1")).toBe(true);
    expect(hasIdeaMarker(d, "idea-2")).toBe(false);
    expect(ideaTitle("a".repeat(100))).toHaveLength(80);
  });
});

describe("file", () => {
  it("creates one card labelled idea and from-mk with mk's words, and tells the vizier", async () => {
    const r = rig();
    const ideas = desk(r);
    const out = await ideas.file({ project_id: r.tp.id, text: "let people fork a jawntology", idea_id: "i1" });
    expect(out).toMatchObject({ ok: true, replay: false, woke: true });
    const t = r.fake.tasks.find((x) => x.description.includes("home-idea: i1"))!;
    expect(labelNames(r, t)).toEqual(["from-mk", "idea"]);
    expect(ideaWords(t.description)).toBe("let people fork a jawntology");
    const ob = obligations(r);
    expect(ob).toHaveLength(1);
    expect(ob[0]).toMatchObject({ recipient: "thr_viz", decision_id: `idea:${t.id}` });
    expect(ob[0]!.payload).toContain("not work until it is picked");
  });
  it("files once for a double tap or a retry", async () => {
    const r = rig();
    const ideas = desk(r);
    await ideas.file({ project_id: r.tp.id, text: "one", idea_id: "i1" });
    const again = await ideas.file({ project_id: r.tp.id, text: "one", idea_id: "i1" });
    expect(again).toMatchObject({ ok: true, replay: true });
    expect(r.fake.tasks.filter((t) => t.description.includes("home-idea: i1"))).toHaveLength(1);
    expect(obligations(r)).toHaveLength(1);
  });
  it("finds the card after a lost response instead of making a second", async () => {
    const r = rig();
    const ideas = desk(r);
    r.fake.lostResponses.push({ method: "createTask", nth: 1 });
    expect(await ideas.file({ project_id: r.tp.id, text: "one", idea_id: "i1" })).toMatchObject({ ok: false, status: 502 });
    expect(await ideas.file({ project_id: r.tp.id, text: "one", idea_id: "i1" })).toMatchObject({ ok: true, replay: true, woke: true });
    expect(r.fake.tasks.filter((t) => t.description.includes("home-idea: i1"))).toHaveLength(1);
    expect(obligations(r)).toHaveLength(1);
  });
  it("refuses empty text and unknown projects, and files nothing", async () => {
    const r = rig();
    const ideas = desk(r);
    expect(await ideas.file({ project_id: r.tp.id, text: "  ", idea_id: "i1" })).toMatchObject({ ok: false, status: 400 });
    expect(await ideas.file({ project_id: "01HZZZZZZZZZZZZZZZZZZZZZZZ", text: "x", idea_id: "i2" })).toMatchObject({ ok: false, status: 404 });
    expect(r.fake.tasks).toHaveLength(0);
  });
  it("tells the project's coordinator when one is named, else the vizier, and nobody when neither", async () => {
    const r = rig();
    const ideas = desk(r);
    r.svc.store.setSetting("projectCoordinators", JSON.stringify({ [r.tp.prefix]: "thr_coord" }));
    await ideas.file({ project_id: r.tp.id, text: "a", idea_id: "i1" });
    expect(obligations(r)[0]!.recipient).toBe("thr_coord");
    const r2 = rig();
    const none = new Ideas({ tasks: new TasksClient(r2.fake), store: r2.svc.store, now: r2.env.now });
    expect(await none.file({ project_id: r2.tp.id, text: "a", idea_id: "i1" })).toMatchObject({ ok: true, woke: false });
    expect(obligations(r2)).toHaveLength(0);
  });
});

describe("review round 1", () => {
  it("files once when two submissions with one id race", async () => {
    const r = rig();
    const ideas = desk(r);
    const [a, b] = await Promise.all([ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" }), ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" })]);
    expect(a.ok && b.ok).toBe(true);
    expect(r.fake.tasks.filter((t) => t.description.includes("home-idea: i1"))).toHaveLength(1);
    expect(obligations(r)).toHaveLength(1);
  });
  it("repairs a missing wake on a replay once a recipient exists", async () => {
    const r = rig();
    const ideas = new Ideas({ tasks: new TasksClient(r.fake), store: r.svc.store, now: r.env.now });
    expect(await ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" })).toMatchObject({ ok: true, woke: false });
    r.svc.store.setSetting("vizierThreadId", "thr_viz");
    expect(await ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" })).toMatchObject({ ok: true, replay: true, woke: true });
    expect(obligations(r)).toHaveLength(1);
  });
  it("cannot drop or park an idea that was already pursued", async () => {
    const r = rig();
    const ideas = desk(r);
    const f = await ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" });
    if (!f.ok) throw new Error("file");
    await ideas.act({ task_id: f.task_id, action: "pursue" });
    expect(await ideas.act({ task_id: f.task_id, action: "drop" })).toMatchObject({ ok: false, status: 409 });
    expect(r.fake.tasks.find((t) => t.id === f.task_id)!.status).not.toBe("canceled");
    expect(await ideas.act({ task_id: f.task_id, action: "pursue" })).toMatchObject({ ok: true, replay: true });
  });
  it("posts one comment when two identical actions race, and keeps words that look like the provenance rule", async () => {
    const r = rig();
    const ideas = desk(r);
    const tricky = "before\n\n---\nmk's words, verbatim and more";
    const f = await ideas.file({ project_id: r.tp.id, text: tricky, idea_id: "i1" });
    if (!f.ok) throw new Error("file");
    await Promise.all([ideas.act({ task_id: f.task_id, action: "park" }), ideas.act({ task_id: f.task_id, action: "park" })]);
    expect(r.fake.comments.filter((c) => c.taskId === f.task_id && c.body.includes("home-idea-action"))).toHaveLength(1);
    expect(ideaWords(r.fake.tasks.find((t) => t.id === f.task_id)!.description)).toBe(tricky);
  });
});

describe("review round 7", () => {
  it("refuses a retry of a saved submission under another project", async () => {
    const r = rig();
    const ideas = desk(r);
    await ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" });
    const other = r.fake.addProject("Other");
    expect(await ideas.file({ project_id: other.id, text: "x", idea_id: "i1" })).toMatchObject({ ok: false, status: 409 });
    expect(obligations(r)).toHaveLength(1);
  });
});

describe("review round 2", () => {
  it("finishes a partly done action: comment posted, change missing", async () => {
    const r = rig();
    const ideas = desk(r);
    const f = await ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" });
    if (!f.ok) throw new Error("file");
    r.fake.addComment(f.task_id, { body: `Pursue (Home).\n\nhome-idea-action: ${f.task_id}:pursue` });
    expect(await ideas.act({ task_id: f.task_id, action: "pursue" })).toMatchObject({ ok: true, replay: true, woke: true });
    expect(labelNames(r, r.fake.tasks.find((t) => t.id === f.task_id)!)).toContain("pursue");
    expect(await ideas.act({ task_id: f.task_id, action: "drop" })).toMatchObject({ ok: false, status: 409 });
  });
  it("does not claim a state it did not record as mk's choice", async () => {
    const r = rig();
    const ideas = desk(r);
    const f = await ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" });
    if (!f.ok) throw new Error("file");
    r.fake.tasks.find((t) => t.id === f.task_id)!.status = "canceled";
    expect(await ideas.act({ task_id: f.task_id, action: "drop" })).toMatchObject({ ok: false, status: 409 });
    expect(r.fake.comments.filter((c) => c.taskId === f.task_id && c.body.includes("home-idea-action"))).toHaveLength(0);
  });
  it("a retry whose card was decided another way is a conflict and wakes nobody", async () => {
    const r = rig();
    const ideas = desk(r);
    const f = await ideas.file({ project_id: r.tp.id, text: "x", idea_id: "i1" });
    if (!f.ok) throw new Error("file");
    await ideas.act({ task_id: f.task_id, action: "park" });
    r.fake.addComment(f.task_id, { body: `Pursue (Home).\n\nhome-idea-action: ${f.task_id}:pursue` });
    expect(await ideas.act({ task_id: f.task_id, action: "pursue" })).toMatchObject({ ok: false, status: 409 });
    expect(obligations(r).filter((o) => o.op.startsWith("idea-pursue"))).toHaveLength(0);
  });
  it("wakes with the card's words on a replay, not the retry's text", async () => {
    const r = rig();
    const ideas = new Ideas({ tasks: new TasksClient(r.fake), store: r.svc.store, now: r.env.now });
    await ideas.file({ project_id: r.tp.id, text: "the real words", idea_id: "i1" });
    r.svc.store.setSetting("vizierThreadId", "thr_viz");
    await ideas.file({ project_id: r.tp.id, text: "different text", idea_id: "i1" });
    const ob = obligations(r);
    expect(ob).toHaveLength(1);
    expect(ob[0]!.payload).toContain("the real words");
    expect(ob[0]!.payload).not.toContain("different text");
  });
});

describe("list and act", () => {
  async function filed(r: Rig, ideas: Ideas, text = "an idea", id = "i1") {
    const f = await ideas.file({ project_id: r.tp.id, text, idea_id: id });
    if (!f.ok) throw new Error(f.error);
    return f.task_id;
  }
  it("lists open ideas and drops them from the list once pursued, parked or dropped", async () => {
    const r = rig();
    const ideas = desk(r);
    const a = await filed(r, ideas, "a", "i1");
    const b = await filed(r, ideas, "b", "i2");
    const c = await filed(r, ideas, "c", "i3");
    expect((await ideas.list()).map((i) => i.words).sort()).toEqual(["a", "b", "c"]);
    expect(await ideas.act({ task_id: a, action: "pursue" })).toMatchObject({ ok: true, woke: true });
    expect(await ideas.act({ task_id: b, action: "park" })).toMatchObject({ ok: true, woke: false });
    expect(await ideas.act({ task_id: c, action: "drop" })).toMatchObject({ ok: true });
    expect(await ideas.list()).toEqual([]);
    expect(r.fake.tasks.find((t) => t.id === c)!.status).toBe("canceled");
    expect(r.fake.tasks.find((t) => t.id === a)!.status).not.toBe("done");
    expect(labelNames(r, r.fake.tasks.find((t) => t.id === a)!)).toEqual(["from-mk", "idea", "pursue"]);
  });
  it("acts once: a repeat posts no second comment and wakes no second time", async () => {
    const r = rig();
    const ideas = desk(r);
    const a = await filed(r, ideas);
    await ideas.act({ task_id: a, action: "pursue" });
    expect(await ideas.act({ task_id: a, action: "pursue" })).toMatchObject({ ok: true, replay: true });
    expect(r.fake.comments.filter((c) => c.taskId === a && c.body.includes("home-idea-action"))).toHaveLength(1);
    expect(obligations(r).filter((o) => o.op.startsWith("idea-pursue"))).toHaveLength(1);
  });
  it("refuses a card that is not an idea and an unknown card", async () => {
    const r = rig();
    const ideas = desk(r);
    const plain = r.fake.addTask(r.tp.id, { title: "plain" });
    expect(await ideas.act({ task_id: plain.id, action: "drop" })).toMatchObject({ ok: false, status: 409 });
    expect(await ideas.act({ task_id: "01HZZZZZZZZZZZZZZZZZZZZZZZ", action: "drop" })).toMatchObject({ ok: false, status: 404 });
    expect(plain.status).not.toBe("canceled");
  });
  it("leaves a project's own idea-labelled card alone: not listed, not actionable", async () => {
    const r = rig();
    const ideas = desk(r);
    const own = r.fake.addTask(r.tp.id, { title: "our own idea card", labelIds: [r.fake.addLabel(r.tp.id, "idea").id] });
    expect(await ideas.list()).toEqual([]);
    expect(await ideas.act({ task_id: own.id, action: "drop" })).toMatchObject({ ok: false, status: 409 });
    expect(own.status).not.toBe("canceled");
  });
  it("refuses when the idea labels came off between the first read and the write", async () => {
    const r = rig();
    const ideas = desk(r);
    const a = await filed(r, ideas);
    const t = r.fake.tasks.find((x) => x.id === a)!;
    const orig = r.fake.addComment.bind(r.fake);
    r.fake.addComment = (id, over) => { const c = orig(id, over); t.labelIds = []; return c; };
    expect(await ideas.act({ task_id: a, action: "drop" })).toMatchObject({ ok: false, status: 409 });
    expect(t.status).not.toBe("canceled");
  });
  it("files into a project whose labels differ only in case", async () => {
    const r = rig();
    const ideas = desk(r);
    r.fake.addLabel(r.tp.id, "Idea");
    expect(await ideas.file({ project_id: r.tp.id, text: "x", idea_id: "case1" })).toMatchObject({ ok: true });
    expect(await ideas.list()).toHaveLength(1);
  });
  it("reports a refused update as a failure and does not wake", async () => {
    const r = rig();
    const ideas = desk(r);
    const a = await filed(r, ideas);
    const before = obligations(r).length;
    r.fake.refuseUpdates = true;
    expect((await ideas.act({ task_id: a, action: "pursue" })).ok).toBe(false);
    expect(obligations(r)).toHaveLength(before);
  });
  it("does not list or drop a from-mk card Home did not file, nor an idea already in progress", async () => {
    const r = rig();
    const ideas = desk(r);
    const lab = (n: string) => r.fake.addLabel(r.tp.id, n).id;
    const fake = r.fake.addTask(r.tp.id, { title: "labelled by hand", labelIds: [lab("idea"), lab("from-mk")] });
    expect(await ideas.list()).toEqual([]);
    expect(await ideas.act({ task_id: fake.id, action: "drop" })).toMatchObject({ ok: false, status: 409 });
    const a = await filed(r, ideas);
    r.fake.tasks.find((t) => t.id === a)!.status = "in_progress";
    expect(await ideas.list()).toEqual([]);
    expect(await ideas.act({ task_id: a, action: "drop" })).toMatchObject({ ok: false, status: 409 });
    expect(r.fake.tasks.find((t) => t.id === a)!.status).toBe("in_progress");
  });
});

describe("digest", () => {
  it("shows while ideas are open and the week is not cleared, and comes back next week", async () => {
    const r = rig();
    const ideas = desk(r);
    expect(await ideas.digest("2026-W41")).toMatchObject({ show: false });
    await ideas.file({ project_id: r.tp.id, text: "a", idea_id: "i1" });
    expect(await ideas.digest("2026-W41")).toMatchObject({ show: true, cleared: false });
    ideas.clearDigest("2026-W41");
    expect(await ideas.digest("2026-W41")).toMatchObject({ show: false, cleared: true });
    expect(await ideas.digest("2026-W42")).toMatchObject({ show: true });
  });
});
