import { afterEach, describe, expect, it } from "vitest";
import { appendFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { makeBackup } from "../../../../tools/bb-plugin-deploy/lib/data.mjs";

// A filename is bytes, not text. Decoding it as UTF-8 maps every invalid byte to U+FFFD, so the raw name "x\xff" and the
// valid name "x�" became one manifest path and one digest (the UTF-8 twin's), and a backup holding both could be
// tampered with in the raw file and still be accepted as the once-per-commit backup.
describe("data directory backups with names that are not valid UTF-8", () => {
  const dirs: string[] = [];
  afterEach(() => {
    for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
  });
  const tmp = (): string => {
    const d = mkdtempSync(join(tmpdir(), "bpd-rawbyte-"));
    dirs.push(d);
    return d;
  };
  const RAW = Buffer.from([0x78, 0xff]); // "x\xff"
  const bytePath = (dir: string, name: Buffer): Buffer => Buffer.concat([Buffer.from(dir + "/"), name]);

  const setup = () => {
    const root = tmp();
    const src = join(root, "live-dir");
    mkdirSync(src);
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

  it("a raw-byte name beside its U+FFFD twin is refused, not collapsed into one manifest entry", () => {
    const { src, call } = setup();
    writeFileSync(join(src, "x�"), "twin\n");
    writeFileSync(bytePath(src, RAW), "raw\n");
    expect(() => call()).toThrow(/not valid UTF-8/);
  });

  it("the bead's reproduction: tampering with the raw-byte copy cannot make the second call skip as once-per-commit", () => {
    const { root, src, call } = setup();
    writeFileSync(join(src, "x�"), "twin\n");
    writeFileSync(bytePath(src, RAW), "raw\n");
    let first: { dir: string } | undefined;
    try {
      first = call();
    } catch {
      /* refused: the fix */
    }
    if (first) appendFileSync(bytePath(join(first.dir, "dir"), RAW), "tampered\n");
    // Whatever happened the first time, the same commit must never be answered with a skipped, reused backup.
    expect(() => {
      const second = call();
      expect(second.manifest.data[0].skipped).toBeUndefined();
    }).toThrow(/not valid UTF-8/);
    expect(existsSync(join(root, "state", "once-per-commit"))).toBe(false);
  });

  it("a raw-byte name in a subdirectory is refused too", () => {
    const { src, call } = setup();
    mkdirSync(join(src, "sub"));
    writeFileSync(bytePath(join(src, "sub"), RAW), "raw\n");
    expect(() => call()).toThrow(/not valid UTF-8/);
  });

  it("a raw-byte directory name is refused, so its contents cannot be read through the decoded name", () => {
    const { src, call } = setup();
    mkdirSync(join(src, "x�"));
    mkdirSync(bytePath(src, RAW));
    writeFileSync(bytePath(src, Buffer.concat([RAW, Buffer.from("/f")])), "raw\n");
    expect(() => call()).toThrow(/not valid UTF-8/);
  });

  it("a cached backup that later gains a raw-byte name is not reused; a fresh backup is taken from the clean live data", () => {
    const { src, call } = setup();
    writeFileSync(join(src, "plain.txt"), "a\n");
    const first = call();
    expect(first.manifest.data[0].skipped).toBeUndefined();
    writeFileSync(bytePath(join(first.dir, "dir"), RAW), "raw\n");
    const second = call();
    expect(second.manifest.data[0].skipped).toBeUndefined();
    expect(second.dir).not.toBe(first.dir);
  });

  it("a cached backup is refused-then-replaced, never reused, when the live data gains a raw-byte name", () => {
    const { src, call } = setup();
    writeFileSync(join(src, "plain.txt"), "a\n");
    const first = call();
    writeFileSync(bytePath(first.dir + "/dir", RAW), "raw\n");
    writeFileSync(bytePath(src, RAW), "raw\n");
    expect(() => call()).toThrow(/not valid UTF-8/);
  });

  it("ordinary names, including valid non-ASCII UTF-8, still back up and are reused once per commit", () => {
    const { src, call } = setup();
    writeFileSync(join(src, "plain.txt"), "a\n");
    writeFileSync(join(src, "café.txt"), "b\n");
    mkdirSync(join(src, "sub"));
    writeFileSync(join(src, "sub", "c.txt"), "c\n");
    const first = call();
    expect(first.manifest.data[0].skipped).toBeUndefined();
    expect((first.manifest.data[0].files as { path: string }[]).map((f) => f.path)).toEqual(["café.txt", "plain.txt", "sub/c.txt"]);
    expect(readFileSync(join(first.dir, "dir", "plain.txt"), "utf8")).toBe("a\n");
    const second = call();
    expect(second.manifest.data[0].skipped).toBe("once-per-commit");
  });
});
