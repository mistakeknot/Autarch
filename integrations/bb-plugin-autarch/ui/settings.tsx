import type { Delegation } from "./asks.js";
import { useState } from "react";
import type { BindingView, UnboundView } from "./blocks.js";
import { ActionButton } from "./buttons.js";
import { CONFIRM_LABEL, ConfirmTap } from "./confirmtap.js";

export type DelegationInput = { vizierThreadId: string; projects: string[]; dailyCap: number };
export type Parsed = { ok: true; value: DelegationInput } | { ok: false; error: string };

export function parseDelegationForm(f: { vizierThreadId: string; projects: string; dailyCap: string }): Parsed {
  const vizierThreadId = f.vizierThreadId.trim();
  if (vizierThreadId === "") return { ok: false, error: "choose the vizier thread" };
  const projects = f.projects.split(",").map((p) => p.trim()).filter((p) => p !== "");
  const dailyCap = Number(f.dailyCap);
  if (f.dailyCap.trim() === "" || !Number.isInteger(dailyCap) || dailyCap < 0 || dailyCap > 1000) return { ok: false, error: "daily cap must be a whole number from 0 to 1000" };
  return { ok: true, value: { vizierThreadId, projects, dailyCap } };
}

export type BindingInput = { tasks_project_id: string; state: "confirmed" | "rejected"; home_project?: string };

/** A tasks project with cards but no binding row: mk picks the serve project, Confirm sends it as home_project. */
function UnboundRow({ u, serveProjects, onBind }: { u: UnboundView; serveProjects: string[]; onBind: (b: BindingInput) => void }) {
  const [picked, setPicked] = useState<string | null>(null);
  // The default follows the serve list (it may load after the row first renders) until mk picks one.
  const home = picked ?? u.targets.find((t) => serveProjects.includes(t)) ?? "";
  return (
    <li className="flex flex-wrap items-center gap-2" data-unbound={u.tasks_project_id}>
      {`${u.tasks_project_id}: ${u.cards} open card${u.cards === 1 ? "" : "s"}, no binding${u.targets.length > 0 ? `; asks target ${u.targets.join(", ")}` : ""}`}
      <select aria-label={`Citadel project for ${u.tasks_project_id}`} className="min-h-11 rounded-md border border-border bg-background px-2 sm:min-h-8" value={home} onChange={(e) => setPicked(e.target.value)}>
        <option value="">choose a Citadel project</option>
        {serveProjects.map((p) => <option key={p} value={p}>{p}</option>)}
      </select>
      <ActionButton disabled={home === ""} onClick={() => onBind({ tasks_project_id: u.tasks_project_id, state: "confirmed", home_project: home })}>Confirm</ActionButton>
    </li>
  );
}

/** Project bindings (plan 1.3.6): only mk confirms or rejects; a name match stays suggested and allows mk's picks only. */
export function BindingsPanel({ bindings, unbound = [], serveProjects = [], inactive, legacyCount, onBind }: { bindings: BindingView[]; unbound?: UnboundView[]; serveProjects?: string[]; inactive: string[]; legacyCount: number; onBind: (b: BindingInput) => void }) {
  return (
    <section className="space-y-2 p-4 text-sm" data-section="bindings">
      <h2 className="text-xs font-semibold uppercase text-muted-foreground">Project bindings</h2>
      <p className="text-xs text-muted-foreground">Delegation to the vizier needs a confirmed binding. A suggested binding allows your picks only.</p>
      <ul className="space-y-1">
        {bindings.map((b) => (
          <li key={b.tasks_project_id} className="flex flex-wrap items-center gap-2" data-binding={b.tasks_project_id} data-state={b.state}>
            {`${b.tasks_project_id} to ${b.home_project}: ${b.state}`}
            {b.state !== "confirmed" ? <ActionButton onClick={() => onBind({ tasks_project_id: b.tasks_project_id, state: "confirmed" })}>Confirm</ActionButton> : null}
            {b.state !== "rejected" ? (
              <ConfirmTap needsConfirm onConfirmed={() => onBind({ tasks_project_id: b.tasks_project_id, state: "rejected" })} resetKey={`${b.tasks_project_id}|${b.state}`}>
                {({ armed, onClick, onBlur }) => (
                  <ActionButton tone="destructive" onClick={onClick} onBlur={onBlur} data-reject={b.tasks_project_id} {...(armed ? { "data-confirming": "true" } : {})}>{armed ? CONFIRM_LABEL : "Reject"}</ActionButton>
                )}
              </ConfirmTap>
            ) : null}
          </li>
        ))}
        {unbound.map((u) => <UnboundRow key={u.tasks_project_id} u={u} serveProjects={serveProjects} onBind={onBind} />)}
      </ul>
      {bindings.length === 0 && unbound.length === 0 ? <p className="text-xs text-muted-foreground">No tasks project has been seen yet.</p> : null}
      {inactive.length > 0 ? <p data-inactive="true">{`Inactive delegation projects (serve does not know them): ${inactive.join(", ")}`}</p> : null}
      <p data-legacy-count={legacyCount}>{`Legacy asks still draining: ${legacyCount}`}</p>
    </section>
  );
}

