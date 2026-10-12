// A deliberately small, safe Markdown subset for card text: paragraphs, "-" lists, fenced code, `code`, **bold** and
// [text](https://link); bare URLs and owner/repo#N link too. Everything is built as React elements from parsed text, never as HTML, so a card body cannot
// inject markup. Only http(s) links become anchors (new tab, noopener); every other scheme stays literal text.
// Images are NOT fetched (a remote image is a tracking beacon and a layout hazard): ![alt](url) shows as a link to
// the image when the url is https, else as its alt text.
import type { ReactNode } from "react";
import { linkify } from "./links.js";

// Every quantifier is bounded so an unmatched marker repeated many times cannot make the scan quadratic.
const BOLD = /(\*\*[^*\n]{1,500}\*\*)/;
/** Backtick spans, paired left to right; a single unmatched backtick scans to the line's end once. */
const TICKED = /(`[^`\n]*`)/;
/** A backtick span or a [text](url) link, whichever starts first, so a link's label may hold code and code may hold link syntax. */
const CODE_OR_LINK = /(`[^`\n]*`|!?\[[^\]\n]{0,200}\]\([^)\s]{0,2000}\))/;
/** Longer backtick spans show as plain text, not code. */
const CODE_MAX = 500;
const LINK = /^(!?)\[([^\]\n]*)\]\(([^)\s]*)\)$/;
/** Above this size a body is shown as plain wrapped text: no parsing at all. */
export const MARKDOWN_MAX_CHARS = 20_000;

/** Only absolute http(s) URLs may become anchors. */
export function safeHref(raw: string): string | null {
  try {
    const u = new URL(raw);
    return u.protocol === "https:" || u.protocol === "http:" ? u.href : null;
  } catch {
    return null;
  }
}

/**
 * Code spans and [text](url) links are split off first, so bold and bare references are found only between them and
 * nothing inside code ever links (bead mk-qi1s7). A code span longer than CODE_MAX shows as plain text.
 */
function inline(text: string, key: string): ReactNode[] {
  return text.split(CODE_OR_LINK).map((chunk, c) => {
    const k = `${key}-c${c}`;
    if (c % 2 === 0) return chunk.split(BOLD).map((part, i) => (i % 2 === 0 ? linkify(part, `${k}-${i}`) : <strong key={`${k}-${i}`}>{linkify(part.slice(2, -2), `${k}-${i}`)}</strong>));
    if (chunk.startsWith("`")) return code(chunk, k);
    return mdLink(chunk, k);
  });
}

function code(span: string, key: string): ReactNode {
  if (span.length - 2 > CODE_MAX) return span;
  return <code key={key} className="rounded bg-muted px-1 font-mono text-xs [overflow-wrap:anywhere]">{span.slice(1, -1)}</code>;
}

function mdLink(part: string, k: string): ReactNode {
  const m = LINK.exec(part);
  if (!m) return part;
  const [, bang, label, url] = m;
  const href = safeHref(url);
  if (href === null) return part;
  return (
    <a key={k} href={href} target="_blank" rel="noopener noreferrer nofollow" className="underline [overflow-wrap:anywhere]" data-md-link data-md-image={bang ? "" : undefined}>
      {bang ? `[image: ${label || href}]` : label ? label.split(TICKED).map((t, j) => (j % 2 === 0 ? t : code(t, `${k}-${j}`))) : href}
    </a>
  );
}

export function Markdown({ text }: { text: string }) {
  if (text.length > MARKDOWN_MAX_CHARS) return <p className="m-0 whitespace-pre-wrap" data-markdown-plain>{text}</p>;
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  const out: ReactNode[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.startsWith("```")) {
      const body: string[] = [];
      i++;
      while (i < lines.length && !lines[i].startsWith("```")) body.push(lines[i++]);
      i++;
      out.push(<pre key={out.length} className="m-0 overflow-x-auto rounded bg-muted p-2 font-mono text-xs"><code>{body.join("\n")}</code></pre>);
    } else if (/^\s*[-*] /.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^\s*[-*] /.test(lines[i])) items.push(lines[i++].replace(/^\s*[-*] /, ""));
      out.push(<ul key={out.length} className="m-0 list-disc pl-5">{items.map((t, n) => <li key={n}>{inline(t, `${out.length}-${n}`)}</li>)}</ul>);
    } else if (line.trim() === "") {
      i++;
    } else {
      const para: string[] = [];
      while (i < lines.length && lines[i].trim() !== "" && !lines[i].startsWith("```") && !/^\s*[-*] /.test(lines[i])) para.push(lines[i++]);
      out.push(<p key={out.length} className="m-0 whitespace-pre-wrap">{inline(para.join("\n"), String(out.length))}</p>);
    }
  }
  return <div className="flex min-w-0 flex-col gap-1" data-markdown>{out}</div>;
}
