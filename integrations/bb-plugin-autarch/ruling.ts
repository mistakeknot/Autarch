// Ruling files and the root-pinned safe writer [A-19] [C-14].
//
// A ruling is a markdown file with a frontmatter block (unsigned in v1 [D16]). Every
// scalar in the frontmatter is JSON-encoded on one line, and JSON is valid YAML, so
// free text such as an instruction containing `---` or YAML-looking lines round-trips
// as a string without a YAML dependency.
//
// The writer pins the project root the way `serve` resolved it at filing (path plus
// dev and ino). Node has no openat, so a swap between the lstat checks and the write is
// possible for a process running as the same user. That residual is the same-user risk
// accepted in [D21], and is stated in the README.
import {
  closeSync,
  constants,
  fsyncSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  renameSync,
  unlinkSync,
  writeSync,
} from "node:fs";
import { randomBytes } from "node:crypto";
import { join } from "node:path";

export interface RulingOption {
  id: string;
  label: string;
}

export interface Ruling {
  decision_id: string;
  pick_id: string;
  revision: string;
  asking_thread: string;
  subject: string;
  options_shown: RulingOption[];
  picked: string;
  /** For instruction picks: the free-text instruction. */
  instruction?: string;
  ruled_by: "mk" | "vizier";
  /** Set when ruled_by is vizier. */
  delegated_reason?: string;
  ruled_at: string;
  /** The ruling in words. */
  ruling: string;
  session_id?: string;
  source: string;
  supersedes?: string;
  /** Threads that mentioned the decision. */
  mentions?: string[];
}

export type RulingErrorCode =
  | "root-changed"
  | "symlink"
  | "not-directory"
  | "exists-different"
  | "bad-name"
  | "no-uqbar";

export class RulingWriteError extends Error {
  constructor(
    readonly code: RulingErrorCode,
    message: string,
  ) {
    super(message);
  }
}

// ---- format ---------------------------------------------------------------------

const j = (v: unknown) => JSON.stringify(v);

export function renderRuling(r: Ruling): string {
  const lines = [
    "---",
    "ratification:",
    `  ruled_by: ${j(r.ruled_by)}`,
    ...(r.delegated_reason !== undefined ? [`  delegated_reason: ${j(r.delegated_reason)}`] : []),
    `  ruled_at: ${j(r.ruled_at)}`,
    `  ruling: ${j(r.ruling)}`,
    `  transcribed_by: "autarch-home"`,
    ...(r.session_id !== undefined ? [`  session_id: ${j(r.session_id)}`] : []),
    `  source: ${j(r.source)}`,
    ...(r.supersedes !== undefined ? [`  supersedes: ${j(r.supersedes)}`] : []),
    "home:",
    `  decision_id: ${j(r.decision_id)}`,
    `  pick_id: ${j(r.pick_id)}`,
    `  revision: ${j(r.revision)}`,
    `  asking_thread: ${j(r.asking_thread)}`,
    `  subject: ${j(r.subject)}`,
    `  options_shown: ${j(r.options_shown)}`,
    `  picked: ${j(r.picked)}`,
    ...(r.instruction !== undefined ? [`  instruction: ${j(r.instruction)}`] : []),
    ...(r.mentions !== undefined && r.mentions.length > 0 ? [`  mentions: ${j(r.mentions)}`] : []),
    "---",
    "",
    `# ${r.subject.replace(/\s+/g, " ").trim()}`,
    "",
    r.ruling.trim(),
    "",
  ];
  return lines.join("\n");
}

/** Parse what renderRuling wrote. Throws on anything else. */
export function parseRuling(text: string): Ruling {
  const lines = text.split("\n");
  if (lines[0] !== "---") throw new Error("ruling: missing frontmatter");
  const end = lines.indexOf("---", 1);
  if (end < 0) throw new Error("ruling: unterminated frontmatter");
  const blocks: Record<string, Record<string, unknown>> = {};
  let cur: Record<string, unknown> | null = null;
  for (const line of lines.slice(1, end)) {
    const head = /^([a-z_]+):$/.exec(line);
    if (head) {
      cur = blocks[head[1]!] = {};
      continue;
    }
    const kv = /^  ([a-z_]+): (.*)$/.exec(line);
    if (!kv || !cur) throw new Error(`ruling: unexpected frontmatter line ${j(line)}`);
    cur[kv[1]!] = JSON.parse(kv[2]!);
  }
  const rat = blocks.ratification;
  const home = blocks.home;
  if (!rat || !home) throw new Error("ruling: missing ratification or home block");
  const out: Ruling = {
    decision_id: home.decision_id as string,
    pick_id: home.pick_id as string,
    revision: home.revision as string,
    asking_thread: home.asking_thread as string,
    subject: home.subject as string,
    options_shown: home.options_shown as RulingOption[],
    picked: home.picked as string,
    ruled_by: rat.ruled_by as "mk" | "vizier",
    ruled_at: rat.ruled_at as string,
    ruling: rat.ruling as string,
    source: rat.source as string,
  };
  if (home.instruction !== undefined) out.instruction = home.instruction as string;
  if (rat.delegated_reason !== undefined) out.delegated_reason = rat.delegated_reason as string;
  if (rat.session_id !== undefined) out.session_id = rat.session_id as string;
  if (rat.supersedes !== undefined) out.supersedes = rat.supersedes as string;
  if (home.mentions !== undefined) out.mentions = home.mentions as string[];
  return out;
}

