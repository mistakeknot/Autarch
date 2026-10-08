// scripts/install-home-your-move.sh --check against a temp data.db and a stub bb (the --check-only test hook). Never live.
import { spawnSync } from "node:child_process";
import { chmodSync, mkdirSync, statSync, mkdtempSync, readdirSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";

const SCRIPT = resolve(__dirname, "../../../scripts/install-home-your-move.sh");
const dirs: string[] = [];
afterEach(() => dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true })));

function run(version: number | null, stub: string, args = ["--check"]) {
  const root = mkdtempSync(join(tmpdir(), "ihym-"));
  dirs.push(root);
  const data = join(root, "data");
  mkdirSync(data);
  const db = new Database(join(data, "data.db"));
  db.exec("CREATE TABLE migration_log (version INTEGER)");
  if (version !== null) {
    db.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value INTEGER NOT NULL)");
    db.prepare("INSERT INTO schema_meta VALUES ('schema_version', ?)").run(version);
    db.prepare("INSERT INTO migration_log VALUES (?)").run(version);
  }
  db.close();
  const bb = join(root, "bb");
  writeFileSync(bb, `#!/bin/sh\n${stub}\n`);
  chmodSync(bb, 0o755);
  const out = join(root, "out");
  mkdirSync(out);
  const r = spawnSync("bash", [SCRIPT, ...args], { env: { PATH: process.env.PATH!, HOME: root, HOME_YOUR_MOVE_OUT_DIR: out, HOME_YOUR_MOVE_TEST_DATA_DIR: data, HOME_YOUR_MOVE_TEST_BB: bb }, encoding: "utf8" });
  const sub = readdirSync(out)[0];
  const exp = sub ? join(out, sub, "moves-before.json") : "";
  return { r, exportText: (() => { try { return readFileSync(exp, "utf8"); } catch { return null; } })() };
}

const LIST = `if [ "$2" = list ]; then echo '{"plugins":[{"id":"autarch","enabled":true,"status":"running"}]}'; exit 0; fi`;

describe("install-home-your-move.sh export step", () => {
  it("schema 3: no moves table, a stub export is written and the check passes without calling bb home moves", () => {
    const { r, exportText } = run(3, `${LIST}\nif [ "$2" = moves ]; then echo called >&2; exit 9; fi`);
    expect(r.status).toBe(0);
    expect(JSON.parse(exportText!)).toEqual({ moves: [], schema: 3, note: "no moves table at schema 3" });
    expect(r.stdout).toContain("nothing to lose");
  });

  it("schema 4: the export is taken with bb home moves --json, and its failure is fatal", () => {
    const ok = run(4, `${LIST}\nif [ "$2" = moves ]; then echo '{"moves":[{"task_id":"T"}],"groups":{}}'; exit 0; fi`);
    expect(ok.r.status).toBe(0);
    expect(JSON.parse(ok.exportText!).moves).toHaveLength(1);
    const bad = run(4, `${LIST}\nif [ "$2" = moves ]; then exit 1; fi`);
    expect(bad.r.status).toBe(3);
    expect(bad.r.stdout).toContain("ABORT");
    const empty = run(4, `${LIST}\nif [ "$2" = moves ]; then exit 0; fi`);
    expect(empty.r.status).toBe(3);
  });

  it("unknown schema (no schema_meta) is not treated as nothing-to-lose", () => {
    const { r } = run(null, `${LIST}\nif [ "$2" = moves ]; then exit 1; fi`);
    expect(r.status).toBe(3);
  });

  it("export must have a moves array: null and object are fatal at schema 4", () => {
    for (const body of ['{"moves":null}', '{"moves":{}}', '{"groups":{}}']) {
      const { r } = run(4, `${LIST}\nif [ "$2" = moves ]; then echo '${body}'; exit 0; fi`);
      expect(r.status, body).toBe(3);
    }
  });

  it("the hook needs both variables, and the root guard comes first in the script", () => {
    const root = mkdtempSync(join(tmpdir(), "ihym-"));
    dirs.push(root);
    const r = spawnSync("bash", [SCRIPT, "--check"], { env: { PATH: process.env.PATH!, HOME: root, HOME_YOUR_MOVE_OUT_DIR: root, HOME_YOUR_MOVE_TEST_BB: "/bin/true" }, encoding: "utf8" });
    expect(r.status).toBe(64);
    expect(r.stderr).toContain("needs both");
    const src = readFileSync(SCRIPT, "utf8");
    const guard = src.indexOf('[ "$(id -u)" -ne 0 ] || { echo "the test hook is refused as root"');
    expect(guard).toBeGreaterThan(0);
    expect(guard).toBeLessThan(src.indexOf("TESTHOOK=1;"));
    expect(guard).toBeLessThan(src.indexOf("needs both"));
  });

  it("the test hook is refused outside --check", () => {
    const { r } = run(3, LIST, ["--rollback"]);
    expect(r.status).toBe(64);
  });

  it("non-root: the per-run directory is a fresh mktemp directory of mode 700 and the override still works", () => {
    const { r } = run(3, LIST);
    expect(r.status).toBe(0);
    const out = /output dir (\S+)/.exec(r.stdout)![1]!;
    expect(out).toMatch(/home-your-move-\d{8}T\d{6}Z-[A-Za-z0-9]{6}$/);
    expect(statSync(out).mode & 0o777).toBe(0o700);
  });
});

