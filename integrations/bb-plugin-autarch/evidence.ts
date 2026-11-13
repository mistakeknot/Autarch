// Delivery evidence is validated separately from the canonical ask hashes.
export type EvidenceImage = { sha256: string; mime: "image/png" | "image/jpeg" | "image/gif" | "image/webp" };
export type EvidenceSide = { url?: string; image?: EvidenceImage };
export type Evidence = EvidenceSide & { caption: string; before?: EvidenceSide; after?: EvidenceSide };
const fail = (): never => { throw new Error("invalid evidence: expected caption and one HTTPS url, image, or before/after pair"); };
const object = (v: unknown): Record<string, unknown> => v && typeof v === "object" && !Array.isArray(v) ? v as Record<string, unknown> : fail();
const keys = (o: Record<string, unknown>, allowed: string[]) => { if (Object.keys(o).some(k => !allowed.includes(k))) fail(); };
const plain = (s: unknown, max: number): s is string => typeof s === "string" && [...s].length <= max && [...s].every(c => {
 const n = c.charCodeAt(0);
 return n > 0x1f && (n < 0x7f || n > 0x9f);
});
// Identical grammar to evidence_url.go: literal https://; ASCII host <=253 bytes,
// lowercase LDH labels of 1-63 bytes without edge hyphens or xn-- prefixes, or
// four canonical decimal IPv4 octets 0-255. Non-IP final labels contain a letter
// and do not begin 0x, avoiding browser numeric-host interpretation. Optional
// port is canonical decimal 1-65535, without leading zeros. Tail is optional
// /path, ?query, #fragment in order. ASCII unreserved A-Z a-z 0-9 . _ ~ -,
// sub-delimiters !$&'()*+,;=, : @ / and %HH are allowed; query/fragment also allow ?.
// No userinfo, IPv6, non-ASCII, raw space/controls, backslash or escaped host.
const label = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;
const tailPattern = /^(\/[A-Za-z0-9._~!$&'()*+,;=:@%/-]*)?(\?[A-Za-z0-9._~!$&'()*+,;=:@%/?-]*)?(#[A-Za-z0-9._~!$&'()*+,;=:@%/?-]*)?$/;
// Caption trim set: U+0009-000D, 0020, 00A0, 1680, 2000-200A, 2028, 2029,
// 202F, 205F, 3000, FEFF. Only emptiness is trimmed; stored text is unchanged.
function captionWhitespace(s: string): boolean {
 return [...s].every(c => {
  const n = c.charCodeAt(0);
  return (n >= 0x09 && n <= 0x0d) || n === 0x20 || n === 0xa0 || n === 0x1680 ||
   (n >= 0x2000 && n <= 0x200a) || n === 0x2028 || n === 0x2029 || n === 0x202f ||
   n === 0x205f || n === 0x3000 || n === 0xfeff;
 });
}
function evidenceURL(s: string): boolean {
 if (s.length > 2048 || !s.startsWith("https://")) return false;
 if (/[^\x21-\x7e]/u.test(s)) return false;
 const rest = s.slice("https://".length), i = rest.search(/[/?#]/);
 const authority = i < 0 ? rest : rest.slice(0, i), tail = i < 0 ? "" : rest.slice(i);
 const parts = authority.split(":"), host = parts[0];
 if (parts.length > 2) return false;
 if (parts.length === 2 && (!/^[1-9][0-9]{0,4}$/.test(parts[1]) || Number(parts[1]) > 65535)) return false;
 if (!host || host.length > 253) return false;
 const labels = host.split(".");
 if (/^[0-9.]+$/.test(host)) {
  if (labels.length !== 4 || labels.some(p => !/^(0|[1-9][0-9]{0,2})$/.test(p) || Number(p) > 255)) return false;
 } else {
  if (labels.some(p => !label.test(p) || p.startsWith("xn--"))) return false;
  const last = labels[labels.length - 1];
  if (!/[a-z]/.test(last) || last.startsWith("0x")) return false;
 }
 if (!tailPattern.test(tail)) return false;
 for (let j = 0; j < tail.length; j++) {
  if (tail[j] === "%") { if (!/^[0-9a-fA-F]{2}$/.test(tail.slice(j + 1, j + 3))) return false; j += 2; }
 }
 return true;
}
function side(raw: unknown): EvidenceSide {
 const o = object(raw); keys(o, ["url", "image"]);
 if (("url" in o) === ("image" in o)) fail();
 if ("url" in o) {
  if (typeof o.url !== "string" || !evidenceURL(o.url)) fail();
  return { url: o.url as string };
 }
 const im = object(o.image); keys(im, ["sha256", "mime"]);
 if (typeof im.sha256 !== "string" || !/^[0-9a-f]{64}$/.test(im.sha256) || !["image/png","image/jpeg","image/gif","image/webp"].includes(im.mime as string)) fail();
 return { image: { sha256: im.sha256 as string, mime: im.mime as EvidenceImage["mime"] } };
}
export function validateEvidence(raw: unknown): Evidence[] {
 if (!Array.isArray(raw) || raw.length > 6) fail();
 return (raw as unknown[]).map(v => {
  const o = object(v); keys(o, ["caption", "url", "image", "before", "after"]);
  if (!plain(o.caption, 200) || captionWhitespace(o.caption)) fail();
  const caption = o.caption as string;
  if ("before" in o || "after" in o) {
   if ("url" in o || "image" in o) fail();
   return { caption, before: side(o.before), after: side(o.after) };
  }
  const { caption: _, ...source } = o;
  return { caption, ...side(source) };
 });
}
