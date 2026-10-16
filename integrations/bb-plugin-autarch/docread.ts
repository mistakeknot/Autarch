// The document reader's read-only route (bead mk-5zfbg). A card names a file; this reads it for display and
// nothing else. The allowlist is the security core:
//   - only a regular file whose real path (symlinks resolved) lies under a thread-storage root,
//     a machine directory of the user's bb data folder, under thread-storage, is ever opened;
//   - the file is opened with O_NOFOLLOW and the open descriptor is checked again, so a path swapped for a
//     symlink between the check and the read is refused;
//   - the path of the file actually opened is read back from the descriptor (/proc/self/fd) and must be under a root
//     too, so a parent directory swapped for a symlink after the check cannot redirect the open; a file with more
//     than one hard link is refused, since its content may also live outside the roots;
//   - a root counts only if it is a real directory: a thread-storage directory that is itself a symlink gives nothing;
//   - secrets are refused by name, before and after resolving (.env files, keys, credentials, tokens);
//   - text types only, under a size cap, and a file with a NUL byte is refused as binary; a file that changes size
//     while it is read is refused, so a digest check never passes on a prefix.
// Residual risk, accepted and stated in the pull request: the secret check is by name. A secret saved under an
// innocent name inside thread storage can be shown to the one person who already owns that storage.
// Every refusal outside the allowlist carries the same reason, so the route does not tell a caller which paths
// exist. Nothing here writes, lists a directory for the caller, or runs anything.
import { createHash } from "node:crypto";
import { constants as fsc, closeSync, fstatSync, lstatSync, openSync, readdirSync, readlinkSync, readSync, realpathSync } from "node:fs";
import { homedir } from "node:os";
import { basename, extname, join, sep } from "node:path";

export const DOC_MAX_BYTES = 512 * 1024;
const PATH_MAX = 1024;
export const NOT_AVAILABLE = "not a document this reader can show";

const TEXT_EXT = new Set([".md", ".markdown", ".txt", ".log", ".json", ".jsonl", ".yaml", ".yml", ".toml", ".sh", ".patch", ".diff", ".csv"]);
/** A name that holds secrets or keys, whatever its extension. Checked on the given and on the resolved name. */
const SECRET_NAME = /(\.env|(^|[._-])(keys?|keyring|pem|p12|pfx|crt|cert|kdbx|netrc|npmrc|pypirc|auth|authorized_keys)([._-]|$)|^id_[a-z0-9]+|credential|secret|token|passw|private)/i;
const SECRET_DIR = new Set([".ssh", ".gnupg", ".aws", ".kube", ".docker", ".git", "node_modules"]);

export type DocResult =
  | { ok: true; path: string; size: number; sha256: string; text: string; markdown: boolean; matches: boolean | null }
  | { ok: false; error: string };

const refuse = (error = NOT_AVAILABLE): DocResult => ({ ok: false, error });

/**
 * The thread-storage roots that exist under `home`, as real paths. A root counts only when it is a real directory that
 * resolves to itself: neither `.bb-machines`, a machine directory nor `thread-storage` may be a symlink.
 */
export function docRoots(home = homedir()): string[] {
  const out: string[] = [];
  let base: string;
  let machines: string[];
  try {
    base = realpathSync(join(home, ".bb-machines"));
    // The base itself must be where it claims to be: a symlinked .bb-machines would make any directory a root.
    if (base !== join(realpathSync(home), ".bb-machines")) return out;
    machines = readdirSync(base);
  } catch {
    return out;
  }
  for (const m of machines) {
    try {
      const want = join(base, m, "thread-storage");
      if (realpathSync(want) === want && lstatSync(want).isDirectory()) out.push(want);
    } catch {
      // not a machine directory with thread storage
    }
  }
  return out;
}

function nameOk(p: string): boolean {
  const parts = p.split(sep);
  if (parts.some((s) => SECRET_DIR.has(s))) return false;
  const base = basename(p);
  return !SECRET_NAME.test(base) && TEXT_EXT.has(extname(base).toLowerCase());
}

/** The projects folder the file panel may also open (a card names scripts and docs in repositories). Real path, or null when it is not a real directory. */
export function projectRoots(home = homedir()): string[] {
  try {
    const want = join(realpathSync(home), "projects");
    return realpathSync(want) === want && lstatSync(want).isDirectory() ? [want] : [];
  } catch {
    return [];
  }
}