describe("out_parent_resolve (the root-only check on HOME_YOUR_MOVE_OUT_DIR, exercised with the current uid)", () => {
  const me = process.getuid!();
  const resolve_ = (dir: string, uid: number, top: string) => spawnSync("bash", ["-c", `source "${SCRIPT}"; out_parent_resolve "$1" "$2" "$3"`, "x", dir, String(uid), top], { encoding: "utf8" });
  const tmp = () => { const d = mkdtempSync(join(tmpdir(), "ihym-")); dirs.push(d); chmodSync(d, 0o755); return d; };
  const ok = (dir: string, top: string, uid = me) => resolve_(dir, uid, top).status === 0;

  it("accepts a compliant chain and prints the resolved path", () => {
    const top = tmp();
    const a = join(top, "a");
    const b = join(a, "b");
    mkdirSync(b, { recursive: true });
    chmodSync(a, 0o755);
    chmodSync(b, 0o755);
    const r = resolve_(b, me, top);
    expect(r.status).toBe(0);
    expect(r.stdout.trim()).toBe(realpathSync(b));
  });

  it("refuses group or other write on the final directory", () => {
    const top = tmp();
    const d = join(top, "d");
    mkdirSync(d);
    for (const m of [0o775, 0o757, 0o777]) { chmodSync(d, m); expect(ok(d, top), m.toString(8)).toBe(false); }
    chmodSync(d, 0o755);
    expect(ok(d, top)).toBe(true);
  });

  it("refuses an ancestor that others can write, even when the final directory is fine", () => {
    const top = tmp();
    const a = join(top, "a");
    const b = join(a, "b");
    mkdirSync(b, { recursive: true });
    chmodSync(b, 0o755);
    chmodSync(a, 0o775);
    expect(ok(b, top)).toBe(false);
    chmodSync(a, 0o777);
    expect(ok(b, top)).toBe(false);
    chmodSync(a, 0o1777); // sticky is acceptable only for the final directory
    expect(ok(b, top)).toBe(false);
    chmodSync(a, 0o755);
    expect(ok(b, top)).toBe(true);
  });

  it("allows a sticky final directory but not a plain writable one", () => {
    const top = tmp();
    const d = join(top, "d");
    mkdirSync(d);
    chmodSync(d, 0o1777);
    expect(ok(d, top)).toBe(true);
    chmodSync(d, 0o777);
    expect(ok(d, top)).toBe(false);
  });

  it("resolves symlinks first: a link into a writable ancestor chain is refused", () => {
    const top = tmp();
    const bad = join(top, "bad");
    const target = join(bad, "t");
    mkdirSync(target, { recursive: true });
    chmodSync(target, 0o755);
    chmodSync(bad, 0o777);
    symlinkSync(target, join(top, "link"));
    expect(ok(join(top, "link"), top)).toBe(false);
    chmodSync(bad, 0o755);
    expect(resolve_(join(top, "link"), me, top).stdout.trim()).toBe(realpathSync(target));
  });

  it("refuses a resolved path with a newline (or other control character) in a directory name", () => {
    const top = tmp();
    for (const name of ["a\nb", "c\td"]) {
      const d = join(top, name, "x");
      mkdirSync(d, { recursive: true });
      chmodSync(join(top, name), 0o755);
      chmodSync(d, 0o755);
      expect(ok(d, top), JSON.stringify(name)).toBe(false);
    }
  });

  it("refuses an override that ends in a newline even when the path without it exists", () => {
    const top = tmp();
    const d = join(top, "run");
    mkdirSync(d, { mode: 0o755 });
    mkdirSync(d + "\n", { mode: 0o755 });
    expect(ok(d + "\n", top)).toBe(false);
    expect(ok(d, top)).toBe(true);
  });

  it("refuses another owner, a missing path and a plain file", () => {
    const top = tmp();
    expect(ok(top + "/", top, me + 1)).toBe(true); // the anchor itself is not checked
    const d = join(top, "d");
    mkdirSync(d);
    chmodSync(d, 0o755);
    expect(ok(d, top, me + 1)).toBe(false);
    expect(ok(join(top, "missing"), top)).toBe(false);
    writeFileSync(join(top, "f"), "x");
    expect(ok(join(top, "f"), top)).toBe(false);
  });

  it("sourcing the script defines the functions and runs nothing else", () => {
    const r = spawnSync("bash", ["-c", `source "${SCRIPT}"; echo sourced`], { encoding: "utf8" });
    expect(r.status).toBe(0);
    expect(r.stdout).toBe("sourced\n");
  });
});

