// Delegated rulings and superseding overrides (P-10) [D-1] [D-2] [D-3].
//
// The vizier's identity is an advisory claim (ctx.threadId); authenticated separation
// is gate G-15. What is enforced here is the bounded policy: a delegated ruling can
// only be a reversible option on a thread-asked, delegable decision, in an enabled
// project, under the daily cap, with no approval spec, while no settings change is
// unseen by mk. Settings are written only by setDelegation (panel RPC); no CLI verb
// reaches it.
import { identity, normalizedJson, parseAsk, revision, semanticKey } from "./model.js";
import { overrideWrites } from "./cardwrites.js";
import type { DecisionInput } from "./store.js";
import type { PickResult, Service } from "./service.js";

type Ctx = { threadId?: string };
type Fail = { ok: false; status: number; error: string };
export type DelegationSettings = { vizierThreadId: string; projects: string[]; dailyCap: number };
export type OverrideResult = { ok: true; decision_id: string; replay: boolean } | Fail;
export interface Pinned {
  /** The seen-row key: `ruling:<decision>`, `delegation-settings:<seq>` or `vizier-adopted:<seq>`. */
  item: string;
  decision?: string;
  at: string;
  /** Set for a row that is neither a ruling nor a settings change. */
  text?: string;
}
/** The slice of a bb thread the vizier resolver reads (`bb.sdk.threads.list`). */
export interface ThreadRow {
  id: string;
  title: string | null;
  pinnedAt: number | null;
  archivedAt: number | null;
  deletedAt?: number | null;
}
/** Every thread. `cached` may serve a recent answer; authority checks never ask for it. */
export type ThreadLister = (opts?: { cached?: boolean }) => Promise<ThreadRow[]>;
/** One thread by id: null when there is none. May throw. */
export type ThreadGetter = (id: string) => Promise<ThreadRow | null>;
/** The same prefix and rule as `~/.local/bin/vizier-tell`. */
export const VIZIER_TITLE_PREFIX = "Masaq' | vizier";
export type VizierResolution = { ok: true; id: string; via: "stored" | "adopted" | "candidate" } | Fail;

const MK = "mk";
const DAY_MS = 86_400_000;
const APPROVAL_TOKEN = /APPROVED-(MERGE|DEPLOY|RELEASE)/;
const THREAD_ID = /^[A-Za-z0-9:_.-]{1,128}$/;
const refuse = (status: number, error: string): Fail => ({ ok: false, status, error });
const alive = (t: ThreadRow) => t.archivedAt == null && t.deletedAt == null;
const liveThread = (rows: readonly ThreadRow[], id: string) => rows.some((t) => t.id === id && alive(t));

/**
 * Q5 ruling (plan 1.3.6): a card is delegable only when its tasks project is bound to the ask's Home
 * project by a binding mk confirmed. A name match (`suggested`) allows mk's picks only. The one
 * place to change if mk rules otherwise. Null means allowed; a home-filed decision has no binding.
 */
export function bindingRefusal(db: { prepare(sql: string): { get(...a: unknown[]): unknown } }, d: { source?: unknown; tasks_project_id?: unknown; project: string }): string | null {
  if (d.source !== "card") return null;
  const b = db.prepare("SELECT home_project, state FROM project_bindings WHERE tasks_project_id = ?").get(d.tasks_project_id) as { home_project: string; state: string } | undefined;
  if (!b) return "delegation is not enabled for this card: no confirmed project binding";
  if (b.state === "rejected") return "delegation is not enabled for this card: the project binding was rejected";
  if (b.state !== "confirmed") return "delegation is not enabled for this card: binding unconfirmed";
  if (b.home_project !== d.project) return "delegation is not enabled for this card: the card targets a project other than its binding";
  return null;
}

export class Delegation {
  constructor(
    readonly svc: Service,
    private readonly threads?: ThreadLister,
    private readonly getThread?: ThreadGetter,
  ) {}

