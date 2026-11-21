import { it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { GovernorSummary, pacific } from "../ui/governor.js";

it("shows mode, limits, protected hold, and Pacific timestamps", () => {
  const html = renderToStaticMarkup(<GovernorSummary view={{ mode: "shadow", last_run: Date.parse("2026-01-01T12:00:00Z"), state: { level: 1, card: { key: "CARD", level: 2 }, throttle: { level: 1, name: "throttle", allowed_max: 1, cuts_protected: false, pace: 1.5, updated_at: 0, why: "pace" } }, decision: { level: 1, new_level: 1, pace: 1.5, note: "sample", why: "pace", floor: null, held: "waiting on card CARD", applied: [] }, parts: [{ id: "account", rate: .02, sustainable: .01, span_h: 2.5 }] }} />);
  for (const text of ["shadow", "throttle", "1.50", "Executor cap: 1", "Protected work hold", "CARD", "PST", "Account pace details", "Script comparison"]) expect(html).toContain(text);
  expect(html).not.toContain("allowed_max");
  expect(pacific(Date.parse("2026-07-01T12:00:00Z"))).toContain("PDT");
});
it("shows loading before the first status reply", () => {
  const html = renderToStaticMarkup(<GovernorSummary view={null} />);
  expect(html).toContain("Loading quota governor"); expect(html).not.toContain("· off");
});
it("shows first-load errors without loading, and skipped-recipient warnings", () => {
  const failed = renderToStaticMarkup(<GovernorSummary view={null} error="Status unavailable" />);
  expect(failed).toContain("Status unavailable"); expect(failed).not.toContain("Loading");
  const warning = renderToStaticMarkup(<GovernorSummary view={{ mode: "live", warnings: ["Skipped recipient missing"] }} />);
  expect(warning).toContain("Skipped recipient missing");
});
it("renders decoded RPC errors and preserves the separate loading state", () => {
  const view = JSON.parse(JSON.stringify({ mode: "shadow", error: "Quota unavailable" }));
  const error = renderToStaticMarkup(<GovernorSummary view={view} />);
  expect(error).toContain("Quota unavailable"); expect(error).not.toContain("Loading");
  const loading = renderToStaticMarkup(<GovernorSummary view={null} />);
  expect(loading).toContain("Loading quota governor"); expect(loading).not.toContain("Quota unavailable");
});
