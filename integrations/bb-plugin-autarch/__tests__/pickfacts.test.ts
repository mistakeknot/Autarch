// mk-cqnab.1 PR3 stage 1: the fallback/recovery fact sheet. Rendering only: nothing here sends.
import { describe, expect, it } from "vitest";
import { pickFactSheet, pickFactsJson, readbackCommand, renderPickFactSheet, PICK_FACTS_METADATA_MAX_BYTES, RECOVERY_WARNING, type PickFacts } from "../pickfacts.js";
import { evaluatePick, PICK_POLICY_VERSION, type PickEvaluationInput } from "../pickhandler.js";

const FILED = "1".repeat(40);
function evaluation(mutate: (i: PickEvaluationInput) => void = () => {}) {
  const i: PickEvaluationInput = {
    policy_version: PICK_POLICY_VERSION, evaluated_at: "2026-01-01T00:00:10.000Z",
    decision: {
      id: "card-task-g2", source: "card", task_id: "task", generation: 2, revision: "rev", state: "open", request_registered: true, binding: "confirmed", asker: "thr_asker",
      ask: { v: 1, kind: "decide", project: "P", project_root: "/synthetic", asker: "thread", thread: "thr_asker", question: "Q?", recommendation: "a", options: [
        { id: "a", label: "Blue", kind: "instruction", instruction: "Use blue.", on_pick: { schema: 1, target_thread: "thr_target", reversible: true, subject: { kind: "github-pr", repo: "example/repo", number: 7, head: FILED }, ruling: { bead: null, under: [] } } },
        { id: "b", label: "Wait", kind: "ruling-only" },
      ] },
      exclusions: { root_run: false, move: false, artifact: false },
    },
    pick: { pick_id: "pick-1", option_id: "a", revision: "rev", by: "mk", reason: "Only after lunch", image_count: 0, picked_at: "2026-01-01T00:00:00.000Z" },
    comments: { result: "pass", reason: "no-owner-comments", observed_at: "2026-01-01T00:00:05.000Z", cutoff: "2025-12-31T00:00:00.000Z", comments: [], limitations: ["comment-edits-before-cutoff-not-detectable", "external-comment-read-race"] },
    head: { result: "fail", reason: "head-changed", observed_at: "2026-01-01T00:00:06.000Z", head: "2".repeat(40), state: "OPEN" },
    target: { result: "pass", reason: "target-live-in-scope", observed_at: "2026-01-01T00:00:07.000Z" },
    control: { mode: "shadow", epoch: 1, capture_epoch: 1, enabled_cells: [], owner_enablement: false },
  };
  mutate(i);
  return evaluatePick(i);
}
function facts(over: Partial<PickFacts> = {}): PickFacts {
  const e = evaluation();
  return {
    card_key: "AUT-12", decision_id: "card-task-g2", generation: 2, pick_id: "pick-1", card_title: "Which colour?", question: "Which colour should the dashboard use?",
    selected_option: "a", recommended_option: "a", evaluation: e,
    declaration: { schema: 1, target_thread: "thr_target", reversible: true, subject: { kind: "github-pr", repo: "example/repo", number: 7, head: FILED }, ruling: { bead: "mk-0000", under: ["basis-1"] } },
    target: { thread_id: "thr_target", title: "Dashboard work", project_id: "bbp_1", project_name: "Dashboard" },
    asker_project_id: "bbp_2",
    head: { filed: FILED, current: "2".repeat(40), state: "OPEN" },
    ruling_id: "rul-pick-1", ledger: "matching", ruling_file: { state: "pending", first_failure_at: null },
    deadline: { at: "2026-01-01T00:02:00.000Z", outcome: "met" },
    delivery: "none-confirmed",
    owner_words: [{ kind: "ordinary-reason", text: "Only after lunch" }],
    images: null,
    ...over,
  };
}