  /**
   * Plan 1.3.6: delegation settings name Home projects, and each name is re-checked against serve's
   * current list at open. Until a check succeeds in this process, `rule` refuses (fail closed). In
   * memory on purpose: a restart checks again.
   */
  private verified = false;
  get projectsVerified(): boolean {
    return this.verified;
  }

  /**
   * The name check against a project list serve returned. A name serve does not know (exact match; no
   * case folding, no prefixes) leaves `delegation.projects` for `delegation.projects_inactive`, with a
   * `delegation-settings-migrated` event, so Settings can show it. Narrowing only: it is not a settings
   * change mk must re-approve. Idempotent.
   */
  applyProjectList(names: readonly string[]): { moved: string[] } {
    const known = new Set(names);
    const moved: string[] = [];
    this.db
      .transaction(() => {
        const current = this.settings().projects;
        if (current) {
          const kept = current.filter((n) => known.has(n));
          moved.push(...current.filter((n) => !known.has(n)));
          if (moved.length > 0) {
            let was: string[] = [];
            try {
              const v = JSON.parse(this.store.setting("delegation.projects_inactive") ?? "[]");
              if (Array.isArray(v)) was = v.filter((x): x is string => typeof x === "string");
            } catch {
              /* unreadable: start over */
            }
            this.store.setSetting("delegation.projects", JSON.stringify(kept));
            this.store.setSetting("delegation.projects_inactive", JSON.stringify([...new Set([...was, ...moved])]));
            this.store.recordEvent("delegation-settings-migrated", null, { moved, kept });
          }
        }
      })
      .immediate();
    this.verified = true;
    return { moved };
  }

  /** Ask serve for its projects and apply them. A failure leaves delegation refused; the caller retries. */
  async verifyProjects(list: () => Promise<{ name: string }[]>): Promise<boolean> {
    let projects: { name: string }[];
    try {
      projects = await list();
    } catch {
      return false;
    }
    this.applyProjectList(projects.map((p) => p.name));
    return true;
  }

  private get store() {
    return this.svc.store;
  }
  private get db() {
    return this.store.db;
  }

  // ---- settings ---------------------------------------------------------------

  settings(): Partial<DelegationSettings> {
    const out: Partial<DelegationSettings> = {};
    const v = this.store.setting("vizierThreadId");
    if (v) out.vizierThreadId = v;
    try {
      const p = JSON.parse(this.store.setting("delegation.projects") ?? "null");
      if (Array.isArray(p)) out.projects = p.filter((x): x is string => typeof x === "string");
    } catch {
      /* unreadable means not enabled */
    }
    const cap = Number(this.store.setting("delegation.dailyCap"));
    if (Number.isInteger(cap) && cap >= 0) out.dailyCap = cap;
    return out;
  }

  /** Panel RPC only: a thread caller is refused. Suspends `rule` until mk has seen the change [D-1]. */
  setDelegation(s: DelegationSettings, ctx: Ctx): { ok: true; item: string } | Fail {
    if (ctx.threadId !== undefined) return refuse(403, "delegation settings change only from the panel");
    if (typeof s.vizierThreadId !== "string" || !THREAD_ID.test(s.vizierThreadId)) return refuse(400, "vizierThreadId must be a thread id");
    if (!Array.isArray(s.projects) || s.projects.some((p) => typeof p !== "string" || p === "")) return refuse(400, "projects must be a list of project names");
    if (!Number.isInteger(s.dailyCap) || s.dailyCap < 0 || s.dailyCap > 1000) return refuse(400, "dailyCap must be an integer from 0 to 1000");
    this.db
      .transaction(() => {
        this.store.setSetting("vizierThreadId", s.vizierThreadId);
        this.store.setSetting("delegation.projects", JSON.stringify(s.projects));
        this.store.setSetting("delegation.dailyCap", String(s.dailyCap));
        this.store.recordEvent("delegation-settings-changed", null, { vizierThreadId: s.vizierThreadId, projects: s.projects, dailyCap: s.dailyCap });
      })
      .immediate();
    return { ok: true, item: this.latestSettingsItem()! };
  }

