import { expect, it } from "vitest";
import { readTargetEvidence, validatePickTarget, ownerCommentCutoff } from "../picktarget";

const thread = { id: "thr_target", title: "Palette work", projectId: "project-target", archivedAt: null, deletedAt: null };
const project = { id: "project-target", name: "Palette" };
const sdk = (t: unknown = thread, p: unknown = project) => ({ threads: { get: async () => t }, projects: { get: async () => p } });
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
  expect(validatePickTarget({ asker: "thr_asker", blocks: [], bindingConfirmed: true, linkedBbProjectId: "card-project", boundBbProjectId: "card-project" }, target).ok).toBe(false);
});
it("N2 different asker project remains eligible with consistent card binding", async () => {
  const target = await readTargetEvidence(sdk(), "thr_target");
  expect(validatePickTarget({ asker: "thr_asker", askerProjectId: "vizier-project", blocks: ["thread:thr_target"], bindingConfirmed: true, linkedBbProjectId: "card-project", boundBbProjectId: "card-project" }, target).ok).toBe(true);
});
it("N1 first generation has no owner comment cutoff", () => {
  expect(ownerCommentCutoff(1, null)).toBeNull();
});
it("N1 later generation uses previous Home observation time", () => {
  expect(ownerCommentCutoff(2, "2026-01-02T00:00:00.000Z")).toBe("2026-01-02T00:00:00.000Z");
  expect(() => ownerCommentCutoff(2, null)).toThrow();
});
