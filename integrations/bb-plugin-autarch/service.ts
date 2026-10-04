// The Home decision service: file, pick once, reconcile the ruling file, wake and feed.
// Every state change goes through one Store transaction; this layer maps outcomes to
// HTTP-shaped results and never holds state of its own.
import { createHash, randomUUID } from "node:crypto";
import { buildFeed, renderFeed, type Feed } from "./feed.js";
import { cardFingerprint, askingThread, parseCard, toV1, type Card } from "./cards.js";
import type { Task, TaskCommentRow } from "./tasks.js";
import { identity, normalizedJson, parseAsk, revision, semanticKey, type Ask } from "./model.js";
import { pickWrites } from "./cardwrites.js";
import { estateRoot, pinRoot, renderRuling, rulingPath, writeRuling, type PinnedRoot, type Ruling } from "./ruling.js";
import type { DecisionInput, ObligationInput, ObligationRow, PickInput, PickRow, Store } from "./store.js";

export interface ProjectInfo {
  name: string;
  root: string;
  dev: number | bigint | string;
  ino: number | bigint | string;
}

export interface ServiceDeps {
  store: Store;
  projects: () => Promise<ProjectInfo[]>;
  now?: () => string;
  newId?: () => string;
  env?: Record<string, string | undefined>;
  /** Called after a pick commits, so a delivery loop can run without waiting for its timer. */
  nudge?: () => void;
}

export type FileResult =
  | { ok: true; status: 200 | 201; decision_id: string; mentioned?: true; related?: string }
  | { ok: false; status: number; exit: 1 | 2 | 3; error: string };

export type PickResult =
  | { ok: true; status: 200 | 201; pick: PickRow }
  | { ok: false; status: number; error: string };

type Row = Record<string, unknown> & {
  id: string;
  kind: string;
  project: string;
  thread: string;
  subject: string;
  body_json: string;
  revision: string;
  root_dev: string | null;
  root_ino: string | null;
  project_root: string | null;
  supersedes: string | null;
  owner_thread: string | null;
  withdrawn_at: string | null;
  resolved_at: string | null;
};

const RECONCILE_MS = 30_000;
export const ROUTING_PREFIX = "routing: ";
const OBSERVED_BACKOFF_MS = 5_000;
const CLOSED_BACKOFF_MS = 30_000;
const MAX_BACKOFF_MS = 300_000;
const UNROUTABLE_AFTER_MS = 60_000;
const APPROVAL_DEFAULT_TTL = 24 * 3600;
const APPROVAL_MAX_TTL = 7 * 24 * 3600;
const fail = (status: number, error: string, exit: 1 | 2 | 3 = status >= 500 ? 3 : 1): FileResult => ({ ok: false, status, exit, error });
const bad = (error: string): FileResult => ({ ok: false, status: 400, exit: 2, error });
const optionLabelOf = (o: { label: string }) => o.label;
const message = (e: unknown) => (e instanceof Error ? e.message : String(e));

export function isoWeek(ms: number): string {
  const d = new Date(ms);
  const day = (d.getUTCDay() + 6) % 7; // Monday = 0
  const thursday = Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate() - day + 3);
  const year = new Date(thursday).getUTCFullYear();
  const week = Math.floor((thursday - Date.UTC(year, 0, 1)) / 86_400_000 / 7) + 1;
  return `${year}-W${String(week).padStart(2, "0")}`;
}

function median(sorted: number[]): number | null {
  if (sorted.length === 0) return null;
  const m = sorted.length >> 1;
  return sorted.length % 2 ? sorted[m]! : (sorted[m - 1]! + sorted[m]!) / 2;
}

/** The shared validator's message is a vector Go also checks; Home says what mk or the vizier can do about a missing Request line. */
function shownParseError(err: string): string {
  return err === "missing Request line" ? "Request line missing: re-file the ask or restore the Request line" : err;
}

export class Service {
  readonly store: Store;
  private readonly deps: ServiceDeps;
  private timer: ReturnType<typeof setInterval> | null = null;

  constructor(deps: ServiceDeps) {
    this.deps = deps;
    this.store = deps.store;
  }

  private now(): string {
    return (this.deps.now ?? (() => new Date().toISOString()))();
  }
  private newId(): string {
    return (this.deps.newId ?? (() => `dec_${randomUUID().replace(/-/g, "").slice(0, 16)}`))();
  }
  private get db() {
    return this.store.db;
  }
  private row(id: string): Row | undefined {
    return this.store.decision(id) as Row | undefined;
  }
  private askOf(d: Row): Ask {
    return parseAsk(JSON.parse(d.body_json));
  }
  private labelOf(d: Row, optionId: string): string {
    return this.askOf(d).options?.find((o) => o.id === optionId)?.label ?? optionId;
  }
  private replacementOf(id: string): string | undefined {
    return (this.db.prepare("SELECT id FROM decisions WHERE supersedes = ?").get(id) as { id: string } | undefined)?.id;
  }

  // ---- filing ---------------------------------------------------------------

  /** The service clock, for the layers built on it. */
  time(): string {
    return this.now();
  }

  /** A fresh decision id from the service's source. */
  mintId(): string {
    return this.newId();
  }

  /** Let a delivery loop run without waiting for its timer. */
  nudge(): void {
    this.deps.nudge?.();
  }

  /** Who owns a machine blocker: the ask's own owner, else the machineOwners setting for its class. */
  machineOwner(ask: Ask): string | null {
    if (ask.kind !== "machine" || !ask.machine) return null;
    if (ask.machine.owner_thread) return ask.machine.owner_thread;
    try {
      const map = JSON.parse(this.store.setting("machineOwners") ?? "{}") as Record<string, unknown>;
      const t = map[ask.machine.class];
      return typeof t === "string" && t !== "" ? t : null;
    } catch {
      return null;
    }
  }

  /** The owner's one notice, built for the filing's own transaction. */
  private ownerNotice(decisionId: string, ask: Ask, owner: string | null): ObligationInput[] {
    if (ask.kind !== "machine" || !owner) return [];
    return [
      {
        id: `ob:${decisionId}:owner`,
        kind: "owner",
        recipient: owner,
        op: `owner:${decisionId}`,
        payload: [
          `Machine blocker ${decisionId} (${ask.machine?.class ?? ""}) is yours: ${ask.machine?.detail ?? ""}`,
          `Question: ${ask.question}`,
          `Before acting, confirm with \`bb home get --id ${decisionId}\` that this blocker is still open.`,
        ].join("\n"),
      },
    ];
  }

  file(req: unknown, ctx: { threadId?: string } = {}): Promise<FileResult> {
    return this.fileInner(req, ctx);
  }

