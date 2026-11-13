// Delivery evidence is validated separately from the canonical ask hashes.
export type EvidenceImage = { sha256: string; mime: "image/png" | "image/jpeg" | "image/gif" | "image/webp" };
export type EvidenceSide = { url?: string; image?: EvidenceImage };
export type Evidence = EvidenceSide & { caption: string; before?: EvidenceSide; after?: EvidenceSide };
const fail = (): never => { throw new Error("invalid evidence: expected caption and one HTTPS url, image, or before/after pair"); };
const object = (v: unknown): Record<string, unknown> => v && typeof v === "object" && !Array.isArray(v) ? v as Record<string, unknown> : fail();
const keys = (o: Record<string, unknown>, allowed: string[]) => { if (Object.keys(o).some(k => !allowed.includes(k))) fail(); };
const plain = (s: unknown, max: number): s is string => typeof s === "string" && [...s].length <= max && !/[\p{Cc}\p{Cs}]/u.test(s);
function side(raw: unknown): EvidenceSide {
 const o = object(raw); keys(o, ["url", "image"]);
 if (("url" in o) === ("image" in o)) fail();
 if ("url" in o) {
  if (!plain(o.url, 2048) || !/^https:\/\/[^/?#]+/.test(o.url) || /[\\\s]/u.test(o.url)) fail();
  let u: URL; try { u = new URL(o.url as string); } catch { return fail(); }
  if (u.protocol !== "https:" || !u.hostname || u.username || u.password || (o.url as string).split("/")[2]?.includes("@")) fail();
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
  if (!plain(o.caption, 200) || !(o.caption as string).trim()) fail();
  const caption = o.caption as string;
  if ("before" in o || "after" in o) {
   if ("url" in o || "image" in o) fail();
   return { caption, before: side(o.before), after: side(o.after) };
  }
  const { caption: _, ...source } = o;
  return { caption, ...side(source) };
 });
}
