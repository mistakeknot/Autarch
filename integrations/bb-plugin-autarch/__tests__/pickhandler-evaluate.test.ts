// mk-cqnab.1 PR3 stage 1: the pure four-check evaluator. Synthetic snapshots only; no I/O.
import { describe, expect, it } from "vitest";
import { evaluatePick, PICK_POLICY_VERSION, PICK_REASON_CODES, type CheckResult, type PickEvaluationInput } from "../pickhandler.js";
import type { Ask, AskOption } from "../model.js";
import type { OnPick } from "../onpick.js";

const T = "2026-01-01T00:00:10.000Z";
const FILED = "1".repeat(40);
const PR: OnPick["subject"] = { kind: "github-pr", repo: "example/repo", number: 7, head: FILED };
type Subject = "pr" | "none";

function declaration(subject: Subject, over: Partial<OnPick> = {}): OnPick {
  return { schema: 1, target_thread: "thr_target", reversible: true, subject: subject === "pr" ? PR : { kind: "none" }, ruling: { bead: null, under: [] }, ...over };
}
function ask(subject: Subject, over: Partial<Ask> = {}, a: Partial<AskOption> = {}): Ask {
  return {
    v: 1, kind: "decide", project: "Project", project_root: "/synthetic", asker: "thread", thread: "thr_asker", question: "Which colour?", recommendation: "a",
    options: [
      { id: "a", label: "Blue", kind: "instruction", instruction: "Use blue.", reversible: false, on_pick: declaration(subject), ...a },
      { id: "b", label: "Wait", kind: "ruling-only" },
    ],
    ...over,
  };
}
const comments = (result: "pass" | "fail" | "unknown") => ({
  result, reason: result === "pass" ? "no-owner-comments" : result === "fail" ? "owner-comment" : "comments-unavailable",
  observed_at: "2026-01-01T00:00:05.000Z", cutoff: null, comments: [], limitations: ["comment-edits-before-cutoff-not-detectable", "external-comment-read-race"] as const,
});
const head = (result: "pass" | "fail" | "unknown" | "not-applicable", sha: string | null = result === "pass" ? FILED : result === "fail" ? "2".repeat(40) : null) => ({
  result, reason: { pass: "head-unchanged", fail: "head-changed", unknown: "head-unavailable", "not-applicable": "subject-none" }[result],
  observed_at: "2026-01-01T00:00:06.000Z", head: sha, state: sha ? "OPEN" : null,
});

function base(subject: Subject = "pr"): PickEvaluationInput {
  return {
    policy_version: PICK_POLICY_VERSION,
    evaluated_at: T,
    decision: {
      id: "card-task-g1", source: "card", task_id: "task", generation: 1, revision: "rev-1", state: "open",
      request_registered: true, binding: "confirmed", asker: "thr_asker", ask: ask(subject),
      exclusions: { root_run: false, move: false, artifact: false },
    },
    pick: { pick_id: "pick-1", option_id: "a", revision: "rev-1", by: "mk", reason: null, image_count: 0, picked_at: "2026-01-01T00:00:00.000Z" },
    comments: comments("pass"),
    head: subject === "pr" ? head("pass") : head("not-applicable"),
    target: { result: "pass", reason: "target-live-in-scope", observed_at: "2026-01-01T00:00:07.000Z" },
    control: { mode: "shadow", epoch: 3, capture_epoch: 3, enabled_cells: [], owner_enablement: false },
  };
}
const selected = (i: PickEvaluationInput) => i.decision.ask!.options!.find(o => o.id === "a")!;

type R = "pass" | "fail" | "unknown";
const set: Record<"recommendation" | "no-extra-words" | "reversible", Record<R, (i: PickEvaluationInput) => void>> = {
  recommendation: {
    pass: () => {},
    fail: i => { i.decision.ask!.recommendation = "b"; },
    unknown: i => { Reflect.set(i.decision.ask!, "recommendation", 42); },
  },
  "no-extra-words": {
    pass: () => {},
    fail: i => { i.pick.reason = "Only once the checks are green"; },
    unknown: i => { i.comments = comments("unknown"); },
  },
  reversible: {
    pass: () => {},
    fail: i => { selected(i).on_pick = { ...selected(i).on_pick!, reversible: false }; },
    unknown: i => { i.decision.exclusions.root_run = null; },
  },
};
const setHead: Record<R, (i: PickEvaluationInput) => void> = {
  pass: () => {},
  fail: i => { i.head = head("fail"); },
  unknown: i => { i.head = head("unknown"); },
};
const RESULTS: R[] = ["pass", "fail", "unknown"];
const matrix = RESULTS.flatMap(c1 => RESULTS.flatMap(c2 => RESULTS.flatMap(c3 => RESULTS.map(c4 => ({ c1, c2, c3, c4 })))));

