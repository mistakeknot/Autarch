// The Asks view: what is stalled, what mk must decide, the runbook, and what is merely waiting.

import { useState } from "react";

/** What a pick hands back to the card: not ok means the card stays open and shows the error. */
export type PickOutcome = { ok: boolean; error?: string };
export type OnPick = (decisionId: string, optionId: string, revision: string) => void | Promise<PickOutcome | void>;

/** Turns a send into an outcome the card can show: a thrown error or a non-ok result is a failure. */
export async function pickOutcome(send: Promise<{ ok?: boolean; status?: number; error?: string }>): Promise<PickOutcome> {
  try {
    const r = await send;
    if (r?.ok === true) return { ok: true };
    return { ok: false, error: r?.error ?? (r?.status === 409 ? "this ask changed; reread and pick again" : `pick failed${r?.status ? ` (${r.status})` : ""}`) };
  } catch (e) {
    return { ok: false, error: `pick not sent: ${e instanceof Error ? e.message : String(e)}` };
  }
}

export type ApprovalSpec = { kind: string; target: string; identity: string; ttl?: number };
export type Option = { id: string; label: string; kind: string; reversible?: boolean; instruction?: string; approval?: ApprovalSpec };
export type ApprovalRecord = { approval_id: string; decision_id: string; kind: string; target: string; identity: string; minted_at: string; expires_at: string };
export type OwedAsk = {
  id: string;
  project: string;
  thread: string;
  subject: string;
  asker: string;
  filed_at: string;
  revision: string;
  mentions?: number;
  ask: { question: string; recommendation?: string; options: Option[] };
};
export type Lane = { id: string; subject: string; thread: string; owner: string | null; detail: string; updated_at: string; label?: string };
export type RunbookGroup = { thread: string; items: { id: string; subject: string; question: string; steps: string[]; filed_at: string }[] };
export type Obl = { id: string; decision_id?: string; kind?: string; recipient?: string; last_error?: string | null; state?: string };
export type Delegation = { settings: { vizierThreadId?: string; projects?: string[]; dailyCap?: number }; suspended: boolean };
export type AsksData = {
  owed: OwedAsk[];
  runbook: RunbookGroup[];
  lane: Lane[];
  asks: Lane[];
  undeliverable: Obl[];
  failures: Obl[];
  uncertain: Obl[];
  delegation: Delegation;
  approvals?: ApprovalRecord[];
  machineOwners: Record<string, string>;
};

export type ViewItem = { id: string; title: string; detail?: string; thread?: string };
export type Section = { key: "stalled" | "decide" | "runbook" | "waiting"; title: string; items: ViewItem[] };

export function buildAsksView(d: AsksData): Section[] {
  const stalled: ViewItem[] = [
    ...d.undeliverable.map((o) => ({ id: `undeliverable:${o.id}`, title: `Undeliverable ${o.kind ?? "wake"} to ${o.recipient ?? "?"}`, detail: o.last_error ?? undefined })),
    ...d.uncertain.map((o) => ({ id: `uncertain:${o.id}`, title: `Uncertain ${o.kind ?? "wake"} to ${o.recipient ?? "?"}` })),
    ...d.failures.map((o) => ({ id: `failure:${o.id}`, title: `Ruling file failed for ${o.decision_id ?? o.id}`, detail: o.last_error ?? undefined })),
    ...d.asks.map((a) => ({ id: a.id, title: `${a.label ?? "stalled"}: ${a.subject}`, detail: a.detail, thread: a.thread })),
  ];
  const sections: Section[] = [
    { key: "stalled", title: "Stalled", items: stalled },
    { key: "decide", title: "Decide", items: d.owed.map((o) => ({ id: o.id, title: o.subject, thread: o.thread })) },
    { key: "runbook", title: "Runbook", items: d.runbook.flatMap((g) => g.items.map((i) => ({ id: i.id, title: i.subject, detail: i.question, thread: g.thread }))) },
    { key: "waiting", title: "Waiting", items: d.lane.map((l) => ({ id: l.id, title: l.subject, detail: `${l.detail} (owner ${l.owner})`, thread: l.thread })) },
  ];
  return sections.filter((s) => s.items.length > 0);
}

/** How long a recorded approval lasts, said before mk picks. ttl 0 or absent means the 24 h default. */
export function approvalExpiry(ttl: number | undefined): string {
  const s = ttl && ttl > 0 ? ttl : 86_400;
  const text = s > 86_400 && s % 86_400 === 0 ? `${s / 86_400} d` : s % 3600 === 0 ? `${s / 3600} h` : s % 60 === 0 ? `${s / 60} min` : `${s} s`;
  return `expires ${text} after you pick`;
}

