import { afterEach, describe, expect, it } from "vitest";
import { spawnSync } from "node:child_process";
import { appendFileSync, mkdirSync, symlinkSync, mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Rig, type CliResult } from "./rig";
import { globMatcher, verifyClosure, verifyClosureAgainstHead } from "../../../../tools/bb-plugin-deploy/lib/gitops.mjs";

// mk-5wc43: `git hash-object --stdin-paths` reads one path per line and drops a trailing carriage return, so the
// closure paths "x.mjs" and "x.mjs\r" were hashed as the same file. The closure check must refuse such a path.

let rig: Rig;
afterEach(() => rig?.cleanup());

const LIB = "tools/bb-plugin-deploy/lib";
const msg = (r: CliResult) => String(r.json?.message ?? r.stderr);
const verify = () => rig.cli(["record", "--verify-closure"]);
const deployArgs = (sha: string) => ["deploy", "--commit", sha, "--class", "plugin-deploy", "--mode", "agent"];

/** A rig whose installed commit already holds the two twin files, with identical contents. */
const twinRig = (second: string, kind: "git" | "stamped") => {
  rig = new Rig({ installedKind: kind, files: { [`${LIB}/x.mjs`]: "export const x = 1;\n", [`${LIB}/${second}`]: "export const x = 1;\n" } });
};

describe("a control-closure path with a control character is refused", () => {
  for (const kind of ["stamped", "git"] as const) {
    it(`${kind}: tampering only with the "x.mjs\\r" twin is refused (the reviewer's reproduction)`, () => {
      twinRig("x.mjs\r", kind);
      appendFileSync(join(rig.liveRoot() ?? "", LIB, "x.mjs\r"), "export const evil = true;\n");
      const r = verify();
      expect(r.status, msg(r)).toBe(10);
      // An unstamped build also reports the dirty file first; either way the closure is refused.
      expect(msg(r)).toMatch(kind === "git" ? /uncommitted|control character/ : /control character/);
      expect(rig.mutations()).toEqual([]);
    });
  }

  for (const kind of ["stamped", "git"] as const) {
    it(`${kind}: an untouched closure with a CR twin is refused too, not just a tampered one`, () => {
      twinRig("x.mjs\r", kind);
      const r = verify();
      expect(r.status, msg(r)).toBe(10);
      expect(msg(r)).toMatch(/control character/);
    });
  }

  it("a deploy over such an installed build is refused and changes nothing", () => {
    twinRig("x.mjs\r", "stamped");
    const Y = rig.commit({ "plug/app.txt": "v2\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(10);
    expect(rig.mutations()).toEqual([]);
  });

  for (const [name, second] of [["LF", "x.mjs\ny"], ["tab", "x.mjs\ty"], ["SOH", "x.mjs\x01"], ["DEL", "x.mjs\x7f"]] as const) {
    it(`a committed closure path with ${name} is refused`, () => {
      twinRig(second, "stamped");
      const r = verify();
      expect(r.status, msg(r)).toBe(10);
      expect(msg(r)).toMatch(/control character/);
    });
  }

  it("an untracked closure file with a CR on disk is refused", () => {
    rig = new Rig({ installedKind: "stamped" });
    writeFileSync(join(rig.liveRoot() ?? "", LIB, "gc.mjs\r"), readFileSync(join(rig.liveRoot() ?? "", LIB, "gc.mjs")));
    const r = verify();
    expect(r.status, msg(r)).toBe(10);
  });

  it("a plain closure with ordinary names still verifies", () => {
    rig = new Rig({ installedKind: "stamped", files: { [`${LIB}/x.mjs`]: "export const x = 1;\n" } });
    const r = verify();
    expect(r.status, msg(r)).toBe(0);
    expect(r.json?.closureVerified).toBe(true);
  });
});

describe("closure glob matching", () => {
  it("** and * match names that contain CR, LF or other control characters", () => {
    const m = globMatcher(["tools/bb-plugin-deploy/**", "**/AGENTS.md", "scripts/*.sh"]);
    expect(m("tools/bb-plugin-deploy/lib/x.mjs\r")).toBe(true);
    expect(m("tools/bb-plugin-deploy/lib/x\ny.mjs")).toBe(true);
    expect(m("tools/bb-plugin-deploy/a\u2028b/y.mjs")).toBe(true);
    expect(m("a\rb/AGENTS.md")).toBe(true);
    expect(m("scripts/a\r.sh")).toBe(true);
    expect(m("tools/other/x.mjs")).toBe(false);
  });
});

describe("other closure-path decoding", () => {
  it("the first dirty path of an unstamped build is named whole (the status output is not trimmed)", () => {
    rig = new Rig({ installedKind: "git" });
    appendFileSync(join(rig.liveRoot() ?? "", "scripts", "home-gates.sh"), "\necho tampered\n");
    const r = verify();
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/uncommitted: scripts\/home-gates\.sh\)/);
  });

  it("a closure file renamed out of the closure in the index is refused (status runs without rename detection)", () => {
    rig = new Rig({ installedKind: "git", files: { [`${LIB}/x.mjs`]: "export const x = 1;\n" } });
    const root = rig.liveRoot() ?? "";
    mkdirSync(join(root, "elsewhere"), { recursive: true });
    renameSync(join(root, LIB, "x.mjs"), join(root, "elsewhere", "x.mjs"));
    expect(spawnSync("git", ["add", "-A"], { cwd: root }).status).toBe(0);
    const r = verify();
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/uncommitted: tools\/bb-plugin-deploy\/lib\/x\.mjs/);
  });

  it("a closure file renamed out of the closure by a commit counts as a control-path change (diff without rename detection)", () => {
    rig = new Rig({ installedKind: "stamped", files: { [`${LIB}/x.mjs`]: "export const x = 1;\n" } });
    const Y = rig.commit({ [`${LIB}/x.mjs`]: null, "plug/x.mjs": "export const x = 1;\n" });
    const r = rig.cli(deployArgs(Y));
    expect(r.status, msg(r)).toBe(20);
  });
});

