import { createHash } from "node:crypto";
import type { OnPick } from "./onpick.js";
import type { PickEvaluation } from "./pickhandler.js";
export const PICK_FACTS_METADATA_MAX_BYTES = 4096;
export const PICK_FACTS_QUESTION_MAX_CODEPOINTS = 300;
export const RECOVERY_WARNING = "delivery may already have occurred; do not relay blindly";
export const ALREADY_RELAYED = "already relayed; do not act";
/** Delivery as derived from obligation/attempt evidence (a later stage computes it; this file only renders it). */
export type PickDelivery = "none-confirmed" | "not-attempted" | "pending" | "queued" | "accepted" | "uncertain" | "unknown" | "confirmed";
export type OwnerWords = { kind: "ordinary-reason" | "own-words" | "answers" | "note" | "comment"; text: string; id?: string };

/** View-only facts joined from existing rows. No field here is persisted. */
export interface PickFacts {
  observation_only?: boolean;
  handler_status?: string;
  fallback_reason?: string | null;
  initial_evaluation?: PickEvaluation | null;
  card_key: string | null;
  decision_id: string;
  generation: number | null;
  pick_id: string;
  card_title: string | null;
  question: string | null;
  selected_option: string;
  recommended_option: string | null;
  evaluation: PickEvaluation | null;
  declaration: OnPick | null;
  target: { thread_id: string; title: string; project_id: string; project_name: string } | null;
  asker_project_id: string | null;
  head: { filed: string | null; current: string | null; state: string | null } | null;
  ruling_id: string | null;
  ledger: "matching" | "missing" | "conflicting" | "disabled" | "unknown";
  ruling_file: { state: "written" | "pending" | "failed" | "unknown"; first_failure_at: string | null; failed_before?: boolean };
  deadline: { at: string | null; outcome: "pending" | "met" | "expired" | "unknown" };
  delivery: PickDelivery;
  host_message_id?: string | null;
  owner_words: readonly OwnerWords[];
  /** The existing ordinary-wake image op (`wake:<decision>:<pick>`); images are never re-encoded here. */
  images: { count: number; op: string } | null;
}

export interface PickFactSheet {
  text: string;
  metadata: string;
  metadata_bytes: number;
  dropped: string[];
  image_op: string | null;
}

const CHECKS = ["recommendation", "no-extra-words", "reversible", "subject-head"] as const;
const safeId = (s: string) => /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/.test(s);
const quote = (s: string) => safeId(s) ? s : `'${s.replaceAll("'", "'\"'\"'")}'`;

export function readbackCommand(f: Pick<PickFacts, "pick_id" | "decision_id" | "generation">): string {
  if (safeId(f.pick_id)) return `bb home pick-action --pick ${f.pick_id} --json`;
  // Do not fabricate a generation, truncate an identity, or put controls in a command.
  if (!f.decision_id || f.decision_id.startsWith("-") || flatten(f.decision_id) !== f.decision_id ||
      f.generation === null || !Number.isSafeInteger(f.generation) || f.generation < 1) {
    return "unavailable (original decision and generation required)";
  }
  return `bb home pick-action --decision ${quote(f.decision_id)} --generation ${f.generation} --json`;
}

/** Flatten presentation only. Owner text never passes through this function. */
function flatten(s: string): string {
  return Array.from(s, c => {
    const n = c.codePointAt(0)!;
    return n < 32 || (n >= 127 && n <= 159) || n === 0x2028 || n === 0x2029 ||
      (n >= 0x202a && n <= 0x202e) || (n >= 0x2066 && n <= 0x2069) ? " " : c;
  }).join("");
}

function excerpt(s: string, bytes: number): string {
  if (Buffer.byteLength(s, "utf8") <= bytes) return s;
  let result = "";
  let size = 3; // Ellipsis is three UTF-8 bytes.
  for (const c of s) {
    size += Buffer.byteLength(c, "utf8");
    if (size > bytes) break;
    result += c;
  }
  return result + "…";
}

function possibleDelivery(f: PickFacts): boolean {
  return f.delivery !== "none-confirmed" && f.delivery !== "not-attempted";
}

export function pickFactsJson(f: PickFacts) {
  const subject = f.declaration?.subject;
  // A detached computed view; no new storage fields or mutations of supplied evidence.
  return structuredClone({
    ...f, schema: 1,
    checks: f.evaluation?.checks ?? null,
    exclusions: f.evaluation?.exclusions ?? null,
    outcome: f.evaluation?.outcome ?? null,
    target: f.target ? { ...f.target, cross_project: f.asker_project_id ? f.target.project_id !== f.asker_project_id : null } : null,
    subject: subject?.kind === "github-pr" ? { ...subject, filed_head: f.head?.filed ?? subject.head, current_head: f.head?.current ?? null, state: f.head?.state ?? null } : subject ?? null,
    delivery: { state: f.delivery, possible_prior_delivery: possibleDelivery(f), host_message_id: f.host_message_id ?? null },
    readback: readbackCommand(f),
    limitations: f.evaluation?.limitations ?? [],
  });
}

