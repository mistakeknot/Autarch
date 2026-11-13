import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { PickTargetPresentation } from "../ui/asks";
it("shows immutable cross-project title project and exact relay text", () => {
  const html = renderToStaticMarkup(<PickTargetPresentation option={{ id: "a", label: "Blue", kind: "instruction", instruction: "  Use blue.\nExactly.  ", on_pick: { schema: 1, target_thread: "thr_target", reversible: true, subject: { kind: "none" }, ruling: { bead: null, under: [] } } }} evidence={{ target: { schema: 1, thread_id: "thr_target", title: "Palette", project_id: "target", project_name: "Design", archived: false, deleted: false, observed_at: "2026-01-01T00:00:00Z" }, cross_project: true }} />);
  expect(html).toContain("Palette"); expect(html).toContain("Design"); expect(html).toContain("Cross-project");
  expect(html).toContain("  Use blue.\nExactly.  "); expect(html).toContain("No artifact");
});
