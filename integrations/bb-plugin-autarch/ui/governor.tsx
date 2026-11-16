import { useCallback, useEffect, useState } from "react";
import { useRealtime, useRpc } from "@get-bb/plugin-sdk/app";
import type { rpcContract } from "../server.js";
import type { View } from "../governor.js";

export function pacific(ms?: number): string {
  return ms == null ? "—" : new Intl.DateTimeFormat("en-US", { timeZone: "America/Los_Angeles", month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZoneName: "short" }).format(ms);
}
export function GovernorSummary({ view }: { view: View }) {
  const { state, decision } = view;
  return <section className="rounded-lg border p-4 space-y-2" aria-label="Quota governor">
    <h3 className="font-medium">Quota governor · {view.mode}</h3>
    {view.error && <p role="status">{view.error}</p>}
    <p>Level {state?.level ?? "—"} {state?.throttle?.name ?? ""} · pace {decision?.pace == null ? "unknown" : decision.pace.toFixed(2)} · allowed_max {state?.throttle?.allowed_max == null ? "open" : state.throttle.allowed_max}</p>
    <p>Last change: {pacific(state?.since == null ? undefined : state.since * 1000)} · {state?.why ?? "No change recorded"}</p>
    <p>Last run: {pacific(view.last_run)}</p>
    {decision?.held && <p>Protected work hold: {decision.held}</p>}
    {state?.card?.key && <p>Card: {state.card.key}</p>}
    {decision?.floor && <p>Floor: level {decision.floor.level} until {pacific(Date.parse(decision.floor.until))}</p>}
    <details><summary>Account pace details</summary><p>{decision?.note}</p>
      {view.parts?.map(p => <p key={p.id}>{p.id}: recent {(p.rate * 100).toFixed(2)} pts/h · sustainable {(p.sustainable * 100).toFixed(2)} pts/h · span {p.span_h.toFixed(2)} h</p>)}
    </details>
    {view.mode === "shadow" && <details><summary>Script comparison</summary><pre className="whitespace-pre-wrap">{JSON.stringify(view.script ?? null, null, 2)}</pre><p>Shadow proposes level {decision?.new_level ?? "—"}.</p></details>}
  </section>;
}
export function GovernorPanel() {
  const rpc = useRpc<typeof rpcContract>();
  const [view, setView] = useState<View>({ mode: "off" });
  const refresh = useCallback(() => { void rpc.call("quotaGovernor", null).then(v => setView(v as View), e => setView(v => ({ ...v, error: String(e) }))); }, [rpc]);
  useEffect(() => { refresh(); const timer = setInterval(refresh, 60000); return () => clearInterval(timer); }, [refresh]);
  useRealtime("quota-governor-changed", refresh);
  return <GovernorSummary view={view} />;
}
