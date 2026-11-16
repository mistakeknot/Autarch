import { it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { GovernorSummary, pacific } from "../ui/governor.js";

it("shows mode, limits, protected hold, and Pacific timestamps", () => {
  const html = renderToStaticMarkup(<GovernorSummary view={{ mode: "shadow", last_run: Date.parse("2026-01-01T12:00:00Z"), state: { level: 1, card: { key: "CARD", level: 2 }, throttle: { level: 1, name: "throttle", allowed_max: 1, cuts_protected: false, pace: 1.5, updated_at: 0, why: "pace" } }, decision: { level: 1, new_level: 1, pace: 1.5, note: "sample", why: "pace", floor: null, held: "waiting on card CARD", applied: [] }, parts: [{ id: "account", rate: .02, sustainable: .01, span_h: 2.5 }] }} />);
  for (const text of ["shadow", "throttle", "1.50", "allowed_max 1", "Protected work hold", "CARD", "PST", "Account pace details", "Script comparison"]) expect(html).toContain(text);
  expect(pacific(Date.parse("2026-07-01T12:00:00Z"))).toContain("PDT");
});
