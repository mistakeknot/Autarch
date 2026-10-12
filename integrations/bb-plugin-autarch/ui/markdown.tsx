// A deliberately small, safe Markdown subset for card text: paragraphs, "-" lists, fenced code, `code`, **bold** and
// [text](https://link); bare URLs and owner/repo#N link too. Everything is built as React elements from parsed text, never as HTML, so a card body cannot
// inject markup. Only http(s) links become anchors (new tab, noopener); every other scheme stays literal text.
// Images are NOT fetched (a remote image is a tracking beacon and a layout hazard): ![alt](url) shows as a link to
// the image when the url is https, else as its alt text.
import type { ReactNode } from "react";
import { GH_REF_SRC, refNodes, safeHref, URL_SRC, urlParts } from "./links.js";

export { safeHref };

// Every scan is bounded so an unmatched marker repeated many times cannot make it quadratic.
/** Backtick spans, paired left to right: a ``double`` span may hold single backticks. */
const TICKED = /(``[^\n]*?``|`[^`\n]+`)/;
/**
 * Where the next inline token starts, whichever comes first: code, [text](url), a "**" bold opener, a bare address,
 * owner/repo#N. One left-to-right scan decides them all, so nothing inside code ever links (bead mk-qi1s7).
 */
const TOKEN = new RegExp(String.raw`${TICKED.source.slice(1, -1)}|!?\[[^\]\n]{0,200}\]\([^)\s]{0,2000}\)|\*\*|${URL_SRC}|${GH_REF_SRC}`, "g");
const CODE_AT = new RegExp(TICKED.source.slice(1, -1), "y");
const URL_AT = new RegExp(URL_SRC, "y");
/** A bold span is at most this long, so an opener with no closer scans a bounded distance. */
const BOLD_MAX = 1000;
/** Longer backtick spans show as plain text, not code. */
const CODE_MAX = 500;
const LINK = /^(!?)\[([^\]\n]*)\]\(([^)\s]*)\)$/;
/** Above this size a body is shown as plain wrapped text: no parsing at all. */
export const MARKDOWN_MAX_CHARS = 20_000;

function inline(text: string, key: string): ReactNode[] {
  const out: ReactNode[] = [];
  let pos = 0;
  const re = new RegExp(TOKEN);
  for (let m = re.exec(text); m !== null; m = re.exec(text)) {
    const part = m[0];
    const k = `${key}-${m.index}`;
    if (m.index > pos) out.push(text.slice(pos, m.index));
    pos = m.index + part.length;
    if (part === "**") {
      const close = boldClose(text, pos);
      if (close === -1) out.push(part);
      else {
        out.push(<strong key={k}>{inline(text.slice(pos, close), k)}</strong>);
        pos = close + 2;
      }
    } else if (part.startsWith("`")) out.push(code(part, k));
    else if (part.startsWith("[") || part.startsWith("![")) out.push(mdLink(part, k));
    else out.push(...(refNodes(part, k) ?? [part]));
    re.lastIndex = pos;
  }
  if (pos < text.length) out.push(text.slice(pos));
  return out;
}

/**
 * Where the "**" closing a bold opened just before `from` starts, or -1. Code spans and addresses inside are skipped
 * whole, so bold may hold code and a "**" inside an address is not a closer; a stray backtick, a single "*" or a line
 * break means no bold.
 */
function boldClose(text: string, from: number): number {
  // A "**" ending an address inside the bold closes it only when no later "**" does: "**see https://x/**/foo**" links
  // https://x/**/foo, while "**see https://x/**/foo/** now**" keeps the address's glob whole.
  let fallback = -1;
  let q = from;
  while (q < text.length && q - from <= BOLD_MAX) {
    const c = text[q];
    if (c === "*") return text[q + 1] === "*" && q > from ? q : fallback;
    if (c === "\n") return fallback;
    if (c === "`") {
      CODE_AT.lastIndex = q;
      const m = CODE_AT.exec(text);
      if (!m) return fallback;
      q += m[0].length;
      continue;
    }
    if (c === "h" || c === "H") {
      URL_AT.lastIndex = q;
      const m = URL_AT.exec(text);
      if (m && /^https?:\/\//i.test(m[0])) {
        let end = m[0].length;
        while (end > 0 && ".,;:!?'\"])}".includes(m[0][end - 1])) end--;
        if (fallback === -1 && m[0].endsWith("**", end) && q + end - 2 > from) fallback = q + end - 2;
        // The address as linked: emphasis trimmed off its end stays outside it, so "**see https://x.io**" closes.
        q += m[0].length - urlParts(m[0]).rest.length;
        continue;
      }
    }
    q++;
  }
  return fallback;
}

function code(span: string, key: string): ReactNode {
  const ticks = span.startsWith("``") ? 2 : 1;
  const body = span.slice(ticks, -ticks);
  if (body.length > CODE_MAX) return span;
  return <code key={key} className="rounded bg-muted px-1 font-mono text-xs [overflow-wrap:anywhere]">{ticks === 2 ? body.replace(/^ (.*) $/s, "$1") : body}</code>;
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
