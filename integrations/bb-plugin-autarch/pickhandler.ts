import { z } from "zod";
import { authorClass, type ClassifyContext } from "./conversation.js";
import { ownerCommentCutoff } from "./picktarget.js";
import { HOME_NOTE_TRAILER } from "./tasks.js";

export interface OwnerCommentInput {
  generation: number;
  previousObservedAt: string | null;
  comments: unknown;
  complete: boolean;
  observedAt: string;
  context: ClassifyContext;
  expectedWriteId: string;
  expectedStatusId: string;
  noteFence: "absent" | "pending" | "failed" | "recorded" | "unknown";
}
export interface OwnerCommentEvidence {
  result: "pass" | "fail" | "unknown";
  reason: string;
  observed_at: string;
  cutoff: string | null;
  comments: { id: string; created_at: string }[];
  limitations: readonly string[];
}

// Validate at the evidence boundary too: malformed/incomplete reads cannot prove absence.
const evidenceComment = z.object({
  id: z.string().min(1), body: z.string(), createdAt: z.string().refine(v => Number.isFinite(Date.parse(v))),
  kind: z.enum(["user", "agent", "system"]), threadId: z.string().nullable(),
});
const COMMENT_LIMITATIONS = ["comment-edits-before-cutoff-not-detectable", "external-comment-read-race"] as const;

export function evaluateOwnerComments(input: OwnerCommentInput): OwnerCommentEvidence {
  let cutoff: string | null = null;
  const comments: OwnerCommentEvidence["comments"] = [];
  const result = (value: OwnerCommentEvidence["result"], reason: string): OwnerCommentEvidence => ({
    result: value, reason, observed_at: input.observedAt, cutoff, comments, limitations: COMMENT_LIMITATIONS,
  });
  // A note's posting state never clears its permanent generation fence.
  if (["pending", "failed", "recorded"].includes(input.noteFence)) return result("fail", "home-note-fence");
  if (input.noteFence !== "absent") return result("unknown", "note-fence-unavailable");
  try { cutoff = ownerCommentCutoff(input.generation, input.previousObservedAt); }
  catch { return result("unknown", "comment-cutoff-unavailable"); }
  if (!input.complete || !Number.isFinite(Date.parse(input.observedAt))) return result("unknown", "comments-unavailable");
  const parsed = z.array(evidenceComment).safeParse(input.comments);
  if (!parsed.success) return result("unknown", "comments-unavailable");
  let ownerWords = false;
  for (const c of parsed.data) {
    comments.push({ id: c.id, created_at: c.createdAt });
    if (authorClass({ kind: c.kind, author_id: c.threadId }, input.context) !== "mk") continue;
    if (cutoff !== null && Date.parse(c.createdAt) <= Date.parse(cutoff)) continue;
    const lines = c.body.split(/\r?\n/);
    const homeWrite = lines.includes(`home-write: ${input.expectedWriteId}`);
    const homeStatus = lines.includes(`home-pick-status: ${input.expectedStatusId}`);
    if (!HOME_NOTE_TRAILER.test(c.body) && (homeWrite || homeStatus)) continue;
    ownerWords = true;
  }
  return result(ownerWords ? "fail" : "pass", ownerWords ? "owner-comment" : "no-owner-comments");
}
