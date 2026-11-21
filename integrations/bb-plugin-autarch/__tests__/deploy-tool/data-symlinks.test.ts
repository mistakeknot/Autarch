import { afterEach, describe, expect, it } from "vitest";
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readlinkSync, rmSync, symlinkSync, unlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { makeBackup } from "../../../../tools/bb-plugin-deploy/lib/data.mjs";

// makeBackup is plain JavaScript: tsc infers `data: never[]` from its empty-array literal, so a manifest entry is read through this shape.
type DataEntry = { skipped?: string; files?: unknown };
const entry0 = (r: { manifest: { data: unknown[] } }): DataEntry => r.manifest.data[0] as DataEntry;

// A data directory is copied with cpSync (verbatimSymlinks), so a symlink in it is copied as a symlink. The manifest used to list
// only regular files, so a changed or added symlink in a finished backup left the manifest unchanged, and the once-per-commit
// reuse check (backupHolds) accepted that stale backup. Each symlink is now recorded with its target, read without following it.
describe("data directory backups with symlinks (mk-pgjnm)", () => {
  const dirs: string[] = [];
  afterEach(() => {
    for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
  });
  const tmp = (): string => {
    const d = mkdtempSync(join(tmpdir(), "bpd-symlink-"));
    dirs.push(d);
    return d;
  };

  const setup = () => {
    const root = tmp();
    const src = join(root, "live-dir");
    mkdirSync(src);
    writeFileSync(join(src, "a.txt"), "a\n");
    const plugin = { pluginId: "fixture", data: [{ name: "dir", kind: "dir", path: "{bbData}", backup: "once-per-commit" }], export: [] };
    const call = (commit = "a".repeat(40)) =>
      makeBackup({
        plugin,
        vars: { bbData: src },
        backupsRoot: join(root, "backups"),
        stateDir: join(root, "state"),
        commit,
        previousPluginDir: "/nonexistent/previous",
        serverUrl: "http://127.0.0.1:1",
        lockFd: undefined,
        env: {},
      });
    return { root, src, call };
  };

  it("control: an untouched backup of the same commit is reused", () => {
    const { src, call } = setup();
    symlinkSync("a.txt", join(src, "link"));
    call();
    expect(entry0(call()).skipped).toBe("once-per-commit");
  });

  it("records each symlink with its target, without following it", () => {
    const { root, src, call } = setup();
    const outside = join(root, "outside");
    mkdirSync(outside);
    writeFileSync(join(outside, "secret.txt"), "not part of the data\n");
    symlinkSync("a.txt", join(src, "link"));
    symlinkSync(outside, join(src, "dirlink"));
    symlinkSync("missing-target", join(src, "dangling"));
    const files = entry0(call()).files as { path: string; sha256?: string; symlink?: string }[];
    expect(files.map((f) => f.path)).toEqual(["a.txt", "dangling", "dirlink", "link"]);
    expect(files.find((f) => f.path === "link")).toEqual({ path: "link", symlink: "a.txt" });
    expect(files.find((f) => f.path === "dirlink")).toEqual({ path: "dirlink", symlink: outside });
    expect(files.find((f) => f.path === "dangling")).toEqual({ path: "dangling", symlink: "missing-target" });
    expect(files.find((f) => f.path === "a.txt")?.sha256).toMatch(/^[0-9a-f]{64}$/);
  });

  it("the bead's reproduction: a symlink retargeted in the backup copy cannot be reused as the once-per-commit backup", () => {
    const { src, call } = setup();
    symlinkSync("a.txt", join(src, "link"));
    const first = call();
    const copy = join(first.dir, "dir", "link");
    expect(readlinkSync(copy)).toBe("a.txt");
    unlinkSync(copy);
    symlinkSync("/etc/passwd", copy);
    const second = call();
    expect(entry0(second).skipped).toBeUndefined();
    expect(second.dir).not.toBe(first.dir);
    expect(readlinkSync(join(second.dir, "dir", "link"))).toBe("a.txt");
  });

  it("a symlink added to the backup copy cannot be reused as the once-per-commit backup", () => {
    const { call } = setup();
    const first = call();
    symlinkSync("/etc/passwd", join(first.dir, "dir", "extra"));
    expect(entry0(call()).skipped).toBeUndefined();
  });

  it("a symlink removed from the backup copy cannot be reused as the once-per-commit backup", () => {
    const { src, call } = setup();
    symlinkSync("a.txt", join(src, "link"));
    const first = call();
    unlinkSync(join(first.dir, "dir", "link"));
    expect(entry0(call()).skipped).toBeUndefined();
  });

  it("a regular file swapped for a symlink of the same name cannot be reused either", () => {
    const { src, call } = setup();
    const first = call();
    unlinkSync(join(first.dir, "dir", "a.txt"));
    symlinkSync(join(src, "a.txt"), join(first.dir, "dir", "a.txt"));
    expect(lstatSync(join(first.dir, "dir", "a.txt")).isSymbolicLink()).toBe(true);
    expect(entry0(call()).skipped).toBeUndefined();
  });

  it("a symlink target that is not valid UTF-8 is refused, as a file name is, so two targets cannot read back as one", () => {
    const { root, src, call } = setup();
    const raw = Buffer.concat([Buffer.from(root + "/x"), Buffer.from([0xff])]);
    // symlinkSync takes a Buffer target
    symlinkSync(raw, join(src, "bad"));
    expect(() => call()).toThrow(/not valid UTF-8/);
    expect(existsSync(join(root, "state", "once-per-commit"))).toBe(false);
  });
});
