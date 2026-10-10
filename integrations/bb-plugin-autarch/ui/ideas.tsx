// The Idea box and the weekly idea digest (bead mk-2zojo). Presentation plus one small container; the data comes from
// the ideas RPCs, which file and act over the tasks plugin. mk's words are shown as plain text, never rendered as markup.
import { useCallback, useEffect, useMemo, useState } from "react";
import type { IdeaAction, IdeaProject, IdeaView } from "../ideas.js";
import { IDEA_MAX } from "../ideas.js";
import { ActionButton } from "./buttons.js";

export interface IdeaApi {
  projects: () => Promise<IdeaProject[]>;
  digest: () => Promise<{ ideas: IdeaView[]; show: boolean }>;
  file: (project_id: string, text: string, idea_id: string) => Promise<{ ok: boolean; error?: string }>;
  act: (task_id: string, action: IdeaAction) => Promise<{ ok: boolean; error?: string }>;
  clear: () => Promise<void>;
}

const ACTIONS: [IdeaAction, string, "recommended" | "default" | "destructive"][] = [["pursue", "Pursue", "recommended"], ["park", "Park", "default"], ["drop", "Drop", "destructive"]];

/** The Idea box: pick a project, type one line, File. Stateless; the container owns the submission id. */
export function IdeaBox({ projects, project, text, busy, error, notice, onProject, onText, onFile }: {
  projects: IdeaProject[];
  project: string;
  text: string;
  busy: boolean;
  error: string | null;
  notice: string | null;
  onProject: (id: string) => void;
  onText: (t: string) => void;
  onFile: () => void;
}) {
  const left = IDEA_MAX - [...text].length;
  return (
    <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); onFile(); }} data-idea-box>
      <label className="block text-xs font-semibold uppercase text-muted-foreground" htmlFor="idea-project">Project</label>
      <select id="idea-project" className="min-h-11 w-full rounded-md border border-input bg-background px-2 text-sm sm:min-h-8" value={project} disabled={busy} onChange={(e) => onProject(e.target.value)} data-idea-project>
        <option value="">Choose a project</option>
        {projects.map((p) => <option key={p.id} value={p.id}>{`${p.name} (${p.prefix})`}</option>)}
      </select>
      <label className="block text-xs font-semibold uppercase text-muted-foreground" htmlFor="idea-text">Your idea, in a line</label>
      <textarea id="idea-text" rows={2} maxLength={IDEA_MAX} className="w-full rounded-md border border-input bg-background p-2 text-sm" value={text} disabled={busy} onChange={(e) => onText(e.target.value)} placeholder="It would be cool if…" data-idea-text />
      <div className="flex flex-wrap items-center gap-2">
        <ActionButton tone="recommended" type="submit" disabled={busy || project === "" || text.trim() === ""} data-idea-file>File idea</ActionButton>
        <span className="text-xs text-muted-foreground">{`${left} left. It is filed as an idea in that project and is not work until it is picked.`}</span>
      </div>
      {error !== null ? <p role="alert" className="m-0 text-xs font-medium text-destructive" data-idea-failure>{`That did not go through: ${error}`}</p> : null}
      {notice !== null ? <p role="status" className="m-0 text-xs text-muted-foreground" data-idea-filed>{notice}</p> : null}
    </form>
  );
}

/** The open ideas, each with Pursue, Park or Drop; "Not this week" only on the digest. */
export function IdeaList({ ideas, busy, error, onAct, onClear }: { ideas: IdeaView[]; busy: boolean; error: string | null; onAct: (id: string, a: IdeaAction) => void; onClear?: () => void }) {
  return (
    <div data-idea-list>
      {ideas.length === 0 ? <p className="m-0 text-sm text-muted-foreground" data-idea-empty>No open ideas.</p> : (
        <ul className="m-0 list-none space-y-2 p-0">
          {ideas.map((i) => (
            <li key={i.task_id} className="rounded-md border border-border p-2" data-idea={i.task_id}>
              <p className="m-0 text-xs text-muted-foreground">{`${i.project} · ${i.key}`}</p>
              <p className="m-0 whitespace-pre-wrap text-sm [overflow-wrap:anywhere]" data-idea-words>{i.words}</p>
              <div className="mt-1 flex flex-wrap gap-2">
                {ACTIONS.map(([a, label, tone]) => <ActionButton key={a} tone={tone} disabled={busy} onClick={() => onAct(i.task_id, a)} data-idea-act={a}>{label}</ActionButton>)}
              </div>
            </li>
          ))}
        </ul>
      )}
      {onClear && ideas.length > 0 ? <ActionButton tone="quiet" className="mt-2" disabled={busy} onClick={onClear} data-idea-clear>Not this week</ActionButton> : null}
      {error !== null ? <p role="alert" className="mt-1 text-xs font-medium text-destructive" data-idea-failure>{`That did not go through: ${error}`}</p> : null}
    </div>
  );
}

