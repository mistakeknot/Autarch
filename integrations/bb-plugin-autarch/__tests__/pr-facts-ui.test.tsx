import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { PrFacts } from "../ui/yourmove.js";

describe("PrFacts", () => {
  it("shows what merging does, the linked verdict and why it is mk's", () => {
    const h = renderToStaticMarkup(<PrFacts m={{ pr: { summary: "adds <b>X</b>", verdict: "PASS", review_url: "https://github.com/a/b/pull/1", why: "taste", door: null, door_why: null, blast: null } }} />);
    expect(h).toContain("Merging this: adds &lt;b&gt;X&lt;/b&gt;");
    expect(h).toContain('href="https://github.com/a/b/pull/1"');
    expect(h).toContain("Yours because it is a matter of taste.");
  });
  it("renders nothing when there are no facts", () => {
    expect(renderToStaticMarkup(<PrFacts m={{ pr: { summary: null, verdict: null, review_url: null, why: null, door: null, door_why: null, blast: null } }} />)).toBe("");
    expect(renderToStaticMarkup(<PrFacts m={{ pr: null }} />)).toBe("");
  });
  it("shows a one-way door with its reason and the blast radius, escaped, as attention ordering only", () => {
    const h = renderToStaticMarkup(<PrFacts m={{ pr: { summary: null, verdict: null, review_url: null, why: null, door: "one-way", door_why: "data-migration", blast: "rewrites <b>rows</b>" } }} />);
    expect(h).toContain('data-pr-door="one-way"');
    expect(h).toContain("a data migration");
    expect(h).toContain("only orders your attention");
    expect(h).toContain("Blast radius: rewrites &lt;b&gt;rows&lt;/b&gt;");
  });
  it("shows nothing extra for an unknown door value", () => {
    expect(renderToStaticMarkup(<PrFacts m={{ pr: { summary: null, verdict: null, review_url: null, why: null, door: "sideways", door_why: null, blast: null } }} />)).not.toContain("data-pr-door");
  });
});
