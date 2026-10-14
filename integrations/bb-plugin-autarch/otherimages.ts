// Images mk pastes or attaches in the Other box. The bytes live as files, never in the database; the wake sends them as
// localImage parts next to the unchanged text. Every directory and file name here is a hash or generated, never a
// user string. A permanent per-request index holds the identity of what was sent, so a reused id with different
// images is refused even after the image files are swept.
import { createHash, randomBytes } from "node:crypto";
import {
  closeSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  openSync,
  readdirSync,
  readFileSync,
  renameSync,
  rmSync,
  writeSync,
} from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

export const IMG_MAX_COUNT = 4;
export const IMG_MAX_BYTES = 5 * 1024 * 1024;
export const IMG_MAX_TOTAL = 12 * 1024 * 1024;
/** Image sets older than this are swept; the index stays. */
export const IMG_KEEP_MS = 30 * 24 * 3600 * 1000;

export type ImageIn = { mime: string; data_base64: string };
export type ImageKind = "png" | "jpeg" | "gif" | "webp";
const MIME: Record<ImageKind, string> = { png: "image/png", jpeg: "image/jpeg", gif: "image/gif", webp: "image/webp" };
const EXT: Record<ImageKind, string> = { png: "png", jpeg: "jpg", gif: "gif", webp: "webp" };

/** The format by its signature, or null. A signature names a format; it does not prove the image decodes. */
export function detectKind(b: Buffer): ImageKind | null {
  if (b.length >= 8 && b.subarray(0, 8).equals(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]))) return "png";
  if (b.length >= 4 && b[0] === 0xff && b[1] === 0xd8 && b[2] === 0xff) return "jpeg";
  if (b.length >= 6 && (b.subarray(0, 6).toString("latin1") === "GIF87a" || b.subarray(0, 6).toString("latin1") === "GIF89a")) return "gif";
  if (b.length >= 12 && b.subarray(0, 4).toString("latin1") === "RIFF" && b.subarray(8, 12).toString("latin1") === "WEBP") return "webp";
  return null;
}

export type Decoded = { kind: ImageKind; ext: string; buf: Buffer; sha256: string };
export type Validated = { ok: true; files: Decoded[]; digest: string } | { ok: false; error: string };

const B64 = /^[A-Za-z0-9+/]*={0,2}$/;

/** Checks the request's images whole; one bad image rejects all. `digest` identifies the ordered set. */
export function validateImages(images: readonly ImageIn[] | undefined): Validated {
  const list = images ?? [];
  if (list.length > IMG_MAX_COUNT) return { ok: false, error: `at most ${IMG_MAX_COUNT} images` };
  const files: Decoded[] = [];
  let total = 0;
  for (const [i, im] of list.entries()) {
    const s = im.data_base64;
    if (s.length > Math.ceil((IMG_MAX_BYTES * 4) / 3) + 4) return { ok: false, error: `image ${i + 1} is over ${IMG_MAX_BYTES / (1024 * 1024)} MB` };
    if (s.length === 0 || s.length % 4 !== 0 || !B64.test(s)) return { ok: false, error: `image ${i + 1} is not valid base64` };
    const buf = Buffer.from(s, "base64");
    if (buf.length === 0 || buf.length > IMG_MAX_BYTES) return { ok: false, error: `image ${i + 1} is over ${IMG_MAX_BYTES / (1024 * 1024)} MB` };
    const kind = detectKind(buf);
    if (kind === null) return { ok: false, error: `image ${i + 1} is not a PNG, JPEG, GIF or WebP file` };
    if (im.mime !== MIME[kind]) return { ok: false, error: `image ${i + 1}: the file is ${MIME[kind]} but was labelled ${JSON.stringify(im.mime)}` };
    total += buf.length;
    if (total > IMG_MAX_TOTAL) return { ok: false, error: `images total over ${IMG_MAX_TOTAL / (1024 * 1024)} MB` };
    files.push({ kind, ext: EXT[kind], buf, sha256: createHash("sha256").update(buf).digest("hex") });
  }
  const digest = createHash("sha256").update(files.map((f) => f.sha256).join(",")).digest("hex");
  return { ok: true, files, digest };
}

export function defaultImagesRoot(env: Record<string, string | undefined> = process.env): string {
  return env.AUTARCH_HOME_IMAGES ?? join(homedir(), ".autarch", "home-images");
}

type Index = { v: 1; target: string; text_sha: string; digest: string; count: number; hashes: string[]; exts: string[] };
export type PutResult = { ok: true; replay: boolean } | { ok: false; status: number; error: string };
export type PathsResult = { state: "none" } | { state: "missing" } | { state: "ok"; paths: string[] };

const hex = (s: string) => createHash("sha256").update(s).digest("hex");

/** True for a real directory (not a link) at p. */
function isDir(p: string): boolean {
  try {
    const st = lstatSync(p);
    return st.isDirectory() && !st.isSymbolicLink();
  } catch {
    return false;
  }
}
function isFile(p: string): boolean {
  try {
    const st = lstatSync(p);
    return st.isFile() && !st.isSymbolicLink();
  } catch {
    return false;
  }
}

