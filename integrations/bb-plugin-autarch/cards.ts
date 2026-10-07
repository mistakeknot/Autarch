// The card convention (v1): how a tasks card's description text carries a Home ask.
//
// Twin of internal/homeask/card.go. Both read the shared vectors in
// __tests__/fixtures/cards/*.json, and the error messages below are part of that
// contract. The parse is strict: any malformed block makes the whole card
// display-only, and the thrown message is the reason shown. A card is untrusted,
// mutable input.

import { createHash } from "node:crypto";
import { canonicalJson, normalizedJson, parseAsk, type Ask } from "./model.js";
import { parseMove, type Move } from "./moves.js";

export interface BlockRef {
  ref: string;
  kind: string;
  id: string;
  counted: boolean;
}

export interface RequestLine {
  key: string;
  identity: string;
}

export interface RootRun {
  script: string;
  sha256: string;
  timeout: number;
  set: string;
}

export interface Card {
  question: string;
  blocks: BlockRef[];
  request: RequestLine;
  /** "mycroft" for a threadless card, otherwise "". */
  pull: "" | "mycroft";
  root_run: RootRun | null;
  /** The optional home-move/v1 block: what mk owes. Never executed; commands are derived from its fields. */
  move: Move | null;
  /** The home-ask/v2 object as written. */
  ask: Record<string, unknown>;
}

const KEY = /^[A-Za-z0-9:_.-]{1,128}$/;
const IDENT = /^[0-9a-f]{16}$/;
const REQUEST = /^Request: (\S+) sha256:(\S+)$/;
const BLOCK_KIND = /^[a-z][a-z0-9_-]{0,31}$/;
const ROOT_SET = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/;
const ROOT_SHA = /^[0-9a-f]{64}$/;
const ROOT_TIMEOUT = /^[0-9]{1,6}$/;
const ROOT_LINE = /^([a-z0-9_]+):(.*)$/;

const V2_SCHEMA = "home-ask/v2";
const MAX_ROOT_SCRIPT = 4096;
const MAX_ROOT_TIMEOUT = 3600;
const PLACEHOLDER_THREAD = "thr_placeholder";

const V2_ALLOWED = new Set(["schema", "project", "project_root", "question", "options", "ask_key", "subject", "recommendation", "pull"]);
const V1_ONLY = new Set(["v", "kind", "thread", "asker", "request_id", "supersedes", "mention_of", "steps", "machine"]);
/** Block-ref kinds that count toward the Blocks count; others are shown but not counted. */
export const COUNTED = new Set(["bead", "thread", "project"]);

const fail = (msg: string): never => {
  throw new Error(msg);
};

