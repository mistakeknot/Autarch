// Merge recheck (bead mk-g1t73): a merge pick whose PR is still open five minutes later brings the card back.
import { afterEach, describe, expect, it } from "vitest";
import { MergeRecheck, mergePrOf, RECHECK_AFTER_MS, RECHECK_SECOND_LOOK_MS } from "../mergerecheck.js";
import { TasksClient } from "../tasks.js";
import { cleanupEnvs, opened, pollN, rig } from "./card-rig.js";

afterEach(cleanupEnvs);

const URL130 = "https://github.com/mistakeknot/Undersketch/pull/130";
const body = (o: { question?: string; options?: unknown[] }) => JSON.stringify({ question: o.question ?? `PR: ${URL130}\nWill you merge it?`, options: o.options ?? [] });
const MERGE_A = { id: "a", label: "Merge PR 130", kind: "instruction", instruction: "mk merges PR 130 at head 5de17cbf in GitHub." };
const HOLD_B = { id: "b", label: "Hold and name what to change", kind: "instruction", instruction: "Do not merge. mk comments what to change." };

describe("which PR a pick is about", () => {
  it("finds the card's one PR URL for a merge option", () => {
    expect(mergePrOf(body({ options: [MERGE_A, HOLD_B] }), "a", null)).toEqual({ repo: "mistakeknot/Undersketch", number: 130, url: URL130 });
  });
  it("ignores negated, auto-merge-prohibiting and deferred options", () => {
    for (const [label, instruction] of [["Do not auto-merge", "Leave it."], ["Wait for CI before merge", "Hold."], ["Hold", "Don't merge yet."], ["No merge", "Close it."]]) {
      expect(mergePrOf(body({ options: [{ id: "a", label, kind: "instruction", instruction }] }), "a", null)).toBeNull();
    }
  });
  it("ignores a hold, a do-not-merge option and a pick that names no option", () => {
    expect(mergePrOf(body({ options: [MERGE_A, HOLD_B] }), "b", null)).toBeNull();
    expect(mergePrOf(body({ options: [MERGE_A] }), "z", null)).toBeNull();
  });
  it("leaves a card alone when it names two different PRs, but follows the option's own URL", () => {
    const q = `${URL130} and https://github.com/mistakeknot/Undersketch/pull/133`;
    expect(mergePrOf(body({ question: q, options: [MERGE_A] }), "a", null)).toBeNull();
    const own = { id: "a", label: "Merge", instruction: "mk merges https://github.com/mistakeknot/Undersketch/pull/133 in GitHub." };
    expect(mergePrOf(body({ question: q, options: [own] }), "a", null)?.number).toBe(133);
  });
  it("takes a merge approval's target when the text names no URL", () => {
    expect(mergePrOf(body({ question: "ok?", options: [{ id: "a", label: "Ship", instruction: "x" }] }), "a", "mistakeknot/Autarch#145")).toMatchObject({ repo: "mistakeknot/Autarch", number: 145 });
  });
  it("does not treat a non-merge option on a card with a PR URL as a merge", () => {
    expect(mergePrOf(body({ options: [{ id: "a", label: "Deploy it", instruction: "Deploy." }] }), "a", null)).toBeNull();
  });
});