  private async fileInner(req: unknown, ctx: { threadId?: string } = {}): Promise<FileResult> {
    let ask: Ask;
    try {
      ask = parseAsk(req);
    } catch (e) {
      return bad(message(e));
    }
    if (ask.asker === "thread" && ctx.threadId !== undefined && ctx.threadId !== ask.thread) {
      return bad("the asking thread does not match the calling thread");
    }
    const id = identity(ask);
    const requestId = ask.request_id!;

    const known = this.store.registry(requestId);
    if (known) return this.replay(known, id);

    // Scope: the estate Uqbar (G-1), or a project that serve resolves.
    let root: PinnedRoot;
    try {
      if (ask.project === "estate") {
        const dir = estateRoot(this.deps.env ?? process.env);
        if (dir !== ask.project_root) return bad("project_root must be the Uqbar directory for an estate decision");
        root = pinRoot(dir);
      } else {
        let projects: ProjectInfo[];
        try {
          projects = await this.deps.projects();
        } catch {
          return fail(503, "not-filed: project resolution unavailable", 3);
        }
        const p = projects.find((x) => x.name === ask.project);
        if (!p) return bad(`unknown project ${JSON.stringify(ask.project)}`);
        if (p.root !== ask.project_root) return bad("project_root does not match the root serve resolves for this project");
        root = { path: p.root, dev: String(p.dev), ino: String(p.ino) };
      }
    } catch (e) {
      return bad(message(e));
    }

    const base: DecisionInput = {
      id: this.newId(),
      request_id: requestId,
      identity: id,
      revision: revision(ask),
      semantic_key: semanticKey(ask),
      subject: ask.subject ?? "",
      kind: ask.kind,
      ask_key: ask.ask_key,
      project: ask.project,
      project_root: root.path,
      root_dev: root.dev,
      root_ino: root.ino,
      asker: ask.asker,
      thread: ask.thread ?? "",
      owner_thread: this.machineOwner(ask),
      body_json: normalizedJson(ask),
      supersedes: ask.supersedes || null,
      delegable: true,
    };

    if (ask.supersedes) return this.replace(base, ask, id);
    if (ask.mention_of) return this.mentionOf(ask, id, ask.mention_of, requestId);

    if (ask.asker === "thread" && base.semantic_key !== "") {
      const twin = this.db
        .prepare(
          `SELECT d.id FROM decisions d
           WHERE d.semantic_key = ? AND d.project = ? AND d.thread <> ?
             AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
             AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
             AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id)
           ORDER BY d.filed_at, d.id LIMIT 1`,
        )
        .get(base.semantic_key, ask.project, ask.thread) as { id: string } | undefined;
      if (twin) return this.mentionOf(ask, id, twin.id, requestId);
    }

    const res = this.store.insertDecision({ ...base, obligations: this.ownerNotice(base.id, ask, base.owner_thread ?? null) });
    if (!res.inserted) return this.replay(res.existing, id);
    const related = this.db
      .prepare(
        `SELECT d.id FROM decisions d WHERE d.ask_key = ? AND d.project = ? AND d.id <> ?
           AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
           AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
         ORDER BY d.filed_at, d.id LIMIT 1`,
      )
      .get(ask.ask_key, ask.project, base.id) as { id: string } | undefined;
    if (related) {
      this.store.recordEvent("related", base.id, { related: related.id });
      return { ok: true, status: 201, decision_id: base.id, related: related.id };
    }
    return { ok: true, status: 201, decision_id: base.id };
  }

  private replay(known: { identity: string; result: string; decision_id: string }, id: string): FileResult {
    if (known.identity !== id) return fail(409, "request id reused for a different decision");
    return known.result === "mention"
      ? { ok: true, status: 200, decision_id: known.decision_id, mentioned: true }
      : { ok: true, status: 200, decision_id: known.decision_id };
  }

  private replace(base: DecisionInput, ask: Ask, id: string): FileResult {
    const prevId = base.supersedes!;
    const res = this.store.insertReplacement(base, "replace");
    if (res.ok) return { ok: true, status: 201, decision_id: res.decision_id };
    if ("existing" in res) return this.replay(res.existing, id);
    const prev = this.row(prevId);
    switch (res.reason) {
      case "already-ruled": {
        const pick = this.store.pick(prevId);
        return fail(409, `already ruled: ${JSON.stringify(prev && pick ? this.labelOf(prev, pick.option_id) : "")}`);
      }
      case "already-superseded":
        return fail(409, `already superseded by ${this.replacementOf(prevId)}`);
      case "withdrawn":
        return fail(409, "withdrawn");
      default:
        if (!prev) return fail(404, "unknown decision");
        if (prev.kind !== "decide" || ask.kind !== "decide") return fail(409, "only decide asks can be superseded");
        return fail(409, "not replaceable");
    }
  }

  private mentionOf(ask: Ask, id: string, targetId: string, requestId: string): FileResult {
    if (ask.asker !== "thread") return bad("only a thread can mention a decision");
    const t = this.row(targetId);
    if (!t) return fail(409, "unknown");
    if (t.withdrawn_at) return fail(409, "withdrawn");
    if (this.replacementOf(targetId)) return fail(409, "superseded");
    if (this.store.pick(targetId) || t.resolved_at) return fail(409, "picked");
    if (t.thread === ask.thread) return fail(409, "same thread");
    if (t.project !== ask.project) return fail(409, "different project");
    const res = this.store.insertMention({ request_id: requestId, identity: id, decision_id: targetId, thread: ask.thread! });
    if (!res.inserted) return this.replay(res.existing, id);
    return { ok: true, status: 201, decision_id: targetId, mentioned: true };
  }

  // ---- picking --------------------------------------------------------------

