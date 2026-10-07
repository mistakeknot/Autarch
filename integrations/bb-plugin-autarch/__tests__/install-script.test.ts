// scripts/install-home-your-move.sh --check against a temp data.db and a stub bb (the --check-only test hook). Never live.
import { spawnSync } from "node:child_process";
import { chmodSync, mkdirSync, statSync, mkdtempSync, readdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
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

describe("out_parent_ok (the root-only check on HOME_YOUR_MOVE_OUT_DIR, exercised with the current uid)", () => {
  const ok = (dir: string, uid: number) => spawnSync("bash", ["-c", `source "${SCRIPT}"; out_parent_ok "$1" "$2"`, "x", dir, String(uid)], { encoding: "utf8" }).status === 0;
  const me = process.getuid!();
  const tmp = () => { const d = mkdtempSync(join(tmpdir(), "ihym-")); dirs.push(d); return d; };

  it("accepts an own-uid directory nobody else can write, and refuses group or other write", () => {
    const d = tmp();
    chmodSync(d, 0o755);
    expect(ok(d, me)).toBe(true);
    chmodSync(d, 0o775);
    expect(ok(d, me)).toBe(false);
    chmodSync(d, 0o757);
    expect(ok(d, me)).toBe(false);
  });

  it("accepts a writable directory only when sticky", () => {
    const d = tmp();
    chmodSync(d, 0o1777);
    expect(ok(d, me)).toBe(true);
    chmodSync(d, 0o777);
    expect(ok(d, me)).toBe(false);
  });

  it("refuses another owner, a missing path and a plain file; resolves a symlink to its target", () => {
    const d = tmp();
    chmodSync(d, 0o755);
    expect(ok(d, me + 1)).toBe(false);
    expect(ok(join(d, "missing"), me)).toBe(false);
    writeFileSync(join(d, "f"), "x");
    expect(ok(join(d, "f"), me)).toBe(false);
    const open = join(d, "open");
    mkdirSync(open);
    chmodSync(open, 0o777);
    symlinkSync(open, join(d, "link"));
    expect(ok(join(d, "link"), me)).toBe(false);
  });

  it("sourcing the script defines the function and runs nothing else", () => {
    const r = spawnSync("bash", ["-c", `source "${SCRIPT}"; echo sourced`], { encoding: "utf8" });
    expect(r.status).toBe(0);
    expect(r.stdout).toBe("sourced\n");
  });
});
