// The blocks panel (plan 1.4): open card generations across all tasks projects, then the legacy group.
// The server sorts and pins (queueview.ts); this file only renders, and owns the refresh policy.
import { AskCard, type OnNote, type OnPick, type OwedAsk, type RunbookGroup } from "./asks.js";
import { RootRunSection, type RootRunPanelView } from "./rootrun.js";

export type QueueRowView = {
  id: string;
  decision_id: string | null;
  task_id: string;
  card_key: string | null;
  tasks_project_id?: string | null;
  binding_state: "suggested" | "confirmed" | "rejected" | null;
  project: string | null;
  title: string;
  refs: { ref: string; counted: boolean }[];
  blocks_count: number;
  created_at: string;
  thread: string | null;
  pinned: boolean;
  ask: OwedAsk["ask"] | null;
  revision: string | null;
  mentions: number;
  display_reason: string | null;
  display_only: boolean;
  overrides_generation: number | null;
  changed_after_ruling: boolean;
  root: { state: string | null; reason: string | null };
};
export type LegacyView = {
  count: number;
  owed: { id: string; project: string; thread: string; subject: string; filed_at: string; revision: string; mentions: number; ask: OwedAsk["ask"] }[];
  runbook: RunbookGroup[];
  machine: { lane: { id: string; subject: string; thread: string; owner: string | null; detail: string }[]; asks: { id: string; subject: string; thread: string; detail: string; label?: string }[] };
};
export type BindingView = { tasks_project_id: string; home_project: string; state: "suggested" | "confirmed" | "rejected"; suggested_at: string | null; confirmed_at: string | null };
export type UnboundView = { tasks_project_id: string; cards: number; targets: string[] };
export type QueueView = {
  rows: QueueRowView[];
  legacy: LegacyView;
  bindings: BindingView[];
  unbound?: UnboundView[];
  serve_projects?: string[];
  inactive_projects: string[];
  status?: { health: string; last_error: string | null; last_poll_at: string | null; open_cards: number; routing_worst_case_age_ms: number } | null;
};

/** The Queue shows Asks above Blocking: drop from Blocking what Asks already owes, so one ask is never answerable twice. */
export function withoutOwed(q: QueueView, asks: { owed: { id: string }[] }): QueueView {
  const ids = new Set(asks.owed.map((o) => o.id));
  const owed = q.legacy.owed.filter((o) => !ids.has(o.id));
  return {
    ...q,
    rows: q.rows.filter((r) => r.decision_id === null || !ids.has(r.decision_id)),
    legacy: { ...q.legacy, owed, count: q.legacy.count - (q.legacy.owed.length - owed.length) },
  };
}

/** A human age such as "3 d" or "5 min", from an ISO time and a clock. */
export function ageText(createdAt: string, nowMs: number): string {
  const t = Date.parse(createdAt);
  if (Number.isNaN(t)) return "unknown age";
  const s = Math.max(0, Math.floor((nowMs - t) / 1000));
  if (s >= 86_400) return `${Math.floor(s / 86_400)} d`;
  if (s >= 3600) return `${Math.floor(s / 3600)} h`;
  if (s >= 60) return `${Math.floor(s / 60)} min`;
  return `${s} s`;
}

/** A card whose project has no verified root cannot be ruled from Home: it goes to the Unbound projects group. */
export function isUnbound(row: QueueRowView): boolean {
  return row.root.state !== "verified";
}

/** Split the rows into the pinned group ("this thread") and the rest, preserving server order. */
export function groupRows(rows: QueueRowView[]): { pinned: QueueRowView[]; rest: QueueRowView[] } {
  return { pinned: rows.filter((r) => r.pinned), rest: rows.filter((r) => !r.pinned) };
}

type Pick = OnPick;

function RootSection({ root }: { root: QueueRowView["root"] }) {
  if (root.state === null && root.reason === null) return null;
  const ok = root.state === "verified";
  return (
    <p className="mt-1 text-xs" data-root={ok ? "verified" : "unverified"}>
      {ok ? "root run: verified" : `root unverified${root.reason ? `: ${root.reason}` : ""}`}
    </p>
  );
}

