import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { AskCard, RefText, type OwedAsk } from "../ui/asks.js";
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

  it("links references and bare URLs in comments, bold included, but not inside code", () => {
    const html = renderToStaticMarkup(<Markdown text={"Opened o/r#5, see https://github.com/o/r/pull/5.\n\n`o/r#6` and **o/r#7**"} />);
    expect(anchors(html)).toEqual([
      ["o/r#5", "https://github.com/o/r/pull/5"],
      ["https://github.com/o/r/pull/5", "https://github.com/o/r/pull/5"],
      ["o/r#7", "https://github.com/o/r/pull/7"],
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

  it("links a decision card's title, a bold reference and a mixed-case scheme, but not inside a long code span or an email", () => {
    expect(anchors(renderToStaticMarkup(<Markdown text={"**o/r#7** and HtTpS://x.io/a"} />))).toEqual([["o/r#7", "https://github.com/o/r/pull/7"], ["HtTpS://x.io/a", "https://x.io/a"]]);
    const code = "`" + "x ".repeat(300) + "o/r#5 https://x.io`";
    expect(anchors(renderToStaticMarkup(<Markdown text={code} />))).toEqual([]);
    expect(renderToStaticMarkup(<Markdown text={code} />)).toContain("o/r#5 https://x.io");
    expect(anchors(renderToStaticMarkup(<RefText text="a/b#1@example.com" />))).toEqual([]);
  });

  it("adversarial runs stay fast", () => {
    const start = Date.now();
    for (const t of ["https://x.io/" + ")".repeat(50_000), "/.".repeat(25_000), "`".repeat(20_000), "o/r#1".repeat(4_000)]) {
      renderToStaticMarkup(<RefText text={t} />);
      renderToStaticMarkup(<Markdown text={t.slice(0, 19_000)} />);
    }
    expect(Date.now() - start).toBeLessThan(2_000);
  });

  it("a decision card's title links its pull request", () => {
    const ask: OwedAsk = {
      id: "dec1", project: "Autarch", thread: "thr-a", subject: "Merge or decide: mistakeknot/Sylveste#106", asker: "thread",
      filed_at: "2026-09-29T10:00:00Z", revision: "rev1", mentions: 0, task_id: "task-1",
      ask: { question: "Which?", options: [{ id: "a", label: "A", kind: "ruling-only" }] },
    };
    const html = renderToStaticMarkup(<AskCard ask={ask} onPick={(() => {}) as never} onOpen={() => {}} />);
    expect(anchors(html)).toEqual([["mistakeknot/Sylveste#106", "https://github.com/mistakeknot/Sylveste/pull/106"]]);
  });

  it("nothing inside a long code span links, even with bold inside it; emphasis markers stay out of link targets", () => {
    for (const t of ["`" + "a".repeat(501) + " **o/r#5** https://x.io`", "**`o/r#5`**"]) expect(anchors(renderToStaticMarkup(<Markdown text={t} />))).toEqual([]);
    expect(anchors(renderToStaticMarkup(<Markdown text={"**see https://github.com/o/r/pull/58**"} />))).toEqual([["https://github.com/o/r/pull/58", "https://github.com/o/r/pull/58"]]);
    expect(urlParts("https://example.com/a*")).toEqual({ url: "https://example.com/a", rest: "*" });
    expect(anchors(renderToStaticMarkup(<Markdown text={"*https://example.com/a*"} />))).toEqual([["https://example.com/a", "https://example.com/a"]]);
  });

  it("backticks pair left to right, so prose between two code spans never pairs into one, and bold cannot reach into code", () => {
    const t = "`a` " + "x".repeat(600) + " `see https://evil.example/x o/r#5`";
    const html = renderToStaticMarkup(<Markdown text={t} />);
    expect(anchors(html)).toEqual([]);
    expect(html).toContain("<code");
    expect(anchors(renderToStaticMarkup(<Markdown text={"**`a **o/r#5** b`**"} />))).toEqual([]);
    expect(anchors(renderToStaticMarkup(<Markdown text={"`x` o/r#6 `y`"} />))).toEqual([["o/r#6", "https://github.com/o/r/pull/6"]]);
  });

  it("a repo named . or .. is not a reference", () => {
    for (const t of ["o/.#5", "o/..#5"]) {
      expect(refHref(t)).toBeNull();
      expect(anchors(renderToStaticMarkup(<Markdown text={t} />))).toEqual([]);
    }
    expect(refHref("o/.r#5")).toBe("https://github.com/o/.r/pull/5");
  });

  it("a path with many dotted parts is code whole", () => {
    const p = "/tmp/archive.backup.2026.10.11.release.linux.amd64.tar.gz";
    expect(renderToStaticMarkup(<RefText text={`see ${p} now`} />)).toContain(`>${p}</code>`);
  });
});
