import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { RefText } from "../ui/asks.js";
import { Markdown } from "../ui/markdown.js";
import { refHref, URL_MAX, urlParts } from "../ui/links.js";

const anchors = (html: string) => [...html.matchAll(/<a href="([^"]*)" target="_blank" rel="noopener noreferrer nofollow"[^>]*>([^<]*)<\/a>/g)].map((m) => [m[2], m[1]]);

describe("GitHub references and URLs are links", () => {
  it("owner/repo#N goes to the pull request page; a GitHub URL links as is", () => {
    expect(refHref("mistakeknot/Sylveste#106")).toBe("https://github.com/mistakeknot/Sylveste/pull/106");
    expect(refHref("o/repo.name_x-y#7")).toBe("https://github.com/o/repo.name_x-y/pull/7");
    expect(refHref("https://github.com/o/r/issues/3")).toBe("https://github.com/o/r/issues/3");
    expect(refHref("javascript:alert(1)")).toBeNull();
  });

  it("links a card question's references, opening in a new tab, and leaves the rest as text", () => {
    const html = renderToStaticMarkup(<RefText text="Merge or decide: mistakeknot/Sylveste#106 docs (see https://github.com/o/r/pull/4)." />);
    expect(anchors(html)).toEqual([
      ["mistakeknot/Sylveste#106", "https://github.com/mistakeknot/Sylveste/pull/106"],
      ["https://github.com/o/r/pull/4", "https://github.com/o/r/pull/4"],
    ]);
    expect(html).toContain("Merge or decide: ");
    expect(html).toContain(" docs (see ");
  });

  it("does not link a path, an anchor fragment, a word with a slash, or a ref inside a URL twice", () => {
    for (const t of ["/srv/a/b#3", "docs/x.md#heading", "and/or", "a.b/c#1", "x/y#z", "#12"]) {
      expect(anchors(renderToStaticMarkup(<RefText text={t} />))).toEqual([]);
    }
    expect(anchors(renderToStaticMarkup(<RefText text="https://github.com/o/r/pull/4#issuecomment-1" />))).toHaveLength(1);
  });

  it("links references and bare URLs in comments, but not inside code", () => {
    const html = renderToStaticMarkup(<Markdown text={"Opened o/r#5, see https://github.com/o/r/pull/5.\n\n`o/r#6` and **o/r#7**"} />);
    expect(anchors(html)).toEqual([
      ["o/r#5", "https://github.com/o/r/pull/5"],
      ["https://github.com/o/r/pull/5", "https://github.com/o/r/pull/5"],
    ]);
    expect(html).toContain("<code");
  });

  it("an address keeps balanced brackets and drops a closing one it never opened, or trailing punctuation", () => {
    expect(urlParts("https://example.com/a]")).toEqual({ url: "https://example.com/a", rest: "]" });
    expect(urlParts("https://en.wikipedia.org/wiki/Foo_(bar)")).toEqual({ url: "https://en.wikipedia.org/wiki/Foo_(bar)", rest: "" });
    expect(urlParts("https://en.wikipedia.org/wiki/Foo_(bar)).")).toEqual({ url: "https://en.wikipedia.org/wiki/Foo_(bar)", rest: ")." });
    expect(urlParts("https://x.io/a?q=1&b=2,")).toEqual({ url: "https://x.io/a?q=1&b=2", rest: "," });
    expect(urlParts("https://)")).toEqual({ url: null, rest: ")" });
    for (const [t, links] of [
      ["[https://example.com/a]", [["https://example.com/a", "https://example.com/a"]]],
      ["[https://example.com]", [["https://example.com", "https://example.com/"]]],
      ["(https://en.wikipedia.org/wiki/Foo_(bar))", [["https://en.wikipedia.org/wiki/Foo_(bar)", "https://en.wikipedia.org/wiki/Foo_(bar)"]]],
    ] as const) {
      expect(anchors(renderToStaticMarkup(<RefText text={t} />))).toEqual(links);
      expect(anchors(renderToStaticMarkup(<Markdown text={t} />))).toEqual(links);
    }
    expect(renderToStaticMarkup(<RefText text="[https://example.com/a]" />)).toMatch(/<\/a>\]$/);
  });

  it("a long address is linked whole, and one past the limit stays text with nothing inside it linked", () => {
    const long = `https://example.com/${"a".repeat(URL_MAX - 30)}`;
    expect(anchors(renderToStaticMarkup(<RefText text={`${long} next`} />))).toEqual([[long, long]]);
    const over = `https://example.com/${"a".repeat(URL_MAX)}/o/r#5`;
    for (const html of [renderToStaticMarkup(<RefText text={over} />), renderToStaticMarkup(<Markdown text={over} />)]) {
      expect(anchors(html)).toEqual([]);
      expect(html).toContain(over);
    }
  });
});
