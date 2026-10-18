import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { PrFacts } from "../ui/yourmove.js";

describe("PrFacts", () => {
  it("shows what merging does, the linked verdict and why it is mk's", () => {
    const h = renderToStaticMarkup(<PrFacts m={{ pr: { summary: "adds <b>X</b>", verdict: "PASS", review_url: "https://github.com/a/b/pull/1", why: "taste" } }} />);
    expect(h).toContain("Merging this: adds &lt;b&gt;X&lt;/b&gt;");
    expect(h).toContain('href="https://github.com/a/b/pull/1"');
    expect(h).toContain("Yours because it is a matter of taste.");
  });
  it("renders nothing when there are no facts", () => {
    expect(renderToStaticMarkup(<PrFacts m={{ pr: { summary: null, verdict: null, review_url: null, why: null } }} />)).toBe("");
    expect(renderToStaticMarkup(<PrFacts m={{ pr: null }} />)).toBe("");
  });
});