export function AskCard({ ask, onPick, onOpen }: { ask: OwedAsk; onPick: OnPick; onOpen: (thread: string) => void }) {
  const [failure, setFailure] = useState<string | null>(null);
  const pick = async (optionId: string) => {
    setFailure(null);
    const r = await onPick(ask.id, optionId, ask.revision);
    if (r && r.ok === false) setFailure(r.error ?? "pick failed");
  };
  const n = ask.mentions ?? 0;
  return (
    <article className="rounded-lg border border-border bg-card p-4" data-decision={ask.id}>
      <h3 className="text-sm font-medium">{ask.subject}</h3>
      <p className="mt-1 text-sm">{ask.ask.question}</p>
      <p className="mt-1 text-xs text-muted-foreground">
        <button type="button" className="underline" onClick={() => onOpen(ask.thread)}>{ask.thread}</button>
        {n > 0 ? ` - ${`also mentioned in ${n} ${n === 1 ? "thread" : "threads"}`}` : ""}
      </p>
      <ul className="mt-3 space-y-2">
        {ask.ask.options.map((o) => (
          <li key={o.id} className="rounded border border-border p-2" data-reversible={o.reversible === true ? "true" : "false"}>
            {o.approval ? (
              <p className="mb-1 text-xs" data-approval={o.approval.kind}>
                {`Records your approval to ${o.approval.kind} ${o.approval.target} at ${o.approval.identity}; ${approvalExpiry(o.approval.ttl)}. This is a record only and does not authorize anything.`}
              </p>
            ) : null}
            <div className="flex items-center gap-2 text-sm">
              <button type="button" className="font-medium underline" onClick={() => void pick(o.id)}>{o.label}</button>
              <span className="text-xs text-muted-foreground">{o.kind}</span>
              <span className="text-xs">{o.reversible === true ? "reversible" : "not reversible"}</span>
              {ask.ask.recommendation === o.id ? <span className="text-xs font-medium">recommended</span> : null}
            </div>
            {o.instruction !== undefined ? (
              <>
                <pre className="mt-1 whitespace-pre-wrap text-xs">{o.instruction}</pre>
                <p className="text-xs text-muted-foreground">{`sent to ${ask.thread} as written; the agent acts on it under its own permissions`}</p>
              </>
            ) : null}
          </li>
        ))}
      </ul>
      {failure !== null ? <p role="alert" className="mt-2 text-xs font-medium text-destructive" data-pick-failure>{`Your pick did not go through: ${failure}. The ask is still open; try again.`}</p> : null}
    </article>
  );
}

export function ApprovalsList({ approvals, onRevoke }: { approvals: ApprovalRecord[]; onRevoke: (approvalId: string) => void }) {
  return (
    <section data-section="approvals">
      <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">Recorded approvals</h2>
      <p className="mb-2 text-xs text-muted-foreground">Records of what you approved. They do not authorize a merge, deploy or release.</p>
      <ul className="space-y-2">
        {approvals.map((a) => (
          <li key={a.approval_id} className="rounded border border-border p-2 text-sm" data-approval-id={a.approval_id}>
            {`${a.kind} ${a.target} at ${a.identity}, expires ${a.expires_at}`}
            <button type="button" className="ml-2 text-xs underline" onClick={() => onRevoke(a.approval_id)}>Revoke</button>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function AsksPanel({ data, onPick, onOpen, onRevoke }: { data: AsksData; onPick: OnPick; onOpen: (thread: string) => void; onRevoke?: (approvalId: string) => void }) {
  const view = buildAsksView(data);
  const byId = new Map(data.owed.map((o) => [o.id, o]));
  const approvals = data.approvals ?? [];
  if (view.length === 0 && approvals.length === 0) return <p className="p-4 text-sm text-muted-foreground">Nothing needs you.</p>;
  return (
    <div className="space-y-6 p-4">
      {view.map((s) => (
        <section key={s.key} data-section={s.key}>
          <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{s.title}</h2>
          <div className="space-y-3">
            {s.items.map((i) =>
              s.key === "decide" && byId.has(i.id) ? (
                <AskCard key={i.id} ask={byId.get(i.id)!} onPick={onPick} onOpen={onOpen} />
              ) : (
                <div key={i.id} className="rounded border border-border p-3 text-sm">
                  <div>{i.title}</div>
                  {i.detail ? <div className="text-xs text-muted-foreground">{i.detail}</div> : null}
                </div>
              ),
            )}
          </div>
        </section>
      ))}
      {approvals.length > 0 ? <ApprovalsList approvals={approvals} onRevoke={onRevoke ?? (() => {})} /> : null}
    </div>
  );
}

type PickReq = { decision_id: string; option_id: string; revision: string };
type Result = { ok?: boolean; status?: number; error?: string };

/** One pick_id per (decision, option, revision) until the pick resolves, so a retry cannot double-file. */
export class PickController {
  private ids = new Map<string, string>();
  constructor(private mint: () => string) {}

  async send(rpc: (req: PickReq & { pick_id: string }) => Promise<Result>, req: PickReq, reread: () => void): Promise<Result> {
    const key = `${req.decision_id}|${req.option_id}|${req.revision}`;
    let id = this.ids.get(key);
    if (id === undefined) {
      id = this.mint();
      this.ids.set(key, id);
    }
    const r = await rpc({ ...req, pick_id: id }); // a thrown error keeps the id for the retry
    if (r.ok === true) {
      this.ids.delete(key);
    } else if (r.status === 409) {
      this.ids.delete(key);
      reread();
    }
    return r;
  }
}
