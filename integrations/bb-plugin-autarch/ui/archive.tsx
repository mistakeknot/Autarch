// The Archive tab (bead mk-gc2xm): decided cards, newest first, with search and project / date / pick filters.
// Each row with a recorded pick has a Bring back button that reopens the same card as a new generation.
import { useEffect, useRef, useState } from "react";
import type { ArchiveRow } from "../archive.js";
import { ActionButton } from "./buttons.js";

export type ArchiveFilters = { search: string; project: string; from: string; to: string; pick: string };
export type ArchivePage = { rows: ArchiveRow[]; total: number; projects: string[] };
export type ArchiveApi = {
  load: (f: ArchiveFilters, offset: number, limit: number) => Promise<ArchivePage>;
  /** Resolves with an error text when the card could not be brought back, null on success. */
  bringBack: (taskId: string) => Promise<string | null>;
};

const PAGE = 50;
export const emptyFilters: ArchiveFilters = { search: "", project: "", from: "", to: "", pick: "" };

export function ArchiveRowView({ row, onBringBack, busy, error }: { row: ArchiveRow; onBringBack?: (taskId: string) => void; busy?: boolean; error?: string | null }) {
  return (
    <li className="flex flex-col gap-1 rounded-md border border-border p-3" data-archive-row={row.card_key}>
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="font-mono text-xs text-muted-foreground">{row.card_key}</span>
        <span className="text-sm font-semibold">{row.title}</span>
        <span className="ml-auto text-xs text-muted-foreground">{row.decided_at.slice(0, 16).replace("T", " ")}</span>
      </div>
      {row.no_pick ? (
        <p className="m-0 text-xs text-muted-foreground" data-archive-nopick>Closed without a pick recorded in Citadel.</p>
      ) : (
        <p className="m-0 text-sm" data-archive-pick>
          <span className="text-muted-foreground">{row.by === "mk" ? "You picked" : `${row.by} picked`}: </span>
          {row.pick_label ?? row.option_id}
          {row.reason ? <span className="text-muted-foreground"> — {row.reason}</span> : null}
        </p>
      )}
      {error ? <p className="m-0 text-xs text-destructive" role="alert" data-archive-error>Could not bring it back: {error}</p> : null}
      {onBringBack && !row.no_pick ? (
        <div>
          <ActionButton disabled={busy} onClick={() => onBringBack(row.task_id)} data-archive-bringback={row.card_key}>Bring back</ActionButton>
        </div>
      ) : null}
    </li>
  );
}

export function ArchivePanel({ api }: { api: ArchiveApi }) {
  const [f, setF] = useState<ArchiveFilters>(emptyFilters);
  const [q, setQ] = useState<ArchiveFilters>(emptyFilters);
  const [page, setPage] = useState<ArchivePage>({ rows: [], total: 0, projects: [] });
  const [more, setMore] = useState(false);
  const [ready, setReady] = useState(false);
  const gen = useRef(0);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    gen.current += 1;
    setReady(false);
    setPage((o) => ({ ...o, rows: [], total: 0 }));
    setMore(false);
    api.load(q, 0, PAGE).then(
      (p) => { if (live) { setPage(p); setErr(null); setReady(true); } },
      (e) => { if (live) setErr(e instanceof Error ? e.message : String(e)); },
    );
    return () => { live = false; };
  }, [api, q]);
  const showMore = () => {
    const mine = gen.current;
    setMore(true);
    api.load(q, page.rows.length, PAGE).then(
      (p) => { if (mine === gen.current) { setErr(null); setPage((o) => ({ ...p, rows: [...o.rows, ...p.rows.filter((r) => !o.rows.some((x) => x.task_id === r.task_id))] })); } },
      (e) => { if (mine === gen.current) setErr(e instanceof Error ? e.message : String(e)); },
    ).finally(() => { if (mine === gen.current) setMore(false); });
  };
  const [working, setWorking] = useState<string | null>(null);
  const [rowErr, setRowErr] = useState<Record<string, string>>({});
  const bringBack = (taskId: string) => {
    setWorking(taskId);
    api.bringBack(taskId).then(
      (e) => {
        if (e) setRowErr((o) => ({ ...o, [taskId]: e }));
        else {
          // The card is open again, so it leaves the archive.
          setRowErr((o) => { const { [taskId]: _gone, ...rest } = o; return rest; });
          setPage((o) => ({ ...o, rows: o.rows.filter((r) => r.task_id !== taskId), total: Math.max(0, o.total - 1) }));
        }
      },
      (e) => setRowErr((o) => ({ ...o, [taskId]: e instanceof Error ? e.message : String(e) })),
    ).finally(() => setWorking((w) => (w === taskId ? null : w)));
  };
  const set = (k: keyof ArchiveFilters) => (v: string) => setF((o) => ({ ...o, [k]: v }));
  const input = "min-h-11 rounded-md border border-border bg-background px-2 text-sm sm:min-h-0";
  return (
    <section className="flex flex-col gap-3 p-4" aria-label="Archive" data-archive-panel>
      <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); setQ({ ...f }); }}>
        <input className={`${input} grow`} type="search" placeholder="Search key, title, pick, comment" value={f.search} onChange={(e) => set("search")(e.target.value)} aria-label="Search" data-archive-search />
        <select className={input} value={f.project} onChange={(e) => set("project")(e.target.value)} aria-label="Project" data-archive-project>
          <option value="">All projects</option>
          {page.projects.map((p) => <option key={p} value={p}>{p}</option>)}
        </select>
        <input className={input} type="date" value={f.from} onChange={(e) => set("from")(e.target.value)} aria-label="From date" data-archive-from />
        <input className={input} type="date" value={f.to} onChange={(e) => set("to")(e.target.value)} aria-label="To date" data-archive-to />
        <input className={input} type="text" placeholder="Pick contains" value={f.pick} onChange={(e) => set("pick")(e.target.value)} aria-label="Pick" data-archive-pickfilter />
        <ActionButton tone="recommended" type="submit" data-archive-apply>Apply</ActionButton>
      </form>
      {err ? <p className="m-0 text-sm text-destructive" role="alert">Could not load the archive: {err}</p> : null}
      <p className="m-0 text-xs text-muted-foreground" data-archive-count>{page.total} decided card{page.total === 1 ? "" : "s"}.</p>
      <ul className="m-0 flex list-none flex-col gap-2 p-0">
        {page.rows.map((r) => <ArchiveRowView key={r.task_id} row={r} onBringBack={bringBack} busy={working !== null} error={rowErr[r.task_id] ?? null} />)}
      </ul>
      {page.rows.length < page.total ? <ActionButton disabled={more || !ready} onClick={showMore} data-archive-more>Show more</ActionButton> : null}
    </section>
  );
}