describe("evaluatePick four-check matrix", () => {
  it("covers all 81 pass/fail/unknown combinations", () => expect(matrix).toHaveLength(81));
  it.each(matrix)("PR subject: recommendation=$c1 no-extra-words=$c2 reversible=$c3 subject-head=$c4", ({ c1, c2, c3, c4 }) => {
    const i = base("pr");
    set.recommendation[c1](i); set["no-extra-words"][c2](i); set.reversible[c3](i); setHead[c4](i);
    const e = evaluatePick(i);
    expect([e.checks.recommendation.result, e.checks["no-extra-words"].result, e.checks.reversible.result, e.checks["subject-head"].result]).toEqual([c1, c2, c3, c4]);
    const eligible = [c1, c2, c3, c4].every(r => r === "pass");
    expect(e.outcome).toBe(eligible ? "automatic" : "fallback");
    expect(e.exclusions).toEqual([]);
    for (const check of Object.values(e.checks)) for (const reason of check.reasons) expect(PICK_REASON_CODES).toContain(reason);
  });
  const naMatrix = RESULTS.flatMap(c1 => RESULTS.flatMap(c2 => RESULTS.map(c3 => ({ c1, c2, c3 }))));
  it.each(naMatrix)("explicit none: recommendation=$c1 no-extra-words=$c2 reversible=$c3 subject-head=not-applicable", ({ c1, c2, c3 }) => {
    const i = base("none");
    set.recommendation[c1](i); set["no-extra-words"][c2](i); set.reversible[c3](i);
    const e = evaluatePick(i);
    expect(e.checks["subject-head"]).toEqual({ result: "not-applicable", reasons: ["subject-none"] });
    expect(e.outcome).toBe(c1 === "pass" && c2 === "pass" && c3 === "pass" ? "automatic" : "fallback");
  });
  it("not-applicable is never produced for a PR subject and never reads as head unchanged", () => {
    const i = base("pr"); i.head = head("not-applicable");
    const e = evaluatePick(i);
    expect(e.checks["subject-head"]).toEqual({ result: "unknown", reasons: ["head-evidence-mismatch"] });
    const none = evaluatePick(base("none"));
    expect(none.checks["subject-head"].reasons).not.toContain("head-unchanged");
  });
  it("valid none with PR-shaped head evidence is unknown, not a pass", () => {
    const i = base("none"); i.head = head("pass");
    expect(evaluatePick(i).checks["subject-head"]).toEqual({ result: "unknown", reasons: ["head-evidence-mismatch"] });
  });
  it("none contradicted by artifact evidence fails; unknown artifact evidence is unknown", () => {
    const i = base("none"); i.decision.exclusions.artifact = true;
    expect(evaluatePick(i).checks["subject-head"]).toEqual({ result: "fail", reasons: ["none-contradicted-by-artifact"] });
    const j = base("none"); j.decision.exclusions.artifact = null;
    expect(evaluatePick(j).checks["subject-head"]).toEqual({ result: "unknown", reasons: ["artifact-evidence-unavailable"] });
  });
  it("missing or mismatched head evidence is unknown", () => {
    const i = base("pr"); i.head = null;
    expect(evaluatePick(i).checks["subject-head"]).toEqual({ result: "unknown", reasons: ["head-evidence-missing"] });
    const j = base("pr"); j.head = head("pass", "3".repeat(40));
    expect(evaluatePick(j).checks["subject-head"]).toEqual({ result: "unknown", reasons: ["head-evidence-mismatch"] });
    const k = base("pr"); k.head = { ...head("fail"), reason: "pr-not-open", state: "MERGED", head: FILED };
    expect(evaluatePick(k).checks["subject-head"]).toEqual({ result: "fail", reasons: ["pr-not-open"] });
  });
});

