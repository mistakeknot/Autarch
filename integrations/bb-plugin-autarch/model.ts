// The TypeScript side of the Home ask protocol: the shared filing request
// (schema/home-ask/v1), its validation and defaults, and the three hashes
// derived from it (identity, revision, semanticKey).
//
// Twin of internal/homeask/model.go. Both must produce the byte-for-byte
// outputs in schema/home-ask/v1/vectors.json, so the rules below are written to
// match the Go code line for line (ASCII-only case folding, ASCII whitespace,
// counts in code points, UTF-16 key order).

import { createHash } from "node:crypto";

export type AskKind = "decide" | "steps" | "machine";
export type OptionKind = "instruction" | "needs-context" | "ruling-only";

export interface Approval {
  kind: "merge" | "deploy" | "release";
  target: string;
  identity: string;
  ttl?: number;
}

export interface AskOption {
  id?: string;
  label: string;
  kind?: OptionKind;
  reversible?: boolean;
  instruction?: string;
  approval?: Approval;
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

const TOP_FIELDS = ["v", "kind", "request_id", "ask_key", "subject", "project", "project_root", "asker", "thread", "question", "recommendation", "supersedes", "mention_of", "options", "questions", "steps", "machine"];
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

/** Strictly decodes a parsed JSON request (unknown fields are refused) and returns it normalized. */
export function parseAsk(input: unknown): Ask {
  const o = obj(input, "ask");
  known(o, TOP_FIELDS, "ask");
  const a: Record<string, unknown> = {};
  if (o.v !== undefined && o.v !== null) {
    if (typeof o.v !== "number") fail("invalid ask: v must be a number");
    a.v = o.v;
  } else a.v = 0;
  for (const k of ["kind", "request_id", "ask_key", "subject", "project", "project_root", "asker", "thread", "question", "recommendation", "supersedes", "mention_of"]) a[k] = str(o, k);
  if (o.options !== undefined && o.options !== null) {
    if (!Array.isArray(o.options)) fail("invalid ask: options must be an array");
    a.options = (o.options as unknown[]).map((raw) => {
      const p = obj(raw, "option");
      known(p, OPTION_FIELDS, "option");
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
  };

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
    if (blank(q.prompt) || count(q.prompt) > MAX_PROMPT) add(`${at}.prompt`, `prompt must be 1-${MAX_PROMPT} characters`);
    if (q.options.length < 2 || q.options.length > 6) add(`${at}.options`, "a question needs 2-6 options");
    const seen = new Set<string>();
    q.options.forEach((o, j) => {
      const oat = `${at}.options[${j}]`;
      if (!OPTION_ID.test(o.id)) add(`${oat}.id`, "option id must match [a-z0-9-]{1,32}");
      else if (seen.has(o.id)) add(`${oat}.id`, `duplicate option id "${o.id}"`);
      seen.add(o.id);
      if (blank(o.label) || count(o.label) > MAX_LABEL) add(`${oat}.label`, "label must be 1-80 characters");
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
      const p = prune(e);
      if (!isEmpty(p)) out[k] = p;
    }
    return out;
  }
  return v;
}

function write(v: Node): string {
  if (v === null || v === undefined) return "null";
  if (typeof v === "boolean") return v ? "true" : "false";
  if (typeof v === "number") return String(v);
  if (typeof v === "string") return JSON.stringify(nfc(v));
  if (Array.isArray(v)) return `[${v.map(write).join(",")}]`;
  // Default string sort compares UTF-16 code units, which is the required key order.
  const keys = Object.keys(v).map(nfc).sort();
  const src = v as { [k: string]: Node };
  const lookup = new Map(Object.keys(src).map((k) => [nfc(k), src[k]]));
  return `{${keys.map((k) => `${JSON.stringify(k)}:${write(lookup.get(k) as Node)}`).join(",")}}`;
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