/** The optional root-run lookup: views by task id, and a loader. Absent in tests that do not need it. */
export type OnBind = (b: { tasks_project_id: string; state: "confirmed"; home_project: string }) => void;

/** Fail open: a card whose tracker project is not confirmed to a Home project still shows, flagged. */
function BindingFlag({ row, onBind }: { row: QueueRowView; onBind?: OnBind }) {
  if (row.binding_state === "confirmed" || row.binding_state === "rejected") return null;
  const why = row.binding_state === "suggested" ? "project not bound (name match only suggested)" : "project not bound";
  const can = onBind && row.tasks_project_id && row.project;
  return (
    <p className="mt-1 text-xs" role="note" data-binding-flag={row.binding_state ?? "unbound"}>
      {why}
      {can ? (
        <button type="button" className="ml-2 underline" data-bind="true" onClick={() => onBind({ tasks_project_id: row.tasks_project_id!, state: "confirmed", home_project: row.project! })}>
          {`Bind to ${row.project}`}
        </button>
      ) : null}
    </p>
  );
}

export type RootRunHooks = { views: Record<string, RootRunPanelView | { error: string }>; load: (taskId: string) => void };

function RootRunControl({ taskId, hooks }: { taskId: string; hooks: RootRunHooks }) {
  const v = hooks.views[taskId];
  if (v === undefined) return <button type="button" className="mt-1 text-xs underline" data-rootrun-load="true" onClick={() => hooks.load(taskId)}>Check root run</button>;
  if ("error" in v) return <p className="mt-1 text-xs" data-rootrun="error">{`Root run unavailable: ${v.error}`}</p>;
  return <RootRunSection view={v} />;
}

export function BlocksRow({ row, nowMs, onPick, onOpen, rootRun, onBind, onNote }: { row: QueueRowView; nowMs: number; onPick: Pick; onOpen: (thread: string) => void; rootRun?: RootRunHooks; onBind?: OnBind; onNote?: OnNote }) {
  const meta = [row.card_key, row.project, row.thread ? `owner ${row.thread}` : "owner unknown", `age ${ageText(row.created_at, nowMs)}`, `blocks ${row.blocks_count}`].filter((x) => x !== null && x !== "").join(" - ");
  return (
    <article className="rounded-lg border border-border bg-card p-3" data-row={row.id} data-pinned={row.pinned ? "true" : "false"}>
      <p className="text-xs text-muted-foreground">{meta}</p>
      {row.refs.length > 0 ? (
        <p className="text-xs" data-refs="true">
          {row.refs.map((r) => (
            <span key={r.ref} className="mr-2" data-counted={r.counted ? "true" : "false"}>{r.ref}</span>
          ))}
        </p>
      ) : null}
      {row.overrides_generation !== null ? <p className="text-xs font-medium" data-marker="override">{`overrides vizier ruling g${row.overrides_generation}`}</p> : null}
      {row.changed_after_ruling ? <p className="text-xs font-medium" data-marker="changed">changed after ruling</p> : null}
      <BindingFlag row={row} {...(onBind ? { onBind } : {})} />
      {row.display_only ? (
        <div data-display-only="true">
          <h3 className="text-sm font-medium">{row.title}</h3>
          <p className="text-xs" role="note">{`display only: ${row.display_reason ?? "unknown"}`}</p>
          <p className="text-xs text-muted-foreground">
            {row.card_key ? `Open ${row.card_key} in Tasks to fix it. ` : ""}
            {row.thread ? <button type="button" className="underline" onClick={() => onOpen(row.thread!)}>{row.thread}</button> : null}
          </p>
        </div>
      ) : row.ask !== null && row.decision_id !== null && row.revision !== null ? (
        <AskCard
          ask={{ id: row.decision_id, project: row.project ?? "", thread: row.thread ?? "", subject: row.title, key: row.card_key, asker: "card", filed_at: row.created_at, revision: row.revision, mentions: row.mentions, task_id: row.task_id, ask: row.ask }}
          onPick={onPick}
          onOpen={onOpen}
          unbound={isUnbound(row)}
          {...(onNote ? { onNote } : {})}
        />
      ) : null}
      <RootSection root={row.root} />
      {rootRun ? <RootRunControl taskId={row.task_id} hooks={rootRun} /> : null}
    </article>
  );
}