describe("only a merge instruction counts as a merge pick (bead mk-q9m7q)", () => {
  // Fix-round options whose instruction mentions a later merge, as the coordinators write them.
  const NOT_MERGES: [string, string][] = [
    ["One more narrow fix", "Fix only the named finding, then a packaged re-review at the new head. A pass goes to the registrar to merge; any fail comes to mk."],
    ["Fix it, review round 6", "The executor fixes the findings and a cross-lab review runs. When it passes, mk merges PR 154."],
    ["Allow a 6th round", "One more fix round, then re-review. A pass goes to the registrar to merge."],
    ["Allow extra round 2", "Reuse a backup only when its manifest holds the entry. A pass goes to the registrar to merge; any fail stops and comes to mk."],
    ["One more round, then stop", "Run one more fix round; if it passes the vizier merges, otherwise the PR is held."],
    ["Fix it and merge", "Fix the finding."],
    ["Merge readiness review", "Review the PR and report findings."],
    ["Merge after one more fix round", "Fix the finding and re-review."],
    ["Review it", "Merge readiness needs a review."],
    ["Check it", "Merge readiness is checked first."],
    ["Check merge readiness", "Inspect the PR and report findings."],
    ["Check merge conflicts", "Inspect the PR and report findings."],
  ];
  it("does not treat a fix-round option that mentions a later merge as a merge pick", () => {
    for (const [label, instruction] of NOT_MERGES) {
      expect(mergePrOf(body({ options: [{ id: "a", label, kind: "instruction", instruction }] }), "a", null), label).toBeNull();
    }
  });
  it("still takes a merge pick whose label or opening sentence is the merge", () => {
    const cases: [string, string][] = [
      ["Merge it", "The vizier merges at e4e53a0b and runs custody part 2."],
      ["Merge PR 130", "mk merges PR 130 in GitHub. The coordinator confirms."],
      ["Yes", "mk merges PR 130 at head 5de17cbf in GitHub."],
      ["Approve", "Merge the PR with --match-head-commit, then report."],
      ["Yes", "Please merge PR 130."],
      ["Yes", "Squash merge PR 130."],
      ["Approve", "Squash and merge PR 130."],
      ["Approve and merge", "Go."],
    ];
    for (const [label, instruction] of cases) {
      expect(mergePrOf(body({ options: [{ id: "a", label, kind: "instruction", instruction }] }), "a", null)?.url, label).toBe(URL130);
    }
  });
  it("takes a merge pick written with an exclamation mark", () => {
    const o = (label: string) => body({ options: [{ id: "a", label, kind: "instruction", instruction: "Go." }] });
    expect(mergePrOf(o("Merge!"), "a", null)?.url).toBe(URL130);
  });
  it("does not take a non-merge label that ends in an exclamation mark", () => {
    for (const label of ["Fix it!", "Check merge readiness!", "Review it!", "Merge readiness!"]) {
      expect(mergePrOf(body({ options: [{ id: "a", label, kind: "instruction", instruction: "Go." }] }), "a", null), label).toBeNull();
    }
  });
  it("keeps a merge approval on a plain label", () => {
    const o = (label: string) => body({ options: [{ id: "a", label, kind: "instruction", instruction: "Go." }] });
    expect(mergePrOf(o("Ship it"), "a", "mistakeknot/Undersketch#130")?.url).toBe(URL130);
  });
  it("brings nothing back for a fix-round pick on a card that names a PR", async () => {
    const r = rig();
    const ask = {
      question: `PR: ${URL130}\nAllow another fix round?`,
      options: [
        { id: "a", label: "Allow a 6th round", kind: "instruction", instruction: "One more fix round and re-review. A pass goes to the registrar to merge.", reversible: true },
        { id: "b", label: "Hold the PR", kind: "instruction", instruction: "Nothing merges.", reversible: true },
      ],
    };
    const { t, g1 } = await opened(r, { ask });
    expect(r.mkPick(g1.id, "p1", "a")).toMatchObject({ ok: true });
    await pollN(r, 3);
    const calls: string[] = [];
    await twoLooks(r, recheck(r, () => "OPEN", calls));
    expect(calls).toEqual([]);
    expect(r.gens(t.id)).toHaveLength(1);
  });
});

const mergeAsk = (url = URL130) => ({
  question: `PR: ${url}\nWill you merge it?`,
  options: [
    { id: "a", label: "Merge PR 130", kind: "instruction", instruction: "mk merges PR 130 in GitHub. The coordinator confirms.", reversible: true },
    { id: "b", label: "Hold", kind: "instruction", instruction: "Do not merge.", reversible: true },
  ],
});

async function pickedMerge(option = "a") {
  const r = rig();
  const { t, g1 } = await opened(r, { ask: mergeAsk() });
  expect(r.mkPick(g1.id, "p1", option)).toMatchObject({ ok: true });
  await pollN(r, 3);
  return { r, t, g1 };
}

