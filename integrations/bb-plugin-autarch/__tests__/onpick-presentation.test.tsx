import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { PickTargetPresentation, PickActionStatus, AsksPanel } from "../ui/asks";
it("shows immutable cross-project title project and exact relay text", () => {
  const html = renderToStaticMarkup(<PickTargetPresentation option={{ id: "a", label: "Blue", kind: "instruction", instruction: "  Use blue.\nExactly.  ", on_pick: { schema: 1, target_thread: "thr_target", reversible: true, subject: { kind: "none" }, ruling: { bead: null, under: [] } } }} evidence={{ target: { schema: 1, thread_id: "thr_target", title: "Palette", project_id: "target", project_name: "Design", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }, cross_project: true }} />);
  expect(html).toContain("Palette"); expect(html).toContain("Design"); expect(html).toContain("Cross-project");
  expect(html).toContain("  Use blue.\nExactly.  "); expect(html).toContain("No artifact");
});
it("the actual asks panel retains action readback after the owed card disappears", () => {
  const html = renderToStaticMarkup(<AsksPanel nowMs={0} onPick={() => {}} onOpen={() => {}} data={{
    owed: [], runbook: [], lane: [], asks: [], undeliverable: [], failures: [], uncertain: [], machineOwners: {}, delegation: { settings: {}, suspended: false },
    pick_actions: [{ decision_id: "original", generation: 1, status: "already relayed", observation_only: false, ordinary_continuation: false,
      target: "Frozen target / Project", deadline: "2026-01-01T00:02:00Z", deadline_outcome: "met", first_failure: false, readback: "bb home pick-action --pick original-pick --json" }],
  }} />);
  expect(html).toContain("data-pick-action");
  expect(html).toContain("Already relayed; do not act.");
  expect(html).not.toContain("Nothing needs you.");
});
it("derived action status exposes observer/deadline/failure evidence and recovery without a retry button", () => {
  const html = renderToStaticMarkup(<PickActionStatus status={{ decision_id: "original", generation: 1, status: "delivery uncertain", observation_only: true,
    target: "Frozen target / Project", deadline: "2026-01-01T00:02:00Z", deadline_outcome: "expired", ordinary_continuation: false, first_failure: true,
    readback: "bb home pick-action --pick original-pick --json" }} />);
  expect(html).toContain("delivery uncertain");
  expect(html).toContain("Observation only");
  expect(html).toContain("Frozen target / Project");
  expect(html).toContain("2026-01-01T00:02:00Z");
  expect(html).toContain("Ruling file failed");
  expect(html).toContain("do not relay blindly");
  expect(html).toContain("bb home pick-action --pick original-pick --json");
  expect(html).not.toContain("Resend");
  expect(html).not.toContain("ordinary continuation applies");
  expect(html).toContain("expired");
});
