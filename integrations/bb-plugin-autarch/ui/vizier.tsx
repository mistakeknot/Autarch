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
