import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { Markdown, safeHref } from "../ui/markdown.js";

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
});
