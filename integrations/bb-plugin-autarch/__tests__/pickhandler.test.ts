import { describe, expect, it } from "vitest";
import { evaluateOwnerComments, type OwnerCommentInput } from "../pickhandler.js";
import { commentBody } from "../cardwrites.js";
import type { TaskCommentRow } from "../tasks.js";

const before = "2026-01-01T00:00:00.000Z";
const observedAt = "2026-01-02T00:00:00.000Z";
const input: OwnerCommentInput = {
  generation: 1, previousObservedAt: null, comments: [], complete: true, observedAt,
  context: { owner: "thr_owner", vizier: "thr_vizier" },
  expectedWriteIds: ["cw-decision-comment"], expectedStatusIds: ["pa-pick-status"], noteFence: "absent",
};
function comment(overrides: Partial<TaskCommentRow> = {}): TaskCommentRow {
  return { id: "comment", taskId: "task", kind: "user", authorName: "Owner", threadId: null, body: "Only if the checks pass", createdAt: before, ...overrides };
}

describe("owner comment evidence", () => {
  it("mk-7okdy N1 first generation counts all mk-class comments including pre-filing", () => {
    expect(evaluateOwnerComments({ ...input, comments: [comment()] })).toMatchObject({ result: "fail", reason: "owner-comment", cutoff: null, comments: [{ id: "comment", created_at: before }] });
  });
  it("mk-7okdy N1 later cutoff uses previous Home observation, never current filing", () => {
    const previousObservedAt = "2026-01-01T01:00:00.000Z";
    expect(evaluateOwnerComments({ ...input, generation: 2, previousObservedAt, comments: [comment()] })).toMatchObject({ result: "pass", cutoff: previousObservedAt });
    expect(evaluateOwnerComments({ ...input, generation: 2, previousObservedAt, comments: [comment({ createdAt: "2026-01-01T02:00:00.000Z" })] })).toMatchObject({ result: "fail", reason: "owner-comment" });
  });
  it("mk-7okdy missing previous observation is unknown", () => {
    expect(evaluateOwnerComments({ ...input, generation: 2 })).toMatchObject({ result: "unknown", reason: "comment-cutoff-unavailable" });
  });
  it("Home Ruled comment with the expected complete write marker passes", () => {
    const body = commentBody(input.expectedWriteIds[0]!, { option_label: "A", by: "mk", generation: 1 });
    expect(evaluateOwnerComments({ ...input, comments: [comment({ body })] }).result).toBe("pass");
  });
  it("Home status comment with the expected complete marker passes", () => {
    expect(evaluateOwnerComments({ ...input, comments: [comment({ body: `Pending\n\nhome-pick-status: ${input.expectedStatusIds[0]}` })] }).result).toBe("pass");
  });
  it.each(["home-write: arbitrary", "prefix home-write: cw-decision-comment", "home-write: cw-decision-comment extra", "home-pick-status: other"])("prefix prose or another operation is owner words: %s", body => {
    expect(evaluateOwnerComments({ ...input, comments: [comment({ body })] })).toMatchObject({ result: "fail", reason: "owner-comment" });
  });
  it.each(["home-write: cw-decision-comment", "home-pick-status: pa-pick-status"])("HOME_NOTE_TRAILER takes precedence over %s", marker => {
    expect(evaluateOwnerComments({ ...input, comments: [comment({ body: `${marker}\n\nhome-note: note_1` })] })).toMatchObject({ result: "fail", reason: "owner-comment" });
  });
  it.each(["pending", "failed", "recorded"] as const)("permanent %s Home note fence fails even without posted comment", noteFence => {
    expect(evaluateOwnerComments({ ...input, noteFence })).toMatchObject({ result: "fail", reason: "home-note-fence" });
  });
  it("unknown note fence is never absence", () => {
    expect(evaluateOwnerComments({ ...input, noteFence: "unknown" })).toMatchObject({ result: "unknown", reason: "note-fence-unavailable" });
  });
  it.each(["thr_owner", "thr_vizier", "thr_other"])("maps threadId %s through authorClass", threadId => {
    expect(evaluateOwnerComments({ ...input, comments: [comment({ threadId })] }).result).toBe("pass");
  });
  it("null-thread user is mk but null-thread system is not", () => {
    expect(evaluateOwnerComments({ ...input, comments: [comment()] }).result).toBe("fail");
    expect(evaluateOwnerComments({ ...input, comments: [comment({ kind: "system" })] }).result).toBe("pass");
  });
  it.each([null, {}, [null], [comment({ createdAt: "broken" })], [{ ...comment(), threadId: 9 }]])("malformed or unavailable comments are unknown: %j", comments => {
    expect(evaluateOwnerComments({ ...input, comments }).result).toBe("unknown");
  });
  it("explicit incomplete response is unknown", () => {
    expect(evaluateOwnerComments({ ...input, complete: false }).result).toBe("unknown");
  });
  it("requires no edit timestamps or pagination and discloses accepted limitations", () => {
    const result = evaluateOwnerComments(input);
    expect(result.result).toBe("pass");
    expect(result.limitations).toEqual(["comment-edits-before-cutoff-not-detectable", "external-comment-read-race"]);
  });
});