/** What Save says after the server answers: null when it was saved, else the server's reason or that no answer came. */
export function saveError(answer: unknown, failed = false): string | null {
  if (failed) return "Citadel did not answer; try again";
  if (answer && typeof answer === "object" && (answer as { ok?: unknown }).ok === true) return null;
  const why = answer && typeof answer === "object" ? (answer as { error?: unknown }).error : undefined;
  return typeof why === "string" && why !== "" ? why : "Citadel refused the change";
}

/** Routing and machine owners are shown read-only in v1: no RPC sets them. */
export function SettingsPanel({ delegation, machineOwners, onSave }: { delegation: Delegation; machineOwners: Record<string, string>; onSave: (v: DelegationInput) => Promise<unknown> | void }) {
  const s = delegation.settings;
  // A form that does not parse says why next to Save instead of doing nothing.
  const [error, setError] = useState<string | null>(null);
  return (
    <form
      className="space-y-3 p-4 text-sm"
      onSubmit={(e) => {
        e.preventDefault();
        const d = new FormData(e.currentTarget);
        const r = parseDelegationForm({ vizierThreadId: String(d.get("vizierThreadId") ?? ""), projects: String(d.get("projects") ?? ""), dailyCap: String(d.get("dailyCap") ?? "") });
        setError(r.ok ? null : r.error);
        // The server can still refuse (an unknown thread, say) or not answer; either shows next to Save.
        if (r.ok) void Promise.resolve(onSave(r.value)).then((a) => setError(a === undefined ? null : saveError(a)), () => setError(saveError(null, true)));
      }}
    >
      <h2 className="text-xs font-semibold uppercase text-muted-foreground">Delegation</h2>
      {delegation.suspended ? <p role="status" className="text-destructive">Delegation is suspended until you see this change.</p> : null}
      <label className="block">Vizier thread<input name="vizierThreadId" defaultValue={s.vizierThreadId ?? ""} className="mt-1 block w-full rounded border border-border bg-background px-2 py-1" /></label>
      <label className="block">Projects (comma separated)<input name="projects" defaultValue={(s.projects ?? []).join(", ")} className="mt-1 block w-full rounded border border-border bg-background px-2 py-1" /></label>
      <label className="block">Daily cap<input name="dailyCap" defaultValue={String(s.dailyCap ?? 0)} className="mt-1 block w-full rounded border border-border bg-background px-2 py-1" /></label>
      <ActionButton type="submit" tone="recommended" data-save>Save</ActionButton>
      {error ? <p role="alert" className="text-xs font-medium text-destructive" data-save-error>{`Not saved: ${error}.`}</p> : null}
      <h2 className="pt-4 text-xs font-semibold uppercase text-muted-foreground">Machine owners (read-only)</h2>
      <ul>{Object.entries(machineOwners).map(([k, v]) => <li key={k}>{`${k}: ${v}`}</li>)}</ul>
    </form>
  );
}
