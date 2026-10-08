import { ThreadChat } from "@get-bb/plugin-sdk/app";

/** The vizier is an ordinary thread; mk talks to it in the host's own chat, with the thread's permissions. */
export function VizierPanel({ threadId, compact = false }: { threadId: string | undefined; compact?: boolean }) {
  if (!threadId) return <p className="p-4 text-sm text-muted-foreground">No vizier thread is set. Choose one in Settings.</p>;
  return <ThreadChat threadId={threadId} variant={compact ? "compact" : "full"} permissionPolicy="inherit" />;
}

/** The asking thread, compact, beside its decision. */
export function ThreadPanel({ threadId }: { threadId: string }) {
  return <ThreadChat threadId={threadId} variant="compact" permissionPolicy="inherit" />;
}

/** The top of the Asks page: a short note to the vizier without leaving the queue. Closed until mk opens it, so the queue keeps the page. */
export function TellVizier({ threadId }: { threadId: string | undefined }) {
  return (
    <details className="mx-4 mt-4 rounded-lg border border-border" data-tell-vizier>
      <summary className="cursor-pointer px-3 py-2 text-sm font-medium">Tell the vizier</summary>
      <div className="h-64 border-t border-border">
        <VizierPanel threadId={threadId} compact />
      </div>
    </details>
  );
}
