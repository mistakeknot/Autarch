// GitHub references and web addresses in card text become links that open in a new tab (bead mk-qi1s7).
// owner/repo#N goes to its pull request page; GitHub redirects /pull/N to /issues/N when N is an issue.
// Only http(s) becomes an anchor, so a card cannot smuggle in another scheme.
import type { ReactNode } from "react";
import { safeHref } from "./markdown.js";

/** owner/repo#N, not inside a path or a longer word. Owner follows GitHub's rules: letters, digits, inner hyphens; a repo is never "." or "..". */
export const GH_REF_SRC = String.raw`(?<![\w./#-])[A-Za-z0-9][A-Za-z0-9-]{0,38}\/(?!\.\.?#)[A-Za-z0-9._-]{1,100}#\d{1,9}\b(?![@\/])`;
/** A whole web-address token, up to whitespace, an angle bracket or a backtick (a quoted address); urlParts trims what trails it. One run, so the scan stays linear. */
export const URL_SRC = String.raw`[hH][tT][tT][pP][sS]?:\/\/[^\s<>\x60]+`;
/** Longer addresses stay plain text rather than becoming a link that is cut short. */
export const URL_MAX = 2000;
const CLOSERS: Record<string, string> = { ")": "(", "]": "[", "}": "{" };
const GH_REF = /^([A-Za-z0-9][A-Za-z0-9-]{0,38})\/(?!\.\.?#)([A-Za-z0-9._-]{1,100})#(\d{1,9})$/;
const LINKS = new RegExp(`(${URL_SRC}|${GH_REF_SRC})`, "g");

/**
 * Splits a matched address token into the address and the text after it: trailing sentence punctuation and
 * closing brackets with no opener inside the address stay text, so "(see https://x.io/a)." links https://x.io/a
 * and https://en.wikipedia.org/wiki/Foo_(bar) keeps its ")". A trailing "*" is emphasis, not address. The address is null when it is too long or empty.
 */
export function urlParts(token: string): { url: string | null; rest: string } {
  // Bracket counts are taken once and kept as characters are trimmed, so a long run of closers stays linear.
  const count: Record<string, number> = {};
  for (const c of token) if (c in CLOSERS || "([{".includes(c)) count[c] = (count[c] ?? 0) + 1;
  let end = token.length;
  for (;;) {
    const c = token[end - 1];
    if (c !== undefined && ".,;:!?'\"*".includes(c)) end--;
    else if (c !== undefined && CLOSERS[c] !== undefined && (count[c] ?? 0) > (count[CLOSERS[c]] ?? 0)) {
      count[c]--;
      end--;
    } else break;
  }
  const url = token.slice(0, end);
  return { url: /^https?:\/\/./i.test(url) && url.length <= URL_MAX ? url : null, rest: token.slice(end) };
}

/** A matched reference as nodes: a link when it is one (plus any text trimmed off an address), else null. */
export function refNodes(part: string, key: string): ReactNode[] | null {
  if (/^https?:\/\//i.test(part)) {
    const { url, rest } = urlParts(part);
    const href = url === null ? null : safeHref(url);
    return href === null ? [part] : [<LinkOut key={key} href={href}>{url}</LinkOut>, rest];
  }
  const href = refHref(part);
  return href === null ? null : [<LinkOut key={key} href={href}>{part}</LinkOut>];
}

/** Where a matched reference goes, or null when it is not a link. */
export function refHref(part: string): string | null {
  if (/^https?:\/\//i.test(part)) return safeHref(part);
  const m = GH_REF.exec(part);
  return m ? `https://github.com/${m[1]}/${m[2]}/pull/${m[3]}` : null;
}

export function LinkOut({ href, className, children }: { href: string; className?: string; children: ReactNode }) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer nofollow" className={`underline [overflow-wrap:anywhere] ${className ?? ""}`.trim()} data-ref-link>
      {children}
    </a>
  );
}

/** Plain text with its GitHub references and web addresses turned into links; everything else stays text. */
export function linkify(text: string, key: string): ReactNode[] {
  return text.split(LINKS).flatMap((part, i) => (i % 2 === 0 ? [part] : (refNodes(part, `${key}-l${i}`) ?? [part])));
}