  pick(decisionId: string, optionId: string, rev: string, pickId: string, by: string, surface: "home" | "overlay" | "cli", reason?: string, guard?: PickInput["guard"]): PickResult {
    const d = this.row(decisionId);
    if (!d) return { ok: false, status: 404, error: "unknown decision" };
    if (d.kind === "steps") return this.pickSteps(d, optionId, rev, pickId, by, surface);
    if (d.kind !== "decide") return { ok: false, status: 400, error: "only decide and steps asks take a pick" };
    let ask: Ask;
    try {
      ask = this.askOf(d);
    } catch (e) {
      return { ok: false, status: 500, error: `stored decision is unreadable: ${message(e)}` };
    }
    if (revision(ask) !== d.revision) return { ok: false, status: 500, error: "stored revision does not match the stored decision" };
    const option = ask.options?.find((o) => o.id === optionId);
    if (!option) return { ok: false, status: 400, error: `unknown option ${JSON.stringify(optionId)}` };
    if (rev !== d.revision) return { ok: false, status: 409, error: "revision differs" };

    const hash = createHash("sha256").update(`${optionId}|${rev}|${by}`).digest("hex");
    const settled = (existing: PickRow | undefined): PickResult | null => {
      if (!existing) return null;
      if (existing.pick_id === pickId) {
        return existing.params_hash === hash ? { ok: true, status: 200, pick: existing } : { ok: false, status: 409, error: "pick id reused" };
      }
      return { ok: false, status: 409, error: "already ruled" };
    };
    const prior = settled(this.store.pick(decisionId));
    if (prior) return prior;
    if (this.store.pickByPickId(pickId)) return { ok: false, status: 409, error: "pick id reused" };

    const obligations: ObligationInput[] = [{ id: `ob:${decisionId}:ruling-file`, kind: "ruling-file", op: `ruling-file:${decisionId}` }];
    if ((option.kind === "instruction" || option.kind === "needs-context" || by === "vizier") && d.thread !== "") {
      obligations.push({
        id: `ob:${decisionId}:wake:${pickId}`,
        kind: "wake",
        recipient: d.thread,
        op: `wake:${decisionId}:${pickId}`,
        payload: this.wakePayload(d, ask, option.label, option.kind ?? "", option.instruction ?? "", by),
      });
    }
    // A card generation also notifies the threads its own Blocks snapshot names (never a later edit's),
    // and queues the tasks write-backs, all in the pick's transaction.
    const isCard = d.source === "card" && typeof d.task_id === "string";
    if (isCard) obligations.push(...this.noticesFor(d, optionLabelOf(option), by));
    const cardWrites = isCard
      ? pickWrites(
          { id: decisionId, task_id: String(d.task_id) },
          { option_id: optionId, option_label: option.label, by, generation: Number(d.generation), picked_at: this.now(), reason: reason ?? null },
        )
      : [];
    // Only mk's own pick mints, and it does so in the pick's transaction. A record is not an authorization.
    let approval: PickInput["approval"];
    if (by === "mk" && option.approval) {
      const ttl = option.approval.ttl && option.approval.ttl > 0 ? option.approval.ttl : APPROVAL_DEFAULT_TTL;
      approval = {
        approval_id: `apr:${this.newId()}`,
        kind: option.approval.kind,
        target: option.approval.target,
        identity: option.approval.identity,
        expires_at: new Date(Date.parse(this.now()) + Math.min(ttl, APPROVAL_MAX_TTL) * 1000).toISOString(),
      };
    }
    let res;
    try {
      res = this.store.recordPick(
        { decision_id: decisionId, pick_id: pickId, option_id: optionId, revision: rev, by, surface, reason: reason ?? null, params_hash: hash, approval, guard },
        obligations,
        cardWrites,
      );
    } catch (e) {
      if (/UNIQUE/.test(message(e))) return { ok: false, status: 409, error: "pick id reused" };
      throw e;
    }
    if (!res.ok) {
      switch (res.reason) {
        case "guard":
          return { ok: false, status: res.refusal.status, error: res.refusal.error };
        case "superseded":
          return { ok: false, status: 409, error: `superseded by ${this.replacementOf(decisionId)}` };
        case "withdrawn":
          return { ok: false, status: 409, error: "withdrawn" };
        case "already-ruled":
          return settled(res.existing) ?? { ok: false, status: 409, error: "already ruled" };
        default:
          return { ok: false, status: 409, error: "revision differs" };
      }
    }
    try {
      this.reconcile(decisionId);
    } finally {
      this.deps.nudge?.();
    }
    return { ok: true, status: 201, pick: this.store.pick(decisionId)! };
  }

  /** One notice per distinct thread in the generation's Blocks snapshot, except the asking thread. */
  private noticesFor(d: Row, label: string, by: string): ObligationInput[] {
    const refs = this.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ? ORDER BY ref").all(d.id) as { ref: string }[];
    const threads = [...new Set(refs.map((r) => r.ref).filter((r) => r.startsWith("thread:")).map((r) => r.slice("thread:".length)))].filter((t) => t !== "" && t !== d.thread);
    return threads.map((t) => ({
      id: `ob:${d.id}:notice:${t}`,
      kind: "notice",
      recipient: t,
      op: `notice:${d.id}:${t}`,
      payload: `${by === "vizier" ? "The vizier" : "mk"} ruled on the card ${d.id}${d.subject ? ` (${d.subject})` : ""}, which names your thread: picked ${JSON.stringify(label)}. This is not merge, deploy, release or publish authorization.`,
    }));
  }

  /** Read a recorded approval. Always `authorizing: false`; there is no way to spend or consume one. */
  approvalCheck(kind: string, target: string, identity: string) {
    const r = this.store.checkApproval(kind, target, identity);
    return {
      status: r.status,
      authorizing: false as const,
      ...(r.approval ? { approval_id: r.approval.approval_id, minted_at: r.approval.minted_at, expires_at: r.approval.expires_at, revoked_at: r.approval.revoked_at } : {}),
    };
  }

  revokeApproval(approvalId: string) {
    return this.store.revokeApproval(approvalId);
  }

  /** "Done" on a steps ask: one steps-done notice per recipient (asker and mentioners), all in the pick's transaction. */
  private pickSteps(d: Row, optionId: string, rev: string, pickId: string, by: string, surface: "home" | "overlay" | "cli"): PickResult {
    if (optionId !== "done") return { ok: false, status: 400, error: "a steps ask takes only the option done" };
    if (rev !== d.revision) return { ok: false, status: 409, error: "revision differs" };
    const hash = createHash("sha256").update(`${optionId}|${rev}|${by}`).digest("hex");
    const settled = (existing: PickRow | undefined): PickResult | null => {
      if (!existing) return null;
      if (existing.pick_id === pickId) return existing.params_hash === hash ? { ok: true, status: 200, pick: existing } : { ok: false, status: 409, error: "pick id reused" };
      return { ok: false, status: 409, error: "already done" };
    };
    const prior = settled(this.store.pick(d.id));
    if (prior) return prior;
    if (this.store.pickByPickId(pickId)) return { ok: false, status: 409, error: "pick id reused" };
    const ask = this.askOf(d);
    const threads = [...new Set([d.thread, ...this.store.mentions(d.id).map((m) => m.thread)].filter((t) => t !== ""))];
    const obligations: ObligationInput[] = threads.map((t) => ({
      id: `ob:${d.id}:steps-done:${t}`,
      kind: "steps-done",
      recipient: t,
      op: `steps-done:${d.id}:${t}`,
      payload: [
        `mk finished your steps ${d.id}${d.subject ? ` (${d.subject})` : ""}.`,
        ...(ask.steps ?? []).map((s, i) => `${i + 1}. ${s}`),
      ].join("\n"),
    }));
    let res;
    try {
      res = this.store.recordPick({ decision_id: d.id, pick_id: pickId, option_id: optionId, revision: rev, by, surface, reason: null, params_hash: hash }, obligations);
    } catch (e) {
      if (/UNIQUE/.test(message(e))) return { ok: false, status: 409, error: "pick id reused" };
      throw e;
    }
    if (!res.ok) {
      if (res.reason === "already-ruled") return settled(res.existing) ?? { ok: false, status: 409, error: "already done" };
      return { ok: false, status: 409, error: res.reason === "withdrawn" ? "withdrawn" : res.reason };
    }
    this.deps.nudge?.();
    return { ok: true, status: 201, pick: this.store.pick(d.id)! };
  }

