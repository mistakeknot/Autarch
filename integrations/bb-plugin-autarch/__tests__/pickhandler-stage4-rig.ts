import { join } from "node:path";
import { rig } from "./card-rig.js";
import { tmpDir } from "./helpers.js";
import type { ServiceDeps } from "../service.js";

/** Explicit dormant runtime injection; never used by production wiring. */
export async function recoveryRig(options: { svc?: Partial<ServiceDeps>; target?: string; blocks?: string; rulingOnly?: boolean; subject?: { kind: "github-pr"; repo: string; number: number; head: string } } = {}) {
  const temp = tmpDir();
  const target = options.target ?? "thr_target";
  const r = rig({ svc: {
    rulingsLedger: join(temp.dir, "rulings.jsonl"),
    pickCaptureRuntime: { mode: () => "live", beforeSend: async () => ({ result: "permitted", reason: "isolated fixture" }) },
    readTargetEvidence: async id => ({ schema: 1, thread_id: id, title: "Work", project_id: "project", project_name: "Project", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }),
    ...options.svc,
  } });
  r.enableDelegation();
  const task = r.card({ blocks: options.blocks ?? `thread:${target}`, ask: { recommendation: "a", options: [
    { id: "a", label: "Blue", ...(options.rulingOnly ? { kind: "ruling-only" } : { kind: "instruction", instruction: "Use blue.", reversible: true }),
      on_pick: { schema: 1, target_thread: target, reversible: true, subject: options.subject ?? { kind: "none" }, ruling: { bead: null, under: [] }, ...(options.rulingOnly ? { notification: "Blue was selected." } : {}) } },
    { id: "b", label: "Wait", kind: "ruling-only" },
  ] } });
  await r.poll();
  const g = r.gens(task.id)[0];
  r.db.prepare(`UPDATE pick_handler_control SET mode = 'live', epoch = epoch + 1, owner_enablement_json = '{}', reviewed_build = 'test-build', reviewed_scope_json = '{}', enabled_cells_json = '["instruction:none"]'`).run();
  if (options.rulingOnly) r.db.prepare("UPDATE pick_handler_control SET enabled_cells_json = ?, epoch = epoch + 1, owner_enablement_json = ?").run('["ruling-only:none"]', '{"fixture":"ruling-only"}');
  if (options.subject) r.db.prepare("UPDATE pick_handler_control SET enabled_cells_json = ?, epoch = epoch + 1, owner_enablement_json = ?").run('["instruction:github-pr"]', '{"fixture":"head"}');
  return { r, g, task, temp };
}
