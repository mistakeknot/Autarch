import { createHash } from "node:crypto";
import { closeSync, constants, fstatSync, lstatSync, openSync, readSync } from "node:fs";
import { homedir } from "node:os";
import { isAbsolute, join } from "node:path";
import { detectKind } from "./otherimages.js";

export function defaultEvidenceRoot(env: Record<string, string | undefined> = process.env): string {
 const override = env.AUTARCH_HOME_EVIDENCE;
 return override && isAbsolute(override) ? override : join(homedir(), ".autarch", "home-evidence");
}
/** Reads a hash, never a supplied path. All failures look like a missing image. */
export function readEvidence(sha256: string, root = defaultEvidenceRoot()): { ok: true; mime: string; data_base64: string } | { ok: false } {
 if (!/^[0-9a-f]{64}$/.test(sha256)) return { ok: false };
 let fd: number | undefined;
 try {
  const dir = lstatSync(root);
  if (!dir.isDirectory() || dir.isSymbolicLink()) return { ok: false };
  fd = openSync(join(root, sha256), constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  const st = fstatSync(fd);
  if (!st.isFile() || st.size > 2 * 1024 * 1024) return { ok: false };
  const buf = Buffer.alloc(2 * 1024 * 1024 + 1);
  let n = 0;
  while (n < buf.length) { const got = readSync(fd, buf, n, buf.length - n, null); if (!got) break; n += got; }
  const b = buf.subarray(0, n);
  if (b.length > 2 * 1024 * 1024 || createHash("sha256").update(b).digest("hex") !== sha256) return { ok: false };
  const kind = detectKind(b);
  if (!kind) return { ok: false };
  return { ok: true, mime: `image/${kind}`, data_base64: b.toString("base64") };
 } catch { return { ok: false }; }
 finally { if (fd !== undefined) closeSync(fd); }
}