  /** A snapshot taken at pick time, so the wake never depends on rows that may change later. */
  private wakePayload(d: Row, ask: Ask, label: string, kind: string, instruction: string, by: string): string {
    const lines: string[] = [];
    if (by === "vizier") {
      lines.push(
        `The vizier ruled on your decision ${d.id} in mk's place (you marked this option reversible). mk may override it. This is not merge, deploy, release or publish authorization.`,
      );
    }
    const over = this.store.overriddenRuling(d.id);
    if (over) lines.push(`This supersedes the vizier's ruling on ${over.decision_id}.`);
    lines.push(`Ruling on ${d.id}${d.subject ? ` (${d.subject})` : ""}: ${by} picked ${JSON.stringify(label)} [${kind}].`);
    lines.push(`Question: ${ask.question}`);
    if (kind === "instruction") lines.push(`Instruction: ${instruction}`);
    else if (kind === "needs-context") lines.push("This choice needs more context from you before you act.");
    return lines.join("\n");
  }

  // ---- reconcile ------------------------------------------------------------

  /** Write the ruling file for a picked decision, then mark its obligation done. Failures back off. */
  reconcile(decisionId: string): void {
    const ob = this.store.obligationsFor(decisionId).find((o) => o.kind === "ruling-file");
    if (!ob || ob.state !== "pending") return;
    try {
      const d = this.row(decisionId)!;
      const pick = this.store.pick(decisionId);
      if (!pick) throw new Error("no pick recorded");
      const ask = this.askOf(d);
      const option = ask.options?.find((o) => o.id === pick.option_id);
      const estate = d.project === "estate";
      const ruling: Ruling = {
        decision_id: d.id,
        pick_id: pick.pick_id,
        revision: pick.revision,
        asking_thread: d.thread,
        subject: d.subject || ask.question.slice(0, 80),
        options_shown: (ask.options ?? []).map((o) => ({ id: o.id!, label: o.label })),
        picked: pick.option_id,
        ruled_by: pick.by === "vizier" ? "vizier" : "mk",
        ruled_at: pick.picked_at,
        ruling: `Picked ${JSON.stringify(option?.label ?? pick.option_id)} for: ${ask.question}`,
        source: `autarch-home:${pick.surface}`,
      };
      if (option?.kind === "instruction" && option.instruction) ruling.instruction = option.instruction;
      if (pick.by === "vizier" && pick.reason) ruling.delegated_reason = pick.reason;
      if (d.supersedes) ruling.supersedes = d.supersedes;
      if (d.source === "card" && typeof d.task_id === "string") {
        // The stored generation body is the snapshot; the live card is never consulted.
        ruling.card_id = d.task_id;
        const key = (this.db.prepare("SELECT card_key FROM cards WHERE task_id = ?").get(d.task_id) as { card_key: string | null } | undefined)?.card_key;
        if (key) ruling.card_key = key;
        ruling.generation = Number(d.generation);
      }
      const threads = this.store.mentions(d.id).map((m) => m.thread);
      if (threads.length > 0) ruling.mentions = threads;
      const target = rulingPath({ scope: estate ? "estate" : "project", decision_id: d.id, subject: ruling.subject, date: pick.picked_at.slice(0, 10) });
      writeRuling({ path: d.project_root ?? "", dev: d.root_dev ?? "", ino: d.root_ino ?? "" }, target.dirs, target.file, renderRuling(ruling));
      this.store.transition(ob.id, "pending", "done", ob.attempt);
    } catch (e) {
      const delay = Math.min(30_000 * 2 ** Math.min(ob.attempt, 20), 3_600_000);
      this.store.failObligation(ob.id, ob.attempt, message(e), new Date(Date.parse(this.now()) + delay).toISOString());
    }
  }

  reconcileAll(): void {
    for (const o of this.store.dueRulingFiles(this.now())) this.reconcile(o.decision_id);
  }

  start(): void {
    if (this.timer) return;
    this.timer = setInterval(() => this.reconcileAll(), RECONCILE_MS);
    this.timer.unref();
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
  }

  ready(): boolean {
    return true;
  }

  // ---- readers --------------------------------------------------------------

  wakes(): ObligationRow[] {
    return this.store.dueMessages(this.now());
  }

  undeliverable(): ObligationRow[] {
    return this.db.prepare("SELECT * FROM obligations WHERE state = 'undeliverable' ORDER BY rowid").all() as ObligationRow[];
  }

  failures(): ObligationRow[] {
    return this.db
      .prepare("SELECT * FROM obligations WHERE kind = 'ruling-file' AND state = 'pending' AND last_error IS NOT NULL ORDER BY rowid")
      .all() as ObligationRow[];
  }

  dismiss(decisionId: string, obligationId: string): { ok: true } | { ok: false; reason: string } {
    const o = this.store.obligation(obligationId);
    if (!o || o.decision_id !== decisionId) return { ok: false, reason: "unknown" };
    if (o.state !== "undeliverable") return { ok: false, reason: "not-undeliverable" };
    const r = this.store.transition(o.id, "undeliverable", "dismissed", o.attempt);
    return r.ok ? { ok: true } : { ok: false, reason: r.reason };
  }

  /** Open decide asks with no pick, not withdrawn, resolved or replaced. */
  owed(): Row[] {
    return this.db
      .prepare(
        `SELECT d.* FROM decisions d
         WHERE d.kind = 'decide' AND d.withdrawn_at IS NULL AND d.resolved_at IS NULL
           AND NOT EXISTS (SELECT 1 FROM picks k WHERE k.decision_id = d.id)
           AND NOT EXISTS (SELECT 1 FROM decisions r WHERE r.supersedes = d.id)
         ORDER BY d.filed_at, d.id`,
      )
      .all() as Row[];
  }

