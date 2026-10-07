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
// mk-action items come first and stay first (mk q172): Decide, Runbook, then Stalled, then Waiting.
export type Section = { key: "stalled" | "decide" | "runbook" | "waiting"; title: string; items: ViewItem[] };

export function buildAsksView(d: AsksData): Section[] {
  const stalled: ViewItem[] = [
    ...d.undeliverable.map((o) => ({ id: `undeliverable:${o.id}`, title: `Undeliverable ${o.kind ?? "wake"} to ${o.recipient ?? "?"}`, detail: o.last_error ?? undefined })),
    ...d.uncertain.map((o) => ({ id: `uncertain:${o.id}`, title: `Uncertain ${o.kind ?? "wake"} to ${o.recipient ?? "?"}` })),
    ...d.failures.map((o) => ({ id: `failure:${o.id}`, title: `Ruling file failed for ${o.decision_id ?? o.id}`, detail: o.last_error ?? undefined })),
    ...d.asks.map((a) => ({ id: a.id, title: `${a.label ?? "stalled"}: ${a.subject}`, detail: a.detail, thread: a.thread })),
  ];
  const sections: Section[] = [
    { key: "decide", title: "Decide", items: d.owed.map((o) => ({ id: o.id, title: o.subject, thread: o.thread })) },
    { key: "runbook", title: "Runbook", items: d.runbook.flatMap((g) => g.items.map((i) => ({ id: i.id, title: i.subject, detail: i.question, thread: g.thread }))) },
    { key: "stalled", title: "Stalled", items: stalled },
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

/** "3 h", "4 d": how long an ask has waited. Unknown or future times read as "now". */
export function ageText(filedAt: string, nowMs: number): string {
  const ms = nowMs - Date.parse(filedAt);
  if (!(ms > 0)) return "now";
  const m = Math.floor(ms / 60_000);
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  return h < 48 ? `${h} h` : `${Math.floor(h / 24)} d`;
}

/** An ask that has waited this long is probably settled elsewhere or superseded: Home marks it so mk can clear it. */
export const OLD_ASK_MS = 3 * 86_400_000;

/** An ask is old once it has waited more than three days; an unparseable or future time is not old. */
export function isOldAsk(filedAt: string, nowMs: number): boolean {
  return nowMs - Date.parse(filedAt) > OLD_ASK_MS;
}

/** The ask shown in full: the selected one while it is still owed, else the first. */
export function currentAsk<T extends { id: string }>(owed: T[], selected: string | null): T | undefined {
  return owed.find((o) => o.id === selected) ?? owed[0];
}

// Absolute paths, sha256 digests and URLs in an ask's text: shown as code, selectable in one click, and allowed to
// break anywhere so a long path never pushes the card wider than its panel.
const REF = /(https?:\/\/[^\s)]+[^\s).,;:]|(?<![\w/])\/(?:[\w.@+-]+\/)+[\w@+-]+(?:\.[\w@+-]+)*|\b[0-9a-f]{64}\b)/g;

export function RefText({ text }: { text: string }) {
  const parts = text.split(REF);
  return (
    <>
      {parts.map((part, i) =>
        i % 2 === 1 ? (
          <code key={i} className="select-all rounded bg-muted px-1 font-mono text-xs [overflow-wrap:anywhere]" data-ref>
            {part}
          </code>
        ) : (
          part
        ),
      )}
    </>
  );
}

