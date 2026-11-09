import { afterEach, describe, expect, it } from "vitest";
import { spawnSync } from "node:child_process";
import { appendFileSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { Rig, type CliResult } from "./rig";
import { globMatcher } from "../../../../tools/bb-plugin-deploy/lib/gitops.mjs";

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
      expect(msg(r)).toMatch(/control character/);
      expect(rig.mutations()).toEqual([]);
    });
  }

  it("an untouched closure with a CR twin is refused too, not just a tampered one", () => {
    twinRig("x.mjs\r", "stamped");
    const r = verify();
    expect(r.status, msg(r)).toBe(10);
    expect(msg(r)).toMatch(/control character/);
  });

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
