import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MARKDOWN_MAX_CHARS, Markdown, safeHref } from "../ui/markdown.js";

const html = (t: string) => renderToStaticMarkup(<Markdown text={t} />);

describe("Markdown", () => {
  it("escapes raw HTML", () => {
    const h = html("<script>alert(1)</script> <img src=x onerror=y>");
    expect(h).not.toContain("<script");
    expect(h).not.toContain("<img");
    expect(h).toContain("&lt;script&gt;");
  });
  it("links only http(s)", () => {
    expect(html("[a](https://example.com/x)")).toContain('href="https://example.com/x"');
    expect(html("[a](https://example.com/x)")).toContain('rel="noopener noreferrer nofollow"');
    for (const bad of ["javascript:alert(1)", "data:text/html,x", "//evil.com", "/rel"]) {
      expect(html(`[a](${bad})`)).not.toContain("<a ");
    }
    expect(safeHref("file:///etc/passwd")).toBeNull();
  });
  it("never emits an img element and shows images as links", () => {
    const h = html("![shot](https://example.com/a.png) ![x](javascript:1)");
    expect(h).not.toContain("<img");
    expect(h).toContain("[image: shot]");
    expect(h).toContain("![x](javascript:1)");
  });
  it("renders lists, code, bold", () => {
    const h = html("**hi** `c`\n\n- one\n- two\n\n```\n<b>x</b>\n```");
    expect(h).toContain("<strong>hi</strong>");
    expect(h).toContain("<li>");
    expect(h).toContain("&lt;b&gt;x&lt;/b&gt;");
  });
  it("stays fast and does not crash on hostile input", () => {
    const hostile = ["./".repeat(10000), "http://".repeat(2800), "a/".repeat(10000), "[".repeat(16000), "`".repeat(16000), "**".repeat(8000), "```\nunclosed", "[a](".repeat(4000), "- ".repeat(5000)];
    for (const t of hostile) {
      const t0 = Date.now();
      expect(() => html(t)).not.toThrow();
      expect(Date.now() - t0).toBeLessThan(250);
    }
  });
  it("shows a very large body as plain text without parsing", () => {
    const h = html("**x** ".repeat(MARKDOWN_MAX_CHARS));
    expect(h).toContain("data-markdown-plain");
    expect(h).not.toContain("<strong>");
  });
  it("rejects more hostile link forms and escapes attributes", () => {
    for (const bad of ["vbscript:x", " javascript:alert(1)", "java&#115;cript:alert(1)", "JaVaScRiPt:x"]) {
      expect(html(`[a](${bad})`)).not.toContain("<a ");
    }
    const h = html('[x" onmouseover="y](https://example.com/a"b)');
    expect(h).not.toMatch(/<[^>]* onmouseover=/);
  });
  it("parses at the cap and falls back to plain text one character above it", () => {
    expect(html("**x**" + "a".repeat(MARKDOWN_MAX_CHARS - 5))).toContain("<strong>x</strong>");
    expect(html("**x**" + "a".repeat(MARKDOWN_MAX_CHARS - 4))).toContain("data-markdown-plain");
  });
  it("rejects schemes split by tab, CR or LF", () => {
    for (const bad of ["java\tscript:alert(1)", "java\rscript:alert(1)", "java\nscript:alert(1)", "jav&#x09;ascript:x"]) {
      expect(html(`[a](${bad})`)).not.toContain("<a ");
      expect(safeHref(bad)).toBeNull();
    }
  });
});
