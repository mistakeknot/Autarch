// The conversation on a card: its comments as plain text (React text nodes, never HTML), an unread dot, a
// collapsible "Full instruction" behind the conversation, and a "may be stale" notice when the last read failed.
// Display only: nothing here rules, closes or runs anything. The author class and the "posted from Home
// (unattested)" label come from the server, which derives them from the recorded author id, never from the text.
import { createContext, useCallback, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Markdown } from "./markdown.js";

export type AuthorClass = "mk" | "owner" | "vizier" | "other";
export interface CommentViewT {
  id: string;
  author_class: AuthorClass;
  author_name: string;
  body: string;
  created_at: string;
  home_posted: boolean;
  unread: boolean;
}
export interface ConversationData {
  task_id: string;
  comments: CommentViewT[];
  unread: number;
  stale: boolean;
  last_ok_at: string | null;
  options: { id: string; label: string; kind: string; instruction: string | null }[];
}
export interface ConversationApi {
  /** Unread comment counts by card, for the dots on closed cards. */
  unread: Record<string, number>;
  load: (taskId: string) => Promise<ConversationData>;
  markSeen: (taskId: string, through: string) => void;
}

export const ConversationContext = createContext<ConversationApi | null>(null);
export const ConversationProvider = ConversationContext.Provider;

export const HOME_POSTED_LABEL = "posted from Home (unattested)";
const CLASS_LABEL: Record<AuthorClass, string> = { mk: "mk", owner: "owner", vizier: "vizier", other: "other" };

function stamp(iso: string): string {
  const t = Date.parse(iso);
  return Number.isNaN(t) ? "unknown time" : `${new Date(t).toISOString().slice(0, 16).replace("T", " ")} UTC`;
}

export function UnreadDot({ n }: { n: number }) {
  return n > 0 ? <span role="img" aria-label={`${n} unread`} className="inline-block size-2 rounded-full bg-primary align-middle" data-unread-dot /> : null;
}

/** The pure view: used by the container below and rendered directly in tests. */
export function ConversationBody({ data, children }: { data: ConversationData; children?: ReactNode }) {
  return (
    <div className="mt-2 space-y-2" data-conversation={data.task_id}>
      {data.stale ? <p role="status" className="m-0 text-xs font-medium text-muted-foreground" data-stale>{`Comments may be stale${data.last_ok_at ? ` (last read ${stamp(data.last_ok_at)})` : " (not read yet)"}.`}</p> : null}
      {data.comments.length === 0 ? <p className="m-0 text-xs text-muted-foreground">No comments yet.</p> : null}
      <ul className="m-0 list-none space-y-2 p-0">
        {data.comments.map((c) => (
          <li key={c.id} className="min-w-0 rounded border border-border p-2" data-comment={c.id} data-author-class={c.author_class}>
            <p className="m-0 text-xs text-muted-foreground">
              <UnreadDot n={c.unread ? 1 : 0} />
              {c.unread ? " " : ""}
              <span className="font-medium text-foreground" data-author>{`${CLASS_LABEL[c.author_class]}`}</span>
              {c.author_name && c.author_name !== CLASS_LABEL[c.author_class] ? ` (${c.author_name})` : ""}
              {` · ${stamp(c.created_at)}`}
              {c.home_posted ? <span className="ml-1 rounded-sm border border-border px-1" data-home-posted>{HOME_POSTED_LABEL}</span> : null}
            </p>
            <div className="mb-0 mt-1 text-sm [overflow-wrap:anywhere]" data-comment-body><Markdown text={c.body} /></div>
          </li>
        ))}
      </ul>
      {data.options.some((o) => o.instruction !== null) ? (
        <details data-full-instruction>
          <summary className="cursor-pointer text-xs font-medium text-muted-foreground">Full instruction</summary>
          <ul className="mb-0 mt-1 list-none space-y-2 p-0">
            {data.options.map((o) => (
              <li key={o.id} data-instruction-option={o.id}>
                <p className="m-0 text-xs font-medium">{o.label}</p>
                <pre className="m-0 whitespace-pre-wrap rounded bg-muted p-2 font-mono text-xs [overflow-wrap:anywhere]">{o.instruction ?? "Nothing is sent for this option."}</pre>
              </li>
            ))}
          </ul>
        </details>
      ) : null}
      <p className="m-0 text-xs text-muted-foreground">{`A reply you post is shown here as "${HOME_POSTED_LABEL}". Comments are never read as commands.`}</p>
      {children}
    </div>
  );
}

const POLL_MS = 15_000;

/** A collapsed "Conversation" on a card; loads on open, refreshes while open, marks it read when seen. Renders nothing without a provider or a task id. */
export function CardConversation({ taskId, children }: { taskId: string | null | undefined; children?: ReactNode }) {
  const api = useContext(ConversationContext);
  const [open, setOpen] = useState(false);
  const [data, setData] = useState<ConversationData | null>(null);
  const [failed, setFailed] = useState(false);
  const load = useCallback(() => {
    if (!api || !taskId) return;
    api.load(taskId).then(
      (d) => {
        setData(d);
        setFailed(false);
        const last = d.comments[d.comments.length - 1];
        if (last) api.markSeen(taskId, `${last.created_at}#${last.id}`);
      },
      () => setFailed(true),
    );
  }, [api, taskId]);
  useEffect(() => {
    if (!open) return;
    load();
    const t = setInterval(load, POLL_MS);
    return () => clearInterval(t);
  }, [open, load]);
  if (!api || !taskId) return null;
  const unread = open && data ? 0 : (api.unread[taskId] ?? 0);
  return (
    <details className="mt-3" open={open} onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)} data-card-conversation={taskId}>
      <summary className="cursor-pointer text-xs font-medium text-muted-foreground">
        {"Conversation "}
        <UnreadDot n={unread} />
      </summary>
      {failed && !data ? <p role="status" className="m-0 text-xs font-medium text-muted-foreground" data-stale>Comments may be stale: they could not be read.</p> : null}
      {data ? <ConversationBody data={failed ? { ...data, stale: true } : data}>{children}</ConversationBody> : open ? <p className="m-0 text-xs text-muted-foreground">Loading…</p> : null}
    </details>
  );
}
