import { useEffect, useRef, useState } from "react";
import { useSdk } from "@get-bb/plugin-sdk/app";
import { ActionButton } from "./buttons.js";
import type { SecretMetadata } from "../secret.js";

type Sdk = ReturnType<typeof useSdk>;
type Interaction = Awaited<ReturnType<Sdk["threads"]["interactions"]["list"]>>[number];
export type SecretState = { kind: "expired" } | { kind: "submitted" } | { kind: "waiting"; id: string; consumer: string; expires: number; purpose: string | null; request: string; fields: {name: string; existing: boolean; description: string | null}[] };
const SECRET_REQUEST_MAX_TTL_MS = 3_600_000;
export function secretState(metadata: SecretMetadata, interactions: readonly Interaction[], now: number): SecretState {
  const matches = interactions.flatMap(i => {
    if (i.threadId !== metadata.thread || i.origin?.kind !== "plugin" || i.origin.rendererId !== "secret-request" || i.origin.pluginId !== "secrets" || i.payload.kind !== "plugin") return [];
    const d = i.payload.data as Record<string, unknown> | null;
    if (!d || typeof d !== "object") return [];
    const destination = d.destination as { path?: unknown; consumer?: unknown } | undefined;
    const fields = d.fields as { name?: unknown }[] | undefined;
    if (d.threadId !== metadata.thread || d.project !== metadata.project || destination?.path !== metadata.path || typeof destination.consumer !== "string" || !Array.isArray(fields)) return [];
    const names = fields.map(f => f?.name);
    if (names.length !== metadata.names.length || new Set(names).size !== names.length || metadata.names.some(n => !names.includes(n))) return [];
    return [{ i, consumer: destination.consumer, expires: d.expiresAtMs }];
  });
  const pending = matches.filter(({i,expires}) => i.status === "pending" && typeof expires === "number" && Number.isSafeInteger(expires) && expires > now && expires <= i.createdAt + SECRET_REQUEST_MAX_TTL_MS && typeof i.expiresAt === "number" && i.expiresAt > now);
  if (pending.length > 1) return { kind: "expired" };
  if (pending.length === 1) {
    const {i,consumer,expires} = pending[0]!;
    const data = (i.payload as {data: Record<string, unknown>}).data;
    const fields = data.fields as {name: string; existing?: unknown; description?: unknown}[];
    return {kind:"waiting",id:i.id,consumer,expires:Math.min(expires as number,i.expiresAt!,i.createdAt+SECRET_REQUEST_MAX_TTL_MS),
      purpose: typeof data.purpose === "string" ? data.purpose : null,
      request: typeof data.requestId === "string" ? data.requestId : i.id,
      fields: metadata.names.map(name => {const f=fields.find(f=>f.name===name)!;return {name,existing:f.existing===true,description:typeof f.description==="string"?f.description:null};})};
  }
  if (matches.some(({i}) => i.status === "resolved" && i.resolution && "kind" in i.resolution && i.resolution.kind === "plugin_submitted")) return {kind:"submitted"};
  return {kind:"expired"};
}

