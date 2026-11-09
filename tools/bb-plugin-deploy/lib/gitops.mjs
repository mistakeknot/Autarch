// Git operations, build-tree verification and the control-closure check (plan 2.2 and 2.6).
import { appendFileSync, chmodSync, existsSync, lstatSync, mkdirSync, readFileSync, readdirSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { childEnv } from "./bb.mjs";
import { SHA40, ToolError, refuse, run } from "./util.mjs";

const gitEnv = (lockFd) => ({ ...childEnv({ lockFd }), GIT_TERMINAL_PROMPT: "0", GIT_OPTIONAL_LOCKS: "0" });

/** Run git with an argv array. Returns trimmed stdout; throws ToolError unless allowFail. */
export function git(cwd, args, { allowFail = false, input, lockFd, timeoutMs = 600_000 } = {}) {
  const r = run(["git", ...args], { cwd, env: gitEnv(lockFd), input, fds: lockFd ? [lockFd] : [], timeoutMs });
  if (r.error && r.error.code === "ENOENT") throw new ToolError("git was not found on PATH");
  if (r.code !== 0 && !allowFail) throw new ToolError(`git ${args.slice(0, 3).join(" ")} failed in ${cwd ?? "."}: ${(r.stderr || r.stdout).trim().slice(0, 400)}`);
  return allowFail ? r : r.stdout.trim();
}

/** Trimmed stdout on success, null on any failure. */
export function gitTry(cwd, args, opts = {}) {
  const r = git(cwd, args, { ...opts, allowFail: true });
  return r.code === 0 ? r.stdout.trim() : null;
}

/**
 * Plan 2.6 verification order. `dest` must not exist.
 * 1 clone --no-hardlinks, 2 checkout --detach, 3 HEAD == commit, 4 tree == expectTree, 5 write-tree == HEAD^{tree}
 * and an empty status. Returns {commit, tree}.
 */
export function cloneVerify({ repo, commit, expectTree, dest, lockFd }) {
  if (!SHA40.test(commit)) throw refuse(`not a full commit sha: ${commit}`);
  if (existsSync(dest)) throw new ToolError(`build directory ${dest} already exists; refusing to reuse it`);
  mkdirSync(join(dest, ".."), { recursive: true, mode: 0o700 });
  git(undefined, ["clone", "--no-hardlinks", "--quiet", repo, dest], { lockFd });
  git(dest, ["checkout", "--detach", "--quiet", commit], { lockFd });
  const head = git(dest, ["rev-parse", "HEAD"]);
  if (head !== commit) throw refuse(`clone HEAD ${head} is not the requested commit ${commit}`);
  const tree = git(dest, ["rev-parse", "HEAD^{tree}"]);
  if (expectTree && tree !== expectTree) throw refuse(`tree ${tree} of ${commit} does not match the expected tree ${expectTree}`);
  const wt = git(dest, ["write-tree"]);
  if (wt !== tree) throw refuse(`index tree ${wt} differs from HEAD tree ${tree}`);
  const dirty = git(dest, ["status", "--porcelain", "--untracked-files=all"]);
  if (dirty) throw refuse(`fresh clone is not clean:\n${dirty.slice(0, 400)}`);
  // The stamp lives inside the clone but is not part of the tree.
  mkdirSync(join(dest, ".git", "info"), { recursive: true });
  appendFileSync(join(dest, ".git", "info", "exclude"), ".bb-plugin-deploy/\n");
  return { commit: head, tree };
}

// ---- control closure ------------------------------------------------------------------------------------------

/** C0 controls (CR and LF included) and DEL. No legitimate closure path holds one. */
const CONTROL_CHAR = /[\u0000-\u001f\u007f]/;

/**
 * Translate a glob (`**`, `*`, literal) to an anchored regex over '/'-separated repo paths. The `s` flag lets `.`
 * match CR, LF and U+2028/9: without it a name such as "x.mjs\r" would fall outside `dir/**`, i.e. outside the closure.
 */
export function globToRegex(glob) {
  let re = "";
  for (let i = 0; i < glob.length; i++) {
    const c = glob[i];
    if (c === "*" && glob[i + 1] === "*") {
      if (glob[i + 2] === "/") {
        re += "(?:.*/)?";
        i += 2;
      } else {
        re += ".*";
        i += 1;
      }
    } else if (c === "*") re += "[^/]*";
    else if (c === "?") re += "[^/]";
    else re += c.replace(/[.+^${}()|[\]\\]/g, "\\$&");
  }
  return new RegExp(`^${re}$`, "s");
}
export const globMatcher = (globs) => {
  const res = globs.map(globToRegex);
  return (p) => res.some((r) => r.test(p));
};

/** Every git-tracked file of `commit` as Map(path -> {mode, sha}). Uses ls-tree -z. */
export function lsTree(cwd, commit) {
  const out = git(cwd, ["ls-tree", "-r", "-z", "--full-tree", commit]);
  const m = new Map();
  for (const rec of out.split("\0")) {
    if (!rec) continue;
    const tab = rec.indexOf("\t");
    const [mode, , sha] = rec.slice(0, tab).split(" ");
    m.set(rec.slice(tab + 1), { mode, sha });
  }
  return m;
}

function walkDisk(root, onEntry) {
  const stack = [""];
  while (stack.length) {
    const rel = stack.pop();
    const abs = rel ? join(root, rel) : root;
    for (const d of readdirSync(abs, { withFileTypes: true })) {
      if (rel === "" && d.name === ".git") continue;
      if (d.name === "node_modules" || (rel === "" && d.name === ".bb-plugin-deploy")) continue;
      const r = rel ? `${rel}/${d.name}` : d.name;
      onEntry(r, d);
      if (d.isDirectory()) stack.push(r);
    }
  }
}

/**
 * Verify the control closure of `root` against `git ls-tree <commit>`. Hashes use `git hash-object --no-filters`.
 * Refuses on a symlink, a missing file, an extra file under a closure dir, or a mismatch.
 */
export function verifyClosure({ root, commit, controlPaths }) {
  const match = globMatcher(controlPaths);
  const fail = (why) =>
    refuse(`installed deploy tooling does not match commit ${commit.slice(0, 12)} (${why}); use --engine legacy or mk's reviewed install`, { why });
  // `git hash-object --stdin-paths` reads one path per line and drops a trailing CR, so "x.mjs" and "x.mjs\r" would hash
  // as the same file. No closure path may hold a control character; refuse instead of trying to quote it.
  const noControl = (p) => {
    if (CONTROL_CHAR.test(p)) throw fail(`${JSON.stringify(p)} contains a control character`);
  };
  noControl(root);
  const tree = lsTree(root, commit);
  const want = new Map();
  const ancestors = new Set();
  for (const [p, e] of tree) {
    if (!match(p)) continue;
    noControl(p);
    if (e.mode === "120000") throw fail(`${p} is a symlink in the commit`);
    want.set(p, e.sha);
    const parts = p.split("/");
    for (let i = 1; i < parts.length; i++) ancestors.add(parts.slice(0, i).join("/"));
  }
  if (want.size === 0) throw fail("the closure is empty");
  const onDisk = new Set();
  walkDisk(root, (rel, d) => {
    if (match(rel) || ancestors.has(rel)) noControl(rel);
    const isLink = d.isSymbolicLink();
    if (isLink && (match(rel) || ancestors.has(rel) || match(`${rel}/x`))) throw fail(`${rel} is a symlink`);
    if (!d.isDirectory() && match(rel)) onDisk.add(rel);
  });
  for (const a of ancestors) {
    try {
      if (lstatSync(join(root, a)).isSymbolicLink()) throw fail(`${a} is a symlink`);
    } catch (e) {
      if (e instanceof ToolError) throw e;
      throw fail(`${a} is missing`);
    }
  }
  for (const p of onDisk) if (!want.has(p)) throw fail(`extra file ${p}`);
  const paths = [...want.keys()];
  for (const p of paths) {
    let st;
    try {
      st = lstatSync(join(root, p));
    } catch {
      throw fail(`${p} is missing`);
    }
    if (st.isSymbolicLink()) throw fail(`${p} is a symlink`);
  }
  const r = run(["git", "hash-object", "--no-filters", "--stdin-paths"], { cwd: root, input: paths.map((p) => join(root, p)).join("\n") + "\n", env: gitEnv() });
  if (r.code !== 0) throw new ToolError(`git hash-object failed: ${r.stderr.trim().slice(0, 300)}`);
  const hashes = r.stdout.trim().split("\n");
  if (hashes.length !== paths.length) throw fail("hash count mismatch");
  for (let i = 0; i < paths.length; i++) if (hashes[i] !== want.get(paths[i])) throw fail(`${paths[i]} differs`);
  return { files: paths.length };
}

/** For an unstamped build the closure is compared against its HEAD, plus a clean status over the closure paths. */
export function verifyClosureAgainstHead({ root, controlPaths }) {
  const head = gitTry(root, ["rev-parse", "HEAD"]);
  if (head === null || !SHA40.test(head)) throw refuse("installed build is not a git checkout; cannot verify deploy tooling");
  const match = globMatcher(controlPaths);
  // Raw stdout: git() trims it, which would eat the leading space of the first " M path" record.
  const stat = git(root, ["status", "--porcelain", "--untracked-files=all", "--no-renames", "-z"], { allowFail: true });
  if (stat.code !== 0) throw refuse("git status failed in the installed build; cannot verify deploy tooling");
  const dirty = stat.stdout.split("\0").filter(Boolean).map((l) => l.slice(3)).filter((p) => match(p));
  if (dirty.length) throw refuse(`installed deploy tooling does not match commit ${head.slice(0, 12)} (uncommitted: ${dirty[0]}); use --engine legacy or mk's reviewed install`);
  verifyClosure({ root, commit: head, controlPaths });
  return head;
}

/** Remove write bits from the closure files (defence in depth only; the hash check is the control). */
export function lockdownClosure(root, controlPaths) {
  const match = globMatcher(controlPaths);
  walkDisk(root, (rel, d) => {
    if (d.isFile() && match(rel)) {
      try {
        chmodSync(join(root, rel), 0o444 | (lstatSync(join(root, rel)).mode & 0o111));
      } catch {
        /* best effort */
      }
    }
  });
}

/** Paths in the control closure that differ between two commits, from a clone that has both. null when unknown. */
export function controlDiff({ repoDir, from, to, controlPaths }) {
  if (!from || !SHA40.test(from)) return null;
  const r = git(repoDir, ["diff", "--name-only", "--no-renames", "-z", from, to], { allowFail: true });
  if (r.code !== 0) return null;
  const match = globMatcher(controlPaths);
  return r.stdout.split("\0").filter((p) => p && match(p));
}
export const relFrom = (root, abs) => relative(root, abs).split(sep).join("/");
