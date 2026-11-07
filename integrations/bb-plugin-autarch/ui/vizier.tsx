import { ThreadChat } from "@get-bb/plugin-sdk/app";

/** The vizier is an ordinary thread; mk talks to it in the host's own chat, with the thread's permissions. */
export function VizierPanel({ threadId }: { threadId: string | undefined }) {
  if (!threadId) return <p className="p-4 text-sm text-muted-foreground">No vizier thread is set. Choose one in Settings.</p>;
  return <ThreadChat threadId={threadId} variant="full" permissionPolicy="inherit" />;
}

/** The asking thread, compact, beside its decision. */
export function ThreadPanel({ threadId }: { threadId: string }) {
  return <ThreadChat threadId={threadId} variant="compact" permissionPolicy="inherit" />;
}

/** The Vizier tab: "Tell the vizier" on top of the full chat, so a note to the vizier no longer sits above the decisions on the Queue. */
export function TellVizier({ threadId }: { threadId: string | undefined }) {
  return (
    <section className="flex h-full min-h-0 flex-col" aria-label="Tell the vizier" data-tell-vizier>
      <h2 className="m-0 px-4 py-2 text-sm font-semibold">Tell the vizier</h2>
      <div className="min-h-0 flex-1 border-t border-border">
        <VizierPanel threadId={threadId} />
      </div>
    </section>
  );
}