// ---- paths ----------------------------------------------------------------------

const SAFE_ID = /^[A-Za-z0-9_-]{1,64}$/;

export function slugify(subject: string): string {
  const s = subject
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 48)
    .replace(/-+$/g, "");
  return s === "" ? "decision" : s;
}

export interface RulingTarget {
  scope: "project" | "estate";
  decision_id: string;
  subject: string;
  /** YYYY-MM-DD */
  date: string;
}

/** Path components under the trusted root, and the file name. */
export function rulingPath(t: RulingTarget): { dirs: string[]; file: string } {
  if (!SAFE_ID.test(t.decision_id)) throw new RulingWriteError("bad-name", `unsafe decision id ${j(t.decision_id)}`);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(t.date)) throw new RulingWriteError("bad-name", `bad date ${j(t.date)}`);
  const file = `${t.date}-${slugify(t.subject)}-${t.decision_id}.md`;
  return { dirs: t.scope === "estate" ? ["rulings"] : ["docs", "decisions"], file };
}

/**
 * The trusted root for estate rulings, refused when the Uqbar is unset. Called at
 * `file` time so the refusal is early.
 */
export function estateRoot(env: Record<string, string | undefined> = process.env): string {
  const dir = env.AUTARCH_UQBAR_DIR;
  if (!dir) throw new RulingWriteError("no-uqbar", "estate-wide decisions need an Uqbar (see G-1)");
  return dir;
}

// ---- the safe writer -----------------------------------------------------------

export interface PinnedRoot {
  path: string;
  /** Decimal strings, as saved at filing. */
  dev: string;
  ino: string;
}

/** Record a root's identity at filing. */
export function pinRoot(path: string): PinnedRoot {
  const st = lstatSync(path, { bigint: true });
  if (!st.isDirectory()) throw new RulingWriteError("not-directory", `${path} is not a directory`);
  return { path, dev: String(st.dev), ino: String(st.ino) };
}

function checkRoot(root: PinnedRoot, when: string): void {
  let st;
  try {
    st = lstatSync(root.path, { bigint: true });
  } catch {
    throw new RulingWriteError("root-changed", `project root changed ${when}`);
  }
  if (st.isSymbolicLink() || !st.isDirectory() || String(st.dev) !== root.dev || String(st.ino) !== root.ino) {
    throw new RulingWriteError("root-changed", `project root changed ${when}`);
  }
}

export type WriteOutcome = { path: string; written: boolean };

/**
 * Write `content` at root/dirs.../file through the pinned root. Idempotent: the same
 * path with the same bytes is a no-op; the same path with different bytes is refused.
 */
export function writeRuling(root: PinnedRoot, dirs: string[], file: string, content: string): WriteOutcome {
  if (!/^[A-Za-z0-9._-]+$/.test(file) || file.startsWith(".")) throw new RulingWriteError("bad-name", `unsafe file name ${j(file)}`);
  for (const d of dirs) {
    if (!/^[A-Za-z0-9._-]+$/.test(d) || d === "." || d === "..") throw new RulingWriteError("bad-name", `unsafe directory ${j(d)}`);
  }
  // 1. the saved root, unchanged since filing
  checkRoot(root, "since filing");
  // 2. walk one component at a time, refusing symlinks, creating what is missing
  let dir = root.path;
  for (const comp of dirs) {
    dir = join(dir, comp);
    let st = lstatSync(dir, { throwIfNoEntry: false });
    if (!st) {
      try {
        mkdirSync(dir, { mode: 0o755 });
      } catch (e) {
        if ((e as NodeJS.ErrnoException).code !== "EEXIST") throw e;
      }
      st = lstatSync(dir);
    }
    if (st.isSymbolicLink()) throw new RulingWriteError("symlink", `${dir} is a symlink; refusing to write through it`);
    if (!st.isDirectory()) throw new RulingWriteError("not-directory", `${dir} is not a directory`);
  }
  const target = join(dir, file);
  const bytes = Buffer.from(content, "utf8");
  const existing = lstatSync(target, { throwIfNoEntry: false });
  if (existing) {
    if (!existing.isFile()) throw new RulingWriteError("symlink", `${target} exists and is not a regular file`);
    if (readFileSync(target).equals(bytes)) return { path: target, written: false };
    throw new RulingWriteError("exists-different", `${target} exists with different content`);
  }
  // 3. temp file, no follow, exclusive; fsync; rename into place
  const tmp = join(dir, `.tmp-${process.pid}-${randomBytes(6).toString("hex")}`);
  const fd = openSync(tmp, constants.O_CREAT | constants.O_EXCL | constants.O_WRONLY | constants.O_NOFOLLOW, 0o644);
  try {
    try {
      writeSync(fd, bytes);
      fsyncSync(fd);
    } finally {
      closeSync(fd);
    }
    renameSync(tmp, target);
  } catch (e) {
    try {
      unlinkSync(tmp);
    } catch {
      /* already gone */
    }
    throw e;
  }
  const dfd = openSync(dir, constants.O_RDONLY | constants.O_DIRECTORY);
  try {
    fsyncSync(dfd);
  } finally {
    closeSync(dfd);
  }
  // 4. the root must still be the same directory
  checkRoot(root, "during the write");
  return { path: target, written: true };
}