  recent(project: string, limit: number): { id: string; subject: string; kind: string; thread: string; filed_at: string; status: string }[] {
    const rows = this.db
      .prepare("SELECT * FROM decisions WHERE project = ? ORDER BY filed_at DESC, id ASC LIMIT ?")
      .all(project, limit) as Row[];
    return rows.map((d) => ({
      id: d.id,
      subject: d.subject,
      kind: d.kind,
      thread: d.thread,
      filed_at: d.filed_at as string,
      status: d.withdrawn_at
        ? "withdrawn"
        : this.replacementOf(d.id)
          ? "replaced"
          : this.store.pick(d.id)
            ? "picked"
            : d.resolved_at
              ? "resolved"
              : "open",
    }));
  }

  feed(project: string, thread: string): Feed & { text: string } {
    const f = buildFeed(this.db, project, thread, Date.parse(this.now()));
    return { ...f, text: renderFeed(f) };
  }

  stats(since: string) {
    const count = (col: string) => {
      const out: Record<string, number> = {};
      for (const r of this.db.prepare(`SELECT ${col} AS k, COUNT(*) AS n FROM picks WHERE picked_at >= ? GROUP BY ${col}`).all(since) as { k: string; n: number }[]) out[r.k] = r.n;
      return out;
    };
    const filed_per_week: Record<string, number> = {};
    for (const e of this.db.prepare("SELECT at FROM events WHERE type IN ('filed','replaced') AND at >= ? ORDER BY seq").all(since) as { at: string }[]) {
      const w = isoWeek(Date.parse(e.at));
      filed_per_week[w] = (filed_per_week[w] ?? 0) + 1;
    }
    const lat = (
      this.db
        .prepare(`SELECT k.picked_at AS p, d.filed_at AS f FROM picks k JOIN decisions d ON d.id = k.decision_id WHERE k."by" = 'mk' AND k.picked_at >= ?`)
        .all(since) as { p: string; f: string }[]
    )
      .map((r) => Date.parse(r.p) - Date.parse(r.f))
      .sort((a, b) => a - b);
    const p90 = lat.length ? lat[Math.max(0, Math.ceil(0.9 * lat.length) - 1)]! : null;
    const delegated = (this.db.prepare(`SELECT COUNT(*) AS n FROM picks WHERE "by" = 'vizier' AND picked_at >= ?`).get(since) as { n: number }).n;
    const over = this.db.prepare("SELECT at, decision_id FROM events WHERE type = 'overridden' AND at >= ?").all(since) as { at: string; decision_id: string }[];
    const gaps = over
      .map((e) => {
        const prev = (this.row(e.decision_id)?.supersedes ?? null) as string | null;
        const pk = prev ? this.store.pick(prev) : undefined;
        return pk ? Date.parse(e.at) - Date.parse(pk.picked_at) : null;
      })
      .filter((x): x is number => x !== null)
      .sort((a, b) => a - b);
    return {
      filed_per_week,
      picks_by: count(`"by"`),
      picks_by_surface: count("surface"),
      mk_latency_ms: { n: lat.length, median: median(lat), p90 },
      delegated,
      overrides: over.length,
      override_rate: delegated > 0 ? over.length / delegated : null,
      median_override_ms: median(gaps),
    };
  }

  // ---- cards (Task 2.4): ingest and the state machine, plan 1.3.4 ------------------------

  /** serve's project list, for bindings. Rejects when serve is down. */
  serveProjects(): Promise<ProjectInfo[]> {
    return this.deps.projects();
  }

  /**
   * Write `suggested` when the tasks project's name equals exactly one serve project name
   * (case-insensitive, with spaces, hyphens and underscores ignored: "After Them" is after-them; never fuzzy). No row otherwise; an existing row is never touched, because
   * only mk confirms or rejects a binding (plan 1.3.6).
   */
  suggestBinding(tasksProjectId: string, tasksProjectName: string, serve: ProjectInfo[]): void {
    const key = (n: string) => n.toLowerCase().replace(/[\s_-]+/g, "");
    const hits = serve.filter((p) => key(p.name) === key(tasksProjectName));
    if (hits.length !== 1) return;
    this.db
      .prepare("INSERT OR IGNORE INTO project_bindings(tasks_project_id, home_project, state, suggested_at) VALUES (?, ?, 'suggested', ?)")
      .run(tasksProjectId, hits[0]!.name, this.now());
  }

  /**
   * Record what the poller saw of each card (all of a poll's cards before any is materialized, so
   * the canonical card of a duplicated Request is order-independent). Never touches routing columns.
   */
  observeCards(tasks: readonly Task[]): void {
    const at = this.now();
    const up = this.db.prepare(
      `INSERT INTO cards(task_id, project_id, card_key, title, request_key, request_identity, created_at, updated_at, status, labelled, blocks_json, first_seen_at, last_seen_at, deleted_at)
       VALUES (@id, @project, @key, @title, @rkey, @rident, @created, @updated, @status, 1, @blocks, @at, @at, NULL)
       ON CONFLICT(task_id) DO UPDATE SET project_id = excluded.project_id, card_key = excluded.card_key, title = excluded.title,
         request_key = excluded.request_key, request_identity = excluded.request_identity, created_at = excluded.created_at,
         updated_at = excluded.updated_at, status = excluded.status, labelled = 1, blocks_json = excluded.blocks_json,
         last_seen_at = excluded.last_seen_at, deleted_at = NULL`,
    );
    this.store.atomically(() => {
      for (const t of tasks) {
        let rkey: string | null = null;
        let rident: string | null = null;
        let blocks: string | null = null;
        try {
          const c = parseCard(t.description);
          rkey = c.request.key;
          rident = c.request.identity;
          blocks = JSON.stringify(c.blocks.map((b) => b.ref));
        } catch {
          /* not a card yet: shown as display-only by ingestCard */
        }
        up.run({ id: t.id, project: t.projectId, key: t.key, title: t.title, rkey, rident, created: t.createdAt, updated: t.updatedAt, status: t.status, blocks, at });
      }
    });
  }

  /** Ids of cards that were seen labelled and open, for the poller's missing-card re-read. */
  knownOpenCards(): { task_id: string; project_id: string }[] {
    return this.db
      .prepare(`SELECT task_id, project_id FROM cards WHERE labelled = 1 AND deleted_at IS NULL AND status IN ('backlog','todo','in_progress','in_review')`)
      .all() as { task_id: string; project_id: string }[];
  }