describe("evaluatePick recommendation check", () => {
  it.each([
    ["absent", undefined, "no-recommendation"],
    ["empty", "", "no-recommendation"],
    ["not an option of this generation", "z", "recommendation-not-an-option"],
  ])("recommendation %s fails", (_n, recommendation, reason) => {
    const i = base(); i.decision.ask!.recommendation = recommendation;
    expect(evaluatePick(i).checks.recommendation).toEqual({ result: "fail", reasons: [reason] });
  });
  it("labels and order are irrelevant; only the exact generation option id counts", () => {
    const i = base(); i.decision.ask!.options!.reverse(); i.decision.ask!.options!.find(o => o.id === "b")!.label = "Blue";
    expect(evaluatePick(i).checks.recommendation.result).toBe("pass");
    const j = base(); j.decision.ask!.recommendation = "b"; j.decision.ask!.options!.find(o => o.id === "b")!.label = "Blue";
    expect(evaluatePick(j).checks.recommendation).toEqual({ result: "fail", reasons: ["not-recommended"] });
  });
  it("a nonrecommended option fails", () => {
    const i = base(); i.pick.option_id = "b";
    expect(evaluatePick(i).checks.recommendation).toEqual({ result: "fail", reasons: ["not-recommended"] });
  });
  it("a pick on another revision of the generation fails", () => {
    const i = base(); i.pick.revision = "rev-0";
    expect(evaluatePick(i).checks.recommendation).toEqual({ result: "fail", reasons: ["revision-mismatch"] });
  });
  it.each(["other", "mk-own-words", "mk-own-words-2", "answers"])("virtual option %s never passes, even when named as recommendation", id => {
    const i = base(); i.decision.ask!.recommendation = id; i.pick.option_id = id; i.pick.reason = "words";
    const e = evaluatePick(i);
    expect(e.checks.recommendation).toEqual({ result: "fail", reasons: ["virtual-option"] });
    expect(e.checks["no-extra-words"].result).toBe("fail");
    expect(e.checks["no-extra-words"].reasons).toContain(id === "answers" ? "answers" : "own-words");
    expect(e.outcome).toBe("fallback");
  });
  it("a real option whose id collides with the own-words id is an ordinary option", () => {
    const i = base(); const o = selected(i); o.id = "other"; i.decision.ask!.recommendation = "other"; i.pick.option_id = "other";
    const e = evaluatePick(i);
    expect(e.checks.recommendation.result).toBe("pass");
    expect(e.checks["no-extra-words"].result).toBe("pass");
    expect(e.outcome).toBe("automatic");
  });
  it("an unreadable stored ask is unknown everywhere, never a pass", () => {
    const i = base(); i.decision.ask = null;
    const e = evaluatePick(i);
    expect(Object.values(e.checks).map(c => c.result)).toEqual(["unknown", "unknown", "unknown", "unknown"]);
    expect(e.checks.recommendation.reasons).toEqual(["ask-unreadable"]);
    expect(e.outcome).toBe("fallback");
  });
});