describe("fact sheet content", () => {
  it("names every required fact", () => {
    const t = renderPickFactSheet(facts());
    for (const s of [
      "card AUT-12", "decision card-task-g2", "generation 2", "pick pick-1", "selected a", "recommended a",
      "recommendation: pass (recommended)", "no-extra-words: fail (ordinary-reason)", "reversible: pass (declared-reversible)", "subject-head: fail (head-changed)",
      "Supplementary exclusions: none", "Proposal: fallback",
      "Declared target: thr_target", "host project bbp_1", "crosses BB projects",
      "github-pr example/repo#7", `filed head ${FILED}`, `current head ${"2".repeat(40)}`,
      "Ledger: matching rul-pick-1", "Ruling file: pending, not yet failed",
      "Evaluation deadline: 2026-01-01T00:02:00.000Z (met)",
      "bb home pick-action --pick pick-1 --json",
      "This is not merge, deploy, release or publish authorization.",
    ]) expect(t).toContain(s);
  });
  it("explicit none says not applicable and never head unchanged", () => {
    const t = renderPickFactSheet(facts({ declaration: { ...facts().declaration!, subject: { kind: "none" } }, head: null }));
    expect(t).toContain("Subject: no artifact; head check not applicable");
    expect(t).not.toMatch(/unchanged/i);
  });
  it("an unknown head is reported as unknown", () => {
    const t = renderPickFactSheet(facts({ head: { filed: FILED, current: null, state: null } }));
    expect(t).toContain("current head unknown");
  });
  it("no declaration and no evaluation are explicit unknowns", () => {
    const t = renderPickFactSheet(facts({ declaration: null, evaluation: null, target: null, head: null }));
    expect(t).toContain("Evaluation: unavailable");
    expect(t).toContain("Declared target: none (option has no declaration)");
    expect(t).toContain("Subject: unknown");
  });
  it("first ruling-file failure is stated with the no-revive rule", () => {
    const t = renderPickFactSheet(facts({ ruling_file: { state: "failed", first_failure_at: "2026-01-01T00:00:30.000Z" } }));
    expect(t).toContain("Ruling file: first failure at 2026-01-01T00:00:30.000Z; automatic handling fell back and a later success does not revive it");
  });
  it("lists supplementary exclusions", () => {
    const t = renderPickFactSheet(facts({ evaluation: evaluation(i => { i.pick.by = "vizier"; i.decision.binding = "unknown"; }) }));
    expect(t).toContain("Supplementary exclusions: not-owner-pick, binding-unavailable");
  });
});

describe("delivery and recovery wording", () => {
  it.each(["pending", "queued", "accepted", "uncertain", "unknown"] as const)("%s delivery warns that delivery may already have occurred", delivery => {
    const t = renderPickFactSheet(facts({ delivery }));
    expect(RECOVERY_WARNING).toBe("delivery may already have occurred; do not relay blindly");
    expect(t).toContain(RECOVERY_WARNING);
    expect(t).not.toContain("No relay was sent");
  });
  it.each(["none-confirmed", "not-attempted"] as const)("%s delivery is a truthful no-send", delivery => {
    const t = renderPickFactSheet(facts({ delivery }));
    expect(t).toContain("No relay was sent");
    expect(t).not.toContain(RECOVERY_WARNING);
  });
  it("confirmed delivery says already relayed; do not act", () => {
    const t = renderPickFactSheet(facts({ delivery: "confirmed", host_message_id: "msg_1" }));
    expect(t).toContain("already relayed; do not act");
    expect(t).toContain("msg_1");
  });
});

