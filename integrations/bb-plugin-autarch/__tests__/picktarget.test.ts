import { expect, it } from "vitest";
import { readTargetEvidence, validatePickTarget, ownerCommentCutoff } from "../picktarget";

const thread = { id: "thr_target", title: "Palette work", projectId: "project-target", archivedAt: null, deletedAt: null };
const project = { id: "project-target", name: "Palette" };
const sdk = (t: unknown = thread, p: unknown = project) => ({ threads: { get: async () => t }, projects: { get: async () => p } });
it.each(["archivedAt", "deletedAt"])("accepts numeric %s evidence and refuses liveness", async field => {
  const target = await readTargetEvidence(sdk({ ...thread, [field]: 1700000000000 }), "thr_target");
  expect(validatePickTarget({ asker: "thr_target", blocks: [], bindingConfirmed: true }, target).ok).toBe(false);
});
it("distinguishes missing, unsupported and unavailable evidence structurally", async () => {
  await expect(readTargetEvidence(sdk(null), "thr_target")).rejects.toMatchObject({ code: "not-found" });
  await expect(readTargetEvidence(sdk({ ...thread, archivedAt: "2026-01-01" }), "thr_target")).rejects.toMatchObject({ code: "unsupported" });
  await expect(readTargetEvidence({ ...sdk(), threads: { get: async () => { throw new Error("offline"); } } }, "thr_target")).rejects.toMatchObject({ code: "unavailable" });
});
it("resolves host thread project relation without inference", async () => {
  expect(await readTargetEvidence(sdk(), "thr_target")).toMatchObject({ schema: 1, thread_id: "thr_target", title: "Palette work", project_id: "project-target", project_name: "Palette", archived: false, deleted: false });
});
it.each([null, { ...thread, projectId: undefined }, { ...thread, id: "thr_wrong" }, { ...thread, archivedAt: undefined }])("refuses missing or mismatched thread metadata %j", async t => {
  await expect(readTargetEvidence(sdk(t), "thr_target")).rejects.toThrow();
});
it.each([null, { ...project, id: "wrong" }, { id: project.id }])("refuses missing or mismatched project metadata %j", async p => {
  await expect(readTargetEvidence(sdk(thread, p), "thr_target")).rejects.toThrow();
});
it("direct Tasks unlisted target is refused", async () => {
  const target = await readTargetEvidence(sdk(), "thr_target");
  expect(validatePickTarget({ asker: "thr_asker", blocks: [], bindingConfirmed: true }, target).ok).toBe(false);
});
it("N2 different asker project remains eligible", async () => {
  const target = await readTargetEvidence(sdk(), "thr_target");
  const generation = { asker: "thr_asker", askerProjectId: "vizier-project", blocks: ["thread:thr_target"], bindingConfirmed: true, linkedBbProjectId: "card-project" };
  expect(validatePickTarget(generation, target).ok).toBe(true);
});
it("N1 first generation has no owner comment cutoff", () => {
  expect(ownerCommentCutoff(1, null)).toBeNull();
});
it("N1 later generation uses previous Home observation time", () => {
  expect(ownerCommentCutoff(2, "2026-01-02T00:00:00.000Z")).toBe("2026-01-02T00:00:00.000Z");
  expect(() => ownerCommentCutoff(2, null)).toThrow();
});

it.each([undefined, null, "arbitrary-project", "project-target"])("N2 linkedBbProjectId %j leaves eligibility and target unchanged", async linkedBbProjectId => {
  const target = await readTargetEvidence(sdk(), "thr_target");
  const before = structuredClone(target);
  const generation = { asker: "thr_asker", askerProjectId: "vizier-project", blocks: ["thread:thr_target"], bindingConfirmed: true, linkedBbProjectId };
  expect(validatePickTarget(generation, target)).toEqual({ ok: true });
  expect(target).toEqual(before);
});
