import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DocContent, type DocResultT } from "../ui/docreader.js";
import { DocMarkdown } from "../ui/docmarkdown.js";
import { Markdown } from "../ui/markdown.js";

const html = (text: string) => renderToStaticMarkup(<DocMarkdown text={text} />);

// The shape of a design doc a thread writes: headings, prose, bullets, numbered steps, a table, a quote, a rule, code.
const DOC = [
  "# Canvas design",
  "",
  "Intro with **bold** and `code` and [a link](https://example.com/x).",
  "",
  "## Goals",
  "",
  "- first",
  "  - nested one",
  "- second",
  "",
  "1. step one",
  "2. step two",
  "",
  "### Layout",
  "",
  "| Name | Count |",
  "| :--- | ---: |",
  "| alpha | 1 |",
  "| beta | 2 |",
  "",
  "> a quoted note",
  "> over two lines",
  "",
  "---",
  "",
  "```ts",
  "const x = '# not a heading';",
  "```",
].join("\n");

describe("document reader Markdown", () => {
  it("renders headings h1 to h6 as heading elements, not raw hashes", () => {
    const out = html(["# a", "## b", "### c", "#### d", "##### e", "###### f"].join("\n\n"));
    for (const n of [1, 2, 3, 4, 5, 6]) expect(out).toMatch(new RegExp(`<h${n} `));
    expect(out).not.toContain("# ");
  });

  it("renders the whole doc shape: lists, numbered list, table, quote, rule, code", () => {
    const out = html(DOC);
    expect(out).toContain("<h1 ");
    expect(out).toContain("<h2 ");
    expect(out).toContain("<h3 ");
    expect(out).toMatch(/<ul[^>]*><li>first<ul[^>]*><li>nested one<\/li><\/ul><\/li><li>second<\/li><\/ul>/);
    expect(out).toMatch(/<ol[^>]*><li>step one<\/li><li>step two<\/li><\/ol>/);
    expect(out).toContain("<table");
    expect(out).toContain(">Name</th>");
    expect(out).toContain('text-align:right');
    expect(out).toContain(">beta</td>");
    expect(out).toMatch(/<blockquote[^>]*>.*a quoted note\nover two lines/s);
    expect(out).toContain("<hr ");
    expect(out).toContain("const x = &#x27;# not a heading&#x27;;");
    expect(out).toContain("<strong>bold</strong>");
    expect(out).not.toMatch(/(^|>)#{1,6} /);
  });

  it("sets prose in the body font and only code in mono", () => {
    const out = html(DOC);
    expect(out).toMatch(/^<div[^>]*font-sans/);
    expect(out).not.toMatch(/^<div[^>]*font-mono/);
    expect(out).toMatch(/<pre[^>]*font-mono/);
  });

  it("keeps the safety rules: no HTML, no fetched images, http(s) links only", () => {
    const out = html("<img src=x onerror=alert(1)> <script>x</script>\n\n[bad](javascript:alert(1)) ![pic](https://e.com/p.png) [ok](https://e.com)\n\n| a |\n| - |\n| <b>x</b> |");
    expect(out).not.toMatch(/<img|<script|<b>/);
    expect(out).not.toContain('href="javascript');
    expect(out).toContain('href="https://e.com/p.png"');
    expect(out).toContain('rel="noopener noreferrer nofollow"');
    expect(out).not.toMatch(/<img/);
  });

  it("does not swallow a paragraph into a heading, and a rule is not a bullet", () => {
    const out = html("para line\n# Heading\n***\n* real bullet");
    expect(out).toContain("<p");
    expect(out).toContain("<h1 ");
    expect(out).toContain("<hr ");
    expect(out).toContain("<li>real bullet</li>");
  });

  it("survives hostile nesting and long runs without throwing", () => {
    expect(() => html(">".repeat(500) + " deep")).not.toThrow();
    expect(() => html(Array.from({ length: 300 }, (_, i) => `${" ".repeat(i)}- item`).join("\n"))).not.toThrow();
    expect(() => html("|".repeat(5000))).not.toThrow();
  });

  it("is what the reader uses for a rendered .md file, and raw view stays mono", () => {
    const res: DocResultT = { ok: true, path: "/a/canvas-design.md", size: 1, sha256: "a".repeat(64), text: DOC, markdown: true, matches: null };
    const rendered = renderToStaticMarkup(<DocContent res={res} view="rendered" />);
    expect(rendered).toContain("data-doc-markdown");
    expect(rendered).toContain("<h1 ");
    const raw = renderToStaticMarkup(<DocContent res={res} view="raw" />);
    expect(raw).toContain("font-mono");
    expect(raw).toContain("# Canvas design");
  });

  it("leaves card text on its small subset: a heading stays literal there", () => {
    const card = renderToStaticMarkup(<Markdown text={"# Not a heading\n\n1. not a list"} />);
    expect(card).not.toMatch(/<h1|<ol|<table/);
  });
});

describe("document reader Markdown: hostile lines", () => {
  it("a very long whitespace or dash line renders fast", () => {
    const t0 = Date.now();
    html(`a |${" ".repeat(200000)}x\n${" ".repeat(200000)}-`);
    html(`| a |\n|${"- ".repeat(100000)}`);
    expect(Date.now() - t0).toBeLessThan(2000);
  });
});

describe("document reader Markdown: review fixes", () => {
  it("keeps the full text of a very long line", () => {
    const long = "word ".repeat(1000) + "END";
    expect(html(long)).toContain("END");
    expect(html("a | b\n" + "x".repeat(3000) + "END")).toContain("END");
  });
  it("shows list items deeper than the nesting limit", () => {
    const deep = Array.from({ length: 10 }, (_, i) => `${"  ".repeat(i)}- level${i}`).join("\n");
    const out = html(deep);
    for (let i = 0; i < 10; i++) expect(out).toContain(`level${i}`);
  });
  it("a table ends at the next block, so a heading after rows stays a heading", () => {
    const out = html("| A |\n| --- |\n| row |\n# Heading | tail\n\n- bullet | pipe");
    expect(out).toContain("<h1 ");
    expect(out).toContain("tail");
    expect(out).toContain("<ul");
    expect(out).not.toContain(">Heading | tail</td>");
  });
  it("splits sibling lists when the marker type changes", () => {
    const out = html("- bullet\n1. numbered");
    expect(out).toMatch(/<ul[^>]*><li>bullet<\/li><\/ul>/);
    expect(out).toMatch(/<ol[^>]*><li>numbered<\/li><\/ol>/);
  });
});
