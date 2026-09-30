import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { clipLabel, FEED_BUDGET, renderFeed, renderLine, type Feed, type FeedLine } from "../feed.js";
import type { Service } from "../service.js";
import { ask, makeEnv, OPTIONS, type Env } from "./service-helpers.js";

const line = (over: Partial<FeedLine> = {}): FeedLine => ({
  decision: "dec-1",
  label: "Collapse per day",
  picked_at: "2026-09-26T14:03:00.000Z",
  by: "mk",
  project: "Autarch",
  status: "effective",
  ...over,
});

describe("clipLabel", () => {
  it("strips newlines and control characters", () => {
    expect(clipLabel("a\nb\r\tc\u0000d\u007fe")).toBe("abcde");
  });
  it("clips to 80 UTF-16 code units without splitting a surrogate pair", () => {
    expect(clipLabel("x".repeat(200))).toHaveLength(80);
    const s = "\u{1F600}".repeat(80);
    const c = clipLabel(s);
    expect(c.length).toBe(80);
    expect(c.isWellFormed()).toBe(true);
    const odd = "a" + "\u{1F600}".repeat(80);
    const c2 = clipLabel(odd);
    expect(c2.isWellFormed()).toBe(true);
    expect(c2.length).toBe(79);
  });
});

describe("renderLine", () => {
  it("renders effective, vizier, supersedes and void lines", () => {
    expect(renderLine(line())).toBe('ruled 2026-09-26T14:03Z "Collapse per day" (dec-1)');
    expect(renderLine(line({ by: "vizier" }))).toBe('ruled 2026-09-26T14:03Z "Collapse per day" (dec-1) [vizier]');
    expect(renderLine(line({ supersedes: "dec-0" }))).toBe('ruled 2026-09-26T14:03Z "Collapse per day" (dec-1), supersedes dec-0');
    expect(renderLine(line({ by: "vizier", supersedes: "dec-0" }))).toBe('ruled 2026-09-26T14:03Z "Collapse per day" (dec-1) [vizier], supersedes dec-0');
    expect(renderLine(line({ status: "void", tip: "dec-2" }))).toBe("void dec-1: overridden, awaiting mk (dec-2)");
  });
});

describe("renderFeed budget", () => {
  const lines = (n: number, prefix: string): FeedLine[] =>
    Array.from({ length: n }, (_, i) => line({ decision: `${prefix}-${i}`, label: "\u{1F600}".repeat(40) }));

  it("is at most 4096 UTF-16 code units", () => {
    const f: Feed = { own: lines(10, "own"), project: lines(10, "prj") };
    for (const l of [...f.own, ...f.project]) l.decision += "-" + "z".repeat(110);
    expect(renderFeed(f).length).toBeLessThanOrEqual(FEED_BUDGET);
  });

  it("drops oldest project lines first, own lines only after every project line", () => {
    const f: Feed = { own: lines(3, "own"), project: lines(3, "prj") };
    const full = renderFeed(f, 100_000);
    for (const id of ["own-0", "own-1", "own-2", "prj-0", "prj-1", "prj-2"]) expect(full).toContain(id);
    const oneLine = renderLine(f.own[0]!).length + 1;
    const head = renderFeed({ own: [], project: [] }, 100_000).length;
    // room for the reserved text, three own lines and one project line
    const t1 = renderFeed(f, head + 4 * oneLine);
    expect(t1).toContain("own-2");
    expect(t1).toContain("prj-0");
    expect(t1).not.toContain("prj-1");
    // room for two own lines only
    const t2 = renderFeed(f, head + 2 * oneLine);
    expect(t2).toContain("own-1");
    expect(t2).not.toContain("own-2");
    expect(t2).not.toContain("prj-0");
    expect(t2.length).toBeLessThanOrEqual(head + 2 * oneLine);
  });

  it("reserves the header and filing note even with no lines", () => {
    const t = renderFeed({ own: [], project: [] });
    expect(t.length).toBeGreaterThan(0);
    expect(t.length).toBeLessThan(400);
  });
});