describe("evaluatePick no extra words or conditions", () => {
  it.each(["", " ", "\t", "\r\n", " \t\r\n "])("ASCII space/tab/CR/LF-only reason %j has no words", reason => {
    const i = base(); i.pick.reason = reason;
    expect(evaluatePick(i).checks["no-extra-words"].result).toBe("pass");
  });
  it.each(["\u00a0", "\u2003", "\u3000", "\u000b", "\u000c", "\u0085", "\u200b", "\u2028", "\ufeff", "x", "非"])("other characters count as words: %j", reason => {
    const i = base(); i.pick.reason = reason;
    expect(evaluatePick(i).checks["no-extra-words"]).toEqual({ result: "fail", reasons: ["ordinary-reason"] });
  });
  it("evaluation preserves the stored reason and never mutates its input", () => {
    const i = base(); i.pick.reason = " \u00a0 words\t";
    const before = structuredClone(i);
    evaluatePick(i);
    expect(i).toEqual(before);
    expect(evaluatePick(i)).toEqual(evaluatePick(structuredClone(before)));
  });
  it("images fail", () => {
    const i = base(); i.pick.image_count = 1;
    expect(evaluatePick(i).checks["no-extra-words"]).toEqual({ result: "fail", reasons: ["images"] });
  });
  it("unknown image evidence is unknown", () => {
    const i = base(); i.pick.image_count = null;
    expect(evaluatePick(i).checks["no-extra-words"]).toEqual({ result: "unknown", reasons: ["images-unavailable"] });
  });
  it("owner comment and Home note evidence fail; unavailable comments are unknown", () => {
    const i = base(); i.comments = { ...comments("fail"), reason: "home-note-fence" };
    expect(evaluatePick(i).checks["no-extra-words"]).toEqual({ result: "fail", reasons: ["home-note-fence"] });
    const j = base(); j.comments = comments("unknown");
    expect(evaluatePick(j).checks["no-extra-words"]).toEqual({ result: "unknown", reasons: ["comments-unavailable"] });
  });
  it("a definite fail is reported with an unknown alongside it", () => {
    const i = base(); i.pick.reason = "words"; i.comments = comments("unknown");
    expect(evaluatePick(i).checks["no-extra-words"]).toEqual({ result: "fail", reasons: ["ordinary-reason", "comments-unavailable"] });
  });
  it("a selected option without on_pick declares no unconditionality", () => {
    const i = base(); delete selected(i).on_pick;
    const e = evaluatePick(i);
    expect(e.checks["no-extra-words"]).toEqual({ result: "fail", reasons: ["no-unconditional-declaration"] });
    expect(e.checks.reversible).toEqual({ result: "fail", reasons: ["no-declaration"] });
    expect(e.checks["subject-head"]).toEqual({ result: "unknown", reasons: ["no-declaration"] });
    expect(e.cell).toBeNull();
    expect(e.outcome).toBe("fallback");
  });
});

describe("evaluatePick reversible and privileged policy", () => {
  it("on_pick.reversible decides; option-level delegation reversibility is irrelevant", () => {
    const i = base(); selected(i).reversible = false;
    expect(evaluatePick(i).checks.reversible.result).toBe("pass");
    const j = base(); selected(j).reversible = true; selected(j).on_pick = { ...selected(j).on_pick!, reversible: false };
    expect(evaluatePick(j).checks.reversible).toEqual({ result: "fail", reasons: ["declared-irreversible"] });
  });
  it.each([
    ["approval member", (i: PickEvaluationInput) => { selected(i).approval = { kind: "merge", target: "t", identity: "x" }; }, "approval-option"],
    ["root-run card", (i: PickEvaluationInput) => { i.decision.exclusions.root_run = true; }, "root-run-card"],
    ["home-move card", (i: PickEvaluationInput) => { i.decision.exclusions.move = true; }, "move-card"],
    ["needs-context option", (i: PickEvaluationInput) => { selected(i).kind = "needs-context"; }, "option-kind-excluded"],
    ["fullwidth label", (i: PickEvaluationInput) => { selected(i).label = "Ｍerge it"; }, "privileged-text"],
    ["dash-folded force-push", (i: PickEvaluationInput) => { selected(i).instruction = "force‐push the branch"; }, "privileged-text"],
    ["underscore force_push", (i: PickEvaluationInput) => { selected(i).instruction = "FORCE_PUSH"; }, "privileged-text"],
    ["deploy instruction", (i: PickEvaluationInput) => { selected(i).instruction = "Then Deploy."; }, "privileged-text"],
    ["admin instruction", (i: PickEvaluationInput) => { selected(i).instruction = "ask an administrator"; }, "privileged-text"],
    ["root substring", (i: PickEvaluationInput) => { selected(i).label = "Uproot"; }, "privileged-text"],
    ["ruling-only notification", (i: PickEvaluationInput) => { const o = selected(i); o.kind = "ruling-only"; delete o.instruction; o.on_pick = { ...o.on_pick!, notification: "Please publish notes" }; }, "privileged-text"],
  ])("%s fails reversible", (_n, mutate, reason) => {
    const i = base(); mutate(i);
    const e = evaluatePick(i);
    expect(e.checks.reversible.result).toBe("fail");
    expect(e.checks.reversible.reasons).toContain(reason);
    expect(e.outcome).toBe("fallback");
  });
  it("screening never rewrites the filed text", () => {
    const i = base(); selected(i).instruction = "Ｕse ｂlue‐now";
    const e = evaluatePick(i);
    expect(selected(i).instruction).toBe("Ｕse ｂlue‐now");
    expect(e.checks.reversible.result).toBe("pass");
  });
  it.each([["root_run"], ["move"]] as const)("unknown %s exclusion is unknown", key => {
    const i = base(); i.decision.exclusions[key] = null;
    expect(evaluatePick(i).checks.reversible).toEqual({ result: "unknown", reasons: ["card-exclusions-unavailable"] });
  });
  it("ruling-only option without notification is a malformed declaration", () => {
    const i = base(); const o = selected(i); o.kind = "ruling-only"; delete o.instruction;
    expect(evaluatePick(i).checks.reversible).toEqual({ result: "fail", reasons: ["relay-text-missing"] });
  });
});