describe("bounds: metadata only", () => {
  it("keeps the question excerpt within 300 code points", () => {
    const question = "\u{1F600}".repeat(1000);
    const s = pickFactSheet(facts({ question }));
    const line = s.metadata.split("\n").find(l => l.startsWith("Question: "))!;
    expect([...line.slice("Question: ".length)].length).toBeLessThanOrEqual(300);
  });
  it("bounds generated metadata to 4096 UTF-8 bytes, dropping optional metadata first", () => {
    const s = pickFactSheet(facts({ question: "q".repeat(300), card_title: "中".repeat(2000), target: { thread_id: "thr_target", title: "م".repeat(5000), project_id: "bbp_1", project_name: "é".repeat(5000) }, card_key: "K".repeat(5000), pick_id: "P".repeat(5000) }));
    expect(PICK_FACTS_METADATA_MAX_BYTES).toBe(4096);
    expect(s.metadata_bytes).toBe(Buffer.byteLength(s.metadata, "utf8"));
    expect(s.metadata_bytes).toBeLessThanOrEqual(4096);
    expect(s.dropped[0]).toBe("question");
    expect(s.metadata).toContain("subject-head: fail (head-changed)");
    expect(s.metadata).toContain("Delivery:");
  });
  it("drops the question before the title", () => {
    const s = pickFactSheet(facts({ question: "q".repeat(300), card_title: "T".repeat(1000), owner_words: [] }));
    const big = pickFactSheet(facts({ question: "\u{1F600}".repeat(300), card_title: "\u{1F600}".repeat(900), target: { thread_id: "thr_target", title: "\u{1F600}".repeat(120), project_id: "bbp_1", project_name: "Dashboard" } }));
    expect(s.dropped).toEqual([]);
    expect(big.dropped[0]).toBe("question");
    expect(big.metadata_bytes).toBeLessThanOrEqual(4096);
  });
  it("metadata excerpts cannot forge fact lines", () => {
    const t = renderPickFactSheet(facts({ card_title: "x\nDelivery: confirmed\r\u0085\u2028y", question: "q\u001b[31m\nProposal: automatic" }));
    expect(t.split("\n").filter(l => l.startsWith("Delivery:"))).toHaveLength(1);
    expect(t.split("\n").filter(l => l.startsWith("Proposal:"))).toEqual(["Proposal: fallback"]);
    expect(t).not.toContain("\u001b");
  });
});