// A filename is bytes, not text. Decoding it as UTF-8 maps every invalid byte to U+FFFD, so the raw name "x\xff.mjs" and the
// valid name "x\uFFFD.mjs" became one Map/Set key and only one of them was hashed (reviewer round 1).
describe("closure paths that are not valid UTF-8", () => {
  const CONTROL = ["tools/bb-plugin-deploy/**"];
  const dirs: string[] = [];
  afterEach(() => {
    for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
  });
  const sh = (cwd: string, args: string[]) => {
    const r = spawnSync("git", args, { cwd, encoding: "utf8" });
    if (r.status !== 0) throw new Error(`git ${args.join(" ")}: ${r.stderr}`);
    return r.stdout.trim();
  };
  /** A repo whose commit holds the valid name "x\uFFFD.mjs" and the raw-byte name "x\xff.mjs", identical contents. */
  const bytesRepo = () => {
    const root = mkdtempSync(join(tmpdir(), "closure-bytes-"));
    dirs.push(root);
    sh(root, ["init", "-q", "-b", "main"]);
    sh(root, ["config", "user.email", "t@example.invalid"]);
    sh(root, ["config", "user.name", "t"]);
    const lib = Buffer.from(join(root, LIB));
    mkdirSync(lib.toString(), { recursive: true });
    const body = "export const x = 1;\n";
    writeFileSync(Buffer.concat([lib, Buffer.from("/x\uFFFD.mjs")]), body);
    writeFileSync(Buffer.concat([lib, Buffer.from([0x2f, 0x78, 0xff, 0x2e, 0x6d, 0x6a, 0x73])]), body);
    return { root, lib, body, rawFile: Buffer.concat([lib, Buffer.from([0x2f, 0x78, 0xff, 0x2e, 0x6d, 0x6a, 0x73])]) };
  };

  it("a committed raw-byte name beside its U+FFFD twin is refused, even when only the raw-byte file changed", () => {
    const { root, rawFile } = bytesRepo();
    sh(root, ["add", "-A"]);
    sh(root, ["commit", "-q", "-m", "c"]);
    const commit = sh(root, ["rev-parse", "HEAD"]);
    appendFileSync(rawFile, "export const evil = true;\n");
    expect(() => verifyClosure({ root, commit, controlPaths: CONTROL })).toThrow(/not valid UTF-8|control character/);
  });

  it("an ignored raw-byte file beside a committed U+FFFD name is refused by both verifiers", () => {
    const { root, rawFile } = bytesRepo();
    rmSync(rawFile);
    sh(root, ["add", "-A"]);
    sh(root, ["commit", "-q", "-m", "c"]);
    const commit = sh(root, ["rev-parse", "HEAD"]);
    mkdirSync(join(root, ".git", "info"), { recursive: true });
    appendFileSync(join(root, ".git", "info", "exclude"), "x*.mjs\n");
    // Ignore only the raw file: the committed twin is tracked, so the ignore rule does not hide it.
    writeFileSync(rawFile, "export const evil = true;\n");
    expect(() => verifyClosure({ root, commit, controlPaths: CONTROL })).toThrow(/not valid UTF-8|control character/);
    expect(() => verifyClosureAgainstHead({ root, controlPaths: CONTROL })).toThrow(/not valid UTF-8|control character/);
  });

  it("an ignored raw-byte directory holding a closure file, beside an empty U+FFFD directory, is refused by both verifiers", () => {
    // Both directory names decode to "x\uFFFD"; traversal would read the empty twin twice and never see AGENTS.md.
    const root = mkdtempSync(join(tmpdir(), "closure-bytes-"));
    dirs.push(root);
    sh(root, ["init", "-q", "-b", "main"]);
    sh(root, ["config", "user.email", "t@example.invalid"]);
    sh(root, ["config", "user.name", "t"]);
    mkdirSync(join(root, LIB), { recursive: true });
    writeFileSync(join(root, LIB, "good.mjs"), "export const x = 1;\n");
    sh(root, ["add", "-A"]);
    sh(root, ["commit", "-q", "-m", "c"]);
    const commit = sh(root, ["rev-parse", "HEAD"]);
    mkdirSync(join(root, "x\uFFFD"));
    const rawDir = Buffer.concat([Buffer.from(root), Buffer.from([0x2f, 0x78, 0xff])]);
    mkdirSync(rawDir);
    writeFileSync(Buffer.concat([rawDir, Buffer.from("/AGENTS.md")]), "extra closure file\n");
    mkdirSync(join(root, ".git", "info"), { recursive: true });
    appendFileSync(join(root, ".git", "info", "exclude"), "x*/\n");
    expect(sh(root, ["status", "--porcelain", "--untracked-files=all"])).toBe("");
    const controlPaths = [...CONTROL, "**/AGENTS.md"];
    expect(() => verifyClosure({ root, commit, controlPaths })).toThrow(/not valid UTF-8/);
    expect(() => verifyClosureAgainstHead({ root, controlPaths })).toThrow(/not valid UTF-8/);
  });

  it("a plain UTF-8 name outside the replacement character still verifies", () => {
    const root = mkdtempSync(join(tmpdir(), "closure-bytes-"));
    dirs.push(root);
    sh(root, ["init", "-q", "-b", "main"]);
    sh(root, ["config", "user.email", "t@example.invalid"]);
    sh(root, ["config", "user.name", "t"]);
    mkdirSync(join(root, LIB), { recursive: true });
    writeFileSync(join(root, LIB, "café-ü.mjs"), "export const x = 1;\n");
    sh(root, ["add", "-A"]);
    sh(root, ["commit", "-q", "-m", "c"]);
    expect(verifyClosure({ root, commit: sh(root, ["rev-parse", "HEAD"]), controlPaths: CONTROL }).files).toBe(1);
  });
});

