import { ActionButton } from "./buttons.js";

export type UpdateInfoView = {
  status: { installed: string; latest: string; count: number; subjects: string[]; checked_at: string } | null;
  result: { sha: string; ok: boolean; finished_at: string; message: string } | null;
  request: { sha: string; clicked_at: string } | null;
  available: boolean;
  pending: boolean;
};

const short = (sha: string) => sha.slice(0, 7);

/**
 * Next to Settings: "Update available (N commits)" opens the commit list and a button that records a
 * request for the latest commit. Home runs nothing; a runner outside the plugin does the install.
 * Nothing renders when there is no update, no waiting request and no result worth showing.
 */
export function UpdateMenu({ info, onRequest, error }: { info: UpdateInfoView | null; onRequest: (sha: string) => void; error?: string | null }) {
  if (!info || !info.status) return null;
  const { status, result } = info;
  const failed = result !== null && !result.ok && result.sha === status.latest;
  const label = info.pending
    ? `Update requested (${short(status.latest)})`
    : failed
      ? "Update failed"
      : info.available
        ? `Update available (${status.count} commit${status.count === 1 ? "" : "s"})`
        : null;
  if (label === null) return null;
  return (
    <details className="relative" data-update-menu data-update-state={info.pending ? "pending" : failed ? "failed" : "available"}>
      <summary className="cursor-pointer text-sm font-medium" data-update-label>{label}</summary>
      <div className="absolute right-0 z-10 mt-1 w-96 rounded-lg border border-border bg-card p-3 text-xs shadow">
        <p>{`Installed ${short(status.installed)}, latest ${short(status.latest)}, checked ${status.checked_at}.`}</p>
        {status.subjects.length > 0 ? (
          <ul className="mt-2 max-h-48 list-disc overflow-auto pl-4" data-update-commits>
            {status.subjects.map((s, n) => <li key={n}>{s}</li>)}
          </ul>
        ) : null}
        {result !== null ? <p className="mt-2" data-update-result data-ok={result.ok ? "true" : "false"}>{`Last update ${short(result.sha)} ${result.ok ? "succeeded" : "failed"} at ${result.finished_at}: ${result.message}`}</p> : null}
        {error ? <p role="alert" className="mt-2 font-medium text-destructive" data-update-error>{error}</p> : null}
        {info.pending ? (
          <p className="mt-2 text-muted-foreground" data-update-pending>Waiting for the update runner to pick this up. It runs on its own schedule.</p>
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
