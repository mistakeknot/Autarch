import type { AskOption } from "./model.js";

export type OnPick = {
  schema: 1;
  target_thread: string;
  reversible: boolean;
  subject: { kind: "none" } | { kind: "github-pr"; repo: string; number: number; head: string };
  ruling: { bead: string | null; under: string[] };
  notification?: string;
};

export interface CardValidationContext {
  source: "card" | "home";
  rootRun?: boolean;
  move?: boolean;
  artifact?: boolean;
}

// Keep in parity with on_pick.go. Screening never modifies the filed payload.
export const ON_PICK_DENY = ["merge", "deploy", "release", "publish", "root", "admin", "force[\\s_-]*push"];
const fail = (reason: string): never => { throw new Error(`on_pick: ${reason}`); };
function object(raw: unknown, fields: string[], required = fields): Record<string, unknown> {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return fail("expected object");
  const o = raw as Record<string, unknown>;
  if (Object.keys(o).some(k => !fields.includes(k)) || required.some(k => !Object.hasOwn(o, k))) fail("missing or unknown member");
  return o;
}
function text(raw: unknown, max: number): string {
  if (typeof raw !== "string" || !raw.isWellFormed() || !raw.trim() || [...raw].length > max || raw.includes("\0")) return fail("invalid text");
  return raw;
}
export function parseOnPick(raw: unknown, option: AskOption, context: CardValidationContext): OnPick {
  if (context.source !== "card") fail("card source required");
  const o = object(raw, ["schema", "target_thread", "reversible", "subject", "ruling", "notification"], ["schema", "target_thread", "reversible", "subject", "ruling"]);
  if (o.schema !== 1 || typeof o.reversible !== "boolean") fail("unsupported schema or reversibility");
  const target = text(o.target_thread, 128);
  if (!/^thr_[A-Za-z0-9_.:-]+$/.test(target)) fail("concrete target thread required");
  const kind = option.kind || (option.instruction ? "instruction" : "ruling-only");
  if (!["instruction", "ruling-only"].includes(kind) || Object.hasOwn(option, "approval") || context.rootRun || context.move) fail("excluded option or card");
  let notification: string | undefined;
  if (kind === "instruction") {
    if (Object.hasOwn(o, "notification")) fail("instruction forbids notification");
  } else notification = text(o.notification, 8000);
  const relay = text(kind === "instruction" ? option.instruction : notification, 8000);
  if (Buffer.byteLength(relay, "utf8") > 32768 || relay.normalize("NFC") !== relay) fail("relay text would change");
  const screened = [option.label, option.instruction ?? "", notification ?? ""].join("\n").normalize("NFKC").replace(/[\u2010-\u2015\u2212\ufe58\ufe63\uff0d]/g, "-");
  if (ON_PICK_DENY.some(pattern => new RegExp(pattern, "i").test(screened))) fail("privileged text excluded");
  const s = object(o.subject, ["kind", "repo", "number", "head"], ["kind"]);
  let subject: OnPick["subject"];
  if (s.kind === "none") {
    object(s, ["kind"]);
    if (context.artifact) fail("none contradicts artifact evidence");
    subject = { kind: "none" };
  } else {
    object(s, ["kind", "repo", "number", "head"]);
    if (s.kind !== "github-pr" || typeof s.repo !== "string" || !/^[A-Za-z0-9_.-]{1,100}\/[A-Za-z0-9_.-]{1,100}$/.test(s.repo) || !Number.isSafeInteger(s.number) || Number(s.number) <= 0 || typeof s.head !== "string" || !/^[0-9a-f]{40}$/.test(s.head)) fail("invalid PR subject");
    subject = { kind: "github-pr", repo: s.repo as string, number: s.number as number, head: s.head as string };
  }
  const r = object(o.ruling, ["bead", "under"]);
  const bead = r.bead === null ? null : text(r.bead, 128);
  if (!Array.isArray(r.under) || r.under.length > 16) fail("invalid under references");
  const under = (r.under as unknown[]).map(v => text(v, 128));
  return { schema: 1, target_thread: target, reversible: o.reversible as boolean, subject, ruling: { bead, under }, ...(notification !== undefined ? { notification } : {}) };
}