  private cardRow(taskId: string): Record<string, any> | undefined {
    return this.db.prepare("SELECT * FROM cards WHERE task_id = ?").get(taskId) as Record<string, any> | undefined;
  }
  /** The current generation is derived, never stored: max(generation). */
  currentGeneration(taskId: string): Row | undefined {
    return this.db.prepare("SELECT * FROM decisions WHERE task_id = ? ORDER BY generation DESC LIMIT 1").get(taskId) as Row | undefined;
  }
  private hasPick(taskId: string): boolean {
    return !!this.db.prepare("SELECT 1 FROM picks k JOIN decisions d ON d.id = k.decision_id WHERE d.task_id = ?").get(taskId);
  }
  /** T11: the latest generation is an override of a vizier pick and is still mk's pending decision. */
  /** T10: an override generation is mk's open question; the card shows as open until mk picks. */
  reopenForOverride(taskId: string): void {
    this.patchCard(taskId, { state: "open", display_reason: null, changed_after_ruling: 0, next_check_at: null });
  }
  private overrideOpen(latest: Row): boolean {
    if (!latest.supersedes || latest.withdrawn_at || latest.resolved_at || this.store.pick(latest.id)) return false;
    return this.store.pick(latest.supersedes)?.by === "vizier";
  }
  private sig(taskId: string): string {
    const c = this.cardRow(taskId);
    const g = this.currentGeneration(taskId);
    return JSON.stringify([c?.state, c?.display_reason, c?.changed_after_ruling, g?.generation, g?.withdrawn_at]);
  }
  private patchCard(taskId: string, set: Record<string, string | number | null>): void {
    const keys = Object.keys(set);
    this.db
      .prepare(`UPDATE cards SET ${keys.map((k) => `${k} = @${k}`).join(", ")}, updated_at = @__at WHERE task_id = @__id`)
      .run({ ...set, __at: this.now(), __id: taskId });
  }
  private isoPlus(ms: number): string {
    return new Date(Date.parse(this.now()) + ms).toISOString();
  }

  /** Ingest one observed card (T1-T5, T8, T11). Never throws for a tasks read failure: `unavailable` says so (T12). */
  async ingestCard(task: Task, ctx: { comments: () => Promise<TaskCommentRow[]> }): Promise<{ changed: boolean; unavailable?: string }> {
    if (!this.cardRow(task.id)) this.observeCards([task]);
    const before = this.sig(task.id);
    const unavailable = await this.ingestInner(task, ctx);
    return unavailable ? { changed: before !== this.sig(task.id), unavailable } : { changed: before !== this.sig(task.id) };
  }

  private async ingestInner(task: Task, ctx: { comments: () => Promise<TaskCommentRow[]> }): Promise<string | void> {
    let card: Card | null = null;
    let parseErr = "";
    try {
      card = parseCard(task.description);
    } catch (e) {
      parseErr = message(e);
    }
    const row = this.cardRow(task.id)!;
    const latest = this.currentGeneration(task.id);
    if (!latest) return this.materializeNew(task, card, parseErr, ctx, row);

    if (this.hasPick(task.id)) {
      if (this.overrideOpen(latest)) return; // T11
      // T8: never a new generation after a pick.
      let changed = card === null;
      if (card) {
        try {
          changed = cardFingerprint({ title: task.title, card, thread: row.routed_thread ?? "", tasksProject: task.projectId }) !== latest.card_fp;
        } catch {
          changed = true;
        }
      }
      this.patchCard(task.id, { state: "ruled", ...(changed ? { changed_after_ruling: 1 } : {}) });
      return;
    }

    const ev = await this.evaluate(task, card, parseErr, ctx, row, latest);
    if ("unavailable" in ev) return ev.unavailable;
    if (!latest.withdrawn_at) {
      if (!ev.ok) this.invalidate(latest, ev.reason, ev.routing);
      else if (ev.fp !== latest.card_fp) this.replaceGeneration(task, ev, latest);
      else if (row.state !== "open") this.patchCard(task.id, { state: "open", display_reason: null });
    } else if (ev.ok) {
      this.reopenGeneration(task, ev, latest); // T5
    }
    // A closed card that is still invalid stays closed with its first reason.
  }

  private async evaluate(
    task: Task,
    card: Card | null,
    parseErr: string,
    ctx: { comments: () => Promise<TaskCommentRow[]> },
    row: Record<string, any>,
    latest: Row,
  ): Promise<{ ok: true; card: Card; thread: string; fp: string } | { ok: false; reason: string; routing?: boolean } | { unavailable: string }> {
    if (!card) return { ok: false, reason: shownParseError(parseErr) };
    const reg = this.db.prepare("SELECT * FROM card_requests WHERE task_id = ?").get(task.id) as { request_key: string; identity: string } | undefined;
    if (reg && (reg.request_key !== card.request.key || reg.identity !== card.request.identity)) return { ok: false, reason: "the Request line changed" };
    const project = String(card.ask.project ?? "");
    const mismatch = this.bindingMismatch(task, project);
    if (mismatch) return { ok: false, reason: mismatch };
    if (project !== latest.project || card.ask.project_root !== latest.project_root) return { ok: false, reason: "project or project_root changed" };
    const mode = card.pull === "mycroft" ? "pull" : "thread";
    if (mode !== row.routing_mode) return { ok: false, reason: `routing mode changed from ${row.routing_mode} to ${mode}`, routing: true };
    let thread = "";
    if (mode === "thread") {
      let comments: TaskCommentRow[];
      try {
        comments = await ctx.comments();
      } catch (e) {
        return { unavailable: message(e) };
      }
      thread = askingThread(comments);
      if (thread === "") return { ok: false, reason: "no agent comment names the asking thread", routing: true };
      if (thread !== row.routed_thread) return { ok: false, reason: "the asking thread changed", routing: true };
    }
    try {
      return { ok: true, card, thread, fp: cardFingerprint({ title: task.title, card, thread, tasksProject: task.projectId }) };
    } catch (e) {
      return { ok: false, reason: message(e) };
    }
  }

  /**
   * Null when the card may open. A confirmed binding must name the ask's project. A tracker project
   * with no binding, or only a name-match suggestion, fails open: the card shows in Home flagged
   * "project not bound" so a filed ask is never hidden; only mk confirms a binding. A rejected
   * binding still refuses.
   */
  private bindingMismatch(task: Task, project: string): string | null {
    const b = this.db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(task.projectId) as
      | { home_project: string; state: string }
      | undefined;
    if (!b || b.state === "suggested") return null;
    if (b.state !== "rejected" && b.home_project === project) return null;
    return `project mismatch: card in ${b.home_project}, ask targets ${project}`;
  }