// Reviewer round 3 (pre-existing, not a decoding gap): the walk does not descend into a symlink, so a directory symlink that
// matches no closure glob could point at a directory holding a closure-named file.
describe("a directory symlink cannot hide a closure file from the walk", () => {
  const dirs: string[] = [];
  afterEach(() => {
    for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
  });
  const sh = (cwd: string, args: string[]) => {
    const r = spawnSync("git", args, { cwd, encoding: "utf8" });
    if (r.status !== 0) throw new Error(`git ${args.join(" ")}: ${r.stderr}`);
    return r.stdout.trim();
  };
  it("an ignored symlink to a directory holding AGENTS.md is refused by both verifiers", () => {
    const root = mkdtempSync(join(tmpdir(), "closure-link-"));
    const outside = mkdtempSync(join(tmpdir(), "closure-link-target-"));
    dirs.push(root, outside);
    sh(root, ["init", "-q", "-b", "main"]);
    sh(root, ["config", "user.email", "t@example.invalid"]);
    sh(root, ["config", "user.name", "t"]);
    mkdirSync(join(root, LIB), { recursive: true });
    writeFileSync(join(root, LIB, "good.mjs"), "export const x = 1;\n");
    sh(root, ["add", "-A"]);
    sh(root, ["commit", "-q", "-m", "c"]);
    const commit = sh(root, ["rev-parse", "HEAD"]);
    writeFileSync(join(outside, "AGENTS.md"), "extra closure file\n");
    symlinkSync(outside, join(root, "hidden"));
    mkdirSync(join(root, ".git", "info"), { recursive: true });
    appendFileSync(join(root, ".git", "info", "exclude"), "hidden\n");
    expect(sh(root, ["status", "--porcelain", "--untracked-files=all"])).toBe("");
    const controlPaths = ["tools/bb-plugin-deploy/**", "**/AGENTS.md"];
    expect(() => verifyClosure({ root, commit, controlPaths })).toThrow(/symlink/);
    expect(() => verifyClosureAgainstHead({ root, controlPaths })).toThrow(/symlink/);
  });

  it("a symlink to a plain file outside every closure glob is still allowed", () => {
    const root = mkdtempSync(join(tmpdir(), "closure-link-"));
    dirs.push(root);
    sh(root, ["init", "-q", "-b", "main"]);
    sh(root, ["config", "user.email", "t@example.invalid"]);
    sh(root, ["config", "user.name", "t"]);
    mkdirSync(join(root, LIB), { recursive: true });
    writeFileSync(join(root, LIB, "good.mjs"), "export const x = 1;\n");
    writeFileSync(join(root, "notes.txt"), "n\n");
    symlinkSync("notes.txt", join(root, "notes-link"));
    sh(root, ["add", "-A"]);
    sh(root, ["commit", "-q", "-m", "c"]);
    expect(verifyClosure({ root, commit: sh(root, ["rev-parse", "HEAD"]), controlPaths: ["tools/bb-plugin-deploy/**", "**/AGENTS.md"] }).files).toBe(1);
  });
});

