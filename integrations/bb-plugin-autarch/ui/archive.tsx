// The Archive tab (bead mk-gc2xm): decided cards, newest first, with search and project / date / pick filters.
// Read-only in this slice; the Bring back button arrives with the reopen path.
import { useEffect, useState } from "react";
import type { ArchiveRow } from "../archive.js";
import { ActionButton } from "./buttons.js";

export type ArchiveFilters = { search: string; project: string; from: string; to: string; pick: string };
export type ArchivePage = { rows: ArchiveRow[]; total: number; projects: string[] };
export type ArchiveApi = { load: (f: ArchiveFilters, offset: number, limit: number) => Promise<ArchivePage> };

const PAGE = 50;
export const emptyFilters: ArchiveFilters = { search: "", project: "", from: "", to: "", pick: "" };

export function ArchiveRowView({ row }: { row: ArchiveRow }) {
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
    </li>
  );
}

export function ArchivePanel({ api }: { api: ArchiveApi }) {
  const [f, setF] = useState<ArchiveFilters>(emptyFilters);
  const [q, setQ] = useState<ArchiveFilters>(emptyFilters);
  const [page, setPage] = useState<ArchivePage>({ rows: [], total: 0, projects: [] });
  const [limit, setLimit] = useState(PAGE);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    let live = true;
    api.load(q, 0, limit).then(
      (p) => { if (live) { setPage(p); setErr(null); } },
      (e) => { if (live) setErr(e instanceof Error ? e.message : String(e)); },
    );
    return () => { live = false; };
  }, [api, q, limit]);
  const set = (k: keyof ArchiveFilters) => (v: string) => setF((o) => ({ ...o, [k]: v }));
  const input = "min-h-11 rounded-md border border-border bg-background px-2 text-sm sm:min-h-0";
  return (
    <section className="flex flex-col gap-3 p-4" aria-label="Archive" data-archive-panel>
      <form className="flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); setLimit(PAGE); setQ(f); }}>
        <input className={`${input} grow`} type="search" placeholder="Search key, title, pick, comment" value={f.search} onChange={(e) => set("search")(e.target.value)} aria-label="Search" data-archive-search />
        <select className={input} value={f.project} onChange={(e) => set("project")(e.target.value)} aria-label="Project" data-archive-project>
          <option value="">All projects</option>
          {page.projects.map((p) => <option key={p} value={p}>{p}</option>)}
        </select>
        <input className={input} type="date" value={f.from} onChange={(e) => set("from")(e.target.value)} aria-label="From date" data-archive-from />
        <input className={input} type="date" value={f.to} onChange={(e) => set("to")(e.target.value ? `${e.target.value}T23:59:59` : "")} aria-label="To date" data-archive-to />
        <input className={input} type="text" placeholder="Pick contains" value={f.pick} onChange={(e) => set("pick")(e.target.value)} aria-label="Pick" data-archive-pickfilter />
        <ActionButton tone="recommended" type="submit" data-archive-apply>Apply</ActionButton>
      </form>
      {err ? <p className="m-0 text-sm text-destructive" role="alert">Could not load the archive: {err}</p> : null}
      <p className="m-0 text-xs text-muted-foreground" data-archive-count>{page.total} decided card{page.total === 1 ? "" : "s"}.</p>
      <ul className="m-0 flex list-none flex-col gap-2 p-0">
        {page.rows.map((r) => <ArchiveRowView key={r.task_id} row={r} />)}
      </ul>
      {page.rows.length < page.total ? <ActionButton onClick={() => setLimit((n) => n + PAGE)} data-archive-more>Show more</ActionButton> : null}
    </section>
  );
}
