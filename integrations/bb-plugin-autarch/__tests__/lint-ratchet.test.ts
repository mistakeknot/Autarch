// The lint ratchet counts escapes per file and moves in one direction only: down.
import { spawnSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { countEscapes, diff, main, mergeWarnings, normalize, scanTree } from "../scripts/check-lint-ratchet.mjs";

let dir: string;
beforeEach(() => {
  dir = mkdtempSync(join(tmpdir(), "lint-ratchet-"));
  vi.spyOn(console, "error").mockImplementation(() => {});
  vi.spyOn(console, "log").mockImplementation(() => {});
});
afterEach(() => {
  vi.restoreAllMocks();
  rmSync(dir, { recursive: true, force: true });
});
const env = () => ({ LINT_RATCHET_ROOT: dir, LINT_RATCHET_NO_OXLINT: "1" });
const put = (name: string, text: string) => {
  mkdirSync(join(dir, name, ".."), { recursive: true });
  writeFileSync(join(dir, name), text);
};

const AS = "a" + "s";
const ANY = AS + " an" + "y";
const UNK = AS + " unknown " + AS;
const DIS = "eslint-" + "disable";

describe("countEscapes", () => {
  it("counts every occurrence, not matched lines", () => {
    const c = countEscapes(`a ${ANY} ${ANY}; b ${AS} unknown   ${AS} T; // ${DIS}-next-line x\n/* ox` + `lint-disable y */`);
    expect(c).toEqual({ as_unknown_as: 1, as_any: 2, lint_disable: 2 });
  });
  it("counts a cast with a comment between its tokens", () => {
    expect(countEscapes(`x ${AS} /* c */ any; y ${AS} unknown /* c */ ${AS} T`)).toEqual({ as_unknown_as: 1, as_any: 1, lint_disable: 0 });
  });
  it("does not count words that merely contain the tokens", () => {
    expect(countEscapes("class any; const has any = 1; alias anything")).toEqual({ as_unknown_as: 0, as_any: 0, lint_disable: 0 });
  });
});

describe("diff", () => {
  it("reports a raise, a fall and a new file separately", () => {
    const base = { "a.ts": { as_any: 2 }, "b.ts": { as_any: 1 } };
    const live = { "a.ts": { as_any: 3 }, "b.ts": {}, "c.ts": { lint_disable: 1 } };
    const d = diff(base, live);
    expect(d.rose).toEqual(["a.ts: as_any 2 -> 3", "c.ts: lint_disable 0 -> 1"]);
    expect(d.fell).toEqual(["b.ts: as_any 1 -> 0"]);
  });
});

describe("mergeWarnings and normalize", () => {
  it("adds warning counts to files with and without escapes and drops zeros", () => {
    const merged = mergeWarnings({ "a.ts": { as_unknown_as: 1, as_any: 0, lint_disable: 0 } }, { "a.ts": 2, "b.ts": 1 });
    expect(normalize(merged)).toEqual({ "a.ts": { as_unknown_as: 1, oxlint_warning: 2 }, "b.ts": { oxlint_warning: 1 } });
  });
});

describe("main", () => {
  it("--write creates the baseline and the check then passes", () => {
    put("a.ts", `const x = y ${ANY};\n`);
    expect(main(["--write"], env())).toBe(0);
    expect(JSON.parse(readFileSync(join(dir, "lint-lock.json"), "utf8"))).toEqual({ "a.ts": { as_any: 1 } });
    expect(main([], env())).toBe(0);
  });
  it("fails when a count rises, and --write refuses to raise it", () => {
    put("a.ts", `const x = y ${ANY};\n`);
    main(["--write"], env());
    put("a.ts", `const x = y ${ANY}; const z = w ${ANY};\n`);
    expect(main([], env())).toBe(1);
    expect(main(["--write"], env())).toBe(1);
    expect(JSON.parse(readFileSync(join(dir, "lint-lock.json"), "utf8"))).toEqual({ "a.ts": { as_any: 1 } });
  });
  it("fails on a new file that carries an escape", () => {
    put("a.ts", "const x = 1;\n");
    main(["--write"], env());
    put("sub/b.ts", `const x = y ${UNK} T;\n`);
    expect(main([], env())).toBe(1);
  });
  it("fails when a count fell until the ledger is regenerated, then passes", () => {
    put("a.ts", `const x = y ${ANY};\n`);
    main(["--write"], env());
    put("a.ts", "const x = y;\n");
    expect(main([], env())).toBe(1);
    expect(main(["--write"], env())).toBe(0);
    expect(main([], env())).toBe(0);
  });
  it("ignores node_modules, dot directories and non-source files", () => {
    put(".gen/x.ts", ANY);
    put("node_modules/p/i.ts", ANY);
    put("notes.md", ANY);
    expect(scanTree(dir)).toEqual({});
  });
  it("scans every source extension oxlint reads and only exempts the script at its exact path", () => {
    for (const f of ["a.js", "b.jsx", "c.cts", "d.cjs"]) put(f, ANY);
    put("sub/check-lint-ratchet.mjs", ANY);
    put("scripts/check-lint-ratchet.mjs", ANY);
    expect(Object.keys(scanTree(dir)).sort()).toEqual(["a.js", "b.jsx", "c.cts", "d.cjs", "sub/check-lint-ratchet.mjs"]);
  });
  it("runs as a CLI from a path with spaces and fails on a new escape", () => {
    const root = join(dir, "with space");
    mkdirSync(join(root, "scripts"), { recursive: true });
    const script = join(root, "scripts", "check-lint-ratchet.mjs");
    copyFileSync(join(__dirname, "..", "scripts", "check-lint-ratchet.mjs"), script);
    writeFileSync(join(root, "a.ts"), `const x = y ${ANY};\n`);
    const e = { ...process.env, LINT_RATCHET_ROOT: root, LINT_RATCHET_NO_OXLINT: "1" };
    expect(spawnSync("node", [script], { env: e }).status).toBe(1);
  });
});