describe("evaluatePick supplementary exclusions", () => {
  it.each([
    ["home source", (i: PickEvaluationInput) => { i.decision.source = "home"; }, "not-card-source"],
    ["vizier ruling", (i: PickEvaluationInput) => { i.pick.by = "vizier"; }, "not-owner-pick"],
    ["evidence auto-close", (i: PickEvaluationInput) => { i.pick.by = "auto"; }, "not-owner-pick"],
    ["withdrawn generation", (i: PickEvaluationInput) => { i.decision.state = "withdrawn"; }, "generation-withdrawn"],
    ["superseded generation", (i: PickEvaluationInput) => { i.decision.state = "superseded"; }, "generation-superseded"],
    ["resolved generation", (i: PickEvaluationInput) => { i.decision.state = "resolved"; }, "generation-resolved"],
    ["unknown generation state", (i: PickEvaluationInput) => { i.decision.state = "unknown"; }, "generation-unavailable"],
    ["missing generation number", (i: PickEvaluationInput) => { i.decision.generation = null; }, "generation-unavailable"],
    ["unregistered request", (i: PickEvaluationInput) => { i.decision.request_registered = false; }, "request-unregistered"],
    ["unknown registration", (i: PickEvaluationInput) => { i.decision.request_registered = null; }, "request-registration-unavailable"],
    ["unconfirmed binding", (i: PickEvaluationInput) => { i.decision.binding = "unconfirmed"; }, "binding-unconfirmed"],
    ["unknown binding", (i: PickEvaluationInput) => { i.decision.binding = "unknown"; }, "binding-unavailable"],
    ["no asking thread", (i: PickEvaluationInput) => { i.decision.asker = ""; }, "no-asking-thread"],
    ["target outside scope", (i: PickEvaluationInput) => { i.target = { result: "fail", reason: "target-outside-scope", observed_at: T }; }, "target-outside-scope"],
    ["target project changed", (i: PickEvaluationInput) => { i.target = { result: "fail", reason: "target-project-changed", observed_at: T }; }, "target-project-changed"],
    ["target unavailable", (i: PickEvaluationInput) => { i.target = { result: "unknown", reason: "target-unavailable", observed_at: null }; }, "target-unavailable"],
    ["older policy", (i: PickEvaluationInput) => { i.policy_version = PICK_POLICY_VERSION + 1; }, "policy-version-mismatch"],
  ])("%s is excluded", (_n, mutate, code) => {
    const i = base(); mutate(i);
    const e = evaluatePick(i);
    expect(e.exclusions.map(x => x.code)).toContain(code);
    expect(PICK_REASON_CODES).toContain(code);
    expect(e.outcome).toBe("fallback");
  });
  it("an unrecognised target reason is reported as unavailable, not trusted", () => {
    const i = base(); i.target = { result: "fail", reason: "made-up", observed_at: T };
    expect(evaluatePick(i).exclusions.map(x => x.code)).toEqual(["target-unavailable"]);
  });
});