  /** The seen-row key of the latest settings change. */
  latestSettingsItem(): string | undefined {
    const r = this.db.prepare("SELECT seq FROM events WHERE type = 'delegation-settings-changed' ORDER BY seq DESC LIMIT 1").get() as { seq: number } | undefined;
    return r ? `delegation-settings:${r.seq}` : undefined;
  }

  // ---- who the vizier is ---------------------------------------------------------

  /**
   * The one answer to "which thread is the vizier", for every check (bind, unbind, note, rule, the
   * panel). The stored `vizierThreadId` stays primary while it names a live thread (checked fresh, not
   * from a cache). When it is unset, or names an archived, deleted or unknown thread, the single pinned,
   * unarchived thread whose title starts with "Masaq' | vizier" is adopted: stored, and recorded as a
   * `vizier-adopted` event, which suspends delegated `rule` until mk has seen it (D-1). Zero or several
   * matches refuse. Fail closed: if the stored thread cannot be confirmed (threads cannot be listed) or
   * the listing changed while it was read, nobody is the vizier until a retry succeeds. The adoption
   * write only lands if the stored id is still the one the decision was based on. Identity stays an
   * advisory claim; this only fixes whose claim counts. `adopt: false` is a read-only peek (the panel):
   * it may show a candidate and never writes.
   */
  async resolveVizier(opts: { adopt?: boolean } = {}): Promise<VizierResolution> {
    const adopt = opts.adopt !== false;
    const stored = this.store.setting("vizierThreadId");
    if (stored) {
      const live = await this.isLive(stored, !adopt);
      if (live === undefined) return refuse(503, "threads could not be listed, so the vizier thread cannot be confirmed");
      if (live) return { ok: true, id: stored, via: "stored" };
    }
    const scan = await this.pinnedMatches(!adopt);
    if (!scan) return refuse(503, "threads could not be listed, so the vizier thread cannot be confirmed");
    if (adopt) {
      // Offset pages have no shared snapshot: both scans must return the very same ordered thread list.
      const second = await this.pinnedMatches(false);
      if (!second || second.all !== scan.all || second.ids.join() !== scan.ids.join()) return refuse(409, "the thread list changed while it was read; try again");
    }
    const first = scan.ids;
    const why = stored ? `the stored vizier thread ${stored} is archived or gone` : "no vizier thread is set";
    if (first.length === 0) return refuse(409, `${why}, and no pinned, unarchived thread has a title starting with "${VIZIER_TITLE_PREFIX}"`);
    if (first.length > 1) return refuse(409, `${why}, and ${first.length} pinned, unarchived threads match "${VIZIER_TITLE_PREFIX}": ${first.join(", ")}`);
    const to = first[0]!;
    if (!adopt) return { ok: true, id: to, via: "candidate" };
    const reason = stored ? "stored-thread-archived-or-gone" : "unset";
    return this.db
      .transaction((): VizierResolution => {
        const now = this.store.setting("vizierThreadId");
        if (now !== stored) return refuse(409, "the vizier changed during adoption; try again");
        this.store.setSetting("vizierThreadId", to);
        this.store.recordEvent("vizier-adopted", null, { from: stored ?? null, to, reason });
        return { ok: true, id: to, via: "adopted" };
      })
      .immediate();
  }

