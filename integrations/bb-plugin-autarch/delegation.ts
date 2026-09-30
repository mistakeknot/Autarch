// Delegated rulings and superseding overrides (P-10) [D-1] [D-2] [D-3].
//
// The vizier's identity is an advisory claim (ctx.threadId); authenticated separation
// is gate G-15. What is enforced here is the bounded policy: a delegated ruling can
// only be a reversible option on a thread-asked, delegable decision, in an enabled
// project, under the daily cap, with no approval spec, while no settings change is
// unseen by mk. Settings are written only by setDelegation (panel RPC); no CLI verb
// reaches it.
import { identity, normalizedJson, parseAsk, revision, semanticKey } from "./model.js";
import type { DecisionInput } from "./store.js";
import type { PickResult, Service } from "./service.js";

type Ctx = { threadId?: string };
type Fail = { ok: false; status: number; error: string };
export type DelegationSettings = { vizierThreadId: string; projects: string[]; dailyCap: number };
export type OverrideResult = { ok: true; decision_id: string; replay: boolean } | Fail;
export interface Pinned {
  /** The seen-row key: `ruling:<decision>` or `delegation-settings:<seq>`. */
  item: string;
  decision?: string;
  at: string;
}

const MK = "mk";
const DAY_MS = 86_400_000;
const APPROVAL_TOKEN = /APPROVED-(MERGE|DEPLOY|RELEASE)/;
const THREAD_ID = /^[A-Za-z0-9:_.-]{1,128}$/;
const refuse = (status: number, error: string): Fail => ({ ok: false, status, error });

export class Delegation {
  constructor(readonly svc: Service) {}

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

  markSeen(account: string, item: string): void {
    this.store.markSeen(account, item);
  }

  private suspended(): boolean {
    const item = this.latestSettingsItem();
    return item !== undefined && !this.store.hasSeen(MK, item);
  }

  /** Everything catch-up keeps in front of mk until mk's account has a seen row for it. */
  pinned(): Pinned[] {
    const out: Pinned[] = [];
    for (const e of this.db.prepare("SELECT seq, at FROM events WHERE type = 'delegation-settings-changed' ORDER BY seq").all() as { seq: number; at: string }[]) {
      const item = `delegation-settings:${e.seq}`;
      if (!this.store.hasSeen(MK, item)) out.push({ item, at: e.at });
    }
    for (const p of this.db.prepare(`SELECT decision_id, picked_at FROM picks WHERE "by" = 'vizier' ORDER BY picked_at, rowid`).all() as { decision_id: string; picked_at: string }[]) {
      const item = `ruling:${p.decision_id}`;
      if (!this.store.hasSeen(MK, item)) out.push({ item, decision: p.decision_id, at: p.picked_at });
    }
    return out;
  }

  // ---- rule -------------------------------------------------------------------

  rule(decisionId: string, optionId: string, reason: string, ctx: Ctx): PickResult {
    const d = this.store.decision(decisionId) as
      | { id: string; kind: string; asker: string; project: string; delegable: number; revision: string; body_json: string; request_id: string }
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
      const now = this.settings();
      if (!now.vizierThreadId || ctx.threadId !== now.vizierThreadId) return { status: 403, error: "only the vizier thread may rule" };
      if (!(now.projects ?? []).includes(d.project)) return { status: 403, error: "delegation is not enabled for this project" };
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
    const input: DecisionInput = {
      id: this.svc.mintId(),
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
    };
    const run = this.db.transaction((): OverrideResult => {
      const res = this.store.insertReplacement(input, "override");
      if (!res.ok) {
        if ("existing" in res) return { ok: true, decision_id: res.existing.decision_id, replay: true };
        return refuse(409, res.reason === "already-superseded" ? "already superseded" : "not overridable");
      }
      const w = this.store.obligationsFor(decisionId).find((o) => o.op === `wake:${decisionId}:${pick.pick_id}`);
      if (w) this.store.voidObligation(w.id);
      this.store.insertObligations(decisionId, [
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
