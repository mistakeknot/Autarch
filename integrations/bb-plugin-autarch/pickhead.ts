import { ghHeadReal } from "./ghview.js";
import type { OnPick } from "./onpick.js";

export type PickSubject = OnPick["subject"];
export type HeadEvidence = {
  result: "pass" | "fail" | "unknown" | "not-applicable";
  reason: string;
  observed_at: string;
  head: string | null;
  state: string | null;
};
export type HeadReader = (repo: string, number: number, timeoutMs: number) => Promise<unknown>;
export interface HeadReadOptions {
  deadlineAt: string;
  now?: () => number;
  read?: HeadReader;
}

const MAX_READ_MS = 20_000;
const SHA = /^[0-9a-f]{40}$/;

/** Evidence only. Callers must still CAS generation, epoch and deadline before sending. */
export async function readSubjectHead(subject: PickSubject, options: HeadReadOptions): Promise<HeadEvidence> {
  const now = options.now ?? Date.now;
  const evidence = (result: HeadEvidence["result"], reason: string, head: string | null = null, state: string | null = null): HeadEvidence =>
    ({ result, reason, observed_at: new Date(now()).toISOString(), head, state });
  if (subject.kind === "none") return Object.keys(subject).length === 1
    ? evidence("not-applicable", "subject-none") : evidence("unknown", "subject-invalid");
  if (subject.kind !== "github-pr" || !/^[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}$/.test(subject.repo)
    || !Number.isSafeInteger(subject.number) || subject.number <= 0 || !SHA.test(subject.head)) return evidence("unknown", "subject-invalid");
  const deadline = Date.parse(options.deadlineAt);
  const remaining = deadline - now();
  if (!Number.isFinite(remaining) || remaining <= 0) return evidence("unknown", "evaluation-timeout");
  const timeoutMs = Math.min(MAX_READ_MS, remaining);
  const timedOut = Symbol("head-timeout");
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    // Bound the caller as well as execFile: injected readers and host promises can hang.
    const timeout = new Promise<typeof timedOut>(resolve => { timer = setTimeout(() => resolve(timedOut), timeoutMs); });
    const response = await Promise.race([(options.read ?? ghHeadReal)(subject.repo, subject.number, timeoutMs), timeout]);
    if (now() >= deadline) return evidence("unknown", "evaluation-timeout");
    if (response === timedOut) return evidence("unknown", "head-timeout");
    if (!response || typeof response !== "object" || !("state" in response) || !("headRefOid" in response)
      || typeof response.state !== "string" || !["OPEN", "CLOSED", "MERGED"].includes(response.state)
      || typeof response.headRefOid !== "string" || !SHA.test(response.headRefOid)) return evidence("unknown", "head-malformed");
    const { state, headRefOid } = response;
    if (state !== "OPEN") return evidence("fail", "pr-not-open", headRefOid, state);
    return evidence(headRefOid === subject.head ? "pass" : "fail", headRefOid === subject.head ? "head-unchanged" : "head-changed", headRefOid, state);
  } catch {
    return evidence("unknown", now() >= deadline ? "evaluation-timeout" : "head-unavailable");
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
}
