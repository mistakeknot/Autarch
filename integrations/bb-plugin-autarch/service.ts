// The Home decision service: file, pick once, reconcile the ruling file, wake and feed.
// Every state change goes through one Store transaction; this layer maps outcomes to
// HTTP-shaped results and never holds state of its own.
import { createHash, randomUUID } from "node:crypto";
import { buildFeed, renderFeed, type Feed } from "./feed.js";
import { identity, normalizedJson, parseAsk, revision, semanticKey, type Ask } from "./model.js";
import { estateRoot, pinRoot, renderRuling, rulingPath, writeRuling, type PinnedRoot, type Ruling } from "./ruling.js";
import type { DecisionInput, ObligationInput, ObligationRow, PickRow, Store } from "./store.js";

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
const fail = (status: number, error: string, exit: 1 | 2 | 3 = status >= 500 ? 3 : 1): FileResult => ({ ok: false, status, exit, error });
const bad = (error: string): FileResult => ({ ok: false, status: 400, exit: 2, error });
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

  /** The owner's one notice, idempotent by op, so a replayed filing repairs a crash between the two writes. */
  private ensureOwnerNotice(decisionId: string): void {
    const d = this.row(decisionId);
    if (!d || d.kind !== "machine" || !d.owner_thread) return;
    const ask = this.askOf(d);
    this.store.insertObligations(decisionId, [
      {
        id: `ob:${decisionId}:owner`,
        kind: "owner",
        recipient: d.owner_thread,
        op: `owner:${decisionId}`,
        payload: [
          `Machine blocker ${decisionId} (${ask.machine?.class ?? ""}) is yours: ${ask.machine?.detail ?? ""}`,
          `Question: ${ask.question}`,
          `Before acting, confirm with \`bb home get --id ${decisionId}\` that this blocker is still open.`,
        ].join("\n"),
      },
    ]);
  }

  async file(req: unknown, ctx: { threadId?: string } = {}): Promise<FileResult> {
    const out = await this.fileInner(req, ctx);
    if (out.ok && !out.mentioned) this.ensureOwnerNotice(out.decision_id);
    return out;
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

    const res = this.store.insertDecision(base);
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

  pick(decisionId: string, optionId: string, rev: string, pickId: string, by: string, surface: "home" | "overlay" | "cli", reason?: string): PickResult {
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
    let res;
    try {
      res = this.store.recordPick(
        { decision_id: decisionId, pick_id: pickId, option_id: optionId, revision: rev, by, surface, reason: reason ?? null, params_hash: hash },
        obligations,
      );
    } catch (e) {
      if (/UNIQUE/.test(message(e))) return { ok: false, status: 409, error: "pick id reused" };
      throw e;
    }
    if (!res.ok) {
      switch (res.reason) {
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
}
