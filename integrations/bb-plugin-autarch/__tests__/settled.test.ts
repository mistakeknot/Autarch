import { execFileSync } from "node:child_process";
import { mkdirSync, symlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { countSettled, readSettled, SETTLED_FILE, SETTLED_MAX_BYTES } from "../settled.js";
import { tmpDir } from "./helpers.js";

const HEADER = "# Settled for mk: a trial (2026-10-10 to 2026-10-24)\n# Agreement: x. Format: date | question | answer | clause | bead\n";

describe("countSettled", () => {
  it("counts only lines that start with a date and a bar, after the header", () => {
    const text = `${HEADER}2026-10-12 | q1 | a1 | c | mk-1\n2026-10-13 | q2 | a2 | c | mk-2\n\nnot a line\n 2026-10-14 | indented\n2026-10-14 x\n2026-10-15 | q3 | a3 | c | mk-3\n`;
    expect(countSettled(text)).toEqual({ total: 3, dates: ["2026-10-12", "2026-10-13", "2026-10-15"] });
  });

  it("does not count header lines, even one that looks like an entry after a leading hash", () => {
    expect(countSettled("# 2026-10-12 | header\n")).toEqual({ total: 0, dates: [] });
    expect(countSettled("")).toEqual({ total: 0, dates: [] });
  });

  it("reads CRLF files and ignores a pipe-less date line", () => {
    expect(countSettled("2026-10-12 | a\r\n2026-10-13 |b\r\n2026-10-14\r\n").total).toBe(2);
  });
});

describe("readSettled", () => {
  let t: ReturnType<typeof tmpDir>;
  beforeEach(() => {
    t = tmpDir();
  });
  afterEach(() => t.cleanup());
  const put = (body: string, name = SETTLED_FILE, dir = t.dir) => {
    const p = join(dir, name);
    writeFileSync(p, body);
    return p;
  };

  it("counts a coordinator's file, in total and over a window", () => {
    const p = put(`${HEADER}2026-10-02 | a\n2026-10-12 | b\n2026-10-13 | c\n`);
    expect(readSettled(p)).toEqual({ ok: true, path: p, total: 3, count: 3, first: "2026-10-02", last: "2026-10-13", per_week: { "2026-W40": 1, "2026-W42": 2 } });
    expect(readSettled(p, "2026-10-10T00:00:00.000Z")).toMatchObject({ ok: true, total: 3, count: 2, first: "2026-10-12", last: "2026-10-13", per_week: { "2026-W42": 2 } });
    expect(readSettled(p, "2026-11-01T00:00:00.000Z")).toMatchObject({ ok: true, total: 3, count: 0, first: null, last: null, per_week: {} });
  });

  it("a header-only file is zero, not an error", () => {
    expect(readSettled(put(HEADER))).toMatchObject({ ok: true, total: 0, count: 0 });
  });

  it("refuses a FIFO named settled-for-mk.md without blocking on it", () => {
    const d = join(t.dir, "fifo");
    mkdirSync(d);
    execFileSync("mkfifo", [join(d, SETTLED_FILE)]);
    const t0 = Date.now();
    expect(readSettled(join(d, SETTLED_FILE))).toMatchObject({ ok: false });
    expect(Date.now() - t0).toBeLessThan(1000);
  });

  it("refuses a relative path, another file name, a missing file, a directory, a symlink and an oversize file, without reading them", () => {
    expect(readSettled(SETTLED_FILE)).toMatchObject({ ok: false });
    expect(readSettled(put("2026-10-12 | a\n", "notes.md"))).toMatchObject({ ok: false });
    expect(readSettled(join(t.dir, "nope", SETTLED_FILE))).toMatchObject({ ok: false });
    const d = join(t.dir, "sub");
    mkdirSync(d);
    mkdirSync(join(d, SETTLED_FILE));
    expect(readSettled(join(d, SETTLED_FILE))).toMatchObject({ ok: false });
    const real = put("2026-10-12 | a\n", "real.md");
    const link = join(t.dir, "l");
    mkdirSync(link);
    symlinkSync(real, join(link, SETTLED_FILE));
    expect(readSettled(join(link, SETTLED_FILE))).toMatchObject({ ok: false });
    const big = join(t.dir, "big");
    mkdirSync(big);
    expect(readSettled(put("x".repeat(SETTLED_MAX_BYTES + 1), SETTLED_FILE, big))).toMatchObject({ ok: false });
  });
});
