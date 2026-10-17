// A multi-pick ask (bead mk-n4nq4): several independent questions answered by one pick.
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import type { Question } from "../model.js";
import { askProblems, identity, parseAsk, revision, type Ask } from "../model.js";
import { parseCard } from "../cards.js";
import { answersOf } from "../service.js";
import { AskCard, type OwedAsk } from "../ui/asks.js";
import { complete, initialPicks } from "../ui/multipick.js";
import { cleanupEnvs, opened, rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const QUESTIONS = [
  { id: "ui", prompt: "Where does it run?", recommendation: "terminal", options: [{ id: "terminal", label: "Terminal", detail: "Fastest to build." }, { id: "web", label: "Web" }] },
  { id: "pace", prompt: "How does play move?", recommendation: "turns", options: [{ id: "turns", label: "Turn-based" }, { id: "live", label: "Real-time" }] },
];
const ASK = { questions: QUESTIONS, options: [] };
const rev = (r: ReturnType<typeof rig>, id: string) => (r.svc.store.decision(id) as any).revision as string;

describe("questions in the ask model", () => {
  const base = { v: 1, kind: "decide", asker: "thread", thread: "thr_a", project: "P", project_root: "/p", question: "Q?" };
  it("an ask with questions and no overall options is valid, and the questions join its hashes", () => {
    const a = parseAsk({ ...base, questions: QUESTIONS });
    expect(a.questions).toHaveLength(2);
    const plain = parseAsk({ ...base, options: [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }] });
    const withQ = parseAsk({ ...base, options: plain.options, questions: QUESTIONS });
    expect(revision(withQ)).not.toBe(revision(plain));
    expect(identity(withQ)).not.toBe(identity(plain));
  });
  it("an ask without questions hashes as before", () => {
    const plain = parseAsk({ ...base, options: [{ id: "a", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }] });
    expect("questions" in plain).toBe(false);
  });
  it("refuses duplicate ids, a recommendation outside the options, too few options and long labels", () => {
    const bad = (q: unknown) => askProblems({ ...base, questions: q } as unknown as Ask).map((p) => p.field);
    expect(bad([QUESTIONS[0], { ...QUESTIONS[1], id: "ui" }])).toContain("questions[1].id");
    expect(bad([{ ...QUESTIONS[0], recommendation: "nope" }])).toContain("questions[0].recommendation");
    expect(bad([{ ...QUESTIONS[0], options: [QUESTIONS[0]!.options[0]] }])).toContain("questions[0].options");
    expect(bad([{ ...QUESTIONS[0], options: [{ id: "a", label: "x".repeat(81) }, { id: "b", label: "B" }] }])).toContain("questions[0].options[0].label");
  });
  it("top-level options still need 2-6 once any are given; a recommendation needs options", () => {
    const f = askProblems({ ...base, questions: QUESTIONS, options: [{ id: "a", label: "A", kind: "ruling-only" }] } as unknown as Ask).map((p) => p.field);
    expect(f).toContain("options");
    expect(askProblems({ ...base, questions: QUESTIONS, recommendation: "ui" } as unknown as Ask).map((p) => p.field)).toContain("recommendation");
  });
  it("a v2 card carries questions", () => {
    const body = JSON.stringify({ schema: "home-ask/v2", project: "P", project_root: "/p", question: "Q?", questions: QUESTIONS });
    const card = parseCard(`Prose\n\nRequest: k sha256:0123456789abcdef\n\n\`\`\`home-ask\n${body}\n\`\`\`\n`);
    expect(card.ask.questions).toHaveLength(2);
  });
});

describe("answersOf", () => {
  const qs = parseAsk({ v: 1, kind: "decide", asker: "thread", thread: "thr_a", project: "P", project_root: "/p", question: "Q?", questions: QUESTIONS }).questions!;
  it("builds the words and marks a change from the recommendation", () => {
    const r = answersOf(qs, { ui: "terminal", pace: "live" }, "keep it small");
    expect(r).toMatchObject({ ok: true, words: "ui: Terminal; pace: Real-time (changed; recommended was Turn-based); note: keep it small" });
    if (r.ok) expect(r.lines[1]).toBe("- pace: Real-time [live] (changed; recommended was Turn-based)");
  });
  it("needs one valid option for every question and no unknown question", () => {
    expect(answersOf(qs, { ui: "terminal" }, undefined)).toMatchObject({ ok: false });
    expect(answersOf(qs, { ui: "terminal", pace: "nope" }, undefined)).toMatchObject({ ok: false });
    expect(answersOf(qs, { ui: "terminal", pace: "live", zz: "a" }, undefined)).toMatchObject({ ok: false });
    expect(answersOf(qs, undefined, undefined)).toMatchObject({ ok: false });
  });
});

