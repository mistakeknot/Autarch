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