// Confirming review: the U+FFFD refusal must cover every walked entry (file, directory, symlink), before any target lookup.
// A symlink whose raw name holds a non-UTF-8 byte was looked up by its decoded name, so statSync failed with ENOENT and the
// link to a real directory went unnoticed.
describe("every walked entry with U+FFFD in its decoded name is refused", () => {
  const dirs: string[] = [];
  afterEach(() => {
    for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
  });
  const sh = (cwd: string, args: string[]) => {
    const r = spawnSync("git", args, { cwd, encoding: "utf8" });
    if (r.status !== 0) throw new Error(`git ${args.join(" ")}: ${r.stderr}`);
    return r.stdout.trim();
  };
  const controlPaths = ["tools/bb-plugin-deploy/**", "**/AGENTS.md"];
  const makeRepo = () => {
    const root = mkdtempSync(join(tmpdir(), "closure-fffd-"));
    dirs.push(root);
    sh(root, ["init", "-q", "-b", "main"]);
    sh(root, ["config", "user.email", "t@example.invalid"]);
    sh(root, ["config", "user.name", "t"]);
    mkdirSync(join(root, LIB), { recursive: true });
    writeFileSync(join(root, LIB, "good.mjs"), "export const x = 1;\n");
    sh(root, ["add", "-A"]);
    sh(root, ["commit", "-q", "-m", "c"]);
    mkdirSync(join(root, ".git", "info"), { recursive: true });
    appendFileSync(join(root, ".git", "info", "exclude"), "hidden*\n");
    return { root, commit: sh(root, ["rev-parse", "HEAD"]) };
  };
  const both = (root: string, commit: string) => {
    expect(sh(root, ["status", "--porcelain", "--untracked-files=all"])).toBe("");
    expect(() => verifyClosure({ root, commit, controlPaths })).toThrow(/not valid UTF-8/);
    expect(() => verifyClosureAgainstHead({ root, controlPaths })).toThrow(/not valid UTF-8/);
  };

  it("an ignored symlink named hidden<FF> that points to a directory holding AGENTS.md", () => {
    const { root, commit } = makeRepo();
    const outside = mkdtempSync(join(tmpdir(), "closure-fffd-target-"));
    dirs.push(outside);
    writeFileSync(join(outside, "AGENTS.md"), "extra closure file\n");
    symlinkSync(outside, Buffer.concat([Buffer.from(join(root, "hidden")), Buffer.from([0xff])]));
    both(root, commit);
  });

  it("an ignored regular file named hidden<U+FFFD>", () => {
    const { root, commit } = makeRepo();
    writeFileSync(join(root, "hidden\uFFFD"), "x\n");
    both(root, commit);
  });

  it("an ignored regular file named hidden<FF> (raw byte)", () => {
    const { root, commit } = makeRepo();
    writeFileSync(Buffer.concat([Buffer.from(join(root, "hidden")), Buffer.from([0xff])]), "x\n");
    both(root, commit);
  });
});
