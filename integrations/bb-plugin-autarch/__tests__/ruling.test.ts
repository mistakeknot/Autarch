import { existsSync, lstatSync, mkdirSync, writeFileSync, mkdtempSync, readdirSync, readFileSync, renameSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  estateRoot,
  parseRuling,
  pinRoot,
  setFsUuidResolverForTest,
  fsUuidFrom,
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
afterEach(() => {
  setFsUuidResolverForTest(null);
  rmSync(base, { recursive: true, force: true });
});

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

  it("round trips an automatic ruling with its evidence line", () => {
    const r = ruling({ ruled_by: "auto", auto_evidence: "PR owner/name#7 merged at 2026-10-14T00:00:00Z" });
    expect(parseRuling(renderRuling(r))).toEqual(r);
    expect(renderRuling(r)).toContain('ruled_by: "auto"');
    expect(renderRuling(r)).toContain('auto_evidence: "PR owner/name#7 merged at 2026-10-14T00:00:00Z"');
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

  it("accepts a dev-only change (the partition's device number moved across a reboot) when the inode is the same", () => {
    const st = lstatSync(root, { bigint: true });
    const pinned = { path: root, dev: String(st.dev - 1n), ino: String(st.ino), fsid: "U1" };
    setFsUuidResolverForTest(() => "U1");
    const p = path();
    expect(writeRuling(pinned, p.dirs, p.file, "x").written).toBe(true);
    expect(existsSync(join(root, ...p.dirs, p.file))).toBe(true);
  });

  it("refuses a dev-only change when the filesystem UUID is not the pinned one (a colliding-inode replacement)", () => {
    const st = lstatSync(root, { bigint: true });
    const pinned = { path: root, dev: String(st.dev - 1n), ino: String(st.ino), fsid: "U1" };
    setFsUuidResolverForTest(() => "U2");
    const p = path();
    expect(() => writeRuling(pinned, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    expect(existsSync(join(root, "docs"))).toBe(false);
  });

  it("refuses a pinned UUID that no longer matches even when the device and inode are unchanged (device number reused after a reboot)", () => {
    const st = lstatSync(root, { bigint: true });
    const pinned = { path: root, dev: String(st.dev), ino: String(st.ino), fsid: "U1" };
    setFsUuidResolverForTest(() => "U2");
    const p = path();
    expect(() => writeRuling(pinned, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    expect(existsSync(join(root, "docs"))).toBe(false);
  });

  it("refuses a dev-only change when no filesystem UUID was pinned or the filesystem has none (FUSE, overlay, tmpfs)", () => {
    const st = lstatSync(root, { bigint: true });
    const p = path();
    setFsUuidResolverForTest(() => "U1");
    expect(() => writeRuling({ path: root, dev: String(st.dev - 1n), ino: String(st.ino) }, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    expect(() => writeRuling({ path: root, dev: String(st.dev - 1n), ino: String(st.ino), fsid: null }, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    setFsUuidResolverForTest(() => null);
    expect(() => writeRuling({ path: root, dev: String(st.dev - 1n), ino: String(st.ino), fsid: "U1" }, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    expect(existsSync(join(root, "docs"))).toBe(false);
  });

  it("still refuses a replaced root when only the device number differs", () => {
    const st = lstatSync(root, { bigint: true });
    setFsUuidResolverForTest(() => "U1");
    const pinned = { path: root, dev: String(st.dev - 1n), ino: String(st.ino), fsid: "U1" };
    renameSync(root, join(base, "moved"));
    mkdirSync(root);
    const p = path();
    expect(() => writeRuling(pinned, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    expect(existsSync(join(root, "docs"))).toBe(false);
  });

  it("refuses a dev-only change when a parent of the root path is a symlink", () => {
    const real = join(base, "real");
    mkdirSync(join(real, "proj"), { recursive: true });
    const via = join(base, "via");
    symlinkSync(real, via);
    const st = lstatSync(join(real, "proj"), { bigint: true });
    setFsUuidResolverForTest(() => "U1");
    const pinned = { path: join(via, "proj"), dev: String(st.dev - 1n), ino: String(st.ino), fsid: "U1" };
    const p = path();
    expect(() => writeRuling(pinned, p.dirs, p.file, "x")).toThrowError("project root changed since filing");
    expect(readdirSync(join(real, "proj"))).toEqual([]);
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

// ---- Task 2.5: card ids in rulings ----
import { mkdtempSync as mkdtemp25, readFileSync as read25, rmSync as rm25, lstatSync as lstat25 } from "node:fs";
import { join as join25 } from "node:path";
import { tmpdir as tmpdir25 } from "node:os";
import { describe as describe25, expect as expect25, it as it25 } from "vitest";
import { parseRuling as parse25, renderRuling as render25, rulingPath as path25, writeRuling as write25 } from "../ruling.js";

describe25("card rulings (finding r1-2)", () => {
  const card = { decision_id: "card-01J0000000000000000000000A-g1", pick_id: "p", revision: "r", asking_thread: "thr_a", subject: "S", options_shown: [{ id: "a", label: "A" }], picked: "a", ruled_by: "mk" as const, ruled_at: "2026-10-01T00:00:00.000Z", ruling: "Picked A", source: "autarch-home:home", card_id: "01J0000000000000000000000A", card_key: "TASK-1", generation: 1, supersedes: "card-01J0000000000000000000000A-g0" };

  it25("round-trips card_id, card_key, generation and supersedes", () => {
    expect25(parse25(render25(card))).toEqual(card);
  });

  it25("writes card-<id>-g1 and refuses the card:<id> form with bad-name", () => {
    const dir = mkdtemp25(join25(tmpdir25(), "ruling-card-"));
    try {
      const st = lstat25(dir);
      const t = path25({ scope: "project", decision_id: card.decision_id, subject: "S", date: "2026-10-01" });
      write25({ path: dir, dev: String(st.dev), ino: String(st.ino) }, t.dirs, t.file, render25(card));
      expect25(read25(join25(dir, ...t.dirs, t.file), "utf8")).toContain("generation: 1");
      expect25(() => path25({ scope: "project", decision_id: "card:01J0000000000000000000000A", subject: "S", date: "2026-10-01" })).toThrow(expect25.objectContaining({ code: "bad-name" }));
    } finally {
      rm25(dir, { recursive: true, force: true });
    }
  });
});

describe("fsUuidFrom (mk-mpa66)", () => {
  it("maps a device number to the UUID of its block-device filesystem and gives null for FUSE, overlay and unknown devices", () => {
    const d = mkdtempSync(join(tmpdir(), "uuid-"));
    try {
      writeFileSync(join(d, "sda1"), "");
      writeFileSync(join(d, "sdb1"), "");
      mkdirSync(join(d, "by-uuid"));
      symlinkSync(join(d, "sda1"), join(d, "by-uuid", "AAAA-1111"));
      const mi = [
        `22 1 259:6 / / rw - ext4 ${join(d, "sda1")} rw`,
        `23 1 259:7 / /data rw - ext4 ${join(d, "sdb1")} rw`,
        "24 1 0:50 / /mnt rw - fuse.sshfs user@host:/x rw",
        "25 1 0:51 / /ov rw - overlay overlay rw",
        "26 1 0:52 / /t rw - tmpfs tmpfs rw",
      ].join("\n");
      const dev = (maj: bigint, min: bigint) => ((maj & 0xfffn) << 8n) | (min & 0xffn) | ((min & ~0xffn) << 12n);
      const yes = () => true;
      expect(fsUuidFrom(dev(259n, 6n), mi, join(d, "by-uuid"), d + "/", yes)).toBe("AAAA-1111");
      // a mount whose source is only labelled like a block device (a regular file here, tmpfs in life) gets no UUID
      expect(fsUuidFrom(dev(259n, 6n), mi, join(d, "by-uuid"), d + "/")).toBeNull();
      expect(fsUuidFrom(dev(259n, 6n), mi, join(d, "by-uuid"), d + "/", () => false)).toBeNull();
      const labelled = `30 1 0:60 / /x rw - tmpfs ${join(d, "sda1")} rw`;
      expect(fsUuidFrom(dev(0n, 60n), labelled, join(d, "by-uuid"), d + "/")).toBeNull();
      expect(fsUuidFrom(dev(259n, 7n), mi, join(d, "by-uuid"), d + "/", yes)).toBeNull();
      expect(fsUuidFrom(dev(0n, 50n), mi, join(d, "by-uuid"), d + "/", yes)).toBeNull();
      expect(fsUuidFrom(dev(0n, 51n), mi, join(d, "by-uuid"), d + "/", yes)).toBeNull();
      expect(fsUuidFrom(dev(9n, 9n), mi, join(d, "by-uuid"), d + "/", yes)).toBeNull();
    } finally {
      rmSync(d, { recursive: true, force: true });
    }
  });
});
