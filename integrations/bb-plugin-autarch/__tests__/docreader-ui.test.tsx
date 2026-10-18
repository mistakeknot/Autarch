import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { pairDigests, RefText } from "../ui/asks.js";
import { DocContent, DocReaderHost, looksLikeDoc, type DocReaderApi, type DocResultT } from "../ui/docreader.js";

const api: DocReaderApi = { read: async () => ({ ok: false, error: "x" }) };
const SHA = "a".repeat(64);
const PATH = "/srv/example/m/docs/thr_a/plan.md";
const ok = (over: Partial<Extract<DocResultT, { ok: true }>> = {}): DocResultT => ({ ok: true, path: PATH, size: 9, sha256: SHA, text: "# Title\n\n<img src=x onerror=alert(1)> **bold**", markdown: true, matches: null, ...over });
const opens = (html: string) => [...html.matchAll(/data-doc-open="([^"]*)"/g)].map((m) => m[1]);

describe("document opener", () => {
  it("shows no opener outside a reader host, so plain RefText is unchanged", () => {
    expect(opens(renderToStaticMarkup(<RefText text={`read ${PATH}`} />))).toEqual([]);
  });

  it("puts an opener beside a document path inside a host, and not beside other paths or digests", () => {
    const html = renderToStaticMarkup(
      <DocReaderHost api={api}>
        <RefText text={`read ${PATH} and /usr/local/bin/tool and ${SHA}`} />
      </DocReaderHost>,
    );
    expect(opens(html)).toEqual([PATH]);
  });

  it("adds an open button only when the host can open the file panel", () => {
    const text = `read ${PATH}`;
    const plain = renderToStaticMarkup(<DocReaderHost api={api}><RefText text={text} /></DocReaderHost>);
    expect(plain).not.toContain("data-file-open");
    const withOpen = renderToStaticMarkup(<DocReaderHost api={{ ...api, openFile: async () => ({ ok: true }) }}><RefText text={text} /></DocReaderHost>);
    expect([...withOpen.matchAll(/data-file-open="([^"]*)"/g)].map((m) => m[1])).toEqual([PATH]);
    expect(withOpen).toContain("data-doc-open");
  });

  it("recognises text document endings only", () => {
    for (const p of ["/a/b.md", "/a/b.TXT", "/a/b.json", "/a/b.sh"]) expect(looksLikeDoc(p), p).toBe(true);
    for (const p of ["/a/b", "/a/b.png", "/a/b.env", "/a/b.pem", "/a/.env"]) expect(looksLikeDoc(p), p).toBe(false);
  });
});

describe("document pane body", () => {
  it("renders Markdown without injecting markup, and shows the raw text on request", () => {
    const rendered = renderToStaticMarkup(<DocContent res={ok()} view="rendered" />);
    expect(rendered).toContain("<strong>bold</strong>");
    expect(rendered).not.toMatch(/<img/);
    const raw = renderToStaticMarkup(<DocContent res={ok()} view="raw" />);
    expect(raw).toContain("<pre");
    expect(raw).toContain("# Title");
    expect(raw).not.toMatch(/<img/);
  });

  it("shows the file's digest, and says whether it matches the card's", () => {
    expect(renderToStaticMarkup(<DocContent res={ok()} view="raw" />)).toContain(SHA);
    expect(renderToStaticMarkup(<DocContent res={ok({ matches: true })} want={SHA} view="raw" />)).toContain('data-doc-match="yes"');
    expect(renderToStaticMarkup(<DocContent res={ok({ matches: false })} want={SHA} view="raw" />)).toContain('data-doc-match="no"');
    expect(renderToStaticMarkup(<DocContent res={ok()} view="raw" />)).not.toContain("data-doc-match");
  });

  it("shows a refusal and a loading state as text", () => {
    expect(renderToStaticMarkup(<DocContent res={{ ok: false, error: "not a document this reader can show" }} view="raw" />)).toContain("not a document this reader can show");
    expect(renderToStaticMarkup(<DocContent res="loading" view="raw" />)).toContain("Loading");
  });
});

describe("pairing a digest with its path", () => {
  const SHA2 = "b".repeat(64);
  const A = "/srv/example/m/docs/thr_a/a.md";
  const B = "/srv/example/m/docs/thr_a/b.md";
  const refs = (text: string) => {
    const html = renderToStaticMarkup(
      <DocReaderHost api={api}>
        <RefText text={text} />
      </DocReaderHost>,
    );
    return html;
  };
  it("gives each of two documents its own digest, not the first one", () => {
    const parts = `${A} ${SHA}; ${B} ${SHA2}`.split(/(\/[^ ;]+|[0-9a-f]{64})/);
    const m = pairDigests(parts);
    expect([...m.values()]).toEqual([SHA, SHA2]);
  });
  it("keeps each digest with the document above it when they are on separate lines", () => {
    const parts = `${A}\nsha256: ${SHA}\n${B}\nsha256: ${SHA2}`.split(/(\/[^ ;\n]+|[0-9a-f]{64})/);
    expect([...pairDigests(parts).entries()].map(([i, v]) => [parts[i], v])).toEqual([[A, SHA], [B, SHA2]]);
  });
  it("gives a digest to the path on its own line, not to one above a blank line", () => {
    const parts = `See ${A}\n\n${SHA}  ${B}`.split(/(\/[^ ;\n]+|[0-9a-f]{64})/);
    expect([...pairDigests(parts).entries()].map(([i, v]) => [parts[i], v])).toEqual([[B, SHA]]);
  });
  it("takes a digest written before its path", () => {
    const parts = `sha256 ${SHA} for ${A}`.split(/(\/[^ ;]+|[0-9a-f]{64})/);
    expect([...pairDigests(parts).values()]).toEqual([SHA]);
  });
  it("uses a digest far from any path for none", () => {
    const parts = `${A} ${"words ".repeat(20)} ${SHA}`.split(/(\/[^ ;]+|[0-9a-f]{64})/);
    expect(pairDigests(parts).size).toBe(0);
  });
  it("still renders an opener per path", () => {
    expect([...refs(`${A} ${SHA}; ${B} ${SHA2}`).matchAll(/data-doc-open="([^"]*)"/g)].map((m) => m[1])).toEqual([A, B]);
  });
});