  /** `bb home handoff <thr_id>`: only the current vizier may name its successor. Not a settings change: no suspension. */
  async handoff(to: string, ctx: Ctx): Promise<{ ok: true; from: string; to: string } | Fail> {
    if (typeof to !== "string" || !THREAD_ID.test(to) || !to.startsWith("thr_")) return refuse(400, "the successor must be a thread id like thr_abc123");
    const cur = await this.resolveVizier();
    if (!cur.ok) return cur;
    if (ctx.threadId === undefined || ctx.threadId !== cur.id) return refuse(403, "only the vizier thread may hand off");
    if (to === ctx.threadId) return refuse(400, "a handoff names another thread, not the caller");
    // Two liveness reads cannot both be last. The caller's is last (its authority is what is being exercised);
    // a successor archived in the instant between the two leaves an archived vizier, which the fallback adoption
    // repairs on the next vizier-only command.
    const live = await this.isLive(to, false);
    if (live === undefined) return refuse(503, "threads could not be listed, so the successor cannot be checked");
    if (!live) return refuse(404, `thread ${to} is archived, deleted or unknown`);
    if ((await this.isLive(cur.id, false)) !== true) return refuse(409, "the vizier thread is no longer live; try again");
    return this.db
      .transaction((): { ok: true; from: string; to: string } | Fail => {
        if (this.store.setting("vizierThreadId") !== cur.id) return refuse(409, "the vizier changed during the handoff; try again");
        this.store.setSetting("vizierThreadId", to);
        this.store.recordEvent("vizier-handoff", null, { from: cur.id, to, by: ctx.threadId });
        return { ok: true, from: cur.id, to };
      })
      .immediate();
  }

  /** True for the stored vizier id, read synchronously: a write path re-checks this right before it writes. */
  isStoredVizier(threadId: string | undefined): boolean {
    const v = this.store.setting("vizierThreadId");
    return v !== undefined && threadId === v;
  }

  /**
   * Live = exists, not archived, not deleted (fresh unless `cached`). Undefined when neither a direct read nor the list can say.
   * A write path awaits this, then compares `isStoredVizier` and writes in the SAME synchronous continuation.
   */
  async isLive(id: string, cached: boolean): Promise<boolean | undefined> {
    if (this.getThread) {
      try {
        const t = await this.getThread(id);
        return t ? alive(t) : false;
      } catch {
        /* a missing thread throws like a failure does; the full list tells them apart */
      }
    }
    const rows = await this.listThreads(cached);
    return rows ? liveThread(rows, id) : undefined;
  }

  /** Ids of the pinned, unarchived threads titled like the vizier, sorted; null when threads cannot be listed. */
  private async pinnedMatches(cached: boolean): Promise<{ ids: string[]; all: string } | null> {
    const rows = await this.listThreads(cached);
    if (!rows) return null;
    const ids = [...new Set(rows.filter((t) => alive(t) && typeof t.title === "string" && t.title.startsWith(VIZIER_TITLE_PREFIX) && t.pinnedAt != null).map((t) => t.id))].sort();
    return { ids, all: rows.map((t) => t.id).join() };
  }

  private async listThreads(cached: boolean): Promise<ThreadRow[] | null> {
    if (!this.threads) return null;
    try {
      return await this.threads({ cached });
    } catch {
      return null;
    }
  }

  /** Adoptions mk has not seen yet, as seen-row keys. */
  private unseenAdoptions(): { item: string; at: string; to: string }[] {
    const out: { item: string; at: string; to: string }[] = [];
    for (const e of this.db.prepare("SELECT seq, at, detail_json FROM events WHERE type = 'vizier-adopted' ORDER BY seq").all() as { seq: number; at: string; detail_json?: string }[]) {
      const item = `vizier-adopted:${e.seq}`;
      if (this.store.hasSeen(MK, item)) continue;
      let to = "";
      try {
        to = String((JSON.parse(e.detail_json ?? "{}") as { to?: string }).to ?? "");
      } catch {
        /* the row still pins */
      }
      out.push({ item, at: e.at, to });
    }
    return out;
  }

  /** True while a settings change or a fallback adoption is unseen by mk: delegated `rule` is refused [D-1]. */
  suspendedNow(): boolean {
    return this.suspended();
  }

  markSeen(account: string, item: string): void {
    this.store.markSeen(account, item);
  }

  private suspended(): boolean {
    const item = this.latestSettingsItem();
    return (item !== undefined && !this.store.hasSeen(MK, item)) || this.unseenAdoptions().length > 0;
  }

