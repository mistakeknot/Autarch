import type { Delegation } from "./asks.js";

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

/** Routing and machine owners are shown read-only in v1: no RPC sets them. */
export function SettingsPanel({ delegation, machineOwners, onSave }: { delegation: Delegation; machineOwners: Record<string, string>; onSave: (v: DelegationInput) => void }) {
  const s = delegation.settings;
  return (
    <form
      className="space-y-3 p-4 text-sm"
      onSubmit={(e) => {
        e.preventDefault();
        const d = new FormData(e.currentTarget);
        const r = parseDelegationForm({ vizierThreadId: String(d.get("vizierThreadId") ?? ""), projects: String(d.get("projects") ?? ""), dailyCap: String(d.get("dailyCap") ?? "") });
        if (r.ok) onSave(r.value);
      }}
    >
      <h2 className="text-xs font-semibold uppercase text-muted-foreground">Delegation</h2>
      {delegation.suspended ? <p role="status" className="text-destructive">delegation suspended until you see this change</p> : null}
      <label className="block">Vizier thread<input name="vizierThreadId" defaultValue={s.vizierThreadId ?? ""} className="mt-1 block w-full rounded border border-border bg-background px-2 py-1" /></label>
      <label className="block">Projects (comma separated)<input name="projects" defaultValue={(s.projects ?? []).join(", ")} className="mt-1 block w-full rounded border border-border bg-background px-2 py-1" /></label>
      <label className="block">Daily cap<input name="dailyCap" defaultValue={String(s.dailyCap ?? 0)} className="mt-1 block w-full rounded border border-border bg-background px-2 py-1" /></label>
      <button type="submit" className="rounded border border-border px-3 py-1">Save</button>
      <h2 className="pt-4 text-xs font-semibold uppercase text-muted-foreground">Machine owners (read-only)</h2>
      <ul>{Object.entries(machineOwners).map(([k, v]) => <li key={k}>{`${k}: ${v}`}</li>)}</ul>
    </form>
  );
}
