import { createHash } from "node:crypto";
import { appendFileSync, linkSync, mkdirSync, mkdtempSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { docRoots, NOT_AVAILABLE, openChecked, projectRoots, readDoc, resolveOpenTarget } from "../docread.js";

let home: string;
let root: string;
let outside: string;
const sha = (s: string | Buffer) => createHash("sha256").update(s).digest("hex");

beforeAll(() => {
  home = realpathSync(mkdtempSync(join(tmpdir(), "docread-")));
  root = join(home, ".bb-machines", "m1", "thread-storage");
  outside = join(home, "elsewhere");
  mkdirSync(join(root, "thr_a"), { recursive: true });
  mkdirSync(outside, { recursive: true });
  writeFileSync(join(root, "thr_a", "plan.md"), "# Plan\n\nhello\n");
  writeFileSync(join(root, "thr_a", "run.sh"), "echo hi\n");
  writeFileSync(join(root, "thr_a", ".env"), "KEY=1\n");
  writeFileSync(join(root, "thr_a", "prod.env"), "KEY=1\n");
  writeFileSync(join(root, "thr_a", "server.key"), "x");
  writeFileSync(join(root, "thr_a", "credentials.json"), "{}");
  writeFileSync(join(root, "thr_a", "image.md"), Buffer.from([0x23, 0, 0x41]));
  writeFileSync(join(root, "thr_a", "big.md"), "x".repeat(2000));
  writeFileSync(join(root, "thr_a", "data.bin"), "x");
  writeFileSync(join(outside, "secret-notes.md"), "no");
  writeFileSync(join(outside, "plain.md"), "outside the roots");
  writeFileSync(join(outside, "key.pem"), "no");
  writeFileSync(join(outside, "single.md"), "single-link outside");
  symlinkSync(outside, join(root, "thr_a", "dir-out2"));
  writeFileSync(join(outside, ".env"), "no");
  symlinkSync(join(outside, "plain.md"), join(root, "thr_a", "link-out.md"));
  symlinkSync(outside, join(root, "thr_a", "dir-out"));
  symlinkSync(join(root, "thr_a", ".env"), join(root, "thr_a", "link-env.md"));
  symlinkSync(join(root, "thr_a", "plan.md"), join(root, "thr_a", "link-in.md"));
  mkdirSync(join(root, "thr_a", ".ssh"));
  writeFileSync(join(root, "thr_a", ".ssh", "notes.md"), "no");
  mkdirSync(join(root, "thr_a", "node_modules"));
  writeFileSync(join(root, "thr_a", "node_modules", "README.md"), "no");
  for (const f of ["keys.json", "api-key.json", "prod.env.json", "my-cert.txt", "auth.json", "authorized_keys.txt"]) writeFileSync(join(root, "thr_a", f), "{}");
  linkSync(join(outside, "plain.md"), join(root, "thr_a", "hard.md"));
  // A sibling whose name starts with the root's name must not pass a prefix check.
  mkdirSync(`${root}-evil`);
  writeFileSync(join(`${root}-evil`, "x.md"), "no");
});
afterAll(() => rmSync(home, { recursive: true, force: true }));

const opts = () => ({ roots: docRoots(home) });
const p = (...s: string[]) => join(root, "thr_a", ...s);

describe("readDoc allowlist", () => {
  it("finds the thread-storage roots as real paths", () => {
    expect(docRoots(home)).toEqual([root]);
    expect(docRoots(join(home, "none"))).toEqual([]);
  });

  it("reads a Markdown file under a root, with its digest and size", () => {
    const r = readDoc(p("plan.md"), opts());
    expect(r).toMatchObject({ ok: true, markdown: true, size: 14, sha256: sha("# Plan\n\nhello\n"), text: "# Plan\n\nhello\n", matches: null });
  });

  it("says whether the file still matches a stated digest", () => {
    expect(readDoc(p("plan.md"), { ...opts(), sha256: sha("# Plan\n\nhello\n").toUpperCase() })).toMatchObject({ ok: true, matches: true });
    expect(readDoc(p("plan.md"), { ...opts(), sha256: sha("other") })).toMatchObject({ ok: true, matches: false });
  });

  it("shows a script as raw text, not Markdown", () => {
    expect(readDoc(p("run.sh"), opts())).toMatchObject({ ok: true, markdown: false });
  });

  it("follows a symlink that stays inside a root, and reports the real path", () => {
    expect(readDoc(p("link-in.md"), opts())).toMatchObject({ ok: true, path: p("plan.md") });
  });

  it("refuses a symlink that leaves the roots, to a file or through a directory", () => {
    expect(readDoc(p("link-out.md"), opts())).toEqual({ ok: false, error: NOT_AVAILABLE });
    expect(readDoc(p("dir-out", "plain.md"), opts())).toEqual({ ok: false, error: NOT_AVAILABLE });
  });

  it("refuses a link named like a document that reaches a secret", () => {
    expect(readDoc(p("link-env.md"), opts())).toEqual({ ok: false, error: NOT_AVAILABLE });
  });

  it("refuses paths outside the roots, including a sibling that shares the root's name prefix", () => {
    expect(readDoc(join(outside, "plain.md"), opts()).ok).toBe(false);
    expect(readDoc(join(`${root}-evil`, "x.md"), opts()).ok).toBe(false);
    expect(readDoc("/etc/passwd", opts()).ok).toBe(false);
    expect(readDoc(root, opts()).ok).toBe(false);
  });

  it("refuses .. segments, relative paths, NULs, empty and non-string input", () => {
    expect(readDoc(`${root}/thr_a/../thr_a/plan.md`, opts()).ok).toBe(false);
    expect(readDoc("thr_a/plan.md", opts()).ok).toBe(false);
    expect(readDoc(`${p("plan.md")}\0.txt`, opts()).ok).toBe(false);
    expect(readDoc("", opts()).ok).toBe(false);
    expect(readDoc(undefined, opts()).ok).toBe(false);
    expect(readDoc({ toString: () => p("plan.md") }, opts()).ok).toBe(false);
    expect(readDoc(`/${"a/".repeat(600)}x.md`, opts()).ok).toBe(false);
  });

  it("refuses .env files, keys, credentials, secret directories and non-text types", () => {
    for (const f of [".env", "prod.env", "server.key", "credentials.json", "data.bin", join(".ssh", "notes.md"), join("node_modules", "README.md")]) {
      expect(readDoc(p(f), opts()), f).toEqual({ ok: false, error: NOT_AVAILABLE });
    }
    // Even with a root that would contain them.
    expect(readDoc(join(outside, ".env"), { roots: [outside] }).ok).toBe(false);
    expect(readDoc(join(outside, "key.pem"), { roots: [outside] }).ok).toBe(false);
    expect(readDoc(join(outside, "secret-notes.md"), { roots: [outside] }).ok).toBe(false);
  });

  it("refuses a directory, a missing file and a file with a NUL byte", () => {
    expect(readDoc(p(), opts()).ok).toBe(false);
    expect(readDoc(p("nope.md"), opts()).ok).toBe(false);
    expect(readDoc(p("image.md"), opts())).toEqual({ ok: false, error: "a binary file is not shown" });
  });

  it("refuses a file over the size cap and reads one at the cap", () => {
    expect(readDoc(p("big.md"), { ...opts(), maxBytes: 1999 })).toMatchObject({ ok: false, error: expect.stringContaining("too large") });
    expect(readDoc(p("big.md"), { ...opts(), maxBytes: 2000 })).toMatchObject({ ok: true, size: 2000 });
  });

  it("gives the same refusal for a missing path and a forbidden one, so existence is not revealed", () => {
    expect(readDoc(join(outside, "plain.md"), opts())).toEqual(readDoc(join(outside, "absent.md"), opts()));
  });

  it("with no roots, refuses everything", () => {
    expect(readDoc(p("plan.md"), { roots: [] }).ok).toBe(false);
  });

  it("refuses more secret-looking names: keys, env variants, certificates, auth files", () => {
    for (const f of ["keys.json", "api-key.json", "prod.env.json", "my-cert.txt", "auth.json", "authorized_keys.txt"]) expect(readDoc(p(f), opts()), f).toEqual({ ok: false, error: NOT_AVAILABLE });
    // Names that only contain those letters inside a word stay readable.
    writeFileSync(p("monkey-notes.md"), "a");
    writeFileSync(p("author.md"), "a");
    expect(readDoc(p("monkey-notes.md"), opts()).ok).toBe(true);
    expect(readDoc(p("author.md"), opts()).ok).toBe(true);
  });

  it("refuses a file with more than one hard link, since its content may also live outside the roots", () => {
    expect(readDoc(p("hard.md"), opts())).toEqual({ ok: false, error: NOT_AVAILABLE });
  });

  it("refuses a thread-storage directory that is itself a symlink, so it is not a root", () => {
    const h = realpathSync(mkdtempSync(join(tmpdir(), "docread-link-")));
    try {
      mkdirSync(join(h, ".bb-machines", "m"), { recursive: true });
      symlinkSync(outside, join(h, ".bb-machines", "m", "thread-storage"));
      mkdirSync(join(h, ".bb-machines", "real", "thread-storage"), { recursive: true });
      expect(docRoots(h)).toEqual([join(h, ".bb-machines", "real", "thread-storage")]);
      const viaLink = readDoc(join(h, ".bb-machines", "m", "thread-storage", "plain.md"), { roots: docRoots(h) });
      expect(viaLink.ok).toBe(false);
    } finally {
      rmSync(h, { recursive: true, force: true });
    }
  });

  it("refuses a .bb-machines directory that is itself a symlink", () => {
    const h = realpathSync(mkdtempSync(join(tmpdir(), "docread-base-")));
    try {
      mkdirSync(join(outside, "m", "thread-storage"), { recursive: true });
      symlinkSync(outside, join(h, ".bb-machines"));
      expect(docRoots(h)).toEqual([]);
    } finally {
      rmSync(h, { recursive: true, force: true });
    }
  });

  describe("what is actually opened (stands in for a swap after the check)", () => {
    // These hand openChecked a path that was never resolved, as if a directory or file had been swapped for a link
    // between the realpath and the open.
    it("refuses when a parent directory is a symlink out of the roots", () => {
      expect(openChecked(p("dir-out2", "single.md"), [root], 1000)).toEqual({ ok: false, error: NOT_AVAILABLE });
    });
    it("refuses a final-component symlink even when it points inside the roots (O_NOFOLLOW)", () => {
      expect(openChecked(p("link-in.md"), [root], 1000)).toEqual({ ok: false, error: NOT_AVAILABLE });
      expect(openChecked(p("link-out.md"), [root], 1000)).toEqual({ ok: false, error: NOT_AVAILABLE });
    });
    it("reads an ordinary file given the resolved path", () => {
      expect(openChecked(p("plan.md"), [root], 1000)).toMatchObject({ ok: true, path: p("plan.md") });
    });
  });

  it("refuses a file that grows or shrinks between being measured and being read", () => {
    writeFileSync(p("mid.md"), "abcde");
    expect(openChecked(p("mid.md"), [root], 1000, undefined, () => appendFileSync(p("mid.md"), "f"))).toEqual({ ok: false, error: expect.stringContaining("changed while it was read") });
    writeFileSync(p("mid.md"), "abcde");
    expect(openChecked(p("mid.md"), [root], 1000, undefined, () => writeFileSync(p("mid.md"), "ab"))).toEqual({ ok: false, error: expect.stringContaining("changed while it was read") });
    writeFileSync(p("mid.md"), "abcde");
    expect(openChecked(p("mid.md"), [root], 1000)).toMatchObject({ ok: true, size: 5 });
  });

  it("reports a changed file by its new digest and size, so a stale stated digest does not match", () => {
    writeFileSync(p("grow.md"), "abcde");
    const first = readDoc(p("grow.md"), { ...opts(), sha256: sha("abcde") });
    expect(first).toMatchObject({ ok: true, matches: true });
    appendFileSync(p("grow.md"), "f");
    expect(readDoc(p("grow.md"), { ...opts(), sha256: sha("abcde") })).toMatchObject({ ok: true, matches: false, size: 6 });
  });
});

describe("resolveOpenTarget (the file panel link, bead mk-98y4s)", () => {
  let projects: string;
  beforeAll(() => {
    projects = join(home, "projects");
    mkdirSync(join(projects, "repo", "scripts"), { recursive: true });
    mkdirSync(join(projects, "repo", ".git"), { recursive: true });
    writeFileSync(join(projects, "repo", "scripts", "go.sh"), "echo hi\n");
    writeFileSync(join(projects, "repo", ".git", "config.md"), "no");
    writeFileSync(join(projects, "repo", "prod.env"), "no");
    symlinkSync(outside, join(projects, "repo", "out"));
  });
  const roots = () => [...docRoots(home), ...projectRoots(home)];
  it("finds the projects folder under the home it is given, and only when it is a real directory", () => {
    expect(projectRoots(home)).toEqual([projects]);
    expect(projectRoots(join(home, "nowhere"))).toEqual([]);
  });
  it("accepts a text file under thread storage or the projects folder, and returns its real path", () => {
    expect(resolveOpenTarget(join(root, "thr_a", "plan.md"), { roots: roots() })).toEqual({ ok: true, path: join(root, "thr_a", "plan.md") });
    expect(resolveOpenTarget(join(projects, "repo", "scripts", "go.sh"), { roots: roots() })).toEqual({ ok: true, path: join(projects, "repo", "scripts", "go.sh") });
  });
  it.each([
    ["relative", () => "repo/scripts/go.sh"],
    ["dot-dot", () => "/x/../etc/passwd.md"],
    ["outside every root", () => join(outside, "plain.md")],
    ["a symlink out of the projects folder", () => join(projects, "repo", "out", "plain.md")],
    ["a .git file", () => join(projects, "repo", ".git", "config.md")],
    ["a secret name", () => join(projects, "repo", "prod.env")],
    ["a missing file", () => join(projects, "repo", "nope.md")],
    ["a directory", () => join(projects, "repo", "scripts")],
    ["a binary type", () => join(root, "thr_a", "data.bin")],
    ["a NUL in the path", () => join(projects, "repo", "scripts", "go.sh") + "\0.md"],
  ])("refuses %s with the one generic reason", (_n, p) => {
    expect(resolveOpenTarget(p(), { roots: roots() })).toEqual({ ok: false, error: NOT_AVAILABLE });
  });
  it("refuses a non-string", () => {
    expect(resolveOpenTarget(42, { roots: roots() })).toEqual({ ok: false, error: NOT_AVAILABLE });
  });
});