  private genInput(task: Task, card: Card, thread: string, fp: string, n: number, supersedes: string | null, root: { project_root: string; root_dev: string; root_ino: string }): DecisionInput {
    const requestId = `card:${task.id}:g${n}`;
    const ask = toV1({ pull: card.pull, ask: { ...card.ask, request_id: requestId, ...(supersedes ? { supersedes } : {}) } }, thread);
    return {
      id: `card-${task.id}-g${n}`,
      request_id: requestId,
      identity: identity(ask),
      revision: revision(ask),
      semantic_key: semanticKey(ask),
      subject: ask.subject ?? "",
      kind: ask.kind,
      ask_key: ask.ask_key,
      project: ask.project,
      project_root: root.project_root,
      root_dev: root.root_dev,
      root_ino: root.root_ino,
      asker: ask.asker,
      thread: ask.thread ?? "",
      owner_thread: null,
      body_json: normalizedJson(ask),
      supersedes,
      delegable: true,
      source: "card",
      task_id: task.id,
      generation: n,
      tasks_project_id: task.projectId,
      card_fp: fp,
    };
  }

  private snapshotBlocks(decisionId: string, card: Card): void {
    const ins = this.db.prepare("INSERT OR IGNORE INTO decision_blocks(decision_id, ref) VALUES (?, ?)");
    for (const b of card.blocks) ins.run(decisionId, b.ref);
  }

  private markOpen(taskId: string): void {
    this.patchCard(taskId, { state: "open", display_reason: null, next_check_at: null, check_attempts: 0, routing_check_at: this.now() });
  }

  /** T4: withdraw the latest generation, state closed. A routing cause enters the closed-card retry set. */
  private invalidate(latest: Row, reason: string, routing = false): void {
    this.store.atomically(() => {
      if (!latest.withdrawn_at) this.store.withdrawDecision(latest.id, reason);
      this.patchCard(String(latest.task_id), {
        state: "closed",
        display_reason: (routing ? ROUTING_PREFIX : "") + reason,
        check_attempts: 0,
        next_check_at: routing ? this.isoPlus(CLOSED_BACKOFF_MS) : null,
      });
    });
  }

  /** T3: a valid edit with a new H5 fingerprint before any pick. A refused replacement falls back to T4 in the same transaction. */
  private replaceGeneration(task: Task, ev: { card: Card; thread: string; fp: string }, latest: Row): void {
    this.store.atomically(() => {
      const n = Number(latest.generation) + 1;
      const input = this.genInput(task, ev.card, ev.thread, ev.fp, n, latest.id, { project_root: String(latest.project_root), root_dev: String(latest.root_dev), root_ino: String(latest.root_ino) });
      const res = this.store.insertReplacement(input, "replace");
      if (!res.ok) {
        this.invalidate(latest, `replacement refused: ${"reason" in res ? res.reason : "request id in use"}`);
        return;
      }
      this.snapshotBlocks(input.id, ev.card);
      this.markOpen(task.id);
    });
  }

  /** T5: a closed card with no pick that is valid again opens g n+1 with no predecessor. */
  private reopenGeneration(task: Task, ev: { card: Card; thread: string; fp: string }, latest: Row): void {
    this.store.atomically(() => {
      const n = Number(latest.generation) + 1;
      const input = this.genInput(task, ev.card, ev.thread, ev.fp, n, null, { project_root: String(latest.project_root), root_dev: String(latest.root_dev), root_ino: String(latest.root_ino) });
      const res = this.store.insertDecision(input);
      if (!res.inserted) throw new Error(`card generation ${input.id} request id is in use`);
      this.snapshotBlocks(input.id, ev.card);
      this.markOpen(task.id);
    });
  }

  private setDisplay(taskId: string, reason: string): void {
    this.patchCard(taskId, { state: "display", display_reason: reason, next_check_at: null, check_attempts: 0 });
  }

  /** T2: stay observed, back off 5 s doubling to 5 min, and say why. */
  private setObserved(row: Record<string, any>, reason: string): void {
    const attempts = Number(row.check_attempts ?? 0);
    const delay = Math.min(OBSERVED_BACKOFF_MS * 2 ** Math.min(attempts, 20), MAX_BACKOFF_MS);
    this.patchCard(row.task_id, { state: "observed", display_reason: reason, check_attempts: attempts + 1, next_check_at: this.isoPlus(delay) });
  }

  private async materializeNew(task: Task, card: Card | null, parseErr: string, ctx: { comments: () => Promise<TaskCommentRow[]> }, row: Record<string, any>): Promise<string | void> {
    if (!card) return this.setDisplay(task.id, shownParseError(parseErr));
    const key = card.request.key;
    const reg = this.db.prepare("SELECT task_id FROM card_requests WHERE request_key = ?").get(key) as { task_id: string } | undefined;
    if (reg && reg.task_id !== task.id) return this.setDisplay(task.id, `duplicate Request of ${key}`);
    if (!reg) {
      const canon = this.db
        .prepare(
          `SELECT task_id FROM cards WHERE request_key = ? AND labelled = 1 AND deleted_at IS NULL
             AND status IN ('backlog','todo','in_progress','in_review') ORDER BY created_at, task_id LIMIT 1`,
        )
        .get(key) as { task_id: string } | undefined;
      if (canon && canon.task_id !== task.id) return this.setDisplay(task.id, `duplicate Request of ${key}`);
    }
    const project = String(card.ask.project ?? "");
    const mismatch = this.bindingMismatch(task, project);
    if (mismatch) {
      // No binding row yet and serve unreachable: the binding could not be suggested, so wait.
      if (!this.db.prepare("SELECT 1 FROM project_bindings WHERE tasks_project_id = ?").get(task.projectId)) {
        try {
          await this.deps.projects();
        } catch (e) {
          return this.setObserved(row, `root unverified: ${message(e)}`);
        }
      }
      return this.setDisplay(task.id, mismatch);
    }

    let thread = "";
    if (card.pull !== "mycroft") {
      let comments: TaskCommentRow[];
      try {
        comments = await ctx.comments();
      } catch (e) {
        return message(e);
      }
      thread = askingThread(comments);
      if (thread === "") {
        const waited = Date.parse(this.now()) - Date.parse(String(row.first_seen_at));
        return this.setObserved(row, waited >= UNROUTABLE_AFTER_MS ? "unroutable" : "waiting for asking thread");
      }
    }
    if (project === "estate") return this.setDisplay(task.id, "estate is not allowed on a card");
    let projects: ProjectInfo[];
    try {
      projects = await this.deps.projects();
    } catch (e) {
      return this.setObserved(row, `root unverified: ${message(e)}`);
    }
    const p = projects.find((x) => x.name === project);
    if (!p) return this.setObserved(row, `root unverified: unknown project ${JSON.stringify(project)}`);
    if (p.root !== card.ask.project_root) return this.setDisplay(task.id, "project_root does not match the root serve resolves for this project");

    let fp: string;
    let input: DecisionInput;
    try {
      fp = cardFingerprint({ title: task.title, card, thread, tasksProject: task.projectId });
      input = this.genInput(task, card, thread, fp, 1, null, { project_root: p.root, root_dev: String(p.dev), root_ino: String(p.ino) });
    } catch (e) {
      return this.setDisplay(task.id, message(e));
    }
    this.store.atomically(() => {
      const res = this.store.insertDecision(input);
      if (!res.inserted) throw new Error(`card generation ${input.id} request id is in use`);
      this.db
        .prepare("INSERT INTO card_requests(request_key, task_id, identity, registered_at) VALUES (?, ?, ?, ?)")
        .run(key, task.id, card.request.identity, this.now());
      this.snapshotBlocks(input.id, card);
      this.patchCard(task.id, {
        routing_mode: card.pull === "mycroft" ? "pull" : "thread",
        routed_thread: card.pull === "mycroft" ? null : thread,
        asking_thread: thread,
        root_state: "verified",
        root_reason: null,
      });
      this.markOpen(task.id);
    });
  }

