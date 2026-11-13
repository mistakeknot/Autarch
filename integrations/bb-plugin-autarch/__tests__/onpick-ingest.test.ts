import { afterEach, expect, it } from "vitest";
import { rig, cleanupEnvs } from "./card-rig";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { createHash } from "node:crypto";
import { TargetEvidenceError } from "../picktarget";
import Database from "better-sqlite3";
import { wireHome } from "../server";
import { createStoreHandle } from "../store";
import type { ServeClient } from "../serve";
afterEach(cleanupEnvs);
const declaration = { schema: 1, target_thread: "thr_target", reversible: true, subject: { kind: "none" }, ruling: { bead: null, under: [] } };
const ask = { options: [{ id: "a", label: "Blue", kind: "instruction", instruction: "  Use blue.\nExactly.  ", on_pick: declaration }, { id: "b", label: "Wait", kind: "ruling-only" }] };
it.each(["original", "bringback", "override"].flatMap(mode => ["title", "project_id", "archived", "unavailable", "unchanged"].map(field => ({ mode, field }))))("owner RPC freshness $mode / $field with unchanged Tasks body", async ({ mode, field }) => {
  let changed = false;
  const evidence = async (id: string) => {
    if (changed && id === "thr_target" && field === "unavailable") throw new Error("offline");
    return { schema: 1 as const, thread_id: id, title: changed && id === "thr_target" && field === "title" ? "Renamed" : "Work", project_id: changed && id === "thr_target" && field === "project_id" ? "other" : "p", project_name: "Project", archived: changed && id === "thr_target" && field === "archived", deleted: false, observed_at: "2026-01-01T00:00:00Z" };
  };
  const r = rig({ svc: { readTargetEvidence: evidence } });
  r.enableDelegation(); const participating = structuredClone(ask); Object.assign(participating.options[0]!, { reversible: true });
  const t = r.card({ ask: participating, blocks: "thread:thr_target" }); await r.poll();
  let g = r.gens(t.id)[0];
  if (mode !== "original") {
    expect((mode === "override" ? r.vizierPick(g.id) : r.mkPick(g.id)).ok).toBe(true);
    expect((mode === "override" ? r.dele.override(g.id, {}) : r.dele.bringBack(t.id, g.id, null, {})).ok).toBe(true);
    g = r.gens(t.id)[1];
  }
  const original = r.svc.pickTargetPresentation(g.id);
  const disposers: (() => void)[] = [];
  const bb = { log: { info() {} }, onDispose: (fn: () => void) => disposers.push(fn), background: { service() {} }, events: { on() {} }, agents: { configure() {} }, cli: { register() {} }, realtime: { publish() {} }, sdk: {
    plugins: r.fake,
    threads: { get: async ({ threadId }: { threadId: string }) => { const e = await evidence(threadId); return { id: e.thread_id, title: e.title, projectId: e.project_id, archivedAt: e.archived ? 1 : null, deletedAt: null }; } },
    projects: { get: async ({ projectId }: { projectId: string }) => ({ id: projectId, name: "Project" }) },
  } };
  const handle = createStoreHandle(() => new Database(r.env.file), {});
  try {
    const home = wireHome(bb as never, handle, { serveAddr: "127.0.0.1:1", serveTokenFile: "/nope", serveProjectDirs: [], autarchBin: "autarch" }, { serve: { projects: async () => [], healthy: async () => true } as unknown as ServeClient });
    changed = true;
    const result = await home.handlers.pick({ decision_id: g.id, option_id: "a", revision: g.revision, pick_id: "owner-fresh" });
    expect(result.ok).toBe(field === "unchanged");
    if (field === "unchanged") return;
    expect(r.svc.store.pick(g.id)).toBeUndefined();
    expect(r.svc.pickTargetPresentation(g.id)).toEqual(original);
    expect(r.db.prepare("SELECT 1 FROM events WHERE decision_id = ? AND type = 'on-pick-target-refresh'").get(g.id)).toBeDefined();
  } finally { disposers.forEach(fn => fn()); handle.dispose(); }
});
it.each(["bringback", "override"])("participating %s preserves snapshots through replacement generations", async mode => {
  const r = rig({ svc: { readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Original", project_id: id, project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }) } });
  r.enableDelegation(); const a = structuredClone(ask); Object.assign(a.options[0]!, { reversible: true }); a.options[0]!.on_pick!.reversible = false;
  Object.assign(a.options[0]!.on_pick!.ruling, { bead: "bead-e\u0301", under: ["basis-e\u0301"] });
  const t = r.card({ ask: a, blocks: "thread:thr_target" }); await r.poll();
  const g = r.gens(t.id)[0];
  if (mode === "override") expect(r.vizierPick(g.id).ok).toBe(true); else expect(r.mkPick(g.id).ok).toBe(true);
  const replacement = mode === "override" ? r.dele.override(g.id, {}) : r.dele.bringBack(t.id, g.id, null, {});
  expect(replacement.ok).toBe(true); if (!replacement.ok) return;
  expect(r.svc.pickTargetPresentation(replacement.decision_id)).toEqual(r.svc.pickTargetPresentation(g.id));
  expect(r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(replacement.decision_id)).toEqual([{ ref: "thread:thr_target" }]);
  expect(JSON.parse((r.svc.store.decision(replacement.decision_id) as any).body_json).options[0]).toMatchObject(a.options[0]!);
  expect(r.mkPick(replacement.decision_id, "second-pick").ok).toBe(true);
  const third = r.dele.bringBack(t.id, replacement.decision_id, null, {});
  expect(third.ok).toBe(true); if (!third.ok) return;
  r.restart();
  expect(r.svc.pickTargetPresentation(third.decision_id)).toEqual(r.svc.pickTargetPresentation(g.id));
  expect(r.db.prepare("SELECT ref FROM decision_blocks WHERE decision_id = ?").all(third.decision_id)).toEqual([{ ref: "thread:thr_target" }]);
  expect(JSON.parse((r.svc.store.decision(third.decision_id) as any).body_json).options[0]).toMatchObject(a.options[0]!);
});
it("retries unavailable host evidence without a Tasks edit", async () => {
  let offline = true;
  const r = rig({ svc: { readTargetEvidence: async id => {
    if (offline) throw new TargetEvidenceError("unavailable", "offline");
    return { schema: 1, thread_id: id, title: "Work", project_id: "p", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" };
  } } });
  r.enableDelegation(); const t = r.card({ ask, blocks: "thread:thr_target" }); await r.poll();
  expect(r.gens(t.id)).toHaveLength(0); offline = false; r.advance(60000); await r.poll();
  expect(r.gens(t.id)).toHaveLength(1);
});
it("reconcile enriches the static ledger once across retry", async () => {
  const r = rig({ svc: { readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Work", project_id: "p", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }) } });
  const path = join(r.env.roots.Autarch!, "ledger.jsonl");
  // Reopen with the ordinary ledger dependency enabled.
  const svc = r.env.open({ rulingsLedger: path, readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Work", project_id: "p", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }) });
  r.enableDelegation();
  const t = r.card({ ask, blocks: "thread:thr_target" }); await r.poll();
  const g = r.gens(t.id)[0];
  r.db.prepare("UPDATE decisions SET root_ino = '0' WHERE id = ?").run(g.id);
  expect(svc.pick(g.id, "a", g.revision, "pick-static", "mk", "home").ok).toBe(true);
  expect(svc.store.obligationsFor(g.id).find(o => o.kind === "ruling-file")?.state).toBe("pending");
  svc.reconcile(g.id);
  const lines = readFileSync(path, "utf8").trim().split("\n").map(x => JSON.parse(x));
  expect(lines).toHaveLength(1);
  expect(lines[0]).toMatchObject({ decision_id: g.id, pick_id: "pick-static", generation: 1, revision: g.revision, kind: "instruction", pr: null, head: null, bead: null, under: [] });
  expect(lines[0].on_pick_sha256).toMatch(/^[a-f0-9]{64}$/);
  expect(lines[0].on_pick_sha256).toBe(createHash("sha256").update('{"reversible":true,"ruling":{"bead":null,"under":[]},"schema":1,"subject":{"kind":"none"},"target_thread":"thr_target"}').digest("hex"));
  expect(lines[0]).not.toHaveProperty("outcome");
});
it("freezes real adapter evidence separately from declaration across rename and restart", async () => {
  let title = "Target title";
  const r = rig({ svc: { readTargetEvidence: async id => ({ schema: 1, thread_id: id, title, project_id: id === "thr_a" ? "asker-project" : "target-project", project_name: "Target project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }) } });
  r.enableDelegation();
  const t = r.card({ ask, blocks: "thread:thr_target" });
  await r.poll();
  const g = r.gens(t.id)[0];
  expect(g).toBeDefined();
  const events = () => r.db.prepare("SELECT detail_json FROM events WHERE type = 'on-pick-targets' AND decision_id = ?").all(g.id) as { detail_json: string }[];
  expect(events()).toHaveLength(1);
  const frozen = JSON.parse(events()[0]!.detail_json);
  expect(frozen.targets.a.title).toBe("Target title");
  expect(frozen.asker.project_id).not.toBe(frozen.targets.a.project_id);
  expect(JSON.parse(g.body_json).options[0].instruction).toBe("  Use blue.\nExactly.  ");
  title = "Renamed"; r.restart(); await r.poll();
  expect(JSON.parse(events()[0]!.detail_json)).toEqual(frozen);
});
it("direct Tasks ingest refuses an unlisted on_pick target", async () => {
  const r = rig();
  r.enableDelegation();
  const t = r.card({ ask });
  await r.poll();
  expect(r.gens(t.id)).toHaveLength(0);
  expect(r.cardRow(t.id).display_reason).toMatch(/on_pick/);
});
it("ingest refuses on_pick when trusted host evidence is unavailable", async () => {
  const r = rig();
  r.enableDelegation();
  const t = r.card({ ask, blocks: "thread:thr_target" });
  await r.poll();
  expect(r.gens(t.id)).toHaveLength(0);
  expect(r.cardRow(t.id).display_reason).toMatch(/on_pick/);
});
it.each([null, "arbitrary", "asker-project"])("N2 ingest ignores Tasks linked project %j", async linkedBbProjectId => {
  const r = rig({ svc: { readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Work", project_id: id === "thr_a" ? "asker-project" : "target-project", project_name: "Design", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }) } });
  r.tp.linkedBbProjectId = linkedBbProjectId;
  r.enableDelegation();
  const t = r.card({ ask, blocks: "thread:thr_target" }); await r.poll();
  const g = r.gens(t.id)[0]; expect(g).toBeDefined();
  expect(r.svc.pickTargetPresentation(g.id).a).toMatchObject({ cross_project: true, target: { project_id: "target-project" } });
});
it("new generation freezes its scope and rejects stale picks without changing Request", async () => {
  const r = rig({ svc: { readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: id, project_id: id, project_name: "Design", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }) } });
  r.enableDelegation(); const t = r.card({ key: "stable", ask, blocks: "thread:thr_target" }); await r.poll();
  const g1 = r.gens(t.id)[0];
  const changed = structuredClone(ask); changed.options[0]!.on_pick!.target_thread = "thr_other";
  r.edit(t, { description: r.desc({ key: "stable", ask: changed, blocks: "thread:thr_other" }) }); await r.poll();
  const g2 = r.gens(t.id)[1]; expect(g2).toBeDefined(); expect(g2.revision).not.toBe(g1.revision);
  expect(r.svc.pick(g1.id, "a", g1.revision, "stale", "mk", "home").ok).toBe(false);
  expect(r.svc.pickTargetPresentation(g1.id).a!.target.thread_id).toBe("thr_target");
  expect(r.svc.pickTargetPresentation(g2.id).a!.target.thread_id).toBe("thr_other");
  expect(r.db.prepare("SELECT COUNT(*) AS n FROM card_requests WHERE task_id = ?").get(t.id)).toEqual({ n: 1 });
});
