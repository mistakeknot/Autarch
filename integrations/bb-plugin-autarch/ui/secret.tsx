import { useEffect, useRef, useState } from "react";
import { useSdk } from "@get-bb/plugin-sdk/app";
import { ActionButton } from "./buttons.js";
import type { SecretMetadata } from "../secret.js";

type Sdk = ReturnType<typeof useSdk>;
type Interaction = Awaited<ReturnType<Sdk["threads"]["interactions"]["list"]>>[number];
export type SecretState = { kind: "expired" } | { kind: "submitted" } | { kind: "waiting"; id: string; consumer: string; expires: number };
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
    return {kind:"waiting",id:i.id,consumer,expires:Math.min(expires as number,i.expiresAt!,i.createdAt+SECRET_REQUEST_MAX_TTL_MS)};
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
  if (!state) return <p role="status">Checking secret request…</p>;
  if (state.kind === "submitted") return <p role="status" data-secret-state="submitted">Submitted to the secure writer.</p>;
  if (state.kind === "expired") return <p role="status" data-secret-state="expired">No pending secret request, or it expired. Ask the thread to re-request.</p>;
  return <section className="mt-3 rounded border border-border p-3" data-secret-state="waiting">
    <p>Destination: <code>{metadata.path}</code></p><p>Project: {metadata.project}</p><p>Used by: {state.consumer}</p><p>Expires: {new Date(state.expires).toLocaleString(undefined, { timeZone: "America/Los_Angeles", timeZoneName: "short" })}</p>
    <form ref={form} autoComplete="off" onSubmit={async event => {
      event.preventDefault(); event.stopPropagation();
      if (busy || Date.now() >= state.expires) { form.current?.reset(); return; }
      const element = event.currentTarget;
      const values: Record<string, string> = Object.create(null);
      for (const name of metadata.names) values[name] = (element.elements.namedItem(name) as HTMLInputElement).value;
      element.reset(); setRevealed({}); setError(null);
      if (Object.values(values).some(v => /[\r\n]/.test(v))) { for (const name of metadata.names) delete values[name]; setError("Values cannot contain line breaks"); return; }
      if (Object.values(values).some(v => !v || v.length > 16384 || /[\r\n\x00]/.test(v))) { for (const name of metadata.names) delete values[name]; setError("Submission did not complete. Check the request and enter the values again."); return; }
      setBusy(true);
      try {
        await sdk.threads.interactions.respond({ threadId: metadata.thread, interactionId: state.id, value: { values } });
        // Keep only the status and interaction id, never a response payload or error object.
        setView({ identity, state: { kind: "submitted" }, submittedId: state.id });
      } catch { setError("Submission did not complete. Check the request and enter the values again."); }
      finally { for (const name of metadata.names) delete values[name]; setBusy(false); }
    }}>
      {metadata.names.map(name => <div key={name} className="mt-3"><label htmlFor={`secret-${state.id}-${name}`}>{name}</label><div className="flex gap-2"><input id={`secret-${state.id}-${name}`} name={name} type={revealed[name] === true ? "text" : "password"} autoComplete="off" autoCapitalize="none" autoCorrect="off" spellCheck={false} required disabled={busy} className="min-w-0 grow rounded border border-border bg-background p-2 text-base" /><ActionButton type="button" disabled={busy} aria-label={`${revealed[name] === true ? "Hide" : "Show"} ${name}`} aria-pressed={revealed[name] === true} onClick={() => setRevealed(r => ({ ...r, [name]: r[name] !== true }))}>{revealed[name] === true ? "Hide" : "Show"}</ActionButton></div></div>)}
      {error ? <p role="alert">{error}</p> : null}
      <button type="submit" disabled={busy} className="mt-3 rounded border border-border px-3 py-2">{busy ? "Submitting…" : "Submit securely"}</button>
    </form>
  </section>;
}