/** The container: loads on mount and when `refreshKey` changes, files with a fresh submission id per idea. */
export function IdeaDesk({ api, refreshKey = 0, onChanged, heading = true }: { api: IdeaApi; refreshKey?: number; onChanged?: () => void; heading?: boolean }) {
  const [projects, setProjects] = useState<IdeaProject[]>([]);
  const [ideas, setIdeas] = useState<IdeaView[]>([]);
  const [project, setProject] = useState("");
  const [text, setText] = useState("");
  const [ideaId, setIdeaId] = useState(() => crypto.randomUUID());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const load = useCallback(() => {
    // Both reads settle before the warning is set or cleared, so one succeeding never hides the other failing.
    void Promise.allSettled([api.projects(), api.digest()]).then(([p, d]) => {
      if (p.status === "fulfilled") setProjects(p.value);
      if (d.status === "fulfilled") setIdeas(d.value.ideas);
      const bad = [p, d].find((x): x is PromiseRejectedResult => x.status === "rejected");
      setLoadError(bad ? (bad.reason instanceof Error ? bad.reason.message : String(bad.reason)) : null);
    });
  }, [api]);
  useEffect(load, [load, refreshKey]);
  const run = useCallback(async (f: () => Promise<{ ok: boolean; error?: string }>, done: () => void) => {
    setBusy(true);
    setError(null);
    try {
      const r = await f();
      if (r.ok) { done(); load(); onChanged?.(); } else setError(r.error ?? "unknown error");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [load, onChanged]);
  const file = () => void run(() => api.file(project, text, ideaId), () => { setText(""); setIdeaId(crypto.randomUUID()); setNotice("Filed."); });
  const act = (id: string, a: IdeaAction) => void run(() => api.act(id, a), () => setNotice(null));
  return (
    <section className="space-y-3 p-2 sm:p-4" aria-label="Idea box" data-idea-desk>
      {heading ? <h2 className="m-0 text-sm font-semibold">Idea box</h2> : null}
      {loadError !== null ? <p role="alert" className="m-0 text-xs font-medium text-destructive" data-idea-load-failure>{`The ideas could not be loaded: ${loadError}. What is shown may be out of date.`}</p> : null}
      <IdeaBox projects={projects} project={project} text={text} busy={busy} error={error} notice={notice} onProject={(id) => { setProject(id); setIdeaId(crypto.randomUUID()); }} onText={(t) => { setText(t); setIdeaId(crypto.randomUUID()); setNotice(null); }} onFile={file} />
      <h3 className="m-0 text-xs font-semibold uppercase text-muted-foreground">{`Open ideas (${ideas.length})`}</h3>
      <IdeaList ideas={ideas} busy={busy} error={null} onAct={act} />
    </section>
  );
}

/** The weekly digest row: shown on the queue while the week is uncleared and ideas are open. Beside the Waiting count, not in it. */
export function IdeaDigest({ api, refreshKey = 0, onChanged }: { api: IdeaApi; refreshKey?: number; onChanged?: () => void }) {
  const [state, setState] = useState<{ ideas: IdeaView[]; show: boolean }>({ ideas: [], show: false });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const load = useCallback(() => void api.digest().then((d) => { setState(d); setLoadError(null); }, (e) => setLoadError(e instanceof Error ? e.message : String(e))), [api]);
  useEffect(load, [load, refreshKey]);
  // The digest is per ISO week: a page left open across a week boundary picks up the new week without an event.
  useEffect(() => { const t = setInterval(load, 10 * 60 * 1000); return () => clearInterval(t); }, [load]);
  const run = useMemo(() => async (f: () => Promise<{ ok: boolean; error?: string } | void>) => {
    setBusy(true);
    setError(null);
    try {
      const r = await f();
      if (r && !r.ok) setError(r.error ?? "unknown error"); else { load(); onChanged?.(); }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [load, onChanged]);
  if (!state.show && loadError === null) return null;
  return (
    <section className="m-2 rounded-lg border border-border p-3 sm:m-4" aria-label="Weekly idea digest" data-idea-digest>
      <h2 className="m-0 text-xs font-semibold uppercase text-muted-foreground">{`This week's ideas (${state.ideas.length}) · not counted in Waiting`}</h2>
      {loadError !== null ? <p role="alert" className="m-0 mt-1 text-xs font-medium text-destructive" data-idea-load-failure>{`The ideas could not be loaded: ${loadError}.`}</p> : null}
      <div className="mt-2">
        <IdeaList ideas={state.ideas} busy={busy} error={error} onAct={(id, a) => void run(() => api.act(id, a))} onClear={() => void run(() => api.clear())} />
      </div>
    </section>
  );
}
