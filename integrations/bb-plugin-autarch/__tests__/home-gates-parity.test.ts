// scripts/home-gates.sh against scripts/home-ui.sh over fixture commits, plus deploy.json against the plugin-file format
// (bb-plugin-deploy/plugin@1). Never live: temp git repos, temp data directories and a stub bb. Nothing here runs a deploy.
import { spawnSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import Database from "better-sqlite3";

const REPO_ROOT = resolve(__dirname, "../../..");
const UI_SCRIPT = join(REPO_ROOT, "scripts/home-ui.sh");
const GATES = join(REPO_ROOT, "scripts/home-gates.sh");
const DEPLOY_JSON = join(REPO_ROOT, "integrations/bb-plugin-autarch/deploy.json");
const dirs: string[] = [];
afterEach(() => dirs.splice(0).forEach((d) => rmSync(d, { recursive: true, force: true })));
const tmp = (p: string) => { const d = mkdtempSync(join(tmpdir(), p)); dirs.push(d); return d; };

// ---- fixture commits -----------------------------------------------------------------------------------------------------------
const GIT_ENV = { GIT_AUTHOR_NAME: "t", GIT_AUTHOR_EMAIL: "t@example.invalid", GIT_COMMITTER_NAME: "t", GIT_COMMITTER_EMAIL: "t@example.invalid", GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_SYSTEM: "/dev/null" };
function git(cwd: string, ...args: string[]) {
  const r = spawnSync("git", args, { cwd, encoding: "utf8", env: { PATH: process.env.PATH!, ...GIT_ENV } });
  if (r.status !== 0) throw new Error(`git ${args.join(" ")}: ${r.stderr}`);
  return r.stdout.trim();
}
const BASE: Record<string, string> = {
  "package.json": '{"name":"x","dependencies":{}}\n',
  "package-lock.json": '{"lockfileVersion":3}\n',
  "migrations.ts": "export const CODE_VERSION = 3;\nexport function migrate() {}\n",
  "server.ts": "export const handler = () => 1;\n",
  "ui/a.tsx": "export const A = 1;\n",
  "__tests__/a.test.ts": "// test\n",
};
// Each fixture is a commit made on top of the base commit, as a map of path -> new content (null deletes).
const FIXTURES: Record<string, Record<string, string | null>> = {
  "no-change": {},
  "ui-only": { "ui/a.tsx": "export const A = 2;\n", "ui/b.tsx": "export const B = 1;\n", "__tests__/a.test.ts": "// test 2\n" },
  "plugin-code": { "server.ts": "export const handler = () => 2;\n", "lib/new.ts": "export const x = 1;\n" },
  "ui-and-plugin-code": { "ui/a.tsx": "export const A = 3;\n", "server.ts": "export const handler = () => 3;\n" },
  "plugin-code-file-deleted": { "server.ts": null },
  "schema-version-bump": { "migrations.ts": "export const CODE_VERSION = 4;\nexport function migrate() {}\n" },
  "migrations-edited-same-version": { "migrations.ts": "export const CODE_VERSION = 3;\nexport function migrate() { /* changed */ }\n" },
  "schema-statement-in-code": { "server.ts": 'db.exec("CREATE TABLE z (x TEXT)");\n' },
  "schema-statement-mixed-case-spaced": { "lib/m.ts": 'db.exec("Alter   Table t ADD COLUMN y");\n' },
  "schema-meta-write-in-code": { "server.ts": "db.prepare(\"UPDATE schema_meta SET value='9'\").run();\n" },
  "pragma-user-version": { "lib/p.ts": 'db.exec("PRAGMA user_version = 9");\n' },
  "schema-statement-only-under-ui": { "ui/m.tsx": "// CREATE TABLE in a ui comment\n" },
  "package-json-changed": { "package.json": '{"name":"x","dependencies":{"left-pad":"1"}}\n' },
  "lockfile-changed": { "package-lock.json": '{"lockfileVersion":3,"x":1}\n' },
  "lockfile-added-yarn": { "yarn.lock": "# yarn\n" },
  "npmrc-added": { ".npmrc": "registry=https://example.invalid/\n" },
  "package-json-removed": { "package.json": null },
};
type Built = { root: string; installed: string; target: (name: string) => string };

function writeTree(dir: string, files: Record<string, string>) {
  for (const [p, c] of Object.entries(files)) { mkdirSync(join(dir, dirname(p)), { recursive: true }); writeFileSync(join(dir, p), c); }
}
function buildFixtures(): Built {
  const root = tmp("hgp-");
  const repo = join(root, "repo");
  mkdirSync(repo);
  git(repo, "init", "-q", "-b", "main");
  writeTree(repo, BASE);
  git(repo, "add", "-A"); git(repo, "commit", "-q", "-m", "base");
  const base = git(repo, "rev-parse", "HEAD");
  const checkout = (commit: string, dest: string) => {
    mkdirSync(dest, { recursive: true });
    const a = spawnSync("sh", ["-c", `git archive ${commit} | tar -x -C "${dest}"`], { cwd: repo, encoding: "utf8", env: { PATH: process.env.PATH! } });
    if (a.status !== 0) throw new Error(a.stderr);
    mkdirSync(join(dest, "node_modules"), { recursive: true });   // ignored by both gates, required to exist by both
  };
  const installed = join(root, "installed");
  checkout(base, installed);
  for (const [name, change] of Object.entries(FIXTURES)) {
    git(repo, "checkout", "-q", "-B", name, base);
    for (const [p, c] of Object.entries(change)) {
      if (c === null) git(repo, "rm", "-q", p);
      else { mkdirSync(join(repo, dirname(p)), { recursive: true }); writeFileSync(join(repo, p), c); }
    }
    git(repo, "add", "-A"); git(repo, "commit", "-q", "--allow-empty", "-m", name);
    checkout(git(repo, "rev-parse", "HEAD"), join(root, "t", name));
  }
  return { root, installed, target: (n) => join(root, "t", n) };
}

function dataDir(root: string, schemaVersion: number | null, migrationMax: number | null = schemaVersion) {
  const data = join(root, "bbdata");
  mkdirSync(join(data, "plugins", "autarch"), { recursive: true });
  const db = new Database(join(data, "plugins", "autarch", "data.db"));
  db.exec("CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT); CREATE TABLE migration_log (version INTEGER, at TEXT)");
  if (schemaVersion !== null) db.prepare("INSERT INTO schema_meta VALUES ('schema_version', ?)").run(String(schemaVersion));
  if (migrationMax !== null) for (let v = 1; v <= migrationMax; v++) db.prepare("INSERT INTO migration_log VALUES (?, 'x')").run(v);
  db.close();
  return data;
}
function stubBb(root: string, installed: string) {
  const bb = join(root, "bb");
  writeFileSync(bb, `#!/bin/sh\ncase "$1 $2" in\n  "plugin source") echo "autarch"; echo "  resolved: path:${installed}" ;;\n  *) exit 0 ;;\nesac\n`);
  chmodSync(bb, 0o755);
  return bb;
}
function sh(script: string, args: string[], env: Record<string, string> = {}) {
  return spawnSync("bash", [script, ...args], { env: { PATH: process.env.PATH!, ...env }, encoding: "utf8" });
}
const legacy = (b: Built, data: string, target: string, flags: string[] = []) =>
  sh(UI_SCRIPT, ["check", "--path", target, ...flags], { HOME: b.root, HOME_BB_BIN: stubBb(b.root, b.installed), HOME_BB_DATA: data, HOME_UI_WAIT: "2" });
const gatesCheck = (b: Built, data: string, target: string, flags: string[] = []) =>
  sh(GATES, ["check", "--installed", b.installed, "--target", target, "--data", data, ...flags], { HOME: b.root });
const gatesGate = (b: Built, data: string, target: string, cls = "plugin-deploy") =>
  sh(GATES, ["gate", "--class", cls, "--installed", b.installed, "--target", target, "--data", data], { HOME: b.root });

// Failure text is compared with the script name and the final summary line normalised; everything else must match line for line.
// home-ui.sh's die() prints all its arguments, so its refusals end in a stray " 3" (the exit code); home-gates.sh prints the message only.
const normalise = (s: string) => s.replace(/^home-(ui|gates): /gm, "home: ").replace(/ 3$/gm, "").split("\n").filter((l) => !/^home: check found \d+ failed gate/.test(l)).join("\n");

const names = Object.keys(FIXTURES);
const expectedLegacy: Record<string, { ui: number; code: number }> = {
  "no-change": { ui: 0, code: 0 },
  "ui-only": { ui: 0, code: 0 },
  "plugin-code": { ui: 3, code: 0 },
  "ui-and-plugin-code": { ui: 3, code: 0 },
  "plugin-code-file-deleted": { ui: 3, code: 0 },
  "schema-version-bump": { ui: 3, code: 3 },
  "migrations-edited-same-version": { ui: 3, code: 3 },
  "schema-statement-in-code": { ui: 3, code: 3 },
  "schema-statement-mixed-case-spaced": { ui: 3, code: 3 },
  "schema-meta-write-in-code": { ui: 3, code: 3 },
  "pragma-user-version": { ui: 3, code: 3 },
  "schema-statement-only-under-ui": { ui: 0, code: 0 },
  "package-json-changed": { ui: 3, code: 3 },
  "lockfile-changed": { ui: 3, code: 3 },
  "lockfile-added-yarn": { ui: 3, code: 3 },
  "npmrc-added": { ui: 3, code: 3 },
  "package-json-removed": { ui: 3, code: 3 },
};

describe("home-gates.sh parity with home-ui.sh check", () => {
  it("covers the fixture list the expectations below name", () => {
    expect(Object.keys(expectedLegacy).sort()).toEqual([...names].sort());
  });

  for (const live of [3, 4, 2]) {
    describe(`live schema ${live}`, () => {
      for (const name of names) {
        it(`${name}: check verdicts and text match, with and without --plugin-code; gate = ui-only OR plugin-code-only`, () => {
          const b = buildFixtures();
          const data = dataDir(b.root, live);
          const target = b.target(name);
          const lUi = legacy(b, data, target);
          const lCode = legacy(b, data, target, ["--plugin-code"]);
          const gUi = gatesCheck(b, data, target);
          const gCode = gatesCheck(b, data, target, ["--plugin-code"]);
          // the legacy script is the reference; the pinned table guards the reference itself at the matching schema
          if (live === 3 && name !== "schema-version-bump") expect({ ui: lUi.status, code: lCode.status }, "legacy verdict").toEqual(expectedLegacy[name]);
          expect(gUi.status, "check (ui-only)").toBe(lUi.status);
          expect(gCode.status, "check --plugin-code").toBe(lCode.status);
          expect(normalise(gUi.stdout)).toBe(normalise(lUi.stdout));
          expect(normalise(gCode.stdout)).toBe(normalise(lCode.stdout));
          expect(normalise(gUi.stderr)).toBe(normalise(lUi.stderr));
          expect(normalise(gCode.stderr)).toBe(normalise(lCode.stderr));
          const g = gatesGate(b, data, target);
          const legacyPass = lUi.status === 0 || lCode.status === 0;
          expect(g.status, `gate: ${g.stderr}`).toBe(legacyPass ? 0 : 10);
        });
      }
    });
  }
});

describe("home-gates.sh gate", () => {
  const lines = (out: string) => out.trim().split("\n").map((l) => JSON.parse(l) as { gate: string; ok: boolean; reason: string });

  it("prints one JSON line per gate and a final plugin-deploy line on pass", () => {
    const b = buildFixtures();
    const r = gatesGate(b, dataDir(b.root, 3), b.target("plugin-code"));
    expect(r.status).toBe(0);
    const j = lines(r.stdout);
    expect(j.every((x) => x.ok === true && typeof x.reason === "string")).toBe(true);
    expect(j.map((x) => x.gate)).toEqual(["build", "node-modules", "live-schema", "code-version", "schema-match", "installed-build", "migrations-identical", "scope", "plugin-deploy"]);
  });

  it("exits 10 with the failing gates named when the schema differs, and points a schema change at the reviewed install script", () => {
    const b = buildFixtures();
    const r = gatesGate(b, dataDir(b.root, 3), b.target("schema-version-bump"));
    expect(r.status).toBe(10);
    const j = lines(r.stdout);
    expect(j.filter((x) => !x.ok).map((x) => x.gate)).toEqual(["schema-match", "migrations-identical", "plugin-deploy"]);
    expect(j.find((x) => x.gate === "schema-match")!.reason).toContain("install-home-your-move.sh");
  });

  it("exits 10 for a migrations.ts change at the same version", () => {
    const b = buildFixtures();
    const r = gatesGate(b, dataDir(b.root, 3), b.target("migrations-edited-same-version"));
    expect(r.status).toBe(10);
    expect(r.stderr).toContain("migrations.ts differs");
  });

  it("refuses the additive-schema class with exit 10 and does not read the data directory", () => {
    const b = buildFixtures();
    const r = gatesGate(b, join(b.root, "no-such-data"), b.target("no-change"), "additive-schema");
    expect(r.status).toBe(10);
    expect(lines(r.stdout)).toEqual([{ gate: "class", ok: false, reason: expect.stringContaining("install-home-your-move.sh") }]);
  });

  it("rejects an unknown class as a usage error (64), not a refusal", () => {
    const b = buildFixtures();
    expect(gatesGate(b, dataDir(b.root, 3), b.target("no-change"), "bogus").status).toBe(64);
  });

  it("fails closed with an error exit (not 0, not 10) when the database cannot be read", () => {
    const b = buildFixtures();
    const r = gatesGate(b, join(b.root, "no-such-data"), b.target("no-change"));
    expect(r.status).toBe(1);
    expect(lines(r.stdout).some((x) => x.gate === "live-schema" && !x.ok)).toBe(true);
  });

  it("fails closed when schema_meta holds no version", () => {
    const b = buildFixtures();
    const r = gatesGate(b, dataDir(b.root, null), b.target("no-change"));
    expect(r.status).toBe(1);
  });

  it("fails closed when the installed build is missing", () => {
    const b = buildFixtures();
    const r = sh(GATES, ["gate", "--class", "plugin-deploy", "--installed", join(b.root, "gone"), "--target", b.target("no-change"), "--data", dataDir(b.root, 3)], { HOME: b.root });
    expect(r.status).toBe(1);
    expect(r.stderr).toContain("cannot tell the installed Home build");
  });

  it("refuses (10) a target that has no node_modules or no package.json: nothing installable, nothing installed", () => {
    const b = buildFixtures();
    const data = dataDir(b.root, 3);
    rmSync(join(b.target("no-change"), "node_modules"), { recursive: true });
    expect(gatesGate(b, data, b.target("no-change")).status).toBe(10);
    rmSync(join(b.target("ui-only"), "package.json"));
    expect(gatesGate(b, data, b.target("ui-only")).status).toBe(10);
  });

  it("refuses a lockfile symlinked in the target, as the legacy gate does", () => {
    const b = buildFixtures();
    const t = b.target("plugin-code");
    rmSync(join(t, "package-lock.json"));
    symlinkSync(join(b.installed, "package-lock.json"), join(t, "package-lock.json"));
    const data = dataDir(b.root, 3);
    expect(gatesCheck(b, data, t, ["--plugin-code"]).status).toBe(3);
    expect(legacy(b, data, t, ["--plugin-code"]).status).toBe(3);
    expect(gatesGate(b, data, t).status).toBe(10);
  });

  it("changes nothing: the database and both build trees are byte-identical afterwards", () => {
    const b = buildFixtures();
    const data = dataDir(b.root, 3);
    const snap = () => spawnSync("sh", ["-c", `find "${b.root}" -path "${b.root}/repo" -prune -o -type f -print0 | sort -z | xargs -0 sha256sum`], { encoding: "utf8" }).stdout;
    const before = snap();
    gatesGate(b, data, b.target("plugin-code")); gatesCheck(b, data, b.target("ui-only"));
    expect(snap()).toBe(before);
  });

  it("does not use bb: it runs with no bb on PATH and no HOME_BB_* variables", () => {
    const b = buildFixtures();
    const r = spawnSync("bash", [GATES, "gate", "--class", "plugin-deploy", "--installed", b.installed, "--target", b.target("ui-only"), "--data", dataDir(b.root, 3)], { env: { PATH: "/usr/bin:/bin" }, encoding: "utf8" });
    expect(r.status).toBe(0);
  });

  it("a usage error is 64 for a missing subcommand or flag value", () => {
    expect(sh(GATES, []).status).toBe(64);
    expect(sh(GATES, ["gate", "--class", "plugin-deploy"]).status).toBe(64);
    expect(sh(GATES, ["post-install"]).status).toBe(64);
    expect(sh(GATES, ["post-install", "--data", "/x", "--plugin-code"]).status).toBe(64);
  });
});

describe("home-gates.sh post-install", () => {
  const codeVersion = (dir: string) => Number(/CODE_VERSION = (\d+);/.exec(readFileSync(join(dir, "migrations.ts"), "utf8"))![1]);
  const pluginDir = join(REPO_ROOT, "integrations/bb-plugin-autarch");

  it("passes when migration_log's max version equals the build's CODE_VERSION", () => {
    const b = buildFixtures();
    const r = sh(GATES, ["post-install", "--data", dataDir(b.root, 3), "--plugin-dir", b.installed]);
    expect(r.status).toBe(0);
    expect(JSON.parse(r.stdout.trim())).toMatchObject({ gate: "migration-level", ok: true });
  });

  it("takes the build from its own checkout when --plugin-dir is not given", () => {
    const b = buildFixtures();
    const r = sh(GATES, ["post-install", "--data", dataDir(b.root, 5, codeVersion(pluginDir))]);
    expect(r.status).toBe(0);
  });

  it("exits 10 when the log is behind or ahead of the build", () => {
    const b = buildFixtures();
    expect(sh(GATES, ["post-install", "--data", dataDir(b.root, 3, 2), "--plugin-dir", b.installed]).status).toBe(10);
    const b2 = buildFixtures();
    expect(sh(GATES, ["post-install", "--data", dataDir(b2.root, 3, 4), "--plugin-dir", b2.installed]).status).toBe(10);
  });

  it("fails closed (exit 1) on an empty log, a missing database, or a build with no CODE_VERSION", () => {
    const b = buildFixtures();
    const empty = sh(GATES, ["post-install", "--data", dataDir(b.root, 3, null), "--plugin-dir", b.installed]);
    expect(empty.status).toBe(1);
    expect(JSON.parse(empty.stdout.trim()).ok).toBe(false);
    expect(sh(GATES, ["post-install", "--data", join(b.root, "nope"), "--plugin-dir", b.installed]).status).toBe(1);
    rmSync(join(b.installed, "migrations.ts"));
    expect(sh(GATES, ["post-install", "--data", join(b.root, "bbdata"), "--plugin-dir", b.installed]).status).toBe(1);
  });

  it("does not write the database", () => {
    const b = buildFixtures();
    const data = dataDir(b.root, 3);
    const db = join(data, "plugins", "autarch", "data.db");
    const before = readFileSync(db);
    sh(GATES, ["post-install", "--data", data, "--plugin-dir", b.installed]);
    expect(readFileSync(db).equals(before)).toBe(true);
  });
});

describe("home-gates.sh migrate-check", () => {
  it("refuses a directory that is not a plugin build, and leaves the live database alone", () => {
    const b = buildFixtures();
    const data = dataDir(b.root, 3);
    const db = join(data, "plugins", "autarch", "data.db");
    const before = readFileSync(db);
    const r = sh(GATES, ["migrate-check", "--path", join(b.root, "nope"), "--data", data]);
    expect(r.status).not.toBe(0);
    expect(readFileSync(db).equals(before)).toBe(true);
  });
});

// ---- deploy.json against the plugin-file format ----------------------------------------------------------------------------------
const PLACEHOLDERS = new Set(["bbData", "stateDir", "installedRoot", "installedPluginDir", "targetRoot", "targetPluginDir", "installedCommit", "targetCommit", "class", "buildDir"]);
const TOP_KEYS = new Set(["format", "pluginId", "repo", "branch", "pluginDir", "build", "data", "export", "configAfterEnable", "health", "preInstall", "classes", "dataRestore", "previewGate", "controlPaths", "retention"]);
const HEALTH_KINDS = new Set(["running", "log-line", "process-count", "check", "setting-equals"]);

function problems(doc: Record<string, unknown>): string[] {
  const bad: string[] = [];
  for (const k of Object.keys(doc)) if (!TOP_KEYS.has(k)) bad.push(`unknown key ${k}`);
  if (doc.format !== "bb-plugin-deploy/plugin@1") bad.push("format");
  for (const k of ["pluginId", "repo", "branch", "pluginDir"]) if (typeof doc[k] !== "string" || !doc[k]) bad.push(`${k} must be a non-empty string`);
  const argv = (v: unknown, where: string) => {
    if (!Array.isArray(v) || v.length === 0 || v.some((a) => typeof a !== "string")) { bad.push(`${where}: argv must be a non-empty array of strings`); return; }
    for (const a of v as string[]) for (const m of a.matchAll(/\{([^{}]*)\}/g)) if (!PLACEHOLDERS.has(m[1])) bad.push(`${where}: unknown placeholder {${m[1]}}`);
  };
  if (!Array.isArray(doc.build)) bad.push("build"); else (doc.build as unknown[]).forEach((c, i) => argv(c, `build[${i}]`));
  if (!Array.isArray(doc.data)) bad.push("data"); else (doc.data as Record<string, unknown>[]).forEach((d, i) => {
    if (!["sqlite", "file", "dir"].includes(String(d.kind))) bad.push(`data[${i}].kind`);
    argv([d.path], `data[${i}].path`);
  });
  if (!Array.isArray(doc.export)) bad.push("export"); else (doc.export as Record<string, unknown>[]).forEach((e, i) => argv(e.run, `export[${i}].run`));
  if (!Array.isArray(doc.health)) bad.push("health"); else (doc.health as Record<string, unknown>[]).forEach((h, i) => {
    if (!HEALTH_KINDS.has(String(h.kind))) bad.push(`health[${i}].kind`);
    if (h.kind === "check") argv(h.run, `health[${i}].run`);
  });
  for (const k of ["preInstall", "previewGate"]) {
    if (!Array.isArray(doc[k])) bad.push(k); else (doc[k] as Record<string, unknown>[]).forEach((c, i) => { if (typeof c.name !== "string") bad.push(`${k}[${i}].name`); argv(c.run, `${k}[${i}].run`); });
  }
  if (!Array.isArray(doc.classes) || doc.classes.some((c) => typeof c !== "string")) bad.push("classes");
  if (!["manual", "auto"].includes(String(doc.dataRestore))) bad.push("dataRestore");
  if (!Array.isArray(doc.controlPaths) || doc.controlPaths.some((c) => typeof c !== "string")) bad.push("controlPaths");
  return bad;
}
const load = () => JSON.parse(readFileSync(DEPLOY_JSON, "utf8")) as Record<string, any>;

describe("deploy.json", () => {
  it("is valid against the bb-plugin-deploy/plugin@1 rules (known keys only, closed placeholder set, argv arrays)", () => {
    expect(problems(load())).toEqual([]);
  });

  it("the validator itself fails closed on an unknown key, an unknown placeholder and a shell string", () => {
    expect(problems({ ...load(), surprise: 1 })).toContain("unknown key surprise");
    const p = load(); p.preInstall[0].run[2] = "{nope}";
    expect(problems(p).join()).toContain("unknown placeholder {nope}");
    const s = load(); s.build = ["npm ci"];
    expect(problems(s).join()).toContain("argv must be");
  });

  it("declares exactly the plugin-deploy class, manual data restore, and the repo's own plugin directory", () => {
    const d = load();
    expect(d.classes).toEqual(["plugin-deploy"]);
    expect(d.dataRestore).toBe("manual");
    expect(existsSync(join(REPO_ROOT, d.pluginDir, "package.json"))).toBe(true);
    expect(d.pluginDir).toBe("integrations/bb-plugin-autarch");
    expect(d.data).toEqual([{ name: "data.db", kind: "sqlite", path: "{bbData}/plugins/autarch/data.db", integrity: true }]);
  });

  it("the build, health and gate commands call only subcommands that home-gates.sh has", () => {
    const d = load();
    const all = [...d.preInstall, ...d.previewGate, ...d.health.filter((h: any) => h.kind === "check")];
    for (const c of all) {
      expect(c.run[0]).toBe("bash");
      expect(c.run[1]).toBe("{installedRoot}/scripts/home-gates.sh");
      expect(["gate", "post-install"]).toContain(c.run[2]);
    }
    expect(existsSync(GATES)).toBe(true);
  });

  it("every control path names something the repo has (or a glob that can match later)", () => {
    for (const p of load().controlPaths as string[]) if (!p.includes("*")) expect(existsSync(join(REPO_ROOT, p)), p).toBe(true);
  });

  it("its preInstall, previewGate and health commands run end to end with placeholders filled in (no live paths)", () => {
    const b = buildFixtures();
    const data = dataDir(b.root, 3);
    const vars: Record<string, string> = { bbData: data, stateDir: join(b.root, "state"), installedRoot: REPO_ROOT, installedPluginDir: b.installed, targetRoot: b.root, targetPluginDir: b.target("ui-only"), installedCommit: "0".repeat(40), targetCommit: "1".repeat(40), class: "plugin-deploy", buildDir: b.root };
    const fill = (a: string) => a.replace(/\{(\w+)\}/g, (_m, k) => vars[k]);
    const d = load();
    for (const c of [...d.preInstall, ...d.previewGate]) {
      const r = spawnSync(fill(c.run[0]), c.run.slice(1).map(fill), { env: { PATH: process.env.PATH! }, encoding: "utf8" });
      expect(r.status, r.stderr).toBe(0);
    }
    const health = d.health.find((h: any) => h.kind === "check");
    const real = join(REPO_ROOT, "integrations/bb-plugin-autarch/migrations.ts");
    const v = Number(/CODE_VERSION = (\d+);/.exec(readFileSync(real, "utf8"))![1]);
    vars.bbData = dataDir(tmp("hgpd-"), v, v);
    const r = spawnSync(fill(health.run[0]), health.run.slice(1).map(fill), { env: { PATH: process.env.PATH! }, encoding: "utf8" });
    expect(r.status, r.stderr).toBe(0);
  });
});