/** Parses a card description. Throws (the display-only reason) on any malformed part. */
export function parseCard(description: string): Card {
  const lines = description.replaceAll("\r\n", "\n").split("\n");
  const prose: string[] = [];
  const blocksText: string[] = [];
  const requests: string[] = [];
  const askBodies: string[] = [];
  const rootBodies: string[] = [];
  const moveBodies: string[] = [];

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!;
    if (line.startsWith("```")) {
      const info = line.slice(3).trim();
      const special = info === "home-ask" || info === "root-run" || info === "home-move";
      const body: string[] = [];
      let closed = false;
      for (i++; i < lines.length; i++) {
        if (lines[i]!.replace(/[ \t]+$/, "") === "```") {
          closed = true;
          break;
        }
        body.push(lines[i]!);
      }
      if (special) {
        if (!closed) fail(`unterminated ${info} block`);
        (info === "home-ask" ? askBodies : info === "home-move" ? moveBodies : rootBodies).push(body.join("\n"));
        continue;
      }
      // An ordinary code fence is prose, and nothing inside it is a field.
      prose.push(line, ...body);
      if (closed) prose.push("```");
      continue;
    }
    if (line.startsWith("Blocks:")) blocksText.push(line.slice("Blocks:".length));
    else if (line.startsWith("Request:")) requests.push(line);
    else prose.push(line);
  }

  const question = prose.join("\n").trim();
  const blocks = parseBlocks(blocksText);

  if (requests.length === 0) fail("missing Request line");
  if (requests.length > 1) fail("more than one Request line");
  const m = REQUEST.exec(requests[0]!);
  if (!m || !KEY.test(m[1]!) || !IDENT.test(m[2]!)) fail("invalid Request line");
  const request: RequestLine = { key: m![1]!, identity: m![2]! };

  if (askBodies.length === 0) fail("missing home-ask block");
  if (askBodies.length > 1) fail("more than one home-ask block");
  const { ask, pull } = parseWireAsk(askBodies[0]!);

  if (rootBodies.length > 1) fail("more than one root-run block");
  const root_run = rootBodies.length === 1 ? parseRootRun(rootBodies[0]!) : null;

  if (moveBodies.length > 1) fail("more than one home-move block");
  const move = moveBodies.length === 1 ? parseMove(moveBodies[0]!) : null;

  const card: Card = { question, blocks, request, pull, root_run, move, ask };
  // The ask must survive the unchanged rev-4 parser, so every v1 rule (approval tokens,
  // option bounds, ...) applies to a card. The thread is a stand-in: routing comes from
  // the card's comments, never from its text.
  toV1(card, pull === "" ? PLACEHOLDER_THREAD : "");
  return card;
}

function parseBlocks(texts: string[]): BlockRef[] {
  const refs: BlockRef[] = [];
  const seen = new Set<string>();
  for (const text of texts) {
    const toks = text.split(/[ \t\n\r\f\v]+/).filter((t) => t !== "");
    if (toks.length === 0) fail("Blocks line has no refs");
    for (const tok of toks) {
      const at = tok.indexOf(":");
      const kind = at < 0 ? "" : tok.slice(0, at);
      const id = at < 0 ? "" : tok.slice(at + 1);
      if (at < 0 || !BLOCK_KIND.test(kind) || !KEY.test(id) || (kind === "thread" && !id.startsWith("thr_"))) fail(`invalid Blocks ref "${tok}"`);
      if (seen.has(tok)) continue;
      seen.add(tok);
      refs.push({ ref: tok, kind, id, counted: COUNTED.has(kind) });
    }
  }
  return refs;
}

function parseWireAsk(body: string): { ask: Record<string, unknown>; pull: "" | "mycroft" } {
  let v: unknown;
  try {
    v = JSON.parse(body);
  } catch (e) {
    return fail(`invalid home-ask JSON: ${(e as Error).message}`);
  }
  if (v === null || typeof v !== "object" || Array.isArray(v)) fail("home-ask must be a JSON object");
  const m = v as Record<string, unknown>;
  if (m.schema !== V2_SCHEMA) fail(`schema must be ${V2_SCHEMA}`);
  for (const k of Object.keys(m).sort()) {
    if (V1_ONLY.has(k)) fail(`v1-only field "${k}" in home-ask/v2`);
    if (!V2_ALLOWED.has(k)) fail(`unknown field "${k}" in home-ask/v2`);
  }
  let pull: "" | "mycroft" = "";
  if (Object.prototype.hasOwnProperty.call(m, "pull")) {
    if (m.pull !== "mycroft") fail("pull must be mycroft");
    pull = "mycroft";
  }
  return { ask: m, pull };
}

/**
 * Converts the wire ask into a rev-4 home-ask/v1 Ask through the unchanged parseAsk.
 * A thread card needs the asking thread (from askingThread); a pull:mycroft card
 * ignores the thread argument and becomes a mycroft ask.
 */
