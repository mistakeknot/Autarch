// The document reader (bead mk-5zfbg): a file path in a card's text opens the document in a side pane, so the card's
// options stay on screen while mk reads. Display only. The server decides what may be read (docread.ts); this file
// never builds a path, and a refusal is shown as the server's one generic reason.
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { Markdown } from "./markdown.js";

export type DocResultT =
  | { ok: true; path: string; size: number; sha256: string; text: string; markdown: boolean; matches: boolean | null }
  | { ok: false; error: string };

export interface DocReaderApi {
  read: (path: string, sha256?: string) => Promise<DocResultT>;
}

const DocReaderContext = createContext<{ open: (path: string, sha256?: string) => void } | null>(null);

/** Text types the server will show; a path with another ending gets no opener. A hint only: the server decides. */
const DOC_PATH = /^\/.+\.(md|markdown|txt|log|json|jsonl|ya?ml|toml|sh|patch|diff|csv)$/i;
export const looksLikeDoc = (p: string): boolean => DOC_PATH.test(p);

/** The opener beside a path in card text. Nothing renders outside a DocReaderHost. */
export function DocLink({ path, sha256 }: { path: string; sha256?: string }) {
  const ctx = useContext(DocReaderContext);
  if (ctx === null || !looksLikeDoc(path)) return null;
  return (
    <button type="button" className="ml-1 rounded border border-border px-1 text-xs text-primary hover:bg-muted" data-doc-open={path} onClick={() => ctx.open(path, sha256)}>
      read
    </button>
  );
}

export type View = "rendered" | "raw";

export function DocPane({ api, path, sha256, onClose }: { api: DocReaderApi; path: string; sha256?: string; onClose: () => void }) {
  const [res, setRes] = useState<DocResultT | "loading">("loading");
  const [view, setView] = useState<View>("rendered");
  // The host keys the pane by path and digest, so a new document starts from "loading" with fresh state.
  useEffect(() => {
    let current = true;
    api.read(path, sha256).then(
      (r) => current && setRes(r),
      (e) => current && setRes({ ok: false, error: e instanceof Error ? e.message : String(e) }),
    );
    return () => {
      current = false;
    };
  }, [api, path, sha256]);
  return (
    <aside className="flex h-full min-h-0 flex-col border-l border-border bg-background" aria-label="Document reader" data-doc-pane>
      <header className="flex items-center gap-2 border-b border-border p-2">
        <code className="min-w-0 flex-1 select-all text-xs [overflow-wrap:anywhere]" data-doc-path>{path}</code>
        {res !== "loading" && res.ok && res.markdown ? (
          <button type="button" className="rounded border border-border px-2 text-xs" data-doc-toggle onClick={() => setView(view === "rendered" ? "raw" : "rendered")}>
            {view === "rendered" ? "Raw" : "Rendered"}
          </button>
        ) : null}
        <button type="button" className="rounded border border-border px-2 text-xs" data-doc-close onClick={onClose}>
          Close
        </button>
      </header>
      <DocContent res={res} want={sha256} view={view} />
    </aside>
  );
}

/** The pane's body for one read result: loading, the refusal, or the digest line over the rendered or raw text. */
export function DocContent({ res, want, view }: { res: DocResultT | "loading"; want?: string; view: View }) {
  if (res === "loading") return <p className="p-3 text-sm text-muted-foreground">Loading…</p>;
  if (!res.ok) return <p role="alert" className="p-3 text-sm text-destructive" data-doc-error>{res.error}</p>;
  return (
    <>
      <DigestLine r={res} want={want} />
      <div className="min-h-0 flex-1 overflow-auto p-3" data-doc-body>
        {res.markdown && view === "rendered" ? <Markdown text={res.text} /> : <pre className="whitespace-pre-wrap font-mono text-xs [overflow-wrap:anywhere]">{res.text}</pre>}
      </div>
    </>
  );
}

function DigestLine({ r, want }: { r: Extract<DocResultT, { ok: true }>; want?: string }) {
  let verdict: ReactNode = null;
  if (want !== undefined) {
    verdict =
      r.matches === true ? (
        <span className="font-medium text-green-600" data-doc-match="yes">matches the sha256 on the card</span>
      ) : (
        <span className="font-medium text-destructive" data-doc-match="no">does NOT match the sha256 on the card: the file changed</span>
      );
  }
  return (
    <p className="m-0 border-b border-border px-3 py-1 text-xs text-muted-foreground [overflow-wrap:anywhere]" data-doc-digest>
      {verdict} {verdict ? "· " : ""}sha256 <code className="select-all">{r.sha256}</code> · {r.size} bytes
    </p>
  );
}

/** Wraps a panel: lays the reader pane beside the children when a document is open, and lets DocLink open one. */
export function DocReaderHost({ api, children }: { api: DocReaderApi; children: ReactNode }) {
  const [open, setOpen] = useState<{ path: string; sha256?: string } | null>(null);
  const ctx = useMemo(() => ({ open: (path: string, sha256?: string) => setOpen({ path, sha256 }) }), []);
  const close = useCallback(() => setOpen(null), []);
  return (
    <DocReaderContext.Provider value={ctx}>
      <div className={open === null ? "contents" : "grid h-full min-h-0 grid-cols-1 lg:grid-cols-2"}>
        <div className={open === null ? "contents" : "min-h-0 overflow-auto"}>{children}</div>
        {open !== null ? <DocPane key={`${open.path}#${open.sha256 ?? ""}`} api={api} path={open.path} sha256={open.sha256} onClose={close} /> : null}
      </div>
    </DocReaderContext.Provider>
  );
}