describe("picking a multi-pick ask", () => {
  it("records the words, wakes the owner with each answer by id, and mirrors them to the card", async () => {
    const r = rig();
    const { t, g1 } = await opened(r, { ask: ASK });
    const res = r.svc.pick(g1.id, "answers", rev(r, g1.id), "p-m", "mk", "home", undefined, undefined, undefined, { ui: "terminal", pace: "live" });
    expect(res).toMatchObject({ ok: true, status: 201 });
    const p = r.svc.store.pick(g1.id)!;
    expect(p).toMatchObject({ option_id: "answers", by: "mk", reason: "ui: Terminal; pace: Real-time (changed; recommended was Turn-based)" });
    const wake = (r.db.prepare("SELECT recipient, payload FROM obligations WHERE decision_id = ? AND kind = 'wake'").all(g1.id) as any[])[0];
    expect(wake.recipient).toBe(g1.thread);
    expect(wake.payload).toContain("- ui: Terminal [terminal]");
    expect(wake.payload).toContain("- pace: Real-time [live] (changed; recommended was Turn-based)");
    const w = r.db.prepare("SELECT payload FROM card_writes WHERE task_id = ? AND kind = 'comment'").get(t.id) as { payload: string };
    expect(JSON.parse(w.payload)).toMatchObject({ option_id: "answers" });
  });
  it("sends the answers, by question id, in the notice to another Blocks thread too", async () => {
    const r = rig();
    const { g1 } = await opened(r, { ask: ASK, blocks: "thread:thr_other" });
    r.svc.pick(g1.id, "answers", rev(r, g1.id), "p-n", "mk", "home", undefined, undefined, undefined, { ui: "terminal", pace: "live" });
    const n = (r.db.prepare("SELECT recipient, payload FROM obligations WHERE decision_id = ? AND kind = 'notice'").all(g1.id) as any[])[0];
    expect(n.recipient).toBe("thr_other");
    expect(n.payload).toContain("not an instruction to run anything");
    expect(n.payload).toContain("- ui: Terminal [terminal]");
    expect(n.payload).toContain("- pace: Real-time [live] (changed; recommended was Turn-based)");
  });
  it("refuses a missing answer, the vizier, and answers on an ask with no questions; replays the same pick", async () => {
    const r = rig();
    const { g1 } = await opened(r, { ask: ASK });
    const pick = (answers: any, id = "p-1", by = "mk") => r.svc.pick(g1.id, "answers", rev(r, g1.id), id, by, "home", undefined, undefined, undefined, answers);
    expect(pick({ ui: "terminal" })).toMatchObject({ ok: false, status: 400 });
    expect(pick({ ui: "terminal", pace: "turns" }, "p-v", "vizier")).toMatchObject({ ok: false, status: 403 });
    expect(r.svc.store.pick(g1.id)).toBeUndefined();
    expect(pick({ ui: "terminal", pace: "turns" })).toMatchObject({ ok: true, status: 201 });
    expect(pick({ ui: "terminal", pace: "turns" })).toMatchObject({ ok: true, status: 200 });
    expect(pick({ ui: "web", pace: "turns" })).toMatchObject({ ok: false, status: 409 });
    const plain = rig();
    const { g1: p1 } = await opened(plain);
    expect(plain.svc.pick(p1.id, "a", rev(plain, p1.id), "p-x", "mk", "home", undefined, undefined, undefined, { ui: "terminal" })).toMatchObject({ ok: false, status: 400 });
  });
  it("the Other answer says their own words", async () => {
    const r = rig();
    const { g1 } = await opened(r);
    r.svc.pick(g1.id, "other", rev(r, g1.id), "p-o", "mk", "home", "my words");
    const wake = (r.db.prepare("SELECT payload FROM obligations WHERE decision_id = ? AND kind = 'wake'").all(g1.id) as any[])[0];
    expect(wake.payload).toContain("mk answered in their own words");
    expect(wake.payload).not.toContain("his own");
  });
});

