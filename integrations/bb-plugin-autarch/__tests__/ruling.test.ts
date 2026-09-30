import { existsSync, lstatSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  estateRoot,
  parseRuling,
  pinRoot,
  renderRuling,
  rulingPath,
  RulingWriteError,
  slugify,
  writeRuling,
  type Ruling,
} from "../ruling.js";

let base: string;
let root: string;
beforeEach(() => {
  base = mkdtempSync(join(tmpdir(), "autarch-ruling-"));
  root = join(base, "project");
  mkdirSync(root);
});
afterEach(() => rmSync(base, { recursive: true, force: true }));

const ruling = (over: Partial<Ruling> = {}): Ruling => ({
  decision_id: "dec-1",
  pick_id: "pick-1",
  revision: "rev-1",
  asking_thread: "thread-a",
  subject: "Which store?",
  options_shown: [
    { id: "a", label: "SQLite" },
    { id: "b", label: "Dolt" },
  ],
  picked: "a",
  ruled_by: "mk",
  ruled_at: "2026-09-29T10:00:00.000Z",
  ruling: "Use SQLite.",
  source: "home",
  ...over,
});

const target = { scope: "project" as const, decision_id: "dec-1", subject: "Which store?", date: "2026-09-29" };
const path = () => rulingPath(target);
const write = (r: Ruling = ruling()) => {
  const p = path();
  return writeRuling(pinRoot(root), p.dirs, p.file, renderRuling(r));
};

describe("format", () => {
  it("round trips options_shown, the instruction and a vizier ruling", () => {
    const r = ruling({
      ruled_by: "vizier",
      delegated_reason: "rule:cheap-and-reversible",
      picked: "instruction",
      instruction: "Use the small one.\nThen tell me.",
      session_id: "sess-9",
      supersedes: "dec-0",
      mentions: ["thread-b", "thread-c"],
    });
    expect(parseRuling(renderRuling(r))).toEqual(r);
    const text = renderRuling(r);
    expect(text).toContain('transcribed_by: "autarch-home"');
    expect(text).toContain('ruled_by: "vizier"');
  });

  it("round trips a minimal ruling without optional fields", () => {
    expect(parseRuling(renderRuling(ruling()))).toEqual(ruling());
  });

  it("an instruction containing --- and YAML-looking lines round-trips as a string", () => {
    const instruction = "---\nruled_by: mk\nhome:\n  picked: evil\n---\n- [x]: {a: b}\n# not a comment";
    const back = parseRuling(renderRuling(ruling({ instruction, picked: "instruction" })));
    expect(back.instruction).toBe(instruction);
    expect(back.ruled_by).toBe("mk");
    expect(back.picked).toBe("instruction");
  });

  it("a subject with a newline stays on the heading line", () => {
    const text = renderRuling(ruling({ subject: "a\nb: c" }));
    expect(text).toContain("\n# a b: c\n");
    expect(parseRuling(text).subject).toBe("a\nb: c");
  });
});

describe("paths", () => {
  it("builds docs/decisions/YYYY-MM-DD-slug-id.md", () => {
    expect(path()).toEqual({ dirs: ["docs", "decisions"], file: "2026-09-29-which-store-dec-1.md" });
    expect(rulingPath({ ...target, scope: "estate" }).dirs).toEqual(["rulings"]);
    expect(slugify("  !!  ")).toBe("decision");
  });

  it("rejects unsafe ids and dates", () => {
    expect(() => rulingPath({ ...target, decision_id: "../x" })).toThrow(RulingWriteError);
    expect(() => rulingPath({ ...target, date: "today" })).toThrow(RulingWriteError);
  });

  it("refuses estate filing when the Uqbar is unset", () => {
    expect(() => estateRoot({})).toThrow("estate-wide decisions need an Uqbar (see G-1)");
    expect(estateRoot({ AUTARCH_UQBAR_DIR: "/x" })).toBe("/x");
  });
});

describe("safe writer", () => {
  it("writes the file, creates docs/decisions at 0755, and leaves no temp file", () => {
    const out = write();
    expect(out.written).toBe(true);
    expect(parseRuling(readFileSync(out.path, "utf8"))).toEqual(ruling());
    expect(lstatSync(join(root, "docs", "decisions")).mode & 0o777).toBe(0o755);
    expect(readdirSync(join(root, "docs", "decisions"))).toEqual(["2026-09-29-which-store-dec-1.md"]);
  });

  it("writing twice gives one file with the same bytes", () => {
    const first = write();
    const bytes = readFileSync(first.path);
    const second = write();
    expect(second).toEqual({ path: first.path, written: false });
    expect(readFileSync(first.path).equals(bytes)).toBe(true);
    expect(readdirSync(join(root, "docs", "decisions"))).toHaveLength(1);
  });

  it("refuses to overwrite the same path with different bytes", () => {
    write();
    expect(() => write(ruling({ ruling: "Use Dolt." }))).toThrowError(/different content/);
  });

  it("refuses a docs symlink pointing outside the root and writes nothing outside", () => {
    const outside = join(base, "outside");
    mkdirSync(outside);
    symlinkSync(outside, join(root, "docs"));
    expect(() => write()).toThrowError(/symlink/);
    expect(readdirSync(outside)).toEqual([]);
  });

  it("refuses a docs/decisions symlink pointing outside the root", () => {
    const outside = join(base, "outside");
    mkdirSync(outside);
    mkdirSync(join(root, "docs"));
    symlinkSync(outside, join(root, "docs", "decisions"));
    expect(() => write()).toThrowError(/symlink/);
    expect(readdirSync(outside)).toEqual([]);
  });

  it("refuses when the root was replaced by a new directory at the same path", () => {
    const pinned = pinRoot(root);
    renameSync(root, join(base, "moved"));
    mkdirSync(root);
    const p = path();
    expect(() => writeRuling(pinned, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    expect(existsSync(join(root, "docs"))).toBe(false);
    expect(existsSync(join(base, "moved", "docs"))).toBe(false);
  });

  it("refuses when the root path is a symlink retargeted after filing", () => {
    const a = join(base, "a");
    const b = join(base, "b");
    mkdirSync(a);
    mkdirSync(b);
    const link = join(base, "link");
    symlinkSync(a, link);
    // serve resolved the link at filing and pinned the directory it pointed at
    const st = lstatSync(a, { bigint: true });
    const pinned = { path: link, dev: String(st.dev), ino: String(st.ino) };
    const p = path();
    expect(() => writeRuling(pinned, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    rmSync(link);
    symlinkSync(b, link);
    expect(() => writeRuling(pinned, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    expect(readdirSync(a)).toEqual([]);
    expect(readdirSync(b)).toEqual([]);
  });

  it("refuses when the root is gone, and unsafe names", () => {
    const pinned = pinRoot(root);
    rmSync(root, { recursive: true });
    expect(() => writeRuling(pinned, ["docs"], "x.md", "x")).toThrowError(/root changed/);
    mkdirSync(root);
    const again = pinRoot(root);
    expect(() => writeRuling(again, ["..", "x"], "x.md", "x")).toThrowError(/unsafe directory/);
    expect(() => writeRuling(again, ["docs"], "../x.md", "x")).toThrowError(/unsafe file name/);
  });
});