export function SecretSection({ metadata }: { metadata: SecretMetadata }) {
  const sdk = useSdk();
  const identity = JSON.stringify(metadata);
  const [view, setView] = useState<{ identity: string; state: SecretState; submittedId?: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const form = useRef<HTMLFormElement>(null);
  const state = view?.identity === identity ? view.state : null;
  useEffect(() => {
    let alive = true;
    const refresh = async () => {
      try {
        const rows = await sdk.threads.interactions.list({ threadId: metadata.thread });
        if (alive) setView(current => {
          const next = secretState(metadata, rows, Date.now());
          // A stale list must not reopen an answered request. Only a new pending id does.
          if (current?.identity === identity && current.state.kind === "submitted" && !(next.kind === "waiting" && next.id !== current.submittedId)) return current;
          return { identity, state: next };
        });
      } catch { if (alive) setView(current => current?.identity === identity && current.state.kind === "submitted" ? current : { identity, state: { kind: "expired" } }); }
    };
    void refresh();
    const timer = setInterval(() => void refresh(), 5000);
    return () => { alive = false; clearInterval(timer); form.current?.reset(); };
  }, [sdk, identity]);
  useEffect(() => {
    if (state?.kind !== "waiting") return;
    const timer = setTimeout(() => { form.current?.reset(); setRevealed({}); setView(current => current?.identity === identity && current.state.kind === "waiting" && current.state.id === state.id ? { identity, state: { kind: "expired" } } : current); }, Math.max(0, state.expires - Date.now()));
    return () => clearTimeout(timer);
  }, [state?.kind === "waiting" ? state.id : null, state?.kind === "waiting" ? state.expires : null, identity]);
  if (!state) return <p className="mt-4 text-sm leading-relaxed text-muted-foreground" role="status">Checking secret request…</p>;
  if (state.kind === "submitted") return <p className="mt-4 text-sm leading-relaxed text-muted-foreground" role="status" data-secret-state="submitted">Submitted to the secure writer.</p>;
  if (state.kind === "expired") return <p className="mt-4 text-sm leading-relaxed text-muted-foreground" role="status" data-secret-state="expired">No pending secret request, or it expired. Ask the thread to re-request.</p>;
  return <section className="@container/secrets mt-4 text-foreground" data-secret-state="waiting">
    <form ref={form} className="space-y-4" autoComplete="off" onSubmit={async event => {
      event.preventDefault(); event.stopPropagation();
      if (busy || Date.now() >= state.expires) { form.current?.reset(); return; }
      const element = event.currentTarget;
      const values: Record<string, string> = Object.create(null);
      for (const name of metadata.names) values[name] = (element.elements.namedItem(name) as HTMLInputElement).value;
      element.reset(); setRevealed({}); setError(null);
      if (Object.values(values).some(v => /[\r\n]/.test(v))) { for (const name of metadata.names) delete values[name]; setError("Values cannot contain line breaks. Every secret must be a non-empty single-line value no larger than 16 KiB."); return; }
      if (Object.values(values).some(v => !v || v.length > 16384 || /[\r\n\x00]/.test(v))) { for (const name of metadata.names) delete values[name]; setError("Every secret must be a non-empty single-line value no larger than 16 KiB."); return; }
      setBusy(true);
      try {
        await sdk.threads.interactions.respond({ threadId: metadata.thread, interactionId: state.id, value: { values } });
        // Keep only the status and interaction id, never a response payload or error object.
        setView({ identity, state: { kind: "submitted" }, submittedId: state.id });
      } catch { setError("Submission did not complete. Check the request and enter the values again."); }
      finally { for (const name of metadata.names) delete values[name]; setBusy(false); }
    }}>
      <div className="space-y-2">
        {state.purpose ? <p className="text-pretty text-sm leading-relaxed text-foreground">{state.purpose}</p> : null}
        <p className="min-w-0 text-xs text-muted-foreground">Secrets will be written directly to <code className="break-all rounded bg-surface-raised px-1.5 py-0.5 font-mono text-foreground">{metadata.path}</code></p>
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
          <dt>Project</dt><dd className="min-w-0 break-all text-foreground">{metadata.project}</dd>
          <dt>Used by</dt><dd className="min-w-0 break-all text-foreground">{state.consumer}</dd>
          <dt>Asked by thread</dt><dd className="min-w-0 break-all font-mono text-foreground">{metadata.thread}</dd>
          <dt>Request</dt><dd className="min-w-0 break-all font-mono text-foreground">{state.request}</dd>
          <dt>Expires</dt><dd className="text-foreground">{new Date(state.expires).toLocaleString(undefined, {timeZone:"America/Los_Angeles",timeZoneName:"short"})}</dd>
        </dl>
        <p className="text-xs text-muted-foreground">Agents on this host can read this file after it is written.</p>
      </div>
      <div className="space-y-3.5">
        {state.fields.map(field => {
          const name=field.name;const inputId=`secret-${state.id}-${name}`;const isRevealed=revealed[name]===true;
          return <div key={name} className="min-w-0 space-y-1.5">
            <div className="space-y-0.5">
              <label htmlFor={inputId} className="block font-mono text-xs font-semibold text-foreground" translate="no">{name}</label>
              <p className="text-xs font-medium text-muted-foreground">{field.existing ? "Replaces existing value" : "New"}</p>
              {field.description ? <p className="text-xs leading-snug text-muted-foreground">{field.description}</p> : null}
            </div>
            <div className="relative">
              <input id={inputId} name={name} type={isRevealed ? "text" : "password"} autoComplete="off" autoCapitalize="none" autoCorrect="off" spellCheck={false} required disabled={busy}
                className="block min-h-11 w-full min-w-0 rounded-md border border-input bg-card py-2 pl-3 pr-11 font-mono text-base text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50" />
              <ActionButton tone="quiet" type="button" disabled={busy} aria-label={`${isRevealed ? "Hide" : "Show"} ${name}`} aria-pressed={isRevealed}
                className="absolute right-1 top-1/2 !size-7 !min-h-0 -translate-y-1/2 !border-0 !p-0 !no-underline hover:bg-state-hover"
                onClick={()=>setRevealed(r=>({...r,[name]:r[name]!==true}))}>
                <svg viewBox="0 0 24 24" className="size-4" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
                  <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" />{isRevealed ? <path d="m3 3 18 18" /> : null}
                </svg>
              </ActionButton>
            </div>
          </div>;
        })}
      </div>
      {error ? <p data-secret-error aria-live="polite" className="rounded-md border border-surface-destructive-border bg-surface-destructive px-2 py-1 text-xs text-destructive-text">{error}</p> : null}
      <div className="flex flex-col gap-2 border-t border-border/70 pt-4 @min-[24rem]/secrets:flex-row @min-[24rem]/secrets:justify-end">
        <ActionButton type="submit" disabled={busy} className="w-full !border-transparent !bg-foreground !text-background hover:!bg-foreground/90 @min-[24rem]/secrets:w-auto">
          {busy ? <svg viewBox="0 0 24 24" className="size-3 animate-spin" width="12" height="12" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><circle cx="12" cy="12" r="9" opacity="0.25" /><path d="M12 3a9 9 0 0 1 9 9" /></svg> : null}
          Add secrets
        </ActionButton>
      </div>
    </form>
  </section>;
}
