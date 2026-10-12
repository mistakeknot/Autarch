// GitHub references and web addresses in card text become links that open in a new tab (bead mk-qi1s7).
// owner/repo#N goes to its pull request page; GitHub redirects /pull/N to /issues/N when N is an issue.
// Only http(s) becomes an anchor, so a card cannot smuggle in another scheme.
import type { ReactNode } from "react";
import { safeHref } from "./markdown.js";

/** owner/repo#N, not inside a path or a longer word. Owner follows GitHub's rules: letters, digits, inner hyphens. */
export const GH_REF_SRC = String.raw`(?<![\w./#-])[A-Za-z0-9][A-Za-z0-9-]{0,38}\/[A-Za-z0-9._-]{1,100}#\d{1,9}\b`;
/** A web address, without trailing sentence punctuation. Bounded so a long run cannot make the scan slow. */
export const URL_SRC = String.raw`https?:\/\/[^\s)<>]{0,2000}[^\s).,;:<>'"]`;
const GH_REF = /^([A-Za-z0-9][A-Za-z0-9-]{0,38})\/([A-Za-z0-9._-]{1,100})#(\d{1,9})$/;
const LINKS = new RegExp(`(${URL_SRC}|${GH_REF_SRC})`, "g");

/** Where a matched reference goes, or null when it is not a link. */
export function refHref(part: string): string | null {
  if (/^https?:\/\//.test(part)) return safeHref(part);
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
  return text.split(LINKS).map((part, i) => {
    if (i % 2 === 0) return part;
    const href = refHref(part);
    return href === null ? part : <LinkOut key={`${key}-l${i}`} href={href}>{part}</LinkOut>;
  });
}
