// The root-run section of a card (plan 1.5, Task 2.8). Pure rendering of the server's RootRunView: a
// paste command or the reason there is none, and read-only run status. Every string came from a card,
// a file or the runner, so it is only ever rendered as text. Nothing here runs or approves anything.

export type RunStatusView =
  | { kind: "none" }
  | { kind: "unavailable"; why: string }
  | {
      kind: "run";
      attempt: number;
      phase: string | null;
      terminal: boolean | string | null;
      exit: number | null;
      signal: string | null;
      reason: string | null;
      started: string | null;
      ended: string | null;
      log_complete: boolean | null;
      owner_alive: boolean | null;
    };

export type RootRunPanelView = {
  present: boolean;
  badge: string;
  state: "match" | "mismatch" | "unreadable" | "invalid" | null;
  problems: { field: string; why: string }[];
  tuple: { script: string; sha256: string; timeout: number; set: string } | null;
  actual_sha256: string | null;
  owner_thread: string | null;
  item_json: string | null;
  command: string | null;
  card_file: string | null;
  status: RunStatusView | null;
};

const STATE_TEXT: Record<string, string> = {
  match: "script matches the card",
  mismatch: "script differs from the card",
  unreadable: "script unreadable",
  invalid: "root-run block invalid",
};
const REASON_TEXT: Record<string, string> = {
  "no-scope": "no scope was available",
  "ready-timeout": "not ready in time",
  "start-not-confirmed": "start was not confirmed",
  "left-processes": "left processes behind",
  "supervisor-died": "the supervisor died",
  other: "other",
};

export function statusLine(s: RunStatusView | null): string {
  if (s === null) return "status not checked";
  if (s.kind === "none") return "no run record yet";
  if (s.kind === "unavailable") return "status unavailable";
  const parts = [`attempt ${s.attempt}`, `phase ${s.phase ?? "unknown"}`];
  if (s.terminal !== null && s.terminal !== false) parts.push(s.terminal === true ? "finished" : `ended: ${s.terminal}`);
  if (s.exit !== null) parts.push(`exit ${s.exit}`);
  if (s.signal !== null) parts.push(`signal ${s.signal}`);
  if (s.reason !== null) parts.push(`reason: ${REASON_TEXT[s.reason] ?? "other"}`);
  if (s.owner_alive !== null) parts.push(s.owner_alive ? "owner alive" : "owner gone");
  if (s.log_complete !== null) parts.push(s.log_complete ? "log complete" : "log incomplete");
  return parts.join(", ");
}

export function RootRunSection({ view, onCopy }: { view: RootRunPanelView; onCopy?: (text: string) => void }) {
  if (!view.present) return <p className="mt-1 text-xs text-muted-foreground" data-rootrun="absent">This card has no root-run block.</p>;
  return (
    <section className="mt-2 rounded border border-border p-2 text-xs" data-rootrun={view.state ?? "none"}>
      <p className="font-medium" data-rootrun-badge="true">{view.badge}</p>
      {view.state !== null ? <p data-rootrun-state={view.state}>{STATE_TEXT[view.state] ?? view.state}</p> : null}
      {view.tuple ? (
        <p className="text-muted-foreground" data-rootrun-tuple="true">{`script ${view.tuple.script} - sha256 ${view.tuple.sha256} - timeout ${view.tuple.timeout} s - set ${view.tuple.set}`}</p>
      ) : null}
      {view.actual_sha256 ? <p data-rootrun-actual="true">{`file sha256 ${view.actual_sha256}`}</p> : null}
      {view.command !== null ? (
        <div data-rootrun-command="true">
          <p>{`Save this card's JSON (task and comments) as ${view.card_file ?? "the card file"}, then paste:`}</p>
          <pre className="overflow-x-auto rounded bg-muted p-1">{view.command}</pre>
          {onCopy ? <button type="button" className="underline" onClick={() => onCopy(view.command!)}>Copy command</button> : null}
          {view.item_json ? <p className="text-muted-foreground" data-rootrun-item="true">{`Item Aleph will build: ${view.item_json}`}</p> : null}
        </div>
      ) : (
        <div data-rootrun-suppressed="true">
          <p>No command: fix the card first.</p>
          <ul className="list-disc pl-5">
            {view.problems.map((p, i) => (
              <li key={i} data-problem={p.field}>{`${p.field}: ${p.why}`}</li>
            ))}
          </ul>
        </div>
      )}
      <p data-rootrun-status={view.status?.kind ?? "none"}>{statusLine(view.status)}</p>
    </section>
  );
}
