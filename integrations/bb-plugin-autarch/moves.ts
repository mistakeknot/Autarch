// The home-move/v1 block: what mk owes on a card (run a script, merge a PR, read, supply context).
//
// A move is untrusted card text, so nothing here ever executes and no command is taken from the text.
// Commands are derived from the validated structured fields (path, sha256, args) only. Parsing is strict:
// any malformed part throws, and the message is the reason shown.

import { shQuote } from "./rootrun.js";

export type MoveKind = "script" | "pr" | "read" | "context";

export interface ScriptRef {
  path: string;
  sha256: string;
}

export interface ScriptMove extends ScriptRef {
  args: string[];
  recover: ScriptRef | null;
}

export type Move =
  | { kind: "script"; script: ScriptMove }
  | { kind: "pr"; url: string }
  | { kind: "read"; url: string }
  | { kind: "context"; need: string };

export interface MoveCommand {
  label: "sha256sum" | "check" | "run" | "recover";
  /** Exactly one line: no prompt, no newline, no leading or trailing space. */
  command: string;
  /** The sha the user should see beside the command (the sha256sum and recovery rows). */
  expectedSha?: string;
}

const SCHEMA = "home-move/v1";
const KINDS = new Set<string>(["script", "pr", "read", "context"]);
const PATH = /^\/[A-Za-z0-9._/+-]+$/;
const SHA = /^[0-9a-f]{64}$/;
const ARG = /^[A-Za-z0-9._/:=+@-]{1,200}$/;
const PR_URL = /^https:\/\/github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/pull\/([1-9][0-9]{0,8})$/;
/** Extra hosts a read move may point at, as a comma-separated list of bare host names. github.com is always allowed. */
export const READ_HOSTS_ENV = "HOME_MOVE_READ_HOSTS";
const HOST_NAME = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/;

/** The hosts a read url may use: github.com plus the valid entries of HOME_MOVE_READ_HOSTS (read at call time). */
function readHosts(): Set<string> {
  const hosts = new Set<string>(["github.com"]);
  for (const h of (process.env[READ_HOSTS_ENV] ?? "").split(",")) {
    const name = h.trim().toLowerCase();
    if (HOST_NAME.test(name)) hosts.add(name);
  }
  return hosts;
}
const MAX_PATH = 1024;
const MAX_ARGS = 8;
const MAX_NEED = 2000;

const fail = (msg: string): never => {
  throw new Error(msg);
};

const isObj = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === "object" && !Array.isArray(v);

function only(o: Record<string, unknown>, allowed: string[], what: string): void {
  for (const k of Object.keys(o).sort()) if (!allowed.includes(k)) fail(`unknown field "${k}" in ${what}`);
}

export function validScriptPath(p: unknown): p is string {
  return typeof p === "string" && p.length <= MAX_PATH && PATH.test(p) && !p.split("/").includes("..");
}

function scriptRef(v: unknown, what: string): ScriptRef {
  if (!isObj(v)) return fail(`${what} must be an object`);
  only(v, ["path", "sha256", "args", "recover"], what);
  if (!validScriptPath(v.path)) fail(`${what} path must be an absolute path of letters, digits and ._/+- with no .. segment`);
  if (typeof v.sha256 !== "string" || !SHA.test(v.sha256)) fail(`${what} sha256 must be 64 lowercase hex characters`);
  return { path: v.path as string, sha256: v.sha256 as string };
}

function readUrl(u: unknown): string {
  if (typeof u !== "string" || u.length > 2048) return fail("read url must be a string");
  let url: URL;
  try {
    url = new URL(u);
  } catch {
    return fail("read url is not a URL");
  }
  if (url.protocol !== "https:" || !readHosts().has(url.hostname) || url.username || url.password || url.port) {
    fail("read url must be https on an allowed host");
  }
  if (url.href !== u) fail("read url must be in canonical form");
  return u;
}

/** Parses a home-move/v1 body. Throws the display-only reason on any malformed part. */
export function parseMove(body: string): Move {
  let v: unknown;
  try {
    v = JSON.parse(body);
  } catch (e) {
    return fail(`invalid home-move JSON: ${(e as Error).message}`);
  }
  if (!isObj(v)) return fail("home-move must be a JSON object");
  if (v.schema !== SCHEMA) fail(`schema must be ${SCHEMA}`);
  if (typeof v.kind !== "string" || !KINDS.has(v.kind)) return fail("kind must be script, pr, read or context");
  const kind = v.kind as MoveKind;
  only(v, ["schema", "kind", kind], "home-move/v1");
  const m = v[kind];
  if (m === undefined) return fail(`${kind} move needs a "${kind}" member`);
  switch (kind) {
    case "script": {
      const ref = scriptRef(m, "script");
      const o = m as Record<string, unknown>;
      let args: string[] = [];
      if (o.args !== undefined) {
        if (!Array.isArray(o.args) || o.args.length > MAX_ARGS) fail(`args must be an array of at most ${MAX_ARGS} strings`);
        for (const a of o.args as unknown[]) if (typeof a !== "string" || !ARG.test(a)) fail("each arg must match ^[A-Za-z0-9._/:=+@-]{1,200}$");
        args = o.args as string[];
      }
      let recover: ScriptRef | null = null;
      if (o.recover !== undefined) {
        const r = o.recover;
        if (isObj(r) && (r.args !== undefined || r.recover !== undefined)) fail("recover takes only path and sha256");
        recover = scriptRef(r, "recover");
      }
      return { kind, script: { ...ref, args, recover } };
    }
    case "pr": {
      if (!isObj(m)) return fail("pr must be an object");
      only(m, ["url"], "pr");
      if (typeof m.url !== "string" || !PR_URL.test(m.url)) return fail("pr url must be https://github.com/<owner>/<repo>/pull/<n>");
      return { kind, url: m.url };
    }
    case "read": {
      if (!isObj(m)) return fail("read must be an object");
      only(m, ["url"], "read");
      return { kind, url: readUrl(m.url) };
    }
    case "context": {
      if (!isObj(m)) return fail("context must be an object");
      only(m, ["need"], "context");
      if (typeof m.need !== "string" || m.need.trim() === "" || m.need.length > MAX_NEED || !m.need.isWellFormed()) {
        return fail(`context need must be 1-${MAX_NEED} characters of text`);
      }
      return { kind, need: m.need };
    }
  }
}

/**
 * The commands for a script move, in run order. Each is one line derived from the validated fields;
 * the card text never contributes a character.
 */
export function scriptCommands(s: ScriptMove): MoveCommand[] {
  const out: MoveCommand[] = [
    { label: "sha256sum", command: `sha256sum ${shQuote(s.path)}`, expectedSha: s.sha256 },
    { label: "check", command: `bash ${shQuote(s.path)} --check` },
    { label: "run", command: ["bash", shQuote(s.path), ...s.args.map(shQuote)].join(" ") },
  ];
  if (s.recover) out.push({ label: "recover", command: `bash ${shQuote(s.recover.path)}`, expectedSha: s.recover.sha256 });
  return out;
}

/**
 * A git step as one pasteable line. Home runs as root on the host, so every git step runs as mk:
 * `runuser -u mk -- git -C <repo> ...`. The repo and every argument are quoted by the same safe builder.
 */
export function gitCommand(repo: string, args: readonly string[]): string {
  return ["runuser", "-u", "mk", "--", "git", "-C", shQuote(repo), ...args.map(shQuote)].join(" ");
}