describe("runtime_names_live_process (reused pid defence, fed a fake proc root)", () => {
  const tmp = () => { const d = mkdtempSync(join(tmpdir(), "ihym-")); dirs.push(d); return d; };
  function check(runtime: object, procs: Record<string, string>, setup?: (root: string) => void): boolean {
    const root = tmp();
    const proc = join(root, "proc");
    mkdirSync(proc, { recursive: true });
    for (const [pid, cmd] of Object.entries(procs)) { mkdirSync(join(proc, pid), { recursive: true }); writeFileSync(join(proc, pid, "cmdline"), cmd.replaceAll("@ROOT@", root)); }
    setup?.(root);
    const f = join(root, "bb-app-runtime.json");
    writeFileSync(f, JSON.stringify(runtime).replaceAll("@ROOT@", root));
    return spawnSync("bash", ["-c", `source "${SCRIPT}"; runtime_names_live_process "$1" "$2"`, "x", f, proc], { encoding: "utf8" }).status === 0;
  }
  const cmd = "node\x00/opt/bb/dist/server.js\x00--port\x00123\x00";

  it("accepts a pid with an argv element exactly equal to the entryPath", () => {
    expect(check({ pid: 42, entryPath: "/opt/bb/dist/server.js" }, { "42": cmd })).toBe(true);
    expect(check({ pid: 42, entryPath: "/opt/bb/dist/server.js" }, { "42": "node\x00/opt/bb/dist/server.js" })).toBe(true); // no trailing NUL
  });
  it("accepts the resolved (symlink-free) form of the entryPath", () => {
    const setup = (root: string) => { mkdirSync(join(root, "real")); writeFileSync(join(root, "real", "server.js"), ""); symlinkSync(join(root, "real"), join(root, "link")); };
    expect(check({ pid: 42, entryPath: "@ROOT@/link/server.js" }, { "42": "node\x00@ROOT@/real/server.js\x00" }, setup)).toBe(true);
  });
  it("refuses a different full path with the same basename", () => {
    expect(check({ pid: 42, entryPath: "/elsewhere/server.js" }, { "42": cmd })).toBe(false);
  });
  it("refuses an argument that only contains the path as a substring", () => {
    expect(check({ pid: 42, entryPath: "/opt/bb/dist/server.js" }, { "42": "sleep\x00--note=/opt/bb/dist/server.js.bak\x00600\x00" })).toBe(false);
    expect(check({ pid: 42, entryPath: "/opt/bb/dist/server.js" }, { "42": "sleep\x00600\x00" })).toBe(false);
  });
  it("refuses a missing process, a missing or non-numeric pid, and a missing entryPath", () => {
    expect(check({ pid: 43, entryPath: "/opt/bb/dist/server.js" }, { "42": cmd })).toBe(false);
    expect(check({ entryPath: "/opt/bb/dist/server.js" }, { "42": cmd })).toBe(false);
    expect(check({ pid: "4x", entryPath: "/opt/bb/dist/server.js" }, { "42": cmd })).toBe(false);
    expect(check({ pid: 42 }, { "42": cmd })).toBe(false);
  });
});