describe("feed queries", () => {
  let env: Env;
  let svc: Service;
  beforeEach(() => {
    env = makeEnv(["Autarch", "Other"], "d".repeat(120));
    svc = env.open();
  });
  afterEach(() => env.cleanup());

  const fileAndPick = async (n: number, over: Record<string, unknown> = {}, label = "day") => {
    const r = await svc.file(ask(env, { question: `q${n}?`, subject: `s${n}`, ...over }), {});
    if (!r.ok) throw new Error(r.error);
    const rev = (svc.store.decision(r.decision_id) as { revision: string }).revision;
    const p = svc.pick(r.decision_id, label, rev, `pk-${n}`, "mk", "home");
    if (!p.ok) throw new Error(p.error);
    return r.decision_id;
  };

  it("own answers survive when project lines fill the budget", async () => {
    const longLabel = "\u{1F600}".repeat(80);
    const opts = [{ id: "day", label: longLabel, kind: "ruling-only" }, { id: "b", label: "b", kind: "ruling-only" }];
    const own: string[] = [];
    for (let i = 0; i < 10; i++) {
      env.clock.t += 1000;
      own.push(await fileAndPick(i, { options: opts }));
    }
    for (let i = 10; i < 20; i++) {
      env.clock.t += 1000;
      await fileAndPick(i, { options: opts, thread: "thr-other" });
    }
    const f = svc.feed("Autarch", "thr-a");
    expect(f.own).toHaveLength(10);
    expect(f.project).toHaveLength(10);
    expect(f.text.length).toBeLessThanOrEqual(FEED_BUDGET);
    expect(f.text.isWellFormed()).toBe(true);
    for (const id of own.slice(-3)) expect(f.text).toContain(id);
    const projectIdsShown = f.project.filter((l) => f.text.includes(l.decision));
    expect(projectIdsShown.length).toBeLessThan(10);
    // newest project lines are the ones kept
    if (projectIdsShown.length > 0) expect(projectIdsShown[0]).toBe(f.project[0]);
  });

  it("own is limited to 30 days and 10 lines, project to 14 days and 10 lines, own excluded from project", async () => {
    const old = await fileAndPick(1);
    env.clock.t += 20 * 86_400_000;
    const mid = await fileAndPick(2, { thread: "thr-other" });
    env.clock.t += 1000;
    const mine = await fileAndPick(3);
    const f = svc.feed("Autarch", "thr-a");
    expect(f.own.map((l) => l.decision)).toEqual([mine, old]);
    expect(f.project.map((l) => l.decision)).toEqual([mid]);
    env.clock.t += 20 * 86_400_000; // old is now 40 days, mid 20 days
    const g = svc.feed("Autarch", "thr-a");
    expect(g.own.map((l) => l.decision)).toEqual([mine]);
    expect(g.project).toEqual([]);
  });

  it("same-timestamp lines order by decision id", async () => {
    const a = await fileAndPick(1);
    const b = await fileAndPick(2);
    const c = await fileAndPick(3);
    const f = svc.feed("Autarch", "thr-a");
    expect(f.own.map((l) => l.decision)).toEqual([a, b, c].sort());
  });

  it("a thread never sees another thread's own lines, and no question or instruction appears", async () => {
    await fileAndPick(1, { question: "SECRET-QUESTION?" }, "project");
    const other = svc.feed("Autarch", "thr-b");
    expect(other.own).toEqual([]);
    const f = svc.feed("Autarch", "thr-a");
    const blob = JSON.stringify(f);
    expect(blob).not.toContain("SECRET-QUESTION");
    expect(blob).not.toContain(OPTIONS[0]!.instruction);
    expect(f.text).not.toContain("SECRET-QUESTION");
    expect(f.text).not.toContain(OPTIONS[0]!.instruction);
    expect(f.text).toContain("Collapse per project");
  });

  it("marks delegated rulings [vizier]", async () => {
    const r = await svc.file(ask(env), {});
    if (!r.ok) throw new Error(r.error);
    const rev = (svc.store.decision(r.decision_id) as { revision: string }).revision;
    svc.pick(r.decision_id, "day", rev, "pk", "vizier", "cli", "reason");
    expect(svc.feed("Autarch", "thr-a").text).toContain("[vizier]");
  });
});