export function AskCard({ ask, onPick, onOpen, nowMs }: { ask: OwedAsk; onPick: OnPick; onOpen: (thread: string) => void; nowMs?: number }) {
  const [failure, setFailure] = useState<string | null>(null);
  const pick = async (optionId: string) => {
    setFailure(null);
    const r = await onPick(ask.id, optionId, ask.revision);
    if (r && r.ok === false) setFailure(r.error ?? "pick failed");
  };
  const n = ask.mentions ?? 0;
  const anyInstruction = ask.ask.options.some((o) => o.instruction !== undefined);
  return (
    <article className="min-w-0 rounded-lg border border-border bg-card p-4" data-decision={ask.id}>
      <h3 className="text-sm font-medium [overflow-wrap:anywhere]">{ask.subject}</h3>
      <p className="mt-1 text-xs text-muted-foreground">
        {ask.project ? `${ask.project} · ` : ""}
        {nowMs !== undefined ? `waiting ${ageText(ask.filed_at, nowMs)} · ` : ""}
        <button type="button" className="underline" onClick={() => onOpen(ask.thread)}>{ask.thread}</button>
        {n > 0 ? ` - ${`also mentioned in ${n} ${n === 1 ? "thread" : "threads"}`}` : ""}
      </p>
      <p className="mt-2 whitespace-pre-wrap text-sm [overflow-wrap:anywhere]"><RefText text={ask.ask.question} /></p>
      <ul className="mb-0 mt-3 list-none space-y-2 p-0">
        {ask.ask.options.map((o) => {
          const recommended = ask.ask.recommendation === o.id;
          return (
            <li key={o.id} className={`rounded border p-2 ${recommended ? "border-primary" : "border-border"}`} data-reversible={o.reversible === true ? "true" : "false"}>
              {o.approval ? (
                <p className="mb-1 text-xs" data-approval={o.approval.kind}>
                  {`Records your approval to ${o.approval.kind} ${o.approval.target} at ${o.approval.identity}; ${approvalExpiry(o.approval.ttl)}. This is a record only and does not authorize anything.`}
                </p>
              ) : null}
              <div className="flex flex-wrap items-baseline gap-x-2 text-sm">
                <button type="button" className="font-medium underline" onClick={() => void pick(o.id)}>{o.label}</button>
                {recommended ? <span className="text-xs font-medium">recommended</span> : null}
                <span className="text-xs" data-reversible-label>{o.reversible === true ? "reversible" : "not reversible"}</span>
                <span className="text-xs text-muted-foreground">{o.kind}</span>
              </div>
              {o.instruction !== undefined ? (
                <details className="mt-1 text-xs">
                  <summary className="cursor-pointer text-muted-foreground">what the agent is told</summary>
                  <pre className="mt-1 whitespace-pre-wrap [overflow-wrap:anywhere]">{o.instruction}</pre>
                </details>
              ) : null}
            </li>
          );
        })}
      </ul>
      {anyInstruction ? (
        <p className="mt-2 text-xs text-muted-foreground">{`An instruction is sent to ${ask.thread} as written; the agent acts on it under its own permissions.`}</p>
      ) : null}
      {failure !== null ? <p role="alert" className="mt-2 text-xs font-medium text-destructive" data-pick-failure>{`Your pick did not go through: ${failure}. The ask is still open; try again.`}</p> : null}
    </article>
  );
}

/** The Decide queue: a short row per ask on the left, the selected ask in full on the right (below when narrow). */
export function DecideQueue({ owed, onPick, onOpen, nowMs }: { owed: OwedAsk[]; onPick: OnPick; onOpen: (thread: string) => void; nowMs: number }) {
  const [selected, setSelected] = useState<string | null>(null);
  const current = currentAsk(owed, selected);
  if (current === undefined) return null;
  return (
    <div className="flex flex-wrap items-start gap-4">
      <ol className="m-0 min-w-0 shrink-0 grow basis-64 list-none space-y-1 p-0" data-decide-list>
        {owed.map((o) => {
          const old = isOldAsk(o.filed_at, nowMs);
          const rec = o.ask.options.find((x) => x.id === o.ask.recommendation);
          return (
            <li key={o.id}>
              <button
                type="button"
                aria-current={o.id === current.id ? "true" : undefined}
                className={`w-full rounded border px-2 py-1.5 text-left ${o.id === current.id ? "border-primary bg-muted" : "border-border"}`}
                onClick={() => setSelected(o.id)}
              >
                <span className="block truncate text-sm">{o.subject}</span>
                {o.project ? <span className="block truncate text-xs text-muted-foreground">{o.project}</span> : null}
                <span className="block text-xs text-muted-foreground [overflow-wrap:anywhere]">
                  {ageText(o.filed_at, nowMs)}
                  {old ? <span className="font-medium" data-old-ask>{" · old: still needed?"}</span> : null}
                  {rec ? ` · rec: ${rec.label}` : ""}
                </span>
              </button>
            </li>
          );
        })}
      </ol>
      <div className="sticky top-0 min-w-0 grow-[3] basis-[28rem]">
        <AskCard key={current.id} ask={current} onPick={onPick} onOpen={onOpen} nowMs={nowMs} />
      </div>
    </div>
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

export function AsksPanel({ data, onPick, onOpen, onRevoke, nowMs = Date.now() }: { data: AsksData; onPick: OnPick; onOpen: (thread: string) => void; onRevoke?: (approvalId: string) => void; nowMs?: number }) {
  const view = buildAsksView(data);
  const approvals = data.approvals ?? [];
  if (view.length === 0 && approvals.length === 0) return <p className="p-4 text-sm text-muted-foreground">Nothing needs you.</p>;
  return (
    <div className="space-y-6 p-4">
      {view.map((s) => (
        <section key={s.key} data-section={s.key}>
          <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{`${s.title} (${s.items.length})`}</h2>
          {s.key === "decide" ? (
            <DecideQueue owed={data.owed} onPick={onPick} onOpen={onOpen} nowMs={nowMs} />
          ) : (
            <div className="space-y-3">
              {s.items.map((i) => (
                <div key={i.id} className="rounded border border-border p-3 text-sm [overflow-wrap:anywhere]">
                  <div><RefText text={i.title} /></div>
                  {i.detail ? <div className="text-xs text-muted-foreground"><RefText text={i.detail} /></div> : null}
                </div>
              ))}
            </div>
          )}
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