  /** T6/T7/T11: the card was closed, unlabelled or deleted in tasks. */
  cardGone(taskId: string, why: "closed" | "unlabelled" | "deleted", status?: string): { changed: boolean } {
    const before = this.sig(taskId);
    this.store.atomically(() => {
      const row = this.cardRow(taskId);
      if (!row) return;
      this.patchCard(taskId, {
        ...(status ? { status } : {}),
        ...(why === "closed" ? {} : { labelled: 0 }),
        ...(why === "deleted" ? { deleted_at: this.now() } : {}),
      });
      const reason = why === "closed" ? "card closed in tasks" : why === "unlabelled" ? "unlabelled" : "card deleted in tasks";
      const latest = this.currentGeneration(taskId);
      if (!latest) {
        this.patchCard(taskId, { state: "closed", display_reason: reason });
        return;
      }
      if (this.hasPick(taskId)) {
        if (!this.overrideOpen(latest)) this.patchCard(taskId, { state: "ruled", display_reason: reason });
        return; // T7, or T11 (nothing at all)
      }
      if (!latest.withdrawn_at) this.store.withdrawDecision(latest.id, reason);
      this.patchCard(taskId, { state: "closed", display_reason: reason, next_check_at: null });
    });
    return { changed: before !== this.sig(taskId) };
  }

  /** Observed cards whose retry time has come (the unresolved-retry set). */
  dueObserved(limit: number): string[] {
    return (
      this.db
        .prepare(`SELECT task_id FROM cards WHERE state = 'observed' AND labelled = 1 AND deleted_at IS NULL AND (next_check_at IS NULL OR next_check_at <= ?) ORDER BY next_check_at IS NOT NULL, next_check_at, task_id LIMIT ?`)
        .all(this.now(), limit) as { task_id: string }[]
    ).map((r) => r.task_id);
  }

  /** Open thread-mode cards to revalidate, the oldest check first. An override generation or a picked card is not in the set. */
  routingDue(limit: number): string[] {
    const rows = this.db
      .prepare(`SELECT task_id FROM cards WHERE state = 'open' AND routing_mode = 'thread' AND labelled = 1 AND deleted_at IS NULL ORDER BY routing_check_at IS NOT NULL, routing_check_at, task_id`)
      .all() as { task_id: string }[];
    const out: string[] = [];
    for (const r of rows) {
      const g = this.currentGeneration(r.task_id);
      if (!g || g.withdrawn_at || this.hasPick(r.task_id)) continue;
      out.push(r.task_id);
      if (out.length >= limit) break;
    }
    return out;
  }

  openThreadCardCount(): number {
    return this.routingDue(Number.MAX_SAFE_INTEGER).length;
  }

  /** Cards T4 closed for a routing cause that may reopen (T5), whose backoff has elapsed. */
  dueRoutingClosed(limit: number): string[] {
    const rows = this.db
      .prepare(
        `SELECT task_id FROM cards WHERE state = 'closed' AND display_reason LIKE '${ROUTING_PREFIX}%' AND labelled = 1 AND deleted_at IS NULL
           AND status IN ('backlog','todo','in_progress','in_review') AND (next_check_at IS NULL OR next_check_at <= ?)
         ORDER BY next_check_at IS NOT NULL, next_check_at, task_id`,
      )
      .all(this.now()) as { task_id: string }[];
    return rows.filter((r) => !this.hasPick(r.task_id)).slice(0, limit).map((r) => r.task_id);
  }

  /** Back off a routing-closed card that stayed closed after a retry: 30 s doubling to 5 min. */
  bumpClosedRetry(taskId: string): void {
    const row = this.cardRow(taskId);
    if (!row || row.state !== "closed") return;
    const attempts = Number(row.check_attempts ?? 0) + 1;
    this.patchCard(taskId, { check_attempts: attempts, next_check_at: this.isoPlus(Math.min(CLOSED_BACKOFF_MS * 2 ** Math.min(attempts, 20), MAX_BACKOFF_MS)) });
  }

  /**
   * Re-read the comments of an open thread-mode card and compare the asking thread to the frozen
   * one. A failed read changes nothing and does not count as a check (T12).
   */
  async revalidateRouting(taskId: string, comments: () => Promise<TaskCommentRow[]>): Promise<{ changed: boolean; unavailable?: string }> {
    const row = this.cardRow(taskId);
    const latest = this.currentGeneration(taskId);
    if (!row || !latest || latest.withdrawn_at || this.hasPick(taskId) || this.overrideOpen(latest) || row.routing_mode !== "thread") return { changed: false };
    let list: TaskCommentRow[];
    try {
      list = await comments();
    } catch (e) {
      return { changed: false, unavailable: message(e) };
    }
    const before = this.sig(taskId);
    const thread = askingThread(list);
    if (thread === "" || thread !== row.routed_thread) {
      this.invalidate(latest, thread === "" ? "no agent comment names the asking thread" : "the asking thread changed", true);
    } else {
      this.patchCard(taskId, { routing_check_at: this.now() });
    }
    return { changed: before !== this.sig(taskId) };
  }

}
