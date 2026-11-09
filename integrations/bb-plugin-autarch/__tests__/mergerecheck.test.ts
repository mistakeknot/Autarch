// Merge recheck (bead mk-g1t73): a merge pick whose PR is still open five minutes later brings the card back.
import { afterEach, describe, expect, it } from "vitest";
import { MergeRecheck, mergePrOf, RECHECK_AFTER_MS } from "../mergerecheck.js";
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
    ];
    for (const [label, instruction] of cases) {
      expect(mergePrOf(body({ options: [{ id: "a", label, kind: "instruction", instruction }] }), "a", null)?.url, label).toBe(URL130);
    }
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
    r.advance(RECHECK_AFTER_MS + 1000);
    const calls: string[] = [];
    await recheck(r, () => "OPEN", calls).pass();
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
  return new MergeRecheck({
    db: r.svc.store.db,
    ghView: async (u) => {
      calls.push(u);
      return { state: state(), mergedAt: null };
    },
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

  it("brings the card back, once, when the PR is still open after five minutes", async () => {
    const { r, t, g1 } = await pickedMerge();
    r.advance(RECHECK_AFTER_MS + 1000);
    const calls: string[] = [];
    const m = recheck(r, () => "OPEN", calls);
    await m.pass();
    expect(calls).toEqual([URL130]);
    expect(r.gens(t.id)).toHaveLength(2);
    expect(r.gens(t.id)[1]?.supersedes).toBe(g1.id);
    expect(r.svc.store.pick(g1.id)).toMatchObject({ by: "mk" }); // mk's pick stays on the first generation
    await m.pass();
    expect(calls).toHaveLength(1); // not asked again
    expect(r.gens(t.id)).toHaveLength(2);
  });

  it("never rechecks a pick the vizier made", async () => {
    const r = rig();
    r.enableDelegation();
    const { t, g1 } = await opened(r, { ask: mergeAsk() });
    expect(r.vizierPick(g1.id)).toMatchObject({ ok: true });
    await pollN(r, 3);
    r.advance(RECHECK_AFTER_MS + 1000);
    const calls: string[] = [];
    await recheck(r, () => "OPEN", calls).pass();
    expect(calls).toEqual([]);
    expect(r.gens(t.id)).toHaveLength(1);
  });

  it("leaves the card alone when the PR merged or was closed", async () => {
    for (const state of ["MERGED", "CLOSED"]) {
      const { r, t } = await pickedMerge();
      r.advance(RECHECK_AFTER_MS + 1000);
      await recheck(r, () => state, []).pass();
      expect(r.gens(t.id)).toHaveLength(1);
    }
  });

  it("does not check a pick that was not a merge", async () => {
    const { r, t } = await pickedMerge("b");
    r.advance(RECHECK_AFTER_MS + 1000);
    const calls: string[] = [];
    await recheck(r, () => "OPEN", calls).pass();
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
      clock: () => Date.parse(r.svc.time()),
      bringBack: async (taskId, decisionId) => r.dele.bringBack(taskId, decisionId, null, {}).ok,
    });
    await m.pass();
    expect(m.stats.errors).toBe(1);
    expect(r.gens(t.id)).toHaveLength(1);
    fail = false;
    await m.pass();
    expect(r.gens(t.id)).toHaveLength(2);
  });
});