function fsyncDir(p: string): void {
  const fd = openSync(p, "r");
  try {
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
}

function writeNew(path: string, data: Buffer | string): void {
  const fd = openSync(path, "wx", 0o600); // exclusive create; never follows an existing link
  try {
    const b = typeof data === "string" ? Buffer.from(data) : data;
    for (let off = 0; off < b.length; ) off += writeSync(fd, b, off);
    fsyncSync(fd);
  } finally {
    closeSync(fd);
  }
}

export class ImageStore {
  constructor(readonly root: string, private readonly now: () => number = Date.now) {}

  private dirs(): { index: string; sets: string; tmp: string } {
    if (!isDir(this.root)) mkdirSync(this.root, { recursive: true, mode: 0o700 });
    if (!isDir(this.root)) throw new Error("images root is not a directory");
    const d = { index: join(this.root, "index"), sets: join(this.root, "sets"), tmp: join(this.root, "tmp") };
    for (const p of Object.values(d)) if (!isDir(p)) mkdirSync(p, { mode: 0o700 });
    for (const p of Object.values(d)) if (!isDir(p)) throw new Error("images directory is not a directory");
    return d;
  }

  /** The request key (an obligation op) as a path-safe name. */
  keyName(op: string): string {
    return hex(`other-images|${op}`);
  }

  /**
   * Records the request's images under `op`. Same op and same content is a replay (and repairs a set that went missing);
   * the same op with different content is refused. `target` and `textSha` bind the set to the card and the text.
   */
  put(op: string, target: string, textSha: string, v: Extract<Validated, { ok: true }>): PutResult {
    if (v.files.length === 0) return { ok: true, replay: true };
    const d = this.dirs();
    const name = this.keyName(op);
    const idxPath = join(d.index, `${name}.json`);
    const want: Index = { v: 1, target, text_sha: textSha, digest: v.digest, count: v.files.length, hashes: v.files.map((f) => f.sha256), exts: v.files.map((f) => f.ext) };
    let replay = false;
    try {
      writeNew(idxPath, JSON.stringify(want));
      fsyncDir(d.index);
    } catch (e) {
      if ((e as NodeJS.ErrnoException).code !== "EEXIST") throw e;
      const have = this.readIndex(op);
      if (!have || have.digest !== want.digest || have.target !== target || have.text_sha !== textSha) {
        return { ok: false, status: 409, error: "this request id was already used with different content" };
      }
      replay = true;
    }
    const setDir = join(d.sets, name);
    if (isDir(setDir) && this.setIntact(setDir, want)) return { ok: true, replay };
    // Stage the whole set, then publish it with one rename. A half-written set is never visible.
    const stage = join(d.tmp, `${name}.${randomBytes(6).toString("hex")}`);
    mkdirSync(stage, { mode: 0o700 });
    try {
      v.files.forEach((f, i) => writeNew(join(stage, `${i}.${f.ext}`), f.buf));
      fsyncDir(stage);
      // A damaged leftover under this hashed name. Re-checked as a real directory right before the recursive delete;
      // the store directories are owned by this process (0700), so no other process is trusted to swap them.
      if (isDir(d.sets) && isDir(setDir)) rmSync(setDir, { recursive: true, force: true });
      renameSync(stage, setDir);
      fsyncDir(d.sets);
    } catch (e) {
      rmSync(stage, { recursive: true, force: true });
      if ((e as NodeJS.ErrnoException).code === "ENOTEMPTY" || (e as NodeJS.ErrnoException).code === "EEXIST") return { ok: true, replay: true }; // a concurrent identical put won
      throw e;
    }
    return { ok: true, replay };
  }

  private readIndex(op: string): Index | null {
    const p = join(this.root, "index", `${this.keyName(op)}.json`);
    if (!isFile(p)) return null;
    try {
      const j = JSON.parse(readFileSync(p, "utf8")) as Index;
      return j && j.v === 1 && Array.isArray(j.hashes) && Array.isArray(j.exts) ? j : null;
    } catch {
      return null;
    }
  }

  /** Number of images recorded for the request, from the permanent index. 0 when none were sent. */
  expectedCount(op: string): number {
    return this.readIndex(op)?.count ?? 0;
  }

  private setIntact(setDir: string, idx: Index): boolean {
    return idx.hashes.every((h, i) => {
      const p = join(setDir, `${i}.${idx.exts[i]}`);
      if (!isFile(p)) return false;
      try {
        return createHash("sha256").update(readFileSync(p)).digest("hex") === h;
      } catch {
        return false;
      }
    });
  }

  /** The image files to send for the request: none expected, expected but missing or altered, or the verified paths. */
  paths(op: string): PathsResult {
    const idx = this.readIndex(op);
    if (!idx || idx.count === 0) return { state: "none" };
    const setDir = join(this.root, "sets", this.keyName(op));
    if (!isDir(this.root) || !isDir(join(this.root, "sets")) || !isDir(setDir) || !this.setIntact(setDir, idx)) return { state: "missing" };
    return { state: "ok", paths: idx.hashes.map((_, i) => join(setDir, `${i}.${idx.exts[i]}`)) };
  }

  /** Removes stale staging dirs and image sets past IMG_KEEP_MS or with no index. The index files stay. Returns how many it removed. */
  sweep(): number {
    if (!isDir(this.root)) return 0;
    let n = 0;
    const cut = this.now() - IMG_KEEP_MS;
    const stale = (p: string) => {
      try {
        return lstatSync(p).mtimeMs < cut;
      } catch {
        return false;
      }
    };
    for (const sub of ["tmp", "sets"] as const) {
      const dir = join(this.root, sub);
      if (!isDir(dir)) continue;
      for (const e of readdirSync(dir)) {
        const p = join(dir, e);
        const orphan = sub === "sets" && !isFile(join(this.root, "index", `${e}.json`));
        if (isDir(p) && (stale(p) || orphan)) {
          rmSync(p, { recursive: true, force: true });
          n++;
        }
      }
    }
    return n;
  }
}

export const textSha = hex;
