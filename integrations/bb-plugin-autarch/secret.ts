// Metadata only. Never accept values or include rejected input in an error.
export interface SecretMetadata { thread: string; names: string[]; path: string; project: string }
export function parseSecret(body: string): SecretMetadata {
  const fail = (): never => { throw new Error("invalid home-secret metadata"); };
  let v: unknown;
  try { v = JSON.parse(body); } catch { return fail(); }
  if (!v || typeof v !== "object" || Array.isArray(v)) return fail();
  const m = v as Record<string, unknown>;
  if (Object.keys(m).length !== 4 || Object.keys(m).some(k => !["thread", "names", "path", "project"].includes(k))) return fail();
  if (typeof m.thread !== "string" || !/^thr_[23456789abcdefghijkmnpqrstuvwxyz]{10}$/.test(m.thread)) return fail();
  if (!Array.isArray(m.names) || !m.names.length || m.names.some(n => typeof n !== "string" || !/^[A-Za-z_][A-Za-z0-9_]*$/.test(n) || n === "__proto__") || new Set(m.names).size !== m.names.length) return fail();
  if (typeof m.path !== "string" || !m.path.startsWith("/") || /[\x00-\x1f\x7f]/.test(m.path)) return fail();
  if (typeof m.project !== "string" || !m.project.trim() || /[\x00-\x1f\x7f]/.test(m.project)) return fail();
  return { thread: m.thread, names: m.names as string[], path: m.path, project: m.project };
}
