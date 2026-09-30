// The Asks view: what is stalled, what mk must decide, the runbook, and what is merely waiting.

export type Option = { id: string; label: string; kind: string; reversible?: boolean; instruction?: string };
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

export function AskCard({ ask, onPick, onOpen }: { ask: OwedAsk; onPick: (decisionId: string, optionId: string, revision: string) => void; onOpen: (thread: string) => void }) {
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
            <div className="flex items-center gap-2 text-sm">
              <button type="button" className="font-medium underline" onClick={() => onPick(ask.id, o.id, ask.revision)}>{o.label}</button>
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
    </article>
  );
}

export function AsksPanel({ data, onPick, onOpen }: { data: AsksData; onPick: (d: string, o: string, r: string) => void; onOpen: (thread: string) => void }) {
  const view = buildAsksView(data);
  const byId = new Map(data.owed.map((o) => [o.id, o]));
  if (view.length === 0) return <p className="p-4 text-sm text-muted-foreground">Nothing needs you.</p>;
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
