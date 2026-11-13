/** Read-only host evidence; never an authorization or a recipient-selection fallback. */
export interface TargetEvidence {
  schema: 1;
  thread_id: string;
  title: string;
  project_id: string;
  project_name: string;
  archived: boolean;
  deleted: boolean;
  observed_at: string;
}
export interface TargetSdk {
  threads: { get(input: { threadId: string }): Promise<unknown> };
  projects: { get(input: { projectId: string }): Promise<unknown> };
}
export class TargetEvidenceError extends Error {
  constructor(readonly code: "not-found" | "unavailable" | "unsupported", detail: string) {
    super(`${code}: ${detail}`);
  }
}
function record(v: unknown): Record<string, unknown> {
  if (v === null || v === undefined) throw new TargetEvidenceError("not-found", "target metadata");
  if (typeof v !== "object" || Array.isArray(v)) throw new TargetEvidenceError("unsupported", "target metadata contract");
  return v as Record<string, unknown>;
}
function text(v: unknown): v is string { return typeof v === "string" && v.trim() !== "" && v.isWellFormed(); }
function timestamp(v: unknown): boolean { return v === null || (typeof v === "number" && Number.isFinite(v)); }
export async function readTargetEvidence(sdk: TargetSdk, threadId: string): Promise<TargetEvidence> {
  if (!/^thr_[A-Za-z0-9_.:-]+$/.test(threadId) || threadId.length > 128) throw new TargetEvidenceError("unsupported", "concrete thread required");
  let t: Record<string, unknown>;
  let p: Record<string, unknown>;
  try {
    t = record(await sdk.threads.get({ threadId }));
    if (t.id !== threadId || !text(t.title) || !text(t.projectId) || !timestamp(t.archivedAt) || !timestamp(t.deletedAt)) throw new TargetEvidenceError("unsupported", "thread metadata contract");
    p = record(await sdk.projects.get({ projectId: t.projectId }));
    if (p.id !== t.projectId || !text(p.name)) throw new TargetEvidenceError("unsupported", "project metadata contract");
  } catch (e) {
    if (e instanceof TargetEvidenceError) throw e;
    throw new TargetEvidenceError("unavailable", e instanceof Error ? e.message : "target metadata");
  }
  return { schema: 1, thread_id: threadId, title: t.title as string, project_id: t.projectId as string, project_name: p.name as string, archived: t.archivedAt !== null, deleted: t.deletedAt !== null, observed_at: new Date().toISOString() };
}
export interface GenerationSnapshot {
  asker: string;
  askerProjectId?: string;
  blocks: readonly string[];
  bindingConfirmed: boolean;
  linkedBbProjectId?: string | null;
  boundBbProjectId?: string | null;
}
export function validatePickTarget(g: GenerationSnapshot, t: TargetEvidence): { ok: boolean; reason?: string } {
  if (!g.asker || !g.bindingConfirmed) return { ok: false, reason: "asking thread and confirmed binding required" };
  if (g.linkedBbProjectId && g.linkedBbProjectId !== g.boundBbProjectId) return { ok: false, reason: "card project binding conflicts" };
  if (t.archived || t.deleted || (t.thread_id !== g.asker && !g.blocks.includes(`thread:${t.thread_id}`))) return { ok: false, reason: "target outside live generation scope" };
  return { ok: true };
}
/** PR3 consumes this rule when evaluating comments; no comment evaluation runs in PR1.
 * First generation: all mk-class comments count. Later: use Home's observation
 * of the previous generation, never Tasks edit time or current ingest time.
 */
export function ownerCommentCutoff(generation: number, previousObservedAt: string | null): string | null {
  if (generation === 1) return null;
  if (!Number.isSafeInteger(generation) || generation < 2 || !previousObservedAt || !Number.isFinite(Date.parse(previousObservedAt))) throw new Error("previous generation observation unavailable");
  return previousObservedAt;
}