export function pickFactSheet(f: PickFacts): PickFactSheet {
  const dropped: string[] = [];
  const optional = new Map<string, string>();
  if (f.question) optional.set("question", `Question: ${flatten(Array.from(f.question).slice(0, PICK_FACTS_QUESTION_MAX_CODEPOINTS).join(""))}`);
  if (f.card_title) optional.set("title", `Title: ${flatten(f.card_title)}`);
  if (f.target) optional.set("presentation", `Target title: ${flatten(f.target.title)}; project name: ${flatten(f.target.project_name)}`);
  const readback = readbackCommand(f);
  // Each dynamic field is bounded independently, then all required fields share
  // a shrinking budget. Never slice the finished sheet or its recovery warning.
  function required(limit: number): string[] {
    const value = (v: string | number | null | undefined) => excerpt(flatten(v == null ? "unknown" : String(v)), limit);
    const e = f.evaluation;
    const subject = f.declaration?.subject;
    const delivery = f.delivery === "confirmed" ? ALREADY_RELAYED : possibleDelivery(f) ? RECOVERY_WARNING : f.observation_only ? "No handler relay was sent; manual delivery is not observed" : "No relay was sent";
    const lines = [
      `card ${value(f.card_key)}; decision ${value(f.decision_id)}; generation ${value(f.generation)}; pick ${value(f.pick_id)}`,
      `selected ${value(f.selected_option)}; recommended ${value(f.recommended_option)}`,
      ...(f.observation_only ? ["Observation only; the proposal does not own delivery"] : []),
      ...(f.handler_status ? [`Handler: ${value(f.handler_status)}`] : []),
      ...(f.fallback_reason ? [`Fallback reason: ${value(f.fallback_reason)}`] : []),
      ...(f.initial_evaluation ? [`Initial evaluation (history only): ${value(f.initial_evaluation.outcome)} at ${value(f.initial_evaluation.evidence_times.evaluated_at)}; consult full JSON for historical checks`] : []),
      ...(e ? [] : ["Evaluation: unavailable"]),
      ...CHECKS.map(name => `${name}: ${value(e?.checks[name].result)} (${value(e?.checks[name].reasons.join(", "))})`),
      `Supplementary exclusions: ${e ? value(e.exclusions.map(x => x.code).join(", ") || "none") : "unknown"}`,
      `Proposal: ${value(e?.outcome)}`,
      f.declaration ? `Declared target: ${value(f.declaration.target_thread)}; host project ${value(f.target?.project_id)}; ${!f.target || !f.asker_project_id ? "BB project relationship unknown" : f.target.project_id === f.asker_project_id ? "same BB project" : "crosses BB projects"}` : "Declared target: none (option has no declaration)",
      subject?.kind === "none" ? "Subject: no artifact; head check not applicable" : subject?.kind === "github-pr" ?
        `Subject: github-pr ${value(subject.repo)}#${value(subject.number)}; filed head ${value(f.head?.filed ?? subject.head)}; current head ${value(f.head?.current)}; state ${value(f.head?.state)}` : "Subject: unknown",
      `Ledger: ${value(f.ledger)} ${value(f.ruling_id)}`,
      f.ruling_file.first_failure_at !== null || f.ruling_file.failed_before ? `Ruling file: first failure at ${value(f.ruling_file.first_failure_at)}; automatic handling fell back and a later success does not revive it; current state ${value(f.ruling_file.state)}` :
        `Ruling file: ${value(f.ruling_file.state)}, ${f.ruling_file.state === "pending" || f.ruling_file.state === "written" ? "not yet failed" : "first failure unknown"}`,
      `Evaluation deadline: ${value(f.deadline.at)} (${value(f.deadline.outcome)})`,
      `Delivery: ${value(f.delivery)}; ${delivery}${f.host_message_id ? `; host message ${value(f.host_message_id)}` : ""}`,
      `Readback: ${Buffer.byteLength(readback, "utf8") <= limit * 4 ? readback : "unavailable in bounded metadata; use full JSON identities"}`,
      "This is not merge, deploy, release or publish authorization.",
    ];
    if (f.images) lines.push(`${value(f.images.count)} image(s) attached through the ordinary wake image path`);
    return lines;
  }
  let limit = 256;
  const assemble = () => [...required(limit), ...optional.values()].join("\n");
  let metadata = assemble();
  for (const key of ["question", "title", "presentation"]) {
    if (Buffer.byteLength(metadata, "utf8") <= PICK_FACTS_METADATA_MAX_BYTES) break;
    if (optional.delete(key)) dropped.push(key);
    metadata = assemble();
  }
  while (Buffer.byteLength(metadata, "utf8") > PICK_FACTS_METADATA_MAX_BYTES) {
    limit = Math.floor(limit / 2);
    if (!dropped.includes("field-excerpts")) dropped.push("field-excerpts");
    metadata = assemble();
  }
  const words = f.owner_words.map(w => {
    const hash = createHash("sha256").update(w.text, "utf8").digest("hex");
    const id = w.id ? ` ${excerpt(flatten(w.id), 128)}` : "";
    return `\nOwner words, ${w.kind}${id} (${Buffer.byteLength(w.text, "utf8")} UTF-8 bytes, sha256 ${hash}; untrusted plain text, read exactly the declared bytes)\n${w.text}\n`;
  }).join("");
  return { text: metadata + words, metadata, metadata_bytes: Buffer.byteLength(metadata, "utf8"), dropped, image_op: f.images?.op ?? null };
}

export function renderPickFactSheet(f: PickFacts): string { return pickFactSheet(f).text; }