  /** Everything catch-up keeps in front of mk until mk's account has a seen row for it. */
  pinned(): Pinned[] {
    const out: Pinned[] = [];
    for (const e of this.db.prepare("SELECT seq, at FROM events WHERE type = 'delegation-settings-changed' ORDER BY seq").all() as { seq: number; at: string }[]) {
      const item = `delegation-settings:${e.seq}`;
      if (!this.store.hasSeen(MK, item)) out.push({ item, at: e.at });
    }
    for (const a of this.unseenAdoptions()) out.push({ item: a.item, at: a.at, text: `Home adopted ${a.to} as the vizier thread; delegated rulings stay suspended until you see this.` });
    for (const p of this.db.prepare(`SELECT decision_id, picked_at FROM picks WHERE "by" = 'vizier' ORDER BY picked_at, rowid`).all() as { decision_id: string; picked_at: string }[]) {
      const item = `ruling:${p.decision_id}`;
      if (!this.store.hasSeen(MK, item)) out.push({ item, decision: p.decision_id, at: p.picked_at });
    }
    return out;
  }

  // ---- rule -------------------------------------------------------------------

  /** `rule` after the resolver has settled who the vizier is (it may adopt a fallback first). */
  async ruleResolved(decisionId: string, optionId: string, reason: string, ctx: Ctx): Promise<PickResult> {
    const r = await this.resolveVizier();
    if (!r.ok) return r;
    if (r.id !== ctx.threadId) return refuse(403, "only the vizier thread may rule");
    return this.rule(decisionId, optionId, reason, ctx);
  }

  /** The checks in `rule` read the stored id, which is the resolver's output: `resolveVizier` runs first on every caller path. */
  rule(decisionId: string, optionId: string, reason: string, ctx: Ctx): PickResult {
    const d = this.store.decision(decisionId) as
      | { id: string; kind: string; asker: string; project: string; delegable: number; revision: string; body_json: string; request_id: string; source?: string; tasks_project_id?: string | null }
      | undefined;
    if (!d) return refuse(404, "unknown decision");
    const cfg = this.settings();
    if (!cfg.vizierThreadId || ctx.threadId !== cfg.vizierThreadId) return refuse(403, "only the vizier thread may rule");
    const pickId = `rule:${decisionId}`;
    const prior = this.store.pick(decisionId);
    if (prior && prior.pick_id === pickId && prior.by === "vizier") return this.svc.pick(decisionId, optionId, d.revision, pickId, "vizier", "cli", reason);
    if (d.kind !== "decide" || d.asker !== "thread") return refuse(403, "only a decision asked by a thread can be delegated");
    if (!d.delegable || d.request_id.startsWith("override:")) return refuse(403, "this decision is not delegable");
    const option = parseAsk(JSON.parse(d.body_json)).options?.find((o) => o.id === optionId);
    if (!option) return refuse(400, `unknown option ${JSON.stringify(optionId)}`);
    if (option.approval) return refuse(403, "an option with an approval spec cannot be delegated");
    if (!option.reversible) return refuse(403, "only a reversible option can be delegated");
    const why = reason?.trim() ?? "";
    if (why === "" || [...why].length > 500) return refuse(400, "a reason of 1-500 characters is required");
    if (APPROVAL_TOKEN.test(reason)) return refuse(400, "a reason may not carry an approval token");
    // Settings, suspension and the daily cap are decided inside the pick's write
    // transaction, so two plugin instances cannot each see the last slot.
    const guard = (): { status: number; error: string } | null => {
      if (!this.verified) return { status: 403, error: "delegation is refused: the project list is not yet verified against serve" };
      const now = this.settings();
      if (!now.vizierThreadId || ctx.threadId !== now.vizierThreadId) return { status: 403, error: "only the vizier thread may rule" };
      if (!(now.projects ?? []).includes(d.project)) return { status: 403, error: "delegation is not enabled for this project" };
      const unbound = bindingRefusal(this.db, d);
      if (unbound) return { status: 403, error: unbound };
      if (this.suspended()) return { status: 403, error: "delegation is suspended: a settings change is unseen by mk" };
      const since = new Date(Date.parse(this.svc.time()) - DAY_MS).toISOString();
      const used = (this.db.prepare(`SELECT COUNT(*) AS n FROM picks WHERE "by" = 'vizier' AND picked_at >= ?`).get(since) as { n: number }).n;
      if (used >= (now.dailyCap ?? 0)) return { status: 403, error: "the daily delegation cap is reached" };
      return null;
    };
    return this.svc.pick(decisionId, optionId, d.revision, pickId, "vizier", "cli", why, guard);
  }