export function LegacyGroup({ legacy, onPick, onOpen }: { legacy: LegacyView; onPick: Pick; onOpen: (thread: string) => void }) {
  if (legacy.count === 0) return null;
  return (
    <section data-section="legacy">
      <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{`Legacy asks (${legacy.count})`}</h2>
      <div className="space-y-3">
        {legacy.owed.map((o) => (
          <AskCard key={o.id} ask={{ ...o, asker: "legacy" }} onPick={onPick} onOpen={onOpen} />
        ))}
        {legacy.runbook.flatMap((g) =>
          g.items.map((i) => (
            <div key={i.id} className="rounded border border-border p-3 text-sm" data-legacy="steps">
              <div>{i.subject}</div>
              <div className="text-xs text-muted-foreground">{i.question}</div>
              <ol className="mt-1 list-decimal pl-5 text-xs">{i.steps.map((s, n) => <li key={n}>{s}</li>)}</ol>
            </div>
          )),
        )}
        {[...legacy.machine.asks, ...legacy.machine.lane].map((m) => (
          <div key={m.id} className="rounded border border-border p-3 text-sm" data-legacy="machine">
            <div>{`${"label" in m && m.label ? `${m.label}: ` : ""}${m.subject}`}</div>
            {m.detail ? <div className="text-xs text-muted-foreground">{m.detail}</div> : null}
          </div>
        ))}
      </div>
    </section>
  );
}

export function BlocksPanel({ data, nowMs, thread, onPick, onOpen, rootRun, onBind, onNote }: { data: QueueView; nowMs: number; thread?: string; onPick: Pick; onOpen: (thread: string) => void; rootRun?: RootRunHooks; onBind?: OnBind; onNote?: OnNote }) {
  const unbound = data.rows.filter(isUnbound);
  const { pinned, rest } = groupRows(data.rows.filter((r) => !isUnbound(r)));
  const row = (r: QueueRowView) => <BlocksRow key={r.id} row={r} nowMs={nowMs} onPick={onPick} onOpen={onOpen} {...(rootRun ? { rootRun } : {})} {...(onBind ? { onBind } : {})} {...(onNote ? { onNote } : {})} />;
  if (data.rows.length === 0 && data.legacy.count === 0) return <p className="p-4 text-sm text-muted-foreground">Nothing is blocking.</p>;
  return (
    <div className="space-y-6 p-4" data-panel="blocks">
      {pinned.length > 0 ? (
        <section data-section="this-thread">
          <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{thread ? "This thread" : "Pinned"}</h2>
          <div className="space-y-3">{pinned.map(row)}</div>
        </section>
      ) : null}
      {rest.length > 0 ? (
        <section data-section="blocking">
          <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">Blocking</h2>
          <div className="space-y-3">{rest.map(row)}</div>
        </section>
      ) : null}
      {unbound.length > 0 ? (
        <section data-section="unbound-projects">
          <h2 className="mb-2 text-xs font-semibold uppercase text-muted-foreground">{`Unbound projects (${unbound.length})`}</h2>
          <p className="mb-2 text-xs text-muted-foreground">These cards belong to a project with no verified root. Home shows them but cannot record a ruling on them, so there are no pick buttons. Bind the project, or use Ask / note to talk to the owner.</p>
          <div className="space-y-3">{unbound.map(row)}</div>
        </section>
      ) : null}
      <LegacyGroup legacy={data.legacy} onPick={onPick} onOpen={onOpen} />
    </div>
  );
}

/**
 * Refresh policy: a refetch on every realtime event and on a fallback interval, so a missed event
 * costs at most one interval. `start` returns the stop function.
 */
export class QueueRefresher {
  private timer: ReturnType<typeof setInterval> | null = null;
  constructor(private refetch: () => void, private intervalMs: number) {}
  start(): () => void {
    this.timer = setInterval(this.refetch, this.intervalMs);
    return () => {
      if (this.timer) clearInterval(this.timer);
      this.timer = null;
    };
  }
  onEvent = (): void => this.refetch();
}