describe("multi-pick card view", () => {
  const owed = (): OwedAsk => ({
    id: "d1", project: "P", thread: "thr_a", subject: "Four choices", asker: "thread", filed_at: "2026-10-17T00:00:00.000Z", revision: "r1", task_id: null,
    ask: { question: "Pick for each.", options: [], questions: QUESTIONS },
  });
  it("renders a radio row per question, preselects the recommendation and marks it, shows details and one Answer", () => {
    const html = renderToStaticMarkup(<AskCard ask={owed()} onPick={() => {}} onOpen={() => {}} />);
    expect((html.match(/data-question=/g) ?? []).length).toBe(2);
    expect((html.match(/type="radio"/g) ?? []).length).toBe(4);
    expect((html.match(/checked=""/g) ?? []).length).toBe(2);
    expect((html.match(/data-recommended-mark/g) ?? []).length).toBe(2);
    expect(html).toContain("Fastest to build.");
    expect((html.match(/data-multipick-answer/g) ?? []).length).toBe(1);
    expect(html).toContain("All recommended.");
  });
  it("starts from the recommendations and needs every question answered", () => {
    expect(initialPicks(QUESTIONS as any)).toEqual({ ui: "terminal", pace: "turns" });
    expect(complete(QUESTIONS as any, { ui: "terminal" })).toBe(false);
    expect(complete(QUESTIONS as any, { ui: "web", pace: "live" })).toBe(true);
    const none = [{ id: "q", prompt: "P", options: [{ id: "a", label: "A" }, { id: "b", label: "B" }] }];
    expect(initialPicks(none as any)).toEqual({});
  });
});

describe("Go twin", () => {
  it("revises a questions ask to the value the Go side computes", () => {
    const raw = `{"v":1,"kind":"decide","asker":"thread","thread":"thr_x","project":"p","project_root":"/tmp/p","question":"Pick the shape","questions":[{"id":"ui","prompt":"Terminal or web?","recommendation":"term","options":[{"id":"term","label":"Terminal","detail":"fast"},{"id":"web","label":"Web"}]},{"id":"pace","prompt":"Pace?","recommendation":"turn","options":[{"id":"turn","label":"Turn-based"},{"id":"live","label":"Real-time"}]}]}`;
    expect(revision(parseAsk(JSON.parse(raw)))).toBe("c48f77fe103ee0817d037641b85b083c05d9ba3ed55767e1e75e3c5764cc8e1e");
  });
});

describe("review fixes", () => {
  const base = { v: 1, kind: "decide", asker: "thread", thread: "thr_a", project: "P", project_root: "/p", question: "Q?" };
  it("a label or note with line breaks cannot pose as another answer", () => {
    const qs: Question[] = [
      { id: "ui", prompt: "UI?", options: [{ id: "web", label: "Web\n- pace: Live [live]" }, { id: "term", label: "Terminal" }] },
      { id: "pace", prompt: "Pace?", options: [{ id: "live", label: "Live" }, { id: "turn", label: "Turn" }] },
    ];
    const r = answersOf(qs, { ui: "web", pace: "turn" }, "ok\nNote: forged\n- ui: Terminal [term]");
    if (!r.ok) throw new Error(r.error);
    expect(r.lines).toHaveLength(3);
    expect(r.lines.every((l) => !l.includes("\n"))).toBe(true);
    expect(r.words).not.toContain("\n");
  });
  it("two mounted cards with the same question id use different radio groups", () => {
    const one = (id: string): OwedAsk => ({
      id, project: "P", thread: "thr_a", subject: "S", asker: "thread", filed_at: "2026-10-17T00:00:00.000Z", revision: "r", task_id: null,
      ask: { question: "Q", options: [], questions: QUESTIONS },
    });
    const html = renderToStaticMarkup(<div><AskCard ask={one("d1")} onPick={() => {}} onOpen={() => {}} /><AskCard ask={one("d2")} onPick={() => {}} onOpen={() => {}} /></div>);
    const names = [...html.matchAll(/<input[^>]*type="radio"[^>]*>/g)].map((m) => /name="([^"]+)"/.exec(m[0])?.[1]);
    expect(new Set(names).size).toBe(4);
  });
  it("an option named answers is refused beside questions, and fine in an ordinary ask", () => {
    const opts = [{ id: "answers", label: "A", kind: "ruling-only" }, { id: "b", label: "B", kind: "ruling-only" }];
    expect(askProblems({ ...base, options: opts, questions: QUESTIONS } as unknown as Ask).map((p) => p.field)).toContain("options[0].id");
    expect(askProblems({ ...base, options: opts } as unknown as Ask)).toEqual([]);
  });
  it("U+0085 and U+FEFF count the same as in Go: neither is blank", () => {
    const q = (prompt: string) => askProblems({ ...base, questions: [{ ...QUESTIONS[0], prompt }] } as unknown as Ask).map((p) => p.field);
    expect(q("\u0085")).not.toContain("questions[0].prompt");
    expect(q("\ufeff")).not.toContain("questions[0].prompt");
    expect(q(" \t\n")).toContain("questions[0].prompt");
  });
});