  // ---- override ---------------------------------------------------------------

  /** Panel only. Supersede a delegated ruling and void its wake, in one transaction [D-3] [G-2]. */
  override(decisionId: string, ctx: Ctx): OverrideResult {
    if (ctx.threadId !== undefined) return refuse(403, "an override comes from the panel");
    const d = this.store.decision(decisionId) as
      | (Record<string, string | null> & { id: string; body_json: string; thread: string; project: string })
      | undefined;
    if (!d) return refuse(404, "unknown decision");
    const requestId = `override:${decisionId}`;
    const known = this.store.registry(requestId);
    if (known) return { ok: true, decision_id: known.decision_id, replay: true };
    const pick = this.store.pick(decisionId);
    if (!pick || pick.by !== "vizier") return refuse(409, "only a delegated ruling can be overridden");

    const ask = parseAsk({ ...JSON.parse(d.body_json), request_id: requestId, supersedes: decisionId });
    const card = d.source === "card" && typeof d.task_id === "string";
    const n = card ? Number(d.generation) + 1 : 0;
    const input: DecisionInput = {
      id: card ? `card-${d.task_id}-g${n}` : this.svc.mintId(),
      request_id: requestId,
      identity: identity(ask),
      revision: revision(ask),
      semantic_key: semanticKey(ask),
      subject: d.subject ?? "",
      kind: "decide",
      ask_key: d.ask_key ?? null,
      project: d.project,
      project_root: d.project_root,
      root_dev: d.root_dev,
      root_ino: d.root_ino,
      asker: d.asker as string,
      thread: d.thread,
      owner_thread: null,
      body_json: normalizedJson(ask),
      supersedes: decisionId,
      delegable: false,
      ...(card ? { source: "card" as const, task_id: d.task_id, generation: n, tasks_project_id: d.tasks_project_id, card_fp: d.card_fp } : {}),
    };
    const run = this.db.transaction((): OverrideResult => {
      const res = this.store.insertReplacement(input, "override");
      if (!res.ok) {
        if ("existing" in res) return { ok: true, decision_id: res.existing.decision_id, replay: true };
        return refuse(409, res.reason === "already-superseded" ? "already superseded" : "not overridable");
      }
      const w = this.store.obligationsFor(decisionId).find((o) => o.op === `wake:${decisionId}:${pick.pick_id}`);
      if (w) this.store.voidObligation(w.id);
      if (card) {
        // T10: g n+1 is mk's open question again, visible once the relabel lands. The pick's notices
        // (one per Blocks thread) are not re-sent; the asking thread gets the void notice below.
        this.store.insertCardWrites(res.decision_id, overrideWrites({ id: res.decision_id, task_id: String(d.task_id) }, decisionId));
        this.svc.reopenForOverride(String(d.task_id));
      }
      this.store.insertObligations(decisionId, d.thread === "" ? [] : [
        {
          id: `ob:${decisionId}:void`,
          kind: "void-notice",
          recipient: d.thread,
          op: `void:${decisionId}`,
          payload: [
            `mk overrode the vizier's ruling on ${decisionId}. mk is answering afresh in the replacement decision ${res.decision_id}.`,
            `Wait for that answer before acting on the vizier's ruling. If you have already received mk's answer for ${res.decision_id}, that answer stands.`,
          ].join("\n"),
        },
      ]);
      return { ok: true, decision_id: res.decision_id, replay: false };
    });
    const out = run.immediate();
    if (out.ok && !out.replay) this.svc.nudge();
    return out;
  }
}