const underRoot = (real: string, roots: readonly string[]) => roots.some((r) => real.startsWith(r + sep));

/**
 * Reads `path` for display. `sha256`, when given, is the digest the card states: `matches` says whether the file on
 * disk still has it. The digest of the file is always returned, so a reader can compare by eye as well.
 */
export function readDoc(path: unknown, opts: { roots?: readonly string[]; maxBytes?: number; sha256?: string } = {}): DocResult {
  if (typeof path !== "string" || path.length === 0 || path.length > PATH_MAX || path.includes("\0") || !path.startsWith("/")) return refuse();
  if (path.split("/").includes("..") || !nameOk(path)) return refuse();
  const roots = opts.roots ?? docRoots();
  let real: string;
  try {
    real = realpathSync(path);
  } catch {
    return refuse();
  }
  // The name is checked again after resolving: a link named notes.md must not reach a key or an .env file.
  if (!underRoot(real, roots) || !nameOk(real)) return refuse();
  return openChecked(real, roots, opts.maxBytes ?? DOC_MAX_BYTES, opts.sha256);
}

export type OpenTarget = { ok: true; path: string } | { ok: false; error: string };

/**
 * The real path of a file the bb file panel may show for a card, or a refusal. Same allowlist as readDoc (thread
 * storage) plus the projects folder; the same name rules (no secrets, no .git or node_modules, text types only). It
 * reads nothing and opens nothing: bb's own panel shows the file, read-only.
 */
export function resolveOpenTarget(path: unknown, opts: { roots?: readonly string[] } = {}): OpenTarget {
  if (typeof path !== "string" || path.length === 0 || path.length > PATH_MAX || path.includes("\0") || !path.startsWith("/")) return refuse();
  if (path.split("/").includes("..") || !nameOk(path)) return refuse();
  const roots = opts.roots ?? [...docRoots(), ...projectRoots()];
  try {
    const real = realpathSync(path);
    if (!underRoot(real, roots) || !nameOk(real)) return refuse();
    const st = lstatSync(real);
    if (!st.isFile() || st.nlink > 1) return refuse();
    return { ok: true, path: real };
  } catch {
    return refuse();
  }
}

/**
 * Opens `file` and checks what was actually opened. Exported for the tests, which hand it a path that has not been
 * resolved to stand in for a swap between the resolve and the open, and `afterStat` to change the file between the
 * measure and the read; the route itself only reaches it through readDoc.
 */
export function openChecked(file: string, roots: readonly string[], max: number, sha256?: string, afterStat?: () => void): DocResult {
  let fd = -1;
  try {
    fd = openSync(file, fsc.O_RDONLY | fsc.O_NOFOLLOW | fsc.O_NONBLOCK);
    // Where the open file really is, now. A parent directory swapped for a symlink after the check shows here.
    const opened = readlinkSync(`/proc/self/fd/${fd}`);
    if (!underRoot(opened, roots) || !nameOk(opened)) return refuse();
    const st = fstatSync(fd);
    if (!st.isFile() || st.nlink > 1) return refuse();
    if (st.size > max) return refuse(`too large to show (${st.size} bytes; the limit is ${max})`);
    afterStat?.();
    // One byte more than the size, so a file that grew after fstat is seen and refused, not shown as its old prefix.
    const buf = Buffer.alloc(st.size + 1);
    let n = 0;
    while (n < buf.length) {
      const r = readSync(fd, buf, n, buf.length - n, n);
      if (r === 0) break;
      n += r;
    }
    if (n !== st.size || fstatSync(fd).size !== st.size) return refuse("the file changed while it was read; try again");
    const data = buf.subarray(0, n);
    if (data.includes(0)) return refuse("a binary file is not shown");
    const digest = createHash("sha256").update(data).digest("hex");
    const want = sha256 === undefined ? null : sha256.toLowerCase();
    return { ok: true, path: opened, size: n, sha256: digest, text: data.toString("utf8"), markdown: /\.(md|markdown)$/i.test(opened), matches: want === null ? null : want === digest };
  } catch {
    return refuse();
  } finally {
    if (fd >= 0) closeSync(fd);
  }
}
