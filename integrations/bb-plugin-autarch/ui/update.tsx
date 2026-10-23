import { ActionButton } from "./buttons.js";

export type UpdateInfoView = {
  status: { installed: string; latest: string; count: number; subjects: string[]; checked_at: string } | null;
  result: { sha: string; ok: boolean; finished_at: string; message: string } | null;
  request: { sha: string; clicked_at: string } | null;
  available: boolean;
  pending: boolean;
  /** Server time (ISO); ages are measured against it. */
  now: string;
  interval_ms: number;
  /** The runner holds its lock: it is checking or installing right now. */
  busy: boolean;
  log_path: string;
  unit: string;
};

/** A request this many check intervals old with no result for it looks stuck. */
export const STUCK_AFTER_INTERVALS = 2;

const age = (ms: number) => {
  const min = Math.max(0, Math.floor(ms / 60_000));
  if (min < 1) return "just now";
  if (min < 60) return `${min} min ago`;
  const h = Math.floor(min / 60);
  return `${h} h ${min % 60} min ago`;
};
const span = (ms: number) => {
  const min = Math.round(ms / 60_000);
  return min >= 1 && min * 60_000 === ms ? `${min} minute${min === 1 ? "" : "s"}` : `${Math.round(ms / 1000)} seconds`;
};
const utc = (ms: number) => `${new Date(ms).toISOString().slice(11, 16)} UTC`;

export type PendingNote = { kind: "installing" | "waiting" | "stuck"; text: string };

/** What to say about a request the runner has not finished: installing now, due by a time, or stuck. */
export function pendingNote(info: UpdateInfoView): PendingNote {
  if (info.busy) return { kind: "installing", text: "Installing… the runner is working on this now. This page updates when it finishes." };
  const at = info.request ? Date.parse(info.request.clicked_at) : NaN;
  const now = Date.parse(info.now);
  if (Number.isNaN(at) || Number.isNaN(now)) return { kind: "waiting", text: `Requested. The runner checks every ${span(info.interval_ms)}.` };
  const old = now - at;
  if (old > STUCK_AFTER_INTERVALS * info.interval_ms) {
    return { kind: "stuck", text: `Requested ${age(old)} and the runner has not reported a result. It looks stuck. Look in ${info.log_path} and run: systemctl --user status ${info.unit}` };
  }
  return { kind: "waiting", text: `Requested ${age(old)}. The runner checks every ${span(info.interval_ms)}, so it should start by ${utc(at + info.interval_ms)}.` };
}

const short = (sha: string) => sha.slice(0, 7);

/**
 * Next to Settings: "Update available (N commits)" opens the commit list and a button that records a
 * request for the latest commit. Home runs nothing; a runner outside the plugin does the install.
 * Nothing renders when there is no update, no waiting request and no result worth showing.
 */
/** The menu's summary line, or null when there is nothing to show (the caller then leaves out the row). */
export function updateLabel(info: UpdateInfoView | null): { label: string; state: "pending" | "failed" | "available" } | null {
  if (!info || !info.status) return null;
  const { status, result } = info;
  const failed = result !== null && !result.ok && result.sha === status.latest;
  if (info.pending) return { label: `${info.busy ? "Installing update" : "Update requested"} (${short(info.request?.sha ?? status.latest)})${info.busy ? "…" : ""}`, state: "pending" };
  if (failed) return { label: "Update failed", state: "failed" };
  if (info.available) return { label: `Update available (${status.count} commit${status.count === 1 ? "" : "s"})`, state: "available" };
  return null;
}

function PendingText({ note }: { note: PendingNote }) {
  return (
    <p className={`mt-2 ${note.kind === "stuck" ? "font-medium text-destructive" : "text-muted-foreground"}`} data-update-pending data-update-pending-kind={note.kind}>
      {note.text}
    </p>
  );
}

export function UpdateMenu({ info, onRequest, error }: { info: UpdateInfoView | null; onRequest: (sha: string) => void; error?: string | null }) {
  const head = updateLabel(info);
  if (!info || !info.status || head === null) return null;
  const { status, result } = info;
  const failed = head.state === "failed";
  const label = head.label;
  return (
    <details data-update-menu data-update-state={head.state}>
      <summary className="cursor-pointer text-sm font-medium" data-update-label>{label}</summary>
      <div className="mt-1 max-w-xl rounded-lg border border-border bg-card p-3 text-xs shadow">
        <p>{`Installed ${short(status.installed)}, latest ${short(status.latest)}, checked ${status.checked_at}.`}</p>
        {status.subjects.length > 0 ? (
          <ul className="mt-2 max-h-48 list-disc overflow-auto pl-4" data-update-commits>
            {status.subjects.map((s, n) => <li key={n}>{s}</li>)}
          </ul>
        ) : null}
        {result !== null ? <p className="mt-2" data-update-result data-ok={result.ok ? "true" : "false"}>{`Last update ${short(result.sha)} ${result.ok ? "succeeded" : "failed"} at ${result.finished_at}: ${result.message}`}</p> : null}
        {error ? <p role="alert" className="mt-2 font-medium text-destructive" data-update-error>{error}</p> : null}
        {info.pending ? (
          <PendingText note={pendingNote(info)} />
        ) : info.available ? (
          <div className="mt-2">
            <ActionButton tone="recommended" onClick={() => onRequest(status.latest)} data-update-request>{`${failed ? "Retry update to" : "Request update to"} ${short(status.latest)}`}</ActionButton>
            <p className="mt-1 text-muted-foreground">The runner checks this commit is on main, backs up the data, installs, and rolls back if it fails.</p>
          </div>
        ) : null}
      </div>
    </details>
  );
}