export function toV1(card: Pick<Card, "ask" | "pull">, thread: string): Ask {
  const v1: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(card.ask)) if (k !== "schema" && k !== "pull") v1[k] = v;
  v1.v = 1;
  v1.kind = "decide";
  if (card.pull === "mycroft") v1.asker = "mycroft";
  else {
    v1.asker = "thread";
    v1.thread = thread;
  }
  try {
    return parseAsk(v1);
  } catch (e) {
    return fail(`home-ask: ${(e as Error).message}`);
  }
}

function parseRootRun(body: string): RootRun {
  const vals = new Map<string, string>();
  for (const line of body.split("\n")) {
    if (line.trim() === "") continue;
    const m = ROOT_LINE.exec(line);
    if (!m) fail("invalid root-run line");
    const key = m![1]!;
    if (!["script", "sha256", "timeout", "set"].includes(key)) fail(`unknown root-run key "${key}"`);
    if (vals.has(key)) fail(`duplicate root-run key "${key}"`);
    vals.set(key, m![2]!.trim());
  }
  for (const k of ["script", "sha256", "timeout", "set"]) if (!vals.has(k)) fail("root-run needs script, sha256, timeout and set");
  const script = vals.get("script")!;
  const sha256 = vals.get("sha256")!;
  const t = vals.get("timeout")!;
  const set = vals.get("set")!;
  if (!validRootScript(script)) fail("script must be an absolute path with no .. segment or control character, at most 4096 bytes");
  if (!ROOT_SHA.test(sha256)) fail("sha256 must be 64 lowercase hex characters");
  const n = Number(t);
  if (!ROOT_TIMEOUT.test(t) || n < 1 || n > MAX_ROOT_TIMEOUT) fail("timeout must be 1-3600 seconds");
  if (!ROOT_SET.test(set)) fail("set must match ^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$");
  return { script, sha256, timeout: n, set };
}

function validRootScript(s: string): boolean {
  if (s.length === 0 || Buffer.byteLength(s, "utf8") > MAX_ROOT_SCRIPT || !s.isWellFormed() || s[0] !== "/") return false;
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    if (c < 0x20 || c === 0x7f) return false;
  }
  return !s.split("/").includes("..");
}

/** The slice of a tasks comment askingThread reads. */
export interface TaskComment {
  id: string;
  kind: string;
  threadId: string | null;
  createdAt: string;
}

/**
 * The one selector for who asked: the threadId of the earliest kind:"agent" comment,
 * ordering by (createdAt, id). It is "" when there is no agent comment, or when the
 * earliest one carries no thread. authorName is never read. The input is not modified.
 */
export function askingThread(comments: readonly TaskComment[]): string {
  let first: TaskComment | undefined;
  for (const c of comments) {
    if (c.kind !== "agent") continue;
    if (!first || c.createdAt < first.createdAt || (c.createdAt === first.createdAt && c.id < first.id)) first = c;
  }
  return first?.threadId ?? "";
}

/**
 * H5: the card fingerprint, the only hash that decides whether a valid edit is a new generation.
 * It covers everything a reader of the card sees: title, the sorted and deduplicated Blocks refs,
 * the ask as v1 (without request_id and supersedes), the root-run tuple, the Request line, the
 * asking thread and the tasks project. Two cards with the same fingerprint are the same card.
 */
export function cardFingerprint(i: { title: string; card: Card; thread: string; tasksProject: string }): string {
  const v1 = toV1({ pull: i.card.pull, ask: i.card.ask }, i.thread);
  const ask = JSON.parse(normalizedJson(v1)) as Record<string, unknown>;
  delete ask.request_id;
  delete ask.supersedes;
  const refs = [...new Set(i.card.blocks.map((b) => b.ref))].sort();
  const text = canonicalJson({
    title: i.title,
    blocks: refs,
    ask,
    root_run: i.card.root_run,
    // Only when present, so every fingerprint of a card without a move is unchanged.
    ...(i.card.move ? { move: i.card.move } : {}),
    request: i.card.request,
    asking_thread: i.thread,
    tasks_project: i.tasksProject,
  });
  return createHash("sha256").update(text).digest("hex");
}
