import { parseOnPick, type OnPick } from "./onpick.js";
import { validateStopped } from "./stopped.js";
// The TypeScript side of the Home ask protocol: the shared filing request
// (schema/home-ask/v1), its validation and defaults, and the three hashes
// derived from it (identity, revision, semanticKey).
//
// Twin of internal/homeask/model.go. Both must produce the byte-for-byte
// outputs in schema/home-ask/v1/vectors.json, so the rules below are written to
// match the Go code line for line (ASCII-only case folding, ASCII whitespace,
// counts in code points, UTF-16 key order).

import { createHash } from "node:crypto";

import { validateEvidence, type Evidence } from "./evidence.js";
import { validateGroup, type MergeGroup } from "./group.js";
export type AskKind = "decide" | "steps" | "machine";
export type OptionKind = "instruction" | "needs-context" | "ruling-only";

export interface Approval {
  kind: "merge" | "deploy" | "release";
  target: string;
  identity: string;
  ttl?: number;
}

type OutcomeDeadline = { at: string } | { after_seconds: number };
type OutcomePR = { repo: string; number: number };
type VersionedOutcome = { schema: 1; by_when: OutcomeDeadline } & (
  | { kind: "merge"; target: { prs: (OutcomePR & { head: string })[] } }
  | { kind: "close"; target: { prs: OutcomePR[] } }
  | { kind: "bead-closed"; target: { tracker: string; bead: string } }
  | { kind: "home-report-ok"; target: { path: string; sha256: string } }
  | { kind: "file-exists"; target: { path: string; sha256?: string } }
  | { kind: "re-ask-by-date"; target: { ask_key: string } }
);

export type ExpectedOutcome = VersionedOutcome | { kind: "merge" | "close"; prs: string[] };

export interface AskOption {
  id?: string;
  label: string;
  kind?: OptionKind;
  reversible?: boolean;
  instruction?: string;
  approval?: Approval;
  on_pick?: OnPick;
  expected_outcome?: ExpectedOutcome;
}

/** One option of a sub-question: a label and an optional one-line detail shown under it. */
export interface QuestionOption {
  id: string;
  label: string;
  detail?: string;
}

/** One independent choice inside a multi-pick ask. Answered together with the others by one pick. */
export interface Question {
  id: string;
  prompt: string;
  options: QuestionOption[];
  recommendation?: string;
}

export interface Machine {
  class: string;
  detail: string;
  owner_thread?: string;
}

export interface Ask {
  v: 1;
  kind: AskKind;
  request_id?: string;
  ask_key?: string;
  subject?: string;
  project: string;
  project_root: string;
  asker: "thread" | "mycroft";
  thread?: string;
  question: string;
  recommendation?: string;
  supersedes?: string;
  mention_of?: string;
  options?: AskOption[];
  questions?: Question[];
  steps?: string[];
  machine?: Machine;
  /** Optional delivery attribute: only "blocker" is accepted. Not part of the identity, revision, semantic key or normalized JSON. */
  urgency?: "blocker";
  stopped?: string;
  // Delivery-only: excluded from identity, revision, semantic key and normalized JSON.
  // A filing or update changing only evidence is a no-op, like Urgency.
  evidence?: Evidence[];
  group?: MergeGroup;
  review_by?: string;
}

const MAX_INSTRUCTION = 8000;
const MAX_LABEL = 80;
const MAX_KEY = 120;
const MAX_QUESTION = 8000;
const MAX_STEPS = 20;
const MAX_QUESTIONS = 8;
const MAX_PROMPT = 500;
const MAX_DETAIL = 300;
const MAX_TTL = 7 * 24 * 3600;

const OPTION_ID = /^[a-z0-9-]{1,32}$/;
const REQUEST_ID = /^[A-Za-z0-9:_.-]{1,128}$/;
const REF_ID = /^[A-Za-z0-9:_.-]{1,128}$/;
const MACHINE_CLASS = /^[a-z0-9-]{1,32}$/;
const APPROVAL_TOKEN = /APPROVED-[A-Z]+/;

const nfc = (s: string) => s.normalize("NFC");
const count = (s: string) => Array.from(s).length;
const blank = (s: string) => s.trim() === "";
/** The twin of Go's strings.Trim(s, " \t\n\r\f\v") == "": blank text in a multi-pick ask, the same set of characters on both sides. */
const blankAscii = (s: string) => /^[ \t\n\r\f\v]*$/.test(s);
const fail = (msg: string): never => {
  throw new Error(msg);
};

/** NFC, ASCII whitespace collapsed, ASCII lowercased (Go's normText). */
function normText(s: string): string {
  s = nfc(s);
  let out = "";
  let space = false;
  for (const ch of s) {
    if (ch === " " || ch === "\t" || ch === "\n" || ch === "\r" || ch === "\f" || ch === "\v") {
      space = true;
      continue;
    }
    if (space && out.length > 0) out += " ";
    space = false;
    out += ch >= "A" && ch <= "Z" ? ch.toLowerCase() : ch;
  }
  return out;
}