describe("evaluatePick activation is separate from the proposal", () => {
  const proposal = (mutate: (i: PickEvaluationInput) => void) => { const i = base(); mutate(i); return evaluatePick(i); };
  it("shadow automatic is a proposal for observation only", () => {
    const e = proposal(() => {});
    expect(e.outcome).toBe("automatic");
    expect(e.activation).toEqual({ mode: "shadow", permitted: false, reason: "shadow-observation-only" });
  });
  it("off never activates", () => {
    expect(proposal(i => { i.control.mode = "off"; }).activation).toMatchObject({ permitted: false, reason: "handler-off" });
  });
  it.each([
    ["epoch changed", (i: PickEvaluationInput) => { i.control.mode = "live"; i.control.owner_enablement = true; i.control.enabled_cells = ["instruction:github-pr"]; i.control.capture_epoch = 2; }, "mode-epoch-changed"],
    ["epoch unknown", (i: PickEvaluationInput) => { i.control.mode = "live"; i.control.owner_enablement = true; i.control.enabled_cells = ["instruction:github-pr"]; i.control.capture_epoch = null; }, "mode-epoch-changed"],
    ["owner enablement missing", (i: PickEvaluationInput) => { i.control.mode = "live"; i.control.enabled_cells = ["instruction:github-pr"]; }, "owner-enablement-missing"],
    ["cell not enabled", (i: PickEvaluationInput) => { i.control.mode = "live"; i.control.owner_enablement = true; i.control.enabled_cells = ["instruction:none", "ruling-only:github-pr"]; }, "cell-not-enabled"],
    ["every guard satisfied", (i: PickEvaluationInput) => { i.control.mode = "live"; i.control.owner_enablement = true; i.control.enabled_cells = ["instruction:github-pr"]; }, "live-unavailable-in-this-build"],
  ])("live control %s does not change the proposal and is not permitted", (_n, mutate, reason) => {
    const e = proposal(mutate);
    expect(e.outcome).toBe("automatic");
    expect(e.activation).toEqual({ mode: "live", permitted: false, reason });
  });
  it("a fallback proposal is never activated", () => {
    const e = proposal(i => { i.pick.reason = "words"; i.control.mode = "live"; i.control.owner_enablement = true; i.control.enabled_cells = ["instruction:github-pr"]; });
    expect(e.activation).toEqual({ mode: "live", permitted: false, reason: "proposal-fallback" });
  });
  it.each([
    ["instruction", "pr", "instruction:github-pr"],
    ["instruction", "none", "instruction:none"],
    ["ruling-only", "pr", "ruling-only:github-pr"],
    ["ruling-only", "none", "ruling-only:none"],
  ] as const)("%s / %s reports cell %s", (kind, subject, cell) => {
    const i = base(subject);
    if (kind === "ruling-only") { const o = selected(i); o.kind = "ruling-only"; delete o.instruction; o.on_pick = { ...o.on_pick!, notification: "Noted: blue." }; }
    const e = evaluatePick(i);
    expect(e.cell).toBe(cell);
    expect(e.outcome).toBe("automatic");
  });
  it("records evidence times, limitations and identities", () => {
    const e = evaluatePick(base());
    expect(e).toMatchObject({
      policy_version: PICK_POLICY_VERSION, decision_id: "card-task-g1", pick_id: "pick-1", generation: 1, selected_option: "a", recommended_option: "a",
      evidence_times: { evaluated_at: T, comments_observed_at: "2026-01-01T00:00:05.000Z", head_observed_at: "2026-01-01T00:00:06.000Z", target_observed_at: "2026-01-01T00:00:07.000Z" },
      limitations: ["comment-edits-before-cutoff-not-detectable", "external-comment-read-race"],
    });
  });
  it("no combination with an unknown ever yields automatic", () => {
    const results: CheckResult[] = [];
    for (const { c1, c2, c3, c4 } of matrix) {
      const i = base(); set.recommendation[c1](i); set["no-extra-words"][c2](i); set.reversible[c3](i); setHead[c4](i);
      const e = evaluatePick(i);
      if ([c1, c2, c3, c4].includes("unknown")) { expect(e.outcome).toBe("fallback"); results.push(e.checks.recommendation.result); }
    }
    expect(results.length).toBe(81 - 16);
  });
});