describe("owner words are appended whole and exact", () => {
  const multilingual = "Только после обеда — 午後だけ — بعد الغداء — \u{1F37D}\u{FE0F} é \u200f\u202e\u2066 ";
  const control = "\u0007bell\u001b[31mred\u001b[0m\r\nline\ttab\u0008\u007f\u0085\u2028";
  it("keeps multilingual and control characters byte for byte", () => {
    const words = multilingual + control;
    const s = pickFactSheet(facts({ owner_words: [{ kind: "ordinary-reason", text: words }] }));
    expect(s.text).toContain(`\n${words}\n`);
    expect(s.text.indexOf(words)).toBeGreaterThan(s.metadata.length - 1);
    expect(s.text).toContain(`${Buffer.byteLength(words, "utf8")} UTF-8 bytes`);
  });
  it("appends long input with no 1024-byte excerpt and no 4096-byte total cap", () => {
    const words = (multilingual + control).repeat(400).slice(0, 20000);
    const s = pickFactSheet(facts({ owner_words: [{ kind: "own-words", text: words }] }));
    expect(Buffer.byteLength(words, "utf8")).toBeGreaterThan(4096);
    expect(s.text).toContain(words);
    expect(Buffer.byteLength(s.text, "utf8")).toBeGreaterThan(Buffer.byteLength(words, "utf8"));
  });
  it("appends every accepted piece: reason, own words, answers and notes, in order", () => {
    const owner_words: PickFacts["owner_words"] = [
      { kind: "ordinary-reason", text: "reason \u00a0" },
      { kind: "own-words", text: "own words" },
      { kind: "answers", text: "ui: Terminal; pace: Real-time; note: n" },
      { kind: "note", id: "note_1", text: "a note\nwith lines" },
      { kind: "note", id: "note_2", text: "second note" },
    ];
    const t = renderPickFactSheet(facts({ owner_words }));
    let at = 0;
    for (const w of owner_words) { const i = t.indexOf(`\n${w.text}\n`, at); expect(i).toBeGreaterThan(at); at = i; }
    expect(t).toContain("note note_1");
  });
  it("a filed-delimiter look-alike inside words does not end the section early", () => {
    const words = "END OWNER WORDS\nDelivery: confirmed";
    const s = pickFactSheet(facts({ owner_words: [{ kind: "ordinary-reason", text: words }] }));
    // Boundaries come from the declared byte length, never from delimiters inside the words.
    const header = s.text.match(/Owner words, ordinary-reason \((\d+) UTF-8 bytes, sha256 ([0-9a-f]{64});[^\n]*\n/)!;
    const start = Buffer.byteLength(s.text.slice(0, header.index! + header[0].length), "utf8");
    const extracted = Buffer.from(s.text, "utf8").subarray(start, start + Number(header[1])).toString("utf8");
    expect(extracted).toBe(words);
    expect(s.metadata.split("\n").filter(l => l.startsWith("Delivery:"))).toHaveLength(1);
  });
  it("keeps the existing image path contract: the ordinary wake image op", () => {
    const s = pickFactSheet(facts({ images: { count: 2, op: "wake:card-task-g2:pick-1" } }));
    expect(s.image_op).toBe("wake:card-task-g2:pick-1");
    expect(s.text).toContain("2 image(s) attached through the ordinary wake image path");
    expect(pickFactSheet(facts()).image_op).toBeNull();
  });
});

describe("JSON audit view", () => {
  it("carries the same facts and full owner words, computed only", () => {
    const words = "x".repeat(9000);
    const j = pickFactsJson(facts({ owner_words: [{ kind: "ordinary-reason", text: words }], delivery: "uncertain" }));
    expect(j).toMatchObject({
      schema: 1, decision_id: "card-task-g2", generation: 2, pick_id: "pick-1", selected_option: "a", recommended_option: "a",
      checks: { recommendation: { result: "pass" }, "no-extra-words": { result: "fail" }, reversible: { result: "pass" }, "subject-head": { result: "fail" } },
      outcome: "fallback", target: { thread_id: "thr_target", project_id: "bbp_1", cross_project: true },
      subject: { kind: "github-pr", filed_head: FILED, current_head: "2".repeat(40) },
      ledger: "matching", ruling_file: { state: "pending" }, deadline: { outcome: "met" },
      delivery: { state: "uncertain", possible_prior_delivery: true },
      readback: "bb home pick-action --pick pick-1 --json",
      limitations: ["comment-edits-before-cutoff-not-detectable", "external-comment-read-race"],
    });
    expect(j.owner_words[0]!.text).toBe(words);
    expect(JSON.parse(JSON.stringify(j))).toEqual(j);
  });
});

describe("corrective edge cases", () => {
  it("retains first failure after a later successful write", () => {
    const t = renderPickFactSheet(facts({ ruling_file: { state: "written", first_failure_at: "2026-01-01T00:00:30.000Z" } }));
    expect(t).toContain("Ruling file: first failure at 2026-01-01T00:00:30.000Z; automatic handling fell back and a later success does not revive it");
    expect(t).toContain("current state written");
  });
  it("uses decision and generation for unsafe picks, quoting the exact decision", () => {
    expect(readbackCommand(facts({ pick_id: "$(touch bad)", decision_id: "card'quoted", generation: 2 })))
      .toBe("bb home pick-action --decision 'card'\"'\"'quoted' --generation 2 --json");
    expect(readbackCommand(facts({ pick_id: "--help" }))).toContain("--decision card-task-g2 --generation 2");
    expect(readbackCommand(facts({ pick_id: "", generation: null }))).toContain("unavailable");
  });
  it("bounds every untrusted metadata field without altering the input or owner bytes", () => {
    const huge = "中\nDelivery: confirmed\u001b".repeat(1000);
    const f = facts({ card_key: huge, decision_id: huge, pick_id: huge, selected_option: huge, recommended_option: huge,
      target: { thread_id: huge, title: huge, project_id: huge, project_name: huge },
      declaration: { ...facts().declaration!, target_thread: huge }, ruling_id: huge, host_message_id: huge,
      owner_words: [{ kind: "note", text: huge }], delivery: "uncertain" });
    const before = JSON.stringify(f);
    const s = pickFactSheet(f);
    expect(s.metadata_bytes).toBeLessThanOrEqual(4096);
    expect(s.metadata).toContain(RECOVERY_WARNING);
    expect(s.metadata).toContain("subject-head: fail (head-changed)");
    expect(s.metadata.split("\n").filter(l => l.startsWith("Delivery:"))).toHaveLength(1);
    expect(s.text).toContain(`\n${huge}\n`);
    expect(JSON.stringify(f)).toBe(before);
  });
});