function firstChars(s: string, n: number): string {
  const chars = Array.from(s);
  return chars.length <= n ? s : chars.slice(0, n).join("");
}

function slug(label: string): string {
  let out = "";
  let dash = false;
  for (const ch of nfc(label).toLowerCase()) {
    if ((ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9")) {
      if (dash && out.length > 0) out += "-";
      dash = false;
      out += ch;
    } else {
      dash = true;
    }
  }
  return firstChars(out, 32).replace(/-+$/, "");
}

const TOP_FIELDS = ["v", "kind", "request_id", "ask_key", "subject", "project", "project_root", "asker", "thread", "question", "recommendation", "supersedes", "mention_of", "options", "questions", "steps", "machine", "urgency", "stopped", "evidence", "group"];
const OPTION_FIELDS = ["id", "label", "kind", "reversible", "instruction", "approval"];
const QUESTION_FIELDS = ["id", "prompt", "options", "recommendation"];
const QUESTION_OPTION_FIELDS = ["id", "label", "detail"];
const APPROVAL_FIELDS = ["kind", "target", "identity", "ttl"];
const MACHINE_FIELDS = ["class", "detail", "owner_thread"];

function obj(v: unknown, what: string): Record<string, unknown> {
  if (v === null || typeof v !== "object" || Array.isArray(v)) fail(`invalid ask: ${what} must be an object`);
  return v as Record<string, unknown>;
}

function known(o: Record<string, unknown>, allowed: string[], what: string) {
  for (const k of Object.keys(o)) if (!allowed.includes(k)) fail(`invalid ask: unknown field "${k}" in ${what}`);
}

function str(o: Record<string, unknown>, k: string): string {
  const v = o[k];
  if (v === undefined || v === null) return "";
  if (typeof v !== "string") fail(`invalid ask: ${k} must be a string`);
  return v as string;
}

const CARD_INSTANT = /^([0-9]{4}-(?:0[1-9]|1[0-2])-(?:0[1-9]|[12][0-9]|3[01])T(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9])(?:\.([0-9]{1,9}))?(Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$(?![\s\S])/;
const OUTCOME_ID = /^[A-Za-z0-9][A-Za-z0-9:_.-]{0,127}$(?![\s\S])/;
const OUTCOME_REPO = /^[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}$(?![\s\S])/;
const OUTCOME_PR_URL = /^https:\/\/github\.com\/([^/]+\/[^/]+)\/pull\/[1-9][0-9]{0,8}$(?![\s\S])/;
const OUTCOME_HEAD = /^[0-9a-f]{40}$(?![\s\S])/;
const OUTCOME_SHA = /^[0-9a-f]{64}$(?![\s\S])/;
const OUTCOME_CONTROL = /[\u0000-\u001f\u007f]/u;

function canonicalCardInstant(value: unknown): string {
  const match = typeof value === "string" && CARD_INSTANT.exec(value);
  if (!match || match[1]!.startsWith("0000") || match[3] === "-00:00") return fail("expected an offset-bearing RFC 3339 timestamp");
  const [, local, fraction, zone] = match;
  // Compare the local calendar before applying the offset, and preserve sub-millisecond precision.
  if (new Date(local + "Z").toISOString().slice(0, 19) !== local) return fail("invalid calendar date");
  const utc = new Date(local + zone).toISOString();
  if (utc.length !== 24 || utc.startsWith("0000")) return fail("invalid UTC year");
  const digits = fraction?.replace(/0+$/, "");
  return utc.slice(0, 19) + (digits ? "." + digits : "") + "Z";
}

const positiveOutcomeInt = (value: unknown, max: number): value is number =>
  typeof value === "number" && Number.isSafeInteger(value) && value > 0 && value <= max;
const outcomeId = (value: unknown): value is string => typeof value === "string" && OUTCOME_ID.test(value);
const outcomeRepo = (value: unknown): value is string =>
  typeof value === "string" && OUTCOME_REPO.test(value) && value.split("/").every(part => part !== "." && part !== "..");

function outcomePRList(input: unknown): unknown[] {
  if (!Array.isArray(input) || input.length === 0 || input.length > 32) return fail("prs must contain 1-32 pull requests");
  return input;
}

function parseExpectedOutcome(input: unknown): ExpectedOutcome {
  const spec = obj(input, "expected_outcome");
  const kind = spec.kind;
  if (!Object.hasOwn(spec, "schema")) {
    known(spec, ["kind", "prs"], "historical expected_outcome");
    if (kind !== "merge" && kind !== "close") return fail("historical outcome requires merge or close");
    const prs = outcomePRList(spec.prs).map(raw => {
      const match = typeof raw === "string" && OUTCOME_PR_URL.exec(raw);
      if (!match || !outcomeRepo(match[1])) return fail("historical prs must contain canonical GitHub PR URLs");
      return match[0];
    });
    return { kind, prs };
  }
  known(spec, ["schema", "kind", "target", "by_when"], "expected_outcome");
  if (spec.schema !== 1) return fail("unsupported expected_outcome schema");
  const due = obj(spec.by_when, "by_when");
  if (Object.keys(due).length !== 1) return fail("by_when requires exactly one of at or after_seconds");
  let by_when: OutcomeDeadline;
  if (Object.hasOwn(due, "at")) by_when = { at: canonicalCardInstant(due.at) };
  else if (positiveOutcomeInt(due.after_seconds, Number.MAX_SAFE_INTEGER)) by_when = { after_seconds: due.after_seconds };
  else return fail("after_seconds must be a positive safe integer");

  const target = obj(spec.target, "expected_outcome target");
  const base = { schema: 1 as const, by_when };
  switch (kind) {
    case "merge":
    case "close": {
      known(target, ["prs"], "target");
      const seen = new Set<string>();
      const prs = outcomePRList(target.prs).map(value => {
        const pr = obj(value, "PR target");
        known(pr, kind === "merge" ? ["repo", "number", "head"] : ["repo", "number"], "PR target");
        if (!outcomeRepo(pr.repo) || !positiveOutcomeInt(pr.number, 999999999)) return fail("PR target requires a repository and positive PR number");
        const key = pr.repo.toLowerCase() + "#" + pr.number;
        if (seen.has(key)) return fail("duplicate PR target");
        seen.add(key);
        return { repo: pr.repo, number: pr.number, head: pr.head };
      });
      if (kind === "merge") return { ...base, kind, target: { prs: prs.map(pr => {
        if (typeof pr.head !== "string" || !OUTCOME_HEAD.test(pr.head)) return fail("merge target requires a full lowercase PR head");
        return { repo: pr.repo, number: pr.number, head: pr.head };
      }) } };
      return { ...base, kind, target: { prs: prs.map(({ repo, number }) => ({ repo, number })) } };
    }
    case "bead-closed":
      known(target, ["tracker", "bead"], "target");
      if (!outcomeId(target.tracker) || !outcomeId(target.bead)) return fail("bead target requires tracker and bead identifiers");
      return { ...base, kind, target: { tracker: target.tracker, bead: target.bead } };
    case "home-report-ok":
    case "file-exists": {
      known(target, ["path", "sha256"], "target");
      const { path, sha256 } = target;
      if (typeof path !== "string" || !path.isWellFormed() || path.normalize("NFC") !== path || new TextEncoder().encode(path).length > 4096 ||
          !path.startsWith("/") || path.includes("\\") || OUTCOME_CONTROL.test(path) ||
          path.slice(1).split("/").some(part => part === "" || part === "." || part === "..")) {
        return fail("target requires a canonical absolute path");
      }
      if (kind === "file-exists" && !Object.hasOwn(target, "sha256")) return { ...base, kind, target: { path } };
      if (typeof sha256 !== "string" || !OUTCOME_SHA.test(sha256)) return fail("target requires a full lowercase sha256");
      return { ...base, kind, target: { path, sha256 } };
    }
    case "re-ask-by-date":
      known(target, ["ask_key"], "target");
      if (!outcomeId(target.ask_key)) return fail("re-ask target requires an ask_key identifier");
      return { ...base, kind, target: { ask_key: target.ask_key } };
    default:
      return fail("unknown expected_outcome kind");
  }
}

/** Strictly decodes a parsed JSON request (unknown fields are refused) and returns it normalized. */
export function parseAsk(input: unknown): Ask { return parseAskValue(input, true); }

/** Read an existing ask; legacy blockers can omit stopped. Present values remain validated. */
export function parseExistingAsk(input: unknown): Ask { return parseAskValue(input, false); }

export function parseStoredCardAsk(input: unknown): Ask { return parseAskValue(input, false, true); }

function parseAskValue(input: unknown, requireStopped: boolean, cardSource = false): Ask {
  const o = obj(input, "ask");
  known(o, cardSource ? [...TOP_FIELDS, "review_by"] : TOP_FIELDS, "ask");
  const a: Record<string, unknown> = {};
  if (o.evidence !== undefined) a.evidence = validateEvidence(o.evidence);
  if (o.group !== undefined) a.group = validateGroup(o.group);
  if (Object.hasOwn(o, "review_by")) a.review_by = canonicalCardInstant(o.review_by);
  if (o.v !== undefined && o.v !== null) {
    if (typeof o.v !== "number") fail("invalid ask: v must be a number");
    a.v = o.v;
  } else a.v = 0;
  for (const k of ["kind", "request_id", "ask_key", "subject", "project", "project_root", "asker", "thread", "question", "recommendation", "supersedes", "mention_of"]) a[k] = str(o, k);
  if (o.options !== undefined && o.options !== null) {
    if (!Array.isArray(o.options)) fail("invalid ask: options must be an array");
    a.options = (o.options as unknown[]).map((raw) => {
      const p = obj(raw, "option");
      known(p, cardSource ? [...OPTION_FIELDS, "on_pick", "expected_outcome"] : OPTION_FIELDS, "option");
      const opt: Record<string, unknown> = { id: str(p, "id"), label: str(p, "label"), kind: str(p, "kind"), instruction: str(p, "instruction") };
      if (p.reversible !== undefined && p.reversible !== null) {
        if (typeof p.reversible !== "boolean") fail("invalid ask: reversible must be a boolean");
        opt.reversible = p.reversible;
      }
      if (p.approval !== undefined && p.approval !== null) {
        const q = obj(p.approval, "approval");
        known(q, APPROVAL_FIELDS, "approval");
        const ap: Record<string, unknown> = { kind: str(q, "kind"), target: str(q, "target"), identity: str(q, "identity") };
        if (q.ttl !== undefined && q.ttl !== null) {
          if (typeof q.ttl !== "number" || !Number.isInteger(q.ttl)) fail("invalid ask: ttl must be an integer");
          ap.ttl = q.ttl;
        }
        opt.approval = ap;
      }
      if (Object.hasOwn(p, "expected_outcome")) opt.expected_outcome = parseExpectedOutcome(p.expected_outcome);
      if (Object.hasOwn(p, "on_pick")) {
        if (o.kind !== "decide" || o.asker !== "thread" || o.questions) fail("on_pick requires a thread card decision");
        opt.on_pick = parseOnPick(p.on_pick, p as unknown as AskOption, { source: "card" });
      }
      return opt;
    });
  }
  if (o.questions !== undefined && o.questions !== null) {
    if (!Array.isArray(o.questions)) fail("invalid ask: questions must be an array");
    a.questions = (o.questions as unknown[]).map((raw) => {
      const q = obj(raw, "question");
      known(q, QUESTION_FIELDS, "question");
      if (q.options !== undefined && q.options !== null && !Array.isArray(q.options)) fail("invalid ask: question options must be an array");
      return {
        id: str(q, "id"),
        prompt: str(q, "prompt"),
        recommendation: str(q, "recommendation"),
        options: ((q.options ?? []) as unknown[]).map((r) => {
          const p = obj(r, "question option");
          known(p, QUESTION_OPTION_FIELDS, "question option");
          return { id: str(p, "id"), label: str(p, "label"), detail: str(p, "detail") };
        }),
      };
    });
  }
  if (o.urgency !== undefined && o.urgency !== null) {
    const u = str(o, "urgency");
    if (u !== "blocker") fail('invalid ask: urgency must be "blocker" when set');
    a.urgency = u;
  }
  const stopped = validateStopped(o, requireStopped);
  if (stopped !== undefined) a.stopped = stopped;
  if (o.steps !== undefined && o.steps !== null) {
    if (!Array.isArray(o.steps) || (o.steps as unknown[]).some((s) => typeof s !== "string")) fail("invalid ask: steps must be an array of strings");
    a.steps = [...(o.steps as string[])];
  }
  if (o.machine !== undefined && o.machine !== null) {
    const m = obj(o.machine, "machine");
    known(m, MACHINE_FIELDS, "machine");
    a.machine = { class: str(m, "class"), detail: str(m, "detail"), owner_thread: str(m, "owner_thread") };
  }
  return normalizeAsk(a as unknown as Ask);
}

interface Work {
  v: number;
  kind: string;
  request_id: string;
  ask_key: string;
  subject: string;
  project: string;
  project_root: string;
  asker: string;
  thread: string;
  question: string;
  recommendation: string;
  supersedes: string;
  mention_of: string;
  options: AskOption[];
  questions: Question[];
  steps: string[];
  machine: Machine | undefined;
  urgency: string;
}

/** One thing wrong with an ask: the field it is about and why. */
export interface Problem {
  field: string;
  why: string;
}

/** Carries every problem an ask has; the message leads with the first, the rest follow after "; also: ". */
export class ValidationError extends Error {
  readonly problems: Problem[];
  constructor(problems: Problem[]) {
    const [first, ...rest] = problems;
    super(first ? first.why + (rest.length ? `; also: ${rest.map((p) => `${p.field}: ${p.why}`).join("; ")}` : "") : "invalid ask");
    this.name = "ValidationError";
    this.problems = problems;
  }
}

/** Every problem with an ask, in the order the checks run; empty when it is valid. Unlike normalizeAsk it does not stop at the first. */
export function askProblems(input: Ask): Problem[] {
  return normalize(input).problems;
}

/** Validates an ask and returns it with defaults applied and strings NFC-normalized. Idempotent. Throws a ValidationError holding every problem. */
export function normalizeAsk(input: Ask): Ask {
  const { ask, problems } = normalize(input);
  if (problems.length > 0 || !ask) throw new ValidationError(problems);
  return ask;
}

function normalize(input: Ask): { ask?: Ask; problems: Problem[] } {
  const ps: Problem[] = [];
  const add = (field: string, why: string) => void ps.push({ field, why });
  const clean = (field: string, s: string): string => {
    if (!s.isWellFormed()) add(field, `${field} must be valid UTF-8`);
    else if (s.includes("\u0000")) add(field, `${field} must not contain NUL`);
    else if (APPROVAL_TOKEN.test(s)) add(field, "approval tokens are not accepted in Home text");
    return nfc(s);
  };
  const a: Work = {
    v: input.v as number,
    kind: input.kind as string,
    request_id: clean("request_id", input.request_id ?? ""),
    ask_key: clean("ask_key", input.ask_key ?? ""),
    subject: clean("subject", input.subject ?? ""),
    project: clean("project", input.project ?? ""),
    project_root: clean("project_root", input.project_root ?? ""),
    asker: input.asker as string,
    thread: clean("thread", input.thread ?? ""),
    question: clean("question", input.question ?? ""),
    recommendation: clean("recommendation", input.recommendation ?? ""),
    supersedes: clean("supersedes", input.supersedes ?? ""),
    mention_of: clean("mention_of", input.mention_of ?? ""),
    options: (input.options ?? []).map((o) => {
      const opt: AskOption = {
        id: clean("option id", o.id ?? ""),
        label: clean("label", o.label ?? ""),
        kind: (o.kind ?? "") as OptionKind,
        instruction: clean("instruction", o.instruction ?? ""),
      };
      if (o.expected_outcome !== undefined) opt.expected_outcome = parseExpectedOutcome(o.expected_outcome);
      if (o.on_pick !== undefined) opt.on_pick = parseOnPick(o.on_pick, o, { source: "card" });
      if (o.reversible) opt.reversible = true;
      if (o.approval) {
        opt.approval = {
          kind: clean("approval kind", o.approval.kind ?? "") as Approval["kind"],
          target: clean("approval target", o.approval.target ?? ""),
          identity: clean("approval identity", o.approval.identity ?? ""),
          ttl: o.approval.ttl ?? 0,
        };
      }
      return opt;
    }),
    questions: (input.questions ?? []).map((q) => ({
      id: clean("question id", q.id ?? ""),
      prompt: clean("question prompt", q.prompt ?? ""),
      recommendation: clean("question recommendation", q.recommendation ?? ""),
      options: (q.options ?? []).map((o) => ({ id: clean("question option id", o.id ?? ""), label: clean("question option label", o.label ?? ""), detail: clean("question option detail", o.detail ?? "") })),
    })),
    steps: (input.steps ?? []).map((s) => clean("step", s)),
    machine: input.machine
      ? {
          class: clean("machine class", input.machine.class ?? ""),
          detail: clean("machine detail", input.machine.detail ?? ""),
          owner_thread: clean("owner_thread", input.machine.owner_thread ?? ""),
        }
      : undefined,
    urgency: input.urgency === undefined ? "" : (input.urgency as string),
  };

  if (a.urgency !== "" && a.urgency !== "blocker") add("urgency", 'urgency must be "blocker" when set');
  if (a.v !== 1) add("v", "v must be 1");
  if (!["decide", "steps", "machine"].includes(a.kind)) add("kind", "kind must be decide, steps or machine");
  if (a.asker === "thread") {
    if (a.thread === "") add("thread", "thread asker needs a thread");
  } else if (a.asker === "mycroft") {
    if (a.thread !== "") add("thread", "mycroft asker takes no thread");
  } else add("asker", "asker must be thread or mycroft");
  if (a.thread !== "" && !REF_ID.test(a.thread)) add("thread", "thread must match [A-Za-z0-9:_.-]{1,128}");
  if (blank(a.project) || count(a.project) > 200) add("project", "project is required (1-200 characters)");
  if (!a.project_root.startsWith("/") || blank(a.project_root)) add("project_root", "project_root must be an absolute path");
  if (blank(a.question) || count(a.question) > MAX_QUESTION) add("question", `question must be 1-8000 characters (got ${count(a.question)})`);
  if (a.request_id !== "" && !REQUEST_ID.test(a.request_id)) add("request_id", "request_id must be 1-128 of [A-Za-z0-9:_.-]");
  for (const [field, v] of [["supersedes", a.supersedes], ["mention_of", a.mention_of]] as const) {
    if (v !== "" && !REF_ID.test(v)) add(field, `${field} must match [A-Za-z0-9:_.-]{1,128}`);
  }
  if (a.supersedes !== "" && a.mention_of !== "") add("supersedes", "supersedes and mention_of cannot be used together");

  a.ask_key = a.ask_key === "" ? firstChars(normText(a.question), MAX_KEY).replace(/ +$/, "") : normText(a.ask_key);
  if (count(a.ask_key) < 1 || count(a.ask_key) > MAX_KEY) add("ask_key", "ask_key must be 1-120 characters");
  if (a.subject !== "") {
    a.subject = normText(a.subject);
    if (count(a.subject) < 1 || count(a.subject) > MAX_KEY) add("subject", "subject must be 1-120 characters");
  }

  switch (a.kind) {
    case "decide":
      if (a.steps.length > 0 || a.machine) add("kind", "decide takes no steps or machine block");
      normalizeQuestions(a, add);
      normalizeOptions(a, add);
      break;
    case "steps":
      if (a.questions.length > 0) add("questions", "questions are only for a decide ask");
      if (a.options.length > 0) add("options", "steps takes no options");
      if (a.machine) add("machine", "steps takes no machine block");
      if (a.recommendation !== "") add("recommendation", "recommendation must name an option");
      if (a.steps.length < 1 || a.steps.length > MAX_STEPS) add("steps", "steps needs 1-20 steps");
      if (a.steps.some((s) => blank(s) || count(s) > MAX_INSTRUCTION)) add("steps", "each step must be 1-8000 characters");
      break;
    case "machine":
      if (a.questions.length > 0) add("questions", "questions are only for a decide ask");
      if (a.options.length > 0 || a.steps.length > 0) add("kind", "machine takes no steps or options");
      if (a.recommendation !== "") add("recommendation", "recommendation must name an option");
      if (!a.machine) add("machine", "machine needs a machine block");
      else {
        if (!MACHINE_CLASS.test(a.machine.class)) add("machine class", "machine class must match [a-z0-9-]{1,32}");
        if (blank(a.machine.detail) || count(a.machine.detail) > MAX_INSTRUCTION) add("machine detail", "machine detail must be 1-8000 characters");
        if (a.machine.owner_thread && !REF_ID.test(a.machine.owner_thread)) add("owner_thread", "owner_thread must match [A-Za-z0-9:_.-]{1,128}");
      }
      break;
  }

  if (ps.length > 0) return { problems: ps };

  const out: Ask = {
    v: 1,
    kind: a.kind as AskKind,
    request_id: a.request_id,
    ask_key: a.ask_key,
    subject: a.subject,
    project: a.project,
    project_root: a.project_root,
    asker: a.asker as Ask["asker"],
    thread: a.thread,
    question: a.question,
    recommendation: a.recommendation,
    supersedes: a.supersedes,
    mention_of: a.mention_of,
    options: a.options,
    steps: a.steps,
    machine: a.machine,
  };
  if (a.questions.length > 0) out.questions = a.questions;
  if (a.urgency === "blocker") out.urgency = "blocker";
  try {
    const stopped = validateStopped(input, false);
    if (stopped !== undefined) out.stopped = stopped;
  } catch (e) {
    add("stopped", (e as Error).message);
  }
  if (input.evidence !== undefined) out.evidence = validateEvidence(input.evidence);
  if (input.group !== undefined) out.group = validateGroup(input.group);
  if (input.review_by !== undefined) out.review_by = canonicalCardInstant(input.review_by);
  if (out.request_id === "") out.request_id = identityOf(out);
  return { ask: out, problems: ps };
}

function normalizeQuestions(a: Work, add: (field: string, why: string) => void) {
  if (a.questions.length > MAX_QUESTIONS) add("questions", `a multi-pick ask takes 1-${MAX_QUESTIONS} questions`);
  const ids = new Set<string>();
  a.questions.forEach((q, i) => {
    const at = `questions[${i}]`;
    if (!OPTION_ID.test(q.id)) add(`${at}.id`, "question id must match [a-z0-9-]{1,32}");
    else if (ids.has(q.id)) add(`${at}.id`, `duplicate question id "${q.id}"`);
    ids.add(q.id);
    if (blankAscii(q.prompt) || count(q.prompt) > MAX_PROMPT) add(`${at}.prompt`, `prompt must be 1-${MAX_PROMPT} characters`);
    if (q.options.length < 2 || q.options.length > 6) add(`${at}.options`, "a question needs 2-6 options");
    const seen = new Set<string>();
    q.options.forEach((o, j) => {
      const oat = `${at}.options[${j}]`;
      if (!OPTION_ID.test(o.id)) add(`${oat}.id`, "option id must match [a-z0-9-]{1,32}");
      else if (seen.has(o.id)) add(`${oat}.id`, `duplicate option id "${o.id}"`);
      seen.add(o.id);
      if (blankAscii(o.label) || count(o.label) > MAX_LABEL) add(`${oat}.label`, "label must be 1-80 characters");
      if (count(o.detail ?? "") > MAX_DETAIL) add(`${oat}.detail`, `detail must be at most ${MAX_DETAIL} characters`);
    });
    if ((q.recommendation ?? "") !== "" && !seen.has(q.recommendation as string)) add(`${at}.recommendation`, "recommendation must be the id of one of the question's options");
  });
}

function normalizeOptions(a: Work, add: (field: string, why: string) => void) {
  // A multi-pick ask may carry no overall options: its answer is the set of picks.
  if (a.questions.length > 0 && a.options.length === 0) {
    if (a.recommendation !== "") add("recommendation", "recommendation must name an option; a multi-pick ask recommends per question");
    return;
  }
  if (a.options.length < 2 || a.options.length > 6) add("options", "decide needs 2-6 options");
  const seen = new Set<string>();
  a.options.forEach((o, i) => {
    const at = `options[${i}]`;
    if (blank(o.label) || count(o.label) > MAX_LABEL) add(`${at}.label`, "label must be 1-80 characters");
    let idOK = true;
    if (!o.id) {
      o.id = slug(o.label);
      if (!o.id) {
        add(`${at}.id`, "option id is required when the label has no ASCII letters or digits");
        idOK = false;
      }
    }
    if (idOK) {
      if (!OPTION_ID.test(o.id as string)) add(`${at}.id`, "option id must match [a-z0-9-]{1,32}");
      else if (seen.has(o.id as string)) add(`${at}.id`, `duplicate option id "${o.id}"`);
      if (a.questions.length > 0 && o.id === "answers") add(`${at}.id`, 'option id "answers" is reserved in a multi-pick ask');
      seen.add(o.id as string);
    }

    if (!o.kind) {
      if (o.instruction) o.kind = "instruction";
      else if (a.asker === "mycroft") o.kind = "ruling-only";
      else o.kind = "needs-context";
    }
    if (!["instruction", "needs-context", "ruling-only"].includes(o.kind)) add(`${at}.kind`, "option kind must be instruction, needs-context or ruling-only");
    if (a.asker === "mycroft") {
      if (o.kind !== "ruling-only") add(`${at}.kind`, "mycroft may offer only ruling-only options");
      if (o.reversible) add(`${at}.reversible`, "reversible is not allowed when asker is mycroft");
    }
    if (o.kind === "instruction" && !o.instruction) add(`${at}.instruction`, "an instruction option needs an instruction");
    if (o.kind !== "instruction" && o.instruction) add(`${at}.instruction`, "instruction is only allowed on an instruction option");
    if (o.instruction && (count(o.instruction) < 1 || count(o.instruction) > MAX_INSTRUCTION)) add(`${at}.instruction`, "instruction must be 1-8000 characters");
    if (o.approval) {
      if (o.reversible) add(`${at}.reversible`, "reversible is not allowed on an option with an approval spec");
      if (!["merge", "deploy", "release"].includes(o.approval.kind)) add(`${at}.approval.kind`, "approval kind must be merge, deploy or release");
      if (blank(o.approval.target) || blank(o.approval.identity)) add(`${at}.approval`, "approval needs a target and an identity");
      const ttl = o.approval.ttl ?? 0;
      if (ttl < 0 || ttl > MAX_TTL) add(`${at}.approval.ttl`, "approval ttl must be 1-604800 seconds");
    }
  });
  if (a.recommendation !== "" && !seen.has(a.recommendation)) add("recommendation", "recommendation must name an option: it is the id of one of the options, exactly, not a sentence");
}

// ---- canonical JSON ----

type Node = string | number | boolean | null | undefined | Node[] | { [k: string]: Node };

const isEmpty = (v: Node): boolean => {
  if (v === null || v === undefined) return true;
  if (typeof v === "string") return v === "";
  if (typeof v === "boolean") return !v;
  if (typeof v === "number") return v === 0;
  if (Array.isArray(v)) return v.length === 0;
  return Object.keys(v).length === 0;
};

/** Drops empty members of objects, recursively; array elements keep their positions. */
function prune(v: Node): Node {
  if (Array.isArray(v)) return v.map(prune);
  if (v !== null && typeof v === "object") {
    const out: { [k: string]: Node } = {};
    for (const [k, e] of Object.entries(v)) {
      if (k === "on_pick") { out[k] = e; continue; }
      const p = prune(e);
      if (!isEmpty(p)) out[k] = p;
    }
    return out;
  }
  return v;
}

function write(v: Node, exact = false): string {
  if (v === null || v === undefined) return "null";
  if (typeof v === "boolean") return v ? "true" : "false";
  if (typeof v === "number") return String(v);
  if (typeof v === "string") return JSON.stringify(exact ? v : nfc(v));
  if (Array.isArray(v)) return `[${v.map(e => write(e, exact)).join(",")}]`;
  // Default string sort compares UTF-16 code units, which is the required key order.
  const keys = Object.keys(v).map(nfc).sort();
  const src = v as { [k: string]: Node };
  const lookup = new Map(Object.keys(src).map((k) => [nfc(k), src[k]]));
  return `{${keys.map((k) => `${JSON.stringify(k)}:${write(lookup.get(k) as Node, exact || k === "on_pick")}`).join(",")}}`;
}

/** Complete validated declaration: sorted keys, exact decoded strings, no pruning. */
export function canonicalOnPick(value: NonNullable<AskOption["on_pick"]>): string {
  return write(value as unknown as Node, true);
}

function canon(v: Node): string {
  return write(prune(v));
}

/** Canonicalizes arbitrary parsed JSON: sorted keys, no whitespace, NFC, empty members dropped, integers only. */
export function canonicalJson(v: unknown): string {
  const check = (x: unknown): void => {
    if (typeof x === "number" && !Number.isInteger(x)) fail(`only integers are canonical: ${x}`);
    if (Array.isArray(x)) x.forEach(check);
    else if (x !== null && typeof x === "object") Object.values(x as object).forEach(check);
  };
  check(v);
  return canon(v as Node);
}

const sha = (v: Node) => createHash("sha256").update(canon(v)).digest("hex");

function optionNode(o: AskOption): Node {
  const n: { [k: string]: Node } = { id: o.id, label: o.label, kind: o.kind, instruction: o.instruction, reversible: o.reversible ?? false };
  if (o.approval) n.approval = { kind: o.approval.kind, target: o.approval.target, identity: o.approval.identity, ttl: o.approval.ttl ?? 0 };
  if (o.expected_outcome) n.expected_outcome = o.expected_outcome;
  if (o.on_pick) n.on_pick = JSON.parse(JSON.stringify(o.on_pick));
  return n;
}
const optionsNode = (os?: AskOption[]): Node => (os ?? []).map(optionNode);
const questionsNode = (qs?: Question[]): Node =>
  (qs ?? []).map((q) => ({ id: q.id, prompt: q.prompt, recommendation: q.recommendation ?? "", options: q.options.map((o) => ({ id: o.id, label: o.label, detail: o.detail ?? "" })) }));
const machineNode = (m: Machine | undefined, withOwner: boolean): Node =>
  m ? (withOwner ? { class: m.class, detail: m.detail, owner_thread: m.owner_thread ?? "" } : { class: m.class, detail: m.detail }) : null;
const scopeNode = (a: Ask): Node => ({ project: a.project, project_root: a.project_root, asker: a.asker, thread: a.thread ?? "" });

function bodyNode(a: Ask): { [k: string]: Node } {
  return {
    v: a.v,
    kind: a.kind,
    ...(a.review_by === undefined ? {} : { review_by: a.review_by }),
    ask_key: a.ask_key,
    subject: a.subject,
    question: a.question,
    recommendation: a.recommendation,
    supersedes: a.supersedes,
    mention_of: a.mention_of,
    options: optionsNode(a.options),
    questions: questionsNode(a.questions),
    steps: a.steps ?? [],
    machine: machineNode(a.machine, true),
  };
}

/** Canonical JSON of the whole normalized ask, request_id included. */
export function normalizedJson(a: Ask): string {
  return canon({ ...bodyNode(a), request_id: a.request_id, ...(scopeNode(a) as { [k: string]: Node }) });
}

function identityOf(a: Ask): string {
  return sha({ scope: scopeNode(a), body: bodyNode(a) });
}

function tryNormalize(a: Ask): Ask | undefined {
  try {
    return normalizeAsk(a);
  } catch {
    return undefined;
  }
}

/** sha256 over the canonical {scope, body}: every field but request_id. Empty for an invalid ask. */
export function identity(a: Ask): string {
  const n = tryNormalize(a);
  return n ? identityOf(n) : "";
}

/** sha256 over the canonical decision including its scope. A pick carries the revision it saw. */
export function revision(a: Ask): string {
  const n = tryNormalize(a);
  if (!n) return "";
  return sha({
    scope: scopeNode(n),
    kind: n.kind,
    ...(n.review_by === undefined ? {} : { review_by: n.review_by }),
    question: n.question,
    options: optionsNode(n.options),
    questions: questionsNode(n.questions),
    recommendation: n.recommendation,
    supersedes: n.supersedes,
    steps: n.steps ?? [],
    machine: machineNode(n.machine, true),
  });
}

/** P-11 semantic key; empty when the ask has no subject. The scope is deliberately absent. */
export function semanticKey(a: Ask): string {
  const n = tryNormalize(a);
  if (!n || !n.subject) return "";
  return sha({
    kind: n.kind,
    subject: n.subject,
    question: normText(n.question),
    options: optionsNode(n.options),
    questions: questionsNode(n.questions),
    recommendation: n.recommendation,
    steps: n.steps ?? [],
    machine: machineNode(n.machine, false),
  });
}