function recheck(r: any, state: () => string, calls: string[]) {
  const tasks = new TasksClient(r.fake, { now: () => r.env.clock.t });
  return new MergeRecheck({
    db: r.svc.store.db,
    ghView: async (u) => {
      calls.push(u);
      return { state: state(), mergedAt: null };
    },
    taskState: (id: string) => tasks.taskState(id),
    clock: () => Date.parse(r.svc.time()),
    bringBack: async (taskId, decisionId) => {
      const out = r.dele.bringBack(taskId, decisionId, "thr_a", {});
      return out.ok && !out.replay;
    },
  });
}

describe("merge recheck", () => {
  it("does nothing before five minutes have passed", async () => {
    const { r, t } = await pickedMerge();
    const calls: string[] = [];
    await recheck(r, () => "OPEN", calls).pass();
    expect(calls).toEqual([]);
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("brings the card back, once, when the PR is still open at both looks", async () => {
    const { r, t, g1 } = await pickedMerge();
    const calls: string[] = [];
    const m = recheck(r, () => "OPEN", calls);
    await twoLooks(r, m);
    expect(calls).toEqual([URL130, URL130]);
    expect(r.gens(t.id)).toHaveLength(2);
    expect(r.gens(t.id)[1]?.supersedes).toBe(g1.id);
    expect(r.svc.store.pick(g1.id)).toMatchObject({ by: "mk" }); // mk's pick stays on the first generation
    await m.pass();
    expect(calls).toHaveLength(2); // not asked again
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("is right to come back when a merge pick's PR is still open at both looks, and not when it merged first", async () => {
    // A merge that landed before the second look must not bring the card back (the race of bead mk-qepye).
    for (const [state, cards] of [["OPEN", 2], ["MERGED", 1]] as const) {
      const { r, t } = await pickedMerge();
      await twoLooks(r, recheck(r, () => state, []));
      expect(r.gens(t.id)).toHaveLength(cards);
    }
  });

  it("never rechecks a pick the vizier made", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r, { ask: mergeAsk() });
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await pollN(r, 3);
    const calls: string[] = [];
    await twoLooks(r, recheck(r, () => "OPEN", calls));
    expect(calls).toEqual([]);
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("leaves the card alone when the PR merged or was closed", async () => {
    for (const state of ["MERGED", "CLOSED"]) {
      const { r, t } = await pickedMerge();
      await twoLooks(r, recheck(r, () => state, []));
      expect(r.gens(t.id)).toHaveLength(1);
    }
  });

  it("does not check a pick that was not a merge", async () => {
    const { r, t } = await pickedMerge("b");
    const calls: string[] = [];
    await twoLooks(r, recheck(r, () => "OPEN", calls));
    expect(calls).toEqual([]);
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("keeps the pick for the next pass when gh fails, and does not reopen on an unknown state", async () => {
    const { r, t } = await pickedMerge();
    r.advance(RECHECK_AFTER_MS + 1000);
    let fail = true;
    const m = new MergeRecheck({
      db: r.svc.store.db,
      ghView: async () => {
        if (fail) throw new Error("gh down");
        return { state: "OPEN", mergedAt: null };
      },
      taskState: (id) => new TasksClient(r.fake, { now: () => r.env.clock.t }).taskState(id),
      clock: () => Date.parse(r.svc.time()),
      bringBack: async (taskId, decisionId) => r.dele.bringBack(taskId, decisionId, null, {}).ok,
    });
    await m.pass();
    expect(m.stats.errors).toBe(1);
    expect(r.gens(t.id)).toHaveLength(1);
    fail = false;
    await twoLooks(r, m);
    expect(r.gens(t.id)).toHaveLength(2);
  });
});

// Bead mk-6k39i. When the recheck fires, the pick's own write-back has already made the task done (or, with a thread
// attached, left it todo). It never cancels a task, so a canceled or deleted task was closed by a person or a thread
// after mk picked: the card is finished and the recheck must not reopen it. A done task stays a true positive, since
// that is how every picked card looks five minutes later.
describe("merge recheck leaves a card whose task is finished (bead mk-6k39i)", () => {
  it("brings a card back whose task the pick itself completed, and one a thread is still working", async () => {
    for (const threads of [[], [{ liveStatus: "working" }]]) {
      const r = rig();
      const { t, g1 } = await opened(r, { ask: mergeAsk() });
      r.fake.threads = threads.map((x) => ({ taskId: t.id, ...x }));
      expect(r.mkPick(g1.id, "p1", "a")).toMatchObject({ ok: true });
      await pollN(r, 3);
      expect(t.status).toBe(threads.length === 0 ? "done" : "todo");
      await twoLooks(r, recheck(r, () => "OPEN", []));
      expect(r.gens(t.id)).toHaveLength(2);
    }
  });

  it("leaves a card alone whose task was canceled after the pick (a duplicate, say)", async () => {
    const { r, t } = await pickedMerge();
    r.edit(t, { status: "canceled" });
    const m = recheck(r, () => "OPEN", []);
    await twoLooks(r, m);
    expect(r.gens(t.id)).toHaveLength(1);
    expect(t.status).toBe("canceled"); // not reopened
    await m.pass();
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("leaves a card alone whose task was deleted after the pick", async () => {
    const { r, t } = await pickedMerge();
    r.fake.tasks.splice(r.fake.tasks.findIndex((x) => x.id === t.id), 1);
    await twoLooks(r, recheck(r, () => "OPEN", []));
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("keeps the pick for the next pass when the task cannot be read", async () => {
    const { r, t } = await pickedMerge();
    r.advance(RECHECK_AFTER_MS + 1000);
    let fail = true;
    const m = new MergeRecheck({
      db: r.svc.store.db,
      ghView: async () => ({ state: "OPEN", mergedAt: null }),
      taskState: async (id: string) => {
        if (fail) throw new Error("tasks down");
        return new TasksClient(r.fake, { now: () => r.env.clock.t }).taskState(id);
      },
      clock: () => Date.parse(r.svc.time()),
      bringBack: async (taskId, decisionId) => r.dele.bringBack(taskId, decisionId, null, {}).ok,
    });
    await twoLooks(r, m);
    expect(m.stats.errors).toBe(1);
    expect(r.gens(t.id)).toHaveLength(1);
    fail = false;
    await m.pass();
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("the manual Bring back still reopens a canceled task (the Archive tab's deliberate rule)", async () => {
    const { r, t, g1 } = await pickedMerge();
    r.edit(t, { status: "canceled" });
    expect(r.dele.bringBack(t.id, g1.id, "thr_a", {})).toMatchObject({ ok: true });
    await pollN(r, 3);
    expect(t.status).toBe("todo");
    expect(r.gens(t.id)).toHaveLength(2);
  });
});

/** The five-minute look, then the ten-minute look. */
async function twoLooks(r: any, m: MergeRecheck) {
  toAge(r, RECHECK_AFTER_MS + 1000);
  await m.pass();
  toAge(r, RECHECK_SECOND_LOOK_MS + 1000);
  await m.pass();
}

// pollN leaves a few minutes on the clock after the pick, so the looks below are placed by the pick's own age.
function toAge(r: any, ageMs: number) {
  const at = (r.svc.store.db.prepare("SELECT MIN(picked_at) AS at FROM picks").get() as { at: string }).at;
  r.advance(Date.parse(at) + ageMs - Date.parse(r.svc.time()));
}

// Bead mk-qepye. A registrar merge can land a few minutes after the five-minute check (observed: pick 01:42:20, first
// check 01:47:40 with the PR still open, merge 01:48:05), and GitHub shows nothing in flight before it lands. So the first
// look only records that the PR was still open; a second look at about ten minutes brings the card back.
describe("merge recheck looks twice before it brings a card back (bead mk-qepye)", () => {
  it("does not bring the card back at the first look while the PR is still open", async () => {
    const { r, t } = await pickedMerge();
    toAge(r, RECHECK_AFTER_MS + 1000);
    const calls: string[] = [];
    const m = recheck(r, () => "OPEN", calls);
    await m.pass();
    expect(calls).toEqual([URL130]);
    expect(r.gens(t.id)).toHaveLength(1);
    expect(m.stats.resurfaced).toBe(0);
  });

  it("brings the card back at the second look, once, when the PR is open both times", async () => {
    const { r, t, g1 } = await pickedMerge();
    toAge(r, RECHECK_AFTER_MS + 1000);
    const calls: string[] = [];
    const m = recheck(r, () => "OPEN", calls);
    await m.pass();
    toAge(r, RECHECK_SECOND_LOOK_MS + 1000);
    await m.pass();
    expect(r.gens(t.id)).toHaveLength(2);
    expect(r.gens(t.id)[1]?.supersedes).toBe(g1.id);
    await m.pass();
    expect(calls).toHaveLength(2); // not asked a third time
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("does not look again before the ten-minute mark", async () => {
    const { r, t } = await pickedMerge();
    toAge(r, RECHECK_AFTER_MS + 1000);
    const calls: string[] = [];
    const m = recheck(r, () => "OPEN", calls);
    await m.pass();
    toAge(r, RECHECK_AFTER_MS + 3 * 60_000);
    await m.pass();
    expect(calls).toHaveLength(1);
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("settles the pick when the PR merged or closed between the looks (the observed race)", async () => {
    for (const later of ["MERGED", "CLOSED"]) {
      const { r, t } = await pickedMerge();
      toAge(r, RECHECK_AFTER_MS + 1000);
      let state = "OPEN";
      const calls: string[] = [];
      const m = recheck(r, () => state, calls);
      await m.pass();
      state = later;
      toAge(r, RECHECK_SECOND_LOOK_MS + 1000);
      await m.pass();
      await m.pass();
      expect(calls).toHaveLength(2);
      expect(r.gens(t.id)).toHaveLength(1);
    }
  });

  it("does not bring the card back when the first look found the PR already merged", async () => {
    const { r, t } = await pickedMerge();
    toAge(r, RECHECK_SECOND_LOOK_MS + 1000);
    const m = recheck(r, () => "MERGED", []);
    await m.pass();
    await m.pass();
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("a restart between the looks neither loses the pick nor brings the card back early", async () => {
    const { r, t } = await pickedMerge();
    toAge(r, RECHECK_AFTER_MS + 1000);
    await recheck(r, () => "OPEN", []).pass(); // first look, then the service restarts: a fresh MergeRecheck
    toAge(r, RECHECK_AFTER_MS + 61_000);
    const calls: string[] = [];
    const m2 = recheck(r, () => "OPEN", calls);
    await m2.pass(); // its own first look, six minutes in
    expect(r.gens(t.id)).toHaveLength(1);
    toAge(r, RECHECK_SECOND_LOOK_MS + 1000);
    await m2.pass(); // ten minutes in: second look
    expect(calls).toHaveLength(2);
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("a restart after the ten-minute mark still takes two looks, and does not lose the pick", async () => {
    const { r, t } = await pickedMerge();
    toAge(r, RECHECK_SECOND_LOOK_MS + 30_000);
    const calls: string[] = [];
    const m = recheck(r, () => "OPEN", calls);
    await m.pass();
    expect(r.gens(t.id)).toHaveLength(1); // nothing was seen open before this run: record it, do not bring back
    r.advance(60_000);
    await m.pass();
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("keeps the two-hour bound: a pick older than two hours is never looked at", async () => {
    const { r, t } = await pickedMerge();
    toAge(r, 2 * 60 * 60_000 + 60_000);
    const calls: string[] = [];
    await recheck(r, () => "OPEN", calls).pass();
    expect(calls).toEqual([]);
    expect(r.gens(t.id)).toHaveLength(1);
  });
});
